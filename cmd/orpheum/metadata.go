// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	coreproviders "github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// jobServices is what the background jobs need: the identification pass, the job queue, the
// matcher that runs its fetch_metadata jobs, the chooser a person uses, the refresher, the artwork its
// matches name, and playback's prepare, subtitle and loudness work.
type jobServices struct {
	pass      *identify.Pass
	queue     *jobs.Queue
	matcher   *identify.Matcher
	chooser   *identify.Chooser
	refresher *identify.Refresher
	settings  *settings.Service
	artwork   *artwork.Service
	// playback runs the prepare jobs, which share the one queue (cut-a §5.3).
	playback *playback.Service
	// nodes says how many prepare slots the nodes have, which as many prepare runners serve.
	nodes *nodes.Service
	// exchanges give node jobs to nodes, and resolve each by its node's word (D234 §7).
	exchanges *nodes.Exchanges
	// subtitler fetches subtitles on request (D224).
	subtitler *opensubtitles.Client
	// outboundClient is the one client every call to another host goes through: TMDB's API and
	// image server, and OpenSubtitles, sharing one transport and its connections.
	outboundClient *http.Client
	// providerServices are each outside host's provider service, which every call to it asks
	// first (D231 §5): TMDB's API, TMDB's image server and OpenSubtitles, in that order.
	providerServices hostProviders
}

// newJobServices builds the providers, the pass and the queue, and returns the jobs a crash left
// running to the queue.
func newJobServices(
	ctx context.Context, opened openedServices, outside wiring, logger *slog.Logger,
) (jobServices, error) {
	providerTimeout, err := opened.settings.Duration(ctx, settings.ProviderTimeoutSeconds)
	if err != nil {
		return jobServices{}, err
	}
	httpClient, err := outbound.NewClient(providerTimeout, version)
	if err != nil {
		return jobServices{}, err
	}
	queue := jobs.NewQueue(jobs.QueueParts{
		Cache: opened.cache, Clock: clock.System{}, Settings: opened.settings, Random: rand.Reader, Logger: logger,
		ArgsMerges: jobArgsMerges(),
	})
	hosts := newHostProviders(queue, opened.secrets, clock.System{}, opened.settings)
	providers := identify.Providers{"tmdb": tmdb.New(httpClient, outside.tmdbBaseURL, opened.secrets, hosts.tmdbAPI)}
	subtitler, err := opensubtitles.New(
		httpClient, outside.openSubtitlesBaseURL, opened.secrets, clock.System{}, hosts.openSubtitles)
	if err != nil {
		return jobServices{}, err
	}
	requeued, err := queue.RequeueRunning(ctx)
	if err != nil {
		return jobServices{}, err
	}
	if requeued > 0 {
		logger.InfoContext(ctx, "returned jobs a stop left running to the queue", "requeuedCount", requeued)
	}
	pass := identify.NewPass(opened.database, clock.System{}, logger, providers, opened.settings)
	if err := keepSortAndSearchKeysCurrent(ctx, pass, opened.settings, logger); err != nil {
		return jobServices{}, err
	}
	pass.AfterPass(func(ctx context.Context, report identify.Report) {
		queueOwedFetches(ctx, queue, report, logger)
		// A pass follows every scan: new files may want measuring (D226 §2), and new items their
		// matches. Every sweep but the pass's own runs after it (D231 §3).
		queue.RequestSweeps(sweepsAfterAPass()...)
	})
	matcher := identify.NewMatcher(opened.database, opened.cache, pass, clock.System{}, logger)
	return jobServices{
		pass: pass, queue: queue, matcher: matcher, settings: opened.settings, subtitler: subtitler,
		outboundClient: httpClient, providerServices: hosts,
		chooser:   identify.NewChooser(matcher, pass),
		refresher: identify.NewRefresher(opened.database, queue, opened.settings, clock.System{}, logger),
	}, nil
}

// hostProviders are the outside hosts' provider services (D231 §5).
type hostProviders struct {
	tmdbAPI, tmdbImages, openSubtitles *coreproviders.Service
}

// all are the services, in the order the System page lists them.
func (hosts hostProviders) all() []*coreproviders.Service {
	return []*coreproviders.Service{hosts.tmdbAPI, hosts.tmdbImages, hosts.openSubtitles}
}

// newHostProviders builds each outside host's provider service, its health kept in the job queue,
// and has a person's change of a host's key or login try it again.
func newHostProviders(
	queue *jobs.Queue, secretStore *secrets.Store, clockSource clock.Clock, settingsReader coreproviders.Settings,
) hostProviders {
	hosts := hostProviders{
		tmdbAPI:       tmdb.APIService(queue, clockSource, settingsReader),
		tmdbImages:    tmdb.ImagesService(queue, clockSource, settingsReader),
		openSubtitles: opensubtitles.Service(queue, clockSource, settingsReader),
	}
	secretStore.OnChange(func(ctx context.Context, name secrets.Name) {
		switch name {
		case secrets.TMDBAPIKey:
			hosts.tmdbAPI.CredentialsChanged(ctx)
		case secrets.OpenSubtitlesUsername, secrets.OpenSubtitlesPassword:
			hosts.openSubtitles.CredentialsChanged(ctx)
		case secrets.TURNSecret, secrets.DNSProviderToken: // no outside host we call uses them yet
		}
	})
	return hosts
}

// jobArgsMerges are the kinds whose repeat requests merge their arguments their own way (D231 §8):
// a fetch's, which identify's arguments are. The others keep the queued job's.
func jobArgsMerges() map[jobs.Kind]jobs.ArgsMerge {
	return map[jobs.Kind]jobs.ArgsMerge{jobs.KindFetchMetadata: identify.FetchArgsMerge()}
}

// jobRunners are the job queue's runners: each kind, its handler, what records a failed job as
// its item's own failure (D231 §4, §10), its retry schedule when it has its own, how many of its
// jobs run at once, and the sweeps that derive its work from persistent.db (D231 §3).
type jobRunners []jobRunner

// jobRunner is one of them.
type jobRunner struct {
	kind          jobs.Kind
	handler       jobs.Handler
	recordFailure jobs.FailureRecorder
	retrySchedule []time.Duration
	concurrency   int
	// derivedBy names the sweeps that queue the kind's work again from persistent.db, or is
	// derivedByAPlay for work a play asks for, which the next play asks for again.
	derivedBy []string
	// assignee, for a kind whose work a node does, is how its jobs given to a node are resolved.
	assignee func(runner nodes.Resolver) nodes.Assignee
}

// derivedByAPlay names what derives the work a play asks for: a prepare, a subtitle's text. It
// needs no sweep, since the next play asks again (D231 §4).
func derivedByAPlay() []string { return []string{"the next play"} }

// runners are the kinds of job the server works, each one at a time, the artwork retry schedule read
// by the caller: one fetch per provider; a node's job is given to its node and answered at once,
// the core holding each node to its slots and devices (D234 §7, D240 §5). A prepare records no
// failure of its own: the next play asks again.
func (services jobServices) runners(artworkRetrySchedule []time.Duration) jobRunners {
	return jobRunners{
		{
			jobs.KindFetchMetadata, services.matcher.Handle, services.matcher.RecordFailure, nil, 1,
			[]string{sweepFirstMatches, sweepRefreshRequests, sweepRechecks, sweepRollingRefresh, sweepPasses},
			nil,
		},
		{
			jobs.KindMakeArtworkSizes, services.artwork.Handle, services.artwork.RecordFailure,
			artworkRetrySchedule, 1,
			[]string{sweepPasses},
			services.artwork.Assignee,
		},
		{
			jobs.KindExtractSubtitle, services.playback.HandleExtractSubtitle, services.playback.RecordSubtitleFailure,
			nil, 1, derivedByAPlay(), services.playback.SubtitleAssignee,
		},
		{
			jobs.KindConvertFetchedSubtitle, services.playback.HandleConvertFetchedSubtitle,
			services.playback.RecordSubtitleFailure, nil, 1, derivedByAPlay(), services.playback.SubtitleAssignee,
		},
		{
			jobs.KindMeasureLoudness, services.playback.HandleMeasureLoudness,
			services.playback.RecordMeasurementFailure, nil, 1,
			[]string{sweepLoudness},
			services.playback.LoudnessAssignee,
		},
		{
			jobs.KindPrepare, services.playback.HandlePrepare, nil, nil, 1, derivedByAPlay(),
			services.playback.PrepareAssignee,
		},
	}
}

// The sweeps, by what they derive (D231 §3): the first match of each film and series with no
// answer yet; the refreshes people asked for that no fetch has answered; the one recheck of each
// image its source said was missing (D213 §1); the refresh of each item whose data is due (D213
// §3); every library identified again from persistent.db, each item's season listings and images
// owed from what the pass reports, as after a scan (D209 §3, D212); and the measurement of each
// file whose sound has none (D226 §2).
const (
	sweepFirstMatches    = "first matches"
	sweepRefreshRequests = "refreshes people asked for"
	sweepRechecks        = "rechecks of missing images"
	sweepRollingRefresh  = "rolling refresh"
	sweepPasses          = "identification passes"
	sweepLoudness        = "loudness measurements"
)

// sweepsAfterAPass are the sweeps an identification pass asks for: every one but the pass's own.
func sweepsAfterAPass() []string {
	return []string{sweepFirstMatches, sweepRefreshRequests, sweepRechecks, sweepRollingRefresh, sweepLoudness}
}

// derivedWorkSweepEvery is how often the sweeps that derive first matches, the refreshes people
// asked for, rechecks and the identification passes run on their own, besides at start, after
// each pass and when a destination comes back: D231 §3's "hourly by default". A named constant
// until the settings work gives each sweep's interval a setting (the owner, 2026-09-30).
const derivedWorkSweepEvery = time.Hour

// sweepCheckEvery is how often the sweeping loop looks for a sweep whose interval has passed: a
// bound on how late one runs, not a choice about how often work is derived.
const sweepCheckEvery = time.Minute

// sweeps are every sweep that derives the server's work from persistent.db (D231 §3), each queuing
// what the library's state implies of one kind's work, as its name says.
func (services jobServices) sweeps(logger *slog.Logger) []jobs.Sweep {
	derivedWorkEvery := func(context.Context) time.Duration { return derivedWorkSweepEvery }
	return []jobs.Sweep{
		{Name: sweepFirstMatches, Derive: services.refresher.QueueFirstMatches, Every: derivedWorkEvery},
		{Name: sweepRefreshRequests, Derive: services.refresher.QueueRequested, Every: derivedWorkEvery},
		{
			Name: sweepRechecks, Every: derivedWorkEvery,
			Derive: func(ctx context.Context) (int, error) {
				due, err := services.artwork.DueRechecks(ctx)
				if err != nil {
					return 0, err
				}
				return services.refresher.QueueRechecks(ctx, due)
			},
		},
		{
			Name: sweepRollingRefresh, Derive: services.refresher.Sweep,
			Every: func(ctx context.Context) time.Duration {
				return lengthOrStarting(ctx, services.settings, settings.MetadataRefreshCheckEveryHours, logger)
			},
		},
		{Name: sweepPasses, Derive: services.pass.IdentifyAll, Every: derivedWorkEvery},
		{
			Name: sweepLoudness, Derive: services.playback.QueueMeasurements,
			Every: func(ctx context.Context) time.Duration {
				return lengthOrStarting(ctx, services.settings, settings.LoudnessSweepEveryHours, logger)
			},
		},
	}
}

// settingReader reads a length-of-time setting: settings.Service.
type settingReader interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// lengthOrStarting is a loop's wait, read afresh each time so a change takes effect at the next
// one; when it cannot be read, the loop goes on at the setting's starting value and says so.
func lengthOrStarting(ctx context.Context, config settingReader, key settings.Key, logger *slog.Logger) time.Duration {
	length, err := config.Duration(ctx, key)
	if err != nil {
		length = settings.StartingDuration(key)
		logger.WarnContext(ctx, "could not read a setting; going on at its starting value", "key", string(key),
			"startingValue", length.String(), "err", err)
	}
	return length
}

// newRunner is a runner of one kind, with its retry schedule and the recording of its failures.
func (services jobServices) newRunner(runnerSpec jobRunner) *jobs.Runner {
	runner := services.queue.NewRunner(runnerSpec.kind, runnerSpec.handler)
	if runnerSpec.retrySchedule != nil {
		runner.WithRetrySchedule(runnerSpec.retrySchedule)
	}
	if runnerSpec.recordFailure != nil {
		runner.WithFailureRecorder(runnerSpec.recordFailure)
	}
	return runner
}

// startBackgroundLoops starts a runner for every kind of job, the sweeps that derive their work
// (at start, on their timers, after each pass and when a destination comes back, D231 §3), and
// the artwork cache's upkeep. They run until the returned stop is called.
func (services jobServices) startBackgroundLoops(ctx context.Context, logger *slog.Logger) (stop func(), err error) {
	artworkRetrySchedule, err := artwork.RetrySchedule(ctx, services.settings)
	if err != nil {
		return nil, err
	}
	if err := services.exchanges.Load(ctx); err != nil {
		return nil, err
	}
	services.queue.OnCancel(services.exchanges.Cancelled)
	services.queue.SetNodeWords(nodeWordsOf(services.exchanges))
	loopsContext, cancel := context.WithCancel(ctx)
	var loops sync.WaitGroup
	startRunner := func(runnerSpec jobRunner) {
		kind, runner := runnerSpec.kind, services.newRunner(runnerSpec)
		loops.Go(func() {
			// A runner looks for due jobs each second, and at once when the queue wakes it (a new
			// job, an unparked destination), so the tick only catches a retry coming due.
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			if err := runner.Run(loopsContext, ticker.C); err != nil {
				logger.ErrorContext(ctx, "a job queue stopped", "kind", string(kind), "err", err)
			}
		})
	}
	for _, runnerSpec := range services.runners(artworkRetrySchedule) {
		// A node's word on a job given to it is taken by a runner of its kind, as its handler's answer.
		if runnerSpec.assignee != nil {
			services.exchanges.Handle(runnerSpec.kind, runnerSpec.assignee(services.newRunner(runnerSpec)))
		}
		for range runnerSpec.concurrency {
			startRunner(runnerSpec)
		}
	}
	loops.Go(func() {
		// A node says where the work it holds is at each of its status ticks; a silence is looked
		// for each second.
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		services.exchanges.Watch(loopsContext, ticker.C)
	})
	for _, sweep := range services.sweeps(logger) {
		services.queue.AddSweep(sweep)
	}
	loops.Go(func() {
		ticker := time.NewTicker(sweepCheckEvery)
		defer ticker.Stop()
		services.queue.Sweeping(loopsContext, ticker.C)
	})
	loops.Go(func() { maintainArtworkCache(loopsContext, services.artwork, services.settings, logger) })
	return func() { cancel(); loops.Wait() }, nil
}

// nodeWordsOf has the job view say where the node holding each assigned job's work last said it is
// (D240 §6).
func nodeWordsOf(exchanges *nodes.Exchanges) jobs.NodeWords {
	return func(jobID int64) (string, bool) {
		word, isSaid := exchanges.NodeWordOf(jobID)
		return string(word), isSaid
	}
}

// keepSortAndSearchKeysCurrent keeps every sort key in the sort_locale setting's locale, and every name's
// fold for search, what this build derives: now, for a change made while the server was stopped,
// a release's new starting value or a build that collates or folds otherwise, and whenever a
// person changes the locale (D47, D48; agent-failures A4).
func keepSortAndSearchKeysCurrent(
	ctx context.Context, pass *identify.Pass, settingsService *settings.Service, logger *slog.Logger,
) error {
	rewrittenCount, err := pass.RefreshSortAndSearchKeys(ctx)
	if err != nil {
		return err
	}
	if rewrittenCount > 0 {
		logger.InfoContext(ctx, "keyed and folded names again for sorting and search", "rewrittenCount", rewrittenCount)
	}
	settingsService.OnChange(settings.SortLocale, func(ctx context.Context) error {
		rewrittenCount, err := pass.RefreshSortAndSearchKeys(ctx)
		if err == nil {
			logger.InfoContext(ctx, "the sort locale changed; keyed and folded names again for sorting and search",
				"rewrittenCount", rewrittenCount)
		}
		return err
	})
	return nil
}

// queueOwedFetches queues what a pass left owing: the first match of each film and series with
// no answer yet (D170; owed until it has one, D231 §3); and a matched series' season listings,
// whenever a new episode needs one (D209 §3). A later match is a person's request.
func queueOwedFetches(ctx context.Context, queue *jobs.Queue, report identify.Report, logger *slog.Logger) {
	queued := 0
	for _, itemID := range report.AwaitingMatch {
		wasQueued, err := queue.Enqueue(ctx, identify.FirstMatchJob(itemID))
		if err != nil {
			logger.ErrorContext(ctx, "could not queue a match", "itemId", itemID, "err", err)
			return
		}
		if wasQueued {
			queued++
		}
	}
	for _, seriesID := range report.AwaitingListings {
		wasQueued, err := queue.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: seriesID}, Destination: identify.FetchDestination,
			Reason: "a season listing a new episode needs",
		})
		if err != nil {
			logger.ErrorContext(ctx, "could not queue a season listing", "seriesId", seriesID, "err", err)
			return
		}
		if wasQueued {
			queued++
		}
	}
	if queued > 0 {
		logger.InfoContext(ctx, "queued fetches", "queuedCount", queued)
	}
}

var _ tmdb.Keys = (*secrets.Store)(nil)
