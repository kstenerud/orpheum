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
	"github.com/kstenerud/orpheum/internal/core/playback"
	coreproviders "github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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
	// subtitler fetches subtitles on request (D224).
	subtitler *opensubtitles.Client
	// outboundClient is the one client every call to another host goes through: TMDB's API and
	// image server, and OpenSubtitles, sharing one transport and its connections.
	outboundClient *http.Client
	// providerServices are each outside host's provider service, which every call to it asks
	// first (D231 §5): TMDB's API, TMDB's image server and OpenSubtitles, in that order.
	providerServices hostProviders
	// measureDue is signalled after each identification pass, which follows every scan: new files
	// may want measuring (D226 §2).
	measureDue chan struct{}
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
	hosts := newHostProviders(queue, opened.secrets)
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
	pass.AfterPass(func(ctx context.Context, report identify.Report) { queueOwedFetches(ctx, queue, report, logger) })
	measureDue := make(chan struct{}, 1)
	pass.AfterPass(func(context.Context, identify.Report) {
		select {
		case measureDue <- struct{}{}:
		default: // a sweep is already due
		}
	})
	matcher := identify.NewMatcher(opened.database, opened.cache, pass, clock.System{}, logger)
	return jobServices{
		pass: pass, queue: queue, matcher: matcher, settings: opened.settings, subtitler: subtitler,
		outboundClient: httpClient, providerServices: hosts, measureDue: measureDue,
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
func newHostProviders(queue *jobs.Queue, secretStore *secrets.Store) hostProviders {
	hosts := hostProviders{
		tmdbAPI:       tmdb.APIService(queue, clock.System{}),
		tmdbImages:    tmdb.ImagesService(queue, clock.System{}),
		openSubtitles: opensubtitles.Service(queue, clock.System{}),
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

// jobRunners are the job queue's runners: each kind, its handler, its retry schedule when it has
// its own, and how many of its jobs run at once.
type jobRunners []struct {
	kind          jobs.Kind
	handler       jobs.Handler
	retrySchedule []time.Duration
	concurrency   int
}

// runners are the kinds of job the server works. Prepares run as many at once as a node has
// slots (cut-a §5.3), so a second person's copy is not held behind the first's.
func (services jobServices) runners() jobRunners {
	return jobRunners{
		{jobs.KindFetchMetadata, services.matcher.Handle, nil, 1},
		{jobs.KindMakeArtworkSizes, services.artwork.Handle, artwork.RetrySchedule(), 1},
		{jobs.KindPrepare, services.playback.HandlePrepare, nil, nodeproto.EncodeSlotCount + nodeproto.RemuxSlotCount},
		{jobs.KindExtractSubtitle, services.playback.HandleExtractSubtitle, nil, 1},
		{jobs.KindConvertFetchedSubtitle, services.playback.HandleConvertFetchedSubtitle, nil, 1},
		{jobs.KindMeasureLoudness, services.playback.HandleMeasureLoudness, nil, 1},
	}
}

// startBackgroundLoops starts a runner for every kind of job, the look for data due a refresh
// (when it starts and every metadata_refresh_check_every_hours, D213 §3), the look for files to
// measure, and the artwork cache's upkeep. They run until the returned stop is called.
func (services jobServices) startBackgroundLoops(ctx context.Context, logger *slog.Logger) (stop func()) {
	loopsContext, cancel := context.WithCancel(ctx)
	var loops sync.WaitGroup
	for _, runnerSpec := range services.runners() {
		for range runnerSpec.concurrency {
			kind, runner := runnerSpec.kind, services.queue.NewRunner(runnerSpec.kind, runnerSpec.handler)
			if runnerSpec.retrySchedule != nil {
				runner.WithRetrySchedule(runnerSpec.retrySchedule)
			}
			loops.Go(func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				if err := runner.Run(loopsContext, ticker.C); err != nil {
					logger.ErrorContext(ctx, "a job queue stopped", "kind", string(kind), "err", err)
				}
			})
		}
	}
	loops.Go(func() { services.queueDueRefreshes(loopsContext, logger) })
	loops.Go(func() { services.queueMeasurements(loopsContext, logger) })
	loops.Go(func() { maintainArtworkCache(loopsContext, services.artwork, logger) })
	return func() { cancel(); loops.Wait() }
}

// queueDueRefreshes queues the rolling refresh: now, then every metadata_refresh_check_every_hours,
// read afresh each time so a change takes effect at the next look.
func (services jobServices) queueDueRefreshes(ctx context.Context, logger *slog.Logger) {
	for {
		queued, err := services.refresher.Sweep(ctx)
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "could not look for data due a refresh", "err", err)
		}
		if queued > 0 {
			logger.InfoContext(ctx, "queued refreshes of data that is due", "queuedCount", queued)
		}
		checkEvery, err := services.settings.Duration(ctx, settings.MetadataRefreshCheckEveryHours)
		if err != nil {
			checkEvery = 24 * time.Hour
		}
		wait := time.NewTimer(checkEvery)
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-wait.C:
		}
	}
}

// measureSweepEvery is how often the server looks for files to measure when no scan has said so.
const measureSweepEvery = time.Hour

// queueMeasurements queues a loudness measurement for each file with none (D226 §2): now, after
// each identification pass, and every measureSweepEvery.
func (services jobServices) queueMeasurements(ctx context.Context, logger *slog.Logger) {
	for {
		queued, err := services.playback.QueueMeasurements(ctx)
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "could not look for files to measure", "err", err)
		}
		if queued > 0 {
			logger.InfoContext(ctx, "queued loudness measurements", "queuedCount", queued)
		}
		wait := time.NewTimer(measureSweepEvery)
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-services.measureDue:
			wait.Stop()
		case <-wait.C:
		}
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

// queueOwedFetches queues what a pass left owing: the first match of each film and series, once
// per item (D170); and a matched series' season listings, whenever a new episode needs one
// (D209 §3). A later match is a person's request.
func queueOwedFetches(ctx context.Context, queue *jobs.Queue, report identify.Report, logger *slog.Logger) {
	queued := 0
	for _, itemID := range report.AwaitingMatch {
		wasEverQueued, err := queue.EverQueued(ctx, jobs.KindFetchMetadata, jobs.Target{ItemID: itemID})
		if err == nil && !wasEverQueued {
			_, err = queue.Enqueue(ctx, jobs.Request{
				Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: itemID},
				Destination: identify.FetchDestination,
				Reason:      "the first match for a newly identified title or series",
			})
			queued++
		}
		if err != nil {
			logger.ErrorContext(ctx, "could not queue a match", "itemId", itemID, "err", err)
			return
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
