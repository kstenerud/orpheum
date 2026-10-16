// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/backup"
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/portmap"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	coreproviders "github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/providers/tmdb"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store"
)

// jobServices is what the background jobs need: the identification pass, the job queue, the
// matcher that runs its fetch_metadata jobs, the chooser a person uses, the refresher, the artwork its
// matches name, playback's prepares, the subtitles and the read-throughs.
type jobServices struct {
	pass      *identify.Pass
	queue     *jobs.Queue
	matcher   *identify.Matcher
	chooser   *identify.Chooser
	refresher *identify.Refresher
	settings  *settings.Service
	artwork   *artwork.Service
	// renditions runs the prepare jobs, which share the one queue (cut-a §5.3).
	renditions *renditions.Service
	// readThrough reads each video through, for its sounds' loudness and its picture's timeline
	// (D226, D247 §10).
	readThrough *readthrough.Service
	// chunks cuts films into chunks made on request, run by run (D247 §10).
	chunks *chunks.Service
	// playback plays files, and counts what its players need of the server's upload (D286 §5).
	playback *playback.Service
	// subtitles has a played video's subtitles made and fetches them on request (D217 §6, D224).
	subtitles *subtitles.Service
	// nodes says how many prepare slots the nodes have, which as many prepare runners serve.
	nodes *nodes.Service
	// exchanges give node jobs to nodes, and resolve each by its node's word (D234 §7).
	exchanges *nodes.Exchanges
	// scans run the scans of the nodes' roots: a listing, then the batches it leaves (D240).
	scans *library.Scans
	// scanSchedule asks for the scans libraries that notice changes by themselves are due (D27,
	// D272 §1).
	scanSchedule *library.ScanSchedule
	// backups makes the backups, scheduled and asked for, and prunes them (D243 §11).
	backups *backup.Service
	// certificates obtains and renews the HTTPS certificates: the server's public name's (D243 §6)
	// and its discovery's (D273).
	certificates *servedCertificates
	// discovery keeps the core enrolled with its discovery, when an admin connected one (D273).
	discovery *discovery.Service
	// portMapping keeps the core's HTTPS port open at the home router while the discovery publishes
	// it (D273 §7).
	portMapping *portmap.Service
	// subtitler fetches subtitles on request (D224).
	subtitler *opensubtitles.Client
	// outboundClient is the one client every call to another host goes through: TMDB's API and
	// image server, and OpenSubtitles, sharing one transport and its connections.
	outboundClient *http.Client
	// providerServices are each outside host's provider service, which every call to it asks
	// first (D231 §5): TMDB's API, TMDB's image server and OpenSubtitles, in that order.
	providerServices hostProviders
	// sessions remembers each session's use, which writeSessionUses writes in batches (D303 §4).
	sessions sessionUseFlusher
	// invariants checks persistent.db's invariants, at start and daily (D303, M7).
	invariants invariantChecker
	// credentials deletes the credentials ended past their keeping, daily (D304 §6).
	credentials credentialPurger
	// removedFiles deletes the files removed past their keeping, daily (D304 §6, D306 §8).
	removedFiles removedFilePurger
}

// removedFilePurger deletes the files removed before a time: library.FilePurger.
type removedFilePurger interface {
	PurgeRemovedFiles(ctx context.Context, removedBefore time.Time, forgets ...library.PurgedFilesForget) (int, error)
}

// credentialPurger deletes the credentials ended past their keeping: auth.Service.
type credentialPurger interface {
	PurgeEndedCredentials(ctx context.Context) (int, error)
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
	wakeOwedPasses(pass, queue)
	matcher := identify.NewMatcher(opened.database, opened.cache, pass, clock.System{}, logger)
	return jobServices{
		sessions: opened.auth, invariants: opened.database, credentials: opened.auth,
		removedFiles: library.NewFilePurger(opened.database),
		pass:         pass, queue: queue, matcher: matcher, settings: opened.settings, subtitler: subtitler,
		outboundClient: httpClient, providerServices: hosts,
		chooser:   identify.NewChooser(matcher, pass),
		refresher: identify.NewRefresher(opened.database, queue, opened.settings, clock.System{}, logger),
	}, nil
}

// wakeOwedPasses has a library marked as owing a pass ask for the owed passes' sweep, which runs
// each such library's pass once however many marked it (D303, DL14).
func wakeOwedPasses(pass *identify.Pass, sweeps sweepRequester) {
	pass.OnPassOwed(func() { sweeps.RequestSweeps(sweepOwedPasses) })
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
	return map[jobs.Kind]jobs.ArgsMerge{
		jobs.KindFetchMetadata: identify.FetchArgsMerge(), jobs.KindBackup: backup.JobArgsMerge(),
	}
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

// derivedByAScanRequest names what derives a scan's work: a scan asked for by an admin, a node's
// hello or the scan schedule's sweep, which a lost scan is asked for again by (D240).
func derivedByAScanRequest() []string { return []string{"the next scan asked for"} }

// runners are the kinds of job the server works, each one at a time, the artwork retry schedule read
// by the caller: one fetch per provider; a node's job is given to its node and answered at once,
// the core holding each node to its slots and devices (D234 §7, D240 §5). A prepare's failure is its
// rendition's, which the next play prepares again.
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
			jobs.KindExtractSubtitle, services.subtitles.HandleExtractSubtitle, services.subtitles.RecordFailure,
			nil, 1, derivedByAPlay(), services.subtitles.Assignee,
		},
		{
			jobs.KindConvertFetchedSubtitle, services.subtitles.HandleConvertFetchedSubtitle,
			services.subtitles.RecordFailure, nil, 1, derivedByAPlay(), services.subtitles.Assignee,
		},
		{
			// A read-through that gave up keeps nothing, so the next sweep asks again (D244 §1).
			jobs.KindReadThrough, services.readThrough.HandleReadThrough, nil, nil, 1,
			[]string{sweepReadThroughs},
			services.readThrough.Assignee,
		},
		{
			// A run lost is forgotten, so the next player's ask for its chunks queues another.
			jobs.KindMakeChunks, services.chunks.HandleMakeChunks, services.chunks.RecordMakeChunksFailure, nil, 1,
			derivedByAPlay(), services.chunks.Assignee,
		},
		{
			jobs.KindPrepare, services.renditions.HandlePrepare, services.renditions.RecordPrepareFailure, nil, 1,
			derivedByAPlay(), services.renditions.Assignee,
		},
		{
			jobs.KindScan, services.scans.HandleScan, services.scans.RecordScanFailure, nil, 1,
			derivedByAScanRequest(), services.scans.ScanAssignee,
		},
		{
			jobs.KindExamine, services.scans.HandleExamine, services.scans.RecordExamineFailure, nil, 1,
			derivedByAScanRequest(), services.scans.ExamineAssignee,
		},
		{jobs.KindBackup, services.backups.HandleBackup, nil, nil, 1, []string{sweepScheduledBackup}, nil},
		{jobs.KindRenewCertificate, services.renewCertificate, nil, nil, 1, []string{sweepCertificate}, nil},
	}
}

// The sweeps, by what they derive (D231 §3): the first match of each film and series with no
// answer yet; the refreshes people asked for that no fetch has answered; the one recheck of each
// image its source said was missing (D213 §1); the refresh of each item whose data is due (D213
// §3); every library identified again from persistent.db, each item's season listings and images
// owed from what the pass reports, as after a scan (D209 §3, D212); and the measurement of each
// video through whose bytes lack their loudness or their picture's timeline (D226 §2, D247 §10).
const (
	sweepFirstMatches     = "first matches"
	sweepReconsidered     = "ambiguous matches reconsidered"
	sweepRefreshRequests  = "refreshes people asked for"
	sweepRechecks         = "rechecks of missing images"
	sweepRollingRefresh   = "rolling refresh"
	sweepPasses           = "identification passes"
	sweepOwedPasses       = "identification passes owed"
	sweepReadThroughs     = "read-throughs"
	sweepScheduledBackup  = "the scheduled backup"
	sweepCertificate      = "the certificate's renewal"
	sweepScheduledScans   = "the scheduled scans"
	sweepInvariants       = "the database's invariants"
	sweepEndedCredentials = "ended credentials past their keeping" //nolint:gosec // G101: a sweep's name
	sweepRemovedFiles     = "removed files past their keeping"
)

// scheduledScansCheckEvery is how often the scan schedule's sweep asks whether a root is due its
// scan: a bound on how late one starts, against intervals of hours, and a read of persistent.db's
// roots, nothing more.
const scheduledScansCheckEvery = 15 * time.Minute

// retentionSweepEvery is how often what is kept a number of days is pruned to it (D304 §6): daily,
// as the limits are in days.
const retentionSweepEvery = 24 * time.Hour

// purgeEndedCredentials is the ended credentials' sweep: it queues no work, and deletes those past
// their keeping.
func purgeEndedCredentials(purger credentialPurger) func(ctx context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		if _, err := purger.PurgeEndedCredentials(ctx); err != nil {
			return 0, fmt.Errorf("purge ended credentials: %w", err)
		}
		return 0, nil
	}
}

// purgeRemovedFiles is the removed files' sweep: it queues no work, and deletes those removed more
// than removed_files_kept_days ago, keeping what was watched of them on their versions and
// forgetting what each person chose in them and the subtitles fetched for them.
func purgeRemovedFiles(
	purger removedFilePurger, config *settings.Service, now clock.Clock,
) func(ctx context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		kept, err := config.Duration(ctx, settings.RemovedFilesKeptDays)
		if err != nil {
			return 0, fmt.Errorf("read removed_files_kept_days: %w", err)
		}
		_, err = purger.PurgeRemovedFiles(ctx, now.Now().Add(-kept),
			playback.FoldWatchingOfPurgedFiles, preferences.ForgetPreferencesOfPurgedFiles,
			subtitles.ForgetSubtitlesOfPurgedFiles)
		if err != nil {
			return 0, fmt.Errorf("purge removed files: %w", err)
		}
		return 0, nil
	}
}

// invariantsCheckEvery is how often persistent.db's invariants are checked beside the check at
// start: daily, as D303 (M7) says, a few full reads of tables at this scale.
const invariantsCheckEvery = 24 * time.Hour

// invariantChecker checks persistent.db's invariants: store.Database.
type invariantChecker interface {
	CheckInvariants(ctx context.Context) ([]store.Violation, error)
}

// checkInvariants is the invariants' sweep: it queues no work, and logs each invariant the data no
// longer keeps as an error, with the first rows it fails at, so a fault that writes what no healthy
// server holds is named the day it starts (D303, M7): the owner's NAS gathered 80,246 empty titles
// under one series, found only when its page took 45 seconds to draw.
func checkInvariants(checker invariantChecker, logger *slog.Logger) func(ctx context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		violations, err := checker.CheckInvariants(ctx)
		if err != nil {
			return 0, fmt.Errorf("check the database's invariants: %w", err)
		}
		for _, violation := range violations {
			logger.ErrorContext(ctx,
				"the database breaks one of its invariants: a fault wrote what no healthy server holds",
				"invariant", violation.Invariant, "rows", violation.Rows)
		}
		return 0, nil
	}
}

// certificateCheckEvery is how often the certificate's sweep asks whether a renewal is due: daily,
// against a month's margin before it expires (D243 §6).
const certificateCheckEvery = 24 * time.Hour

// renewCertificate runs a renewal; a server with no public name and no discovery has none to renew.
func (services jobServices) renewCertificate(ctx context.Context, job jobs.Job) error {
	if services.certificates == nil {
		return jobs.MarkPermanent(errNoPublicName)
	}
	return services.certificates.HandleRenew(ctx, job)
}

// queueCertificateRenewal asks for a renewal when one is due; none for a server with no public name.
func (services jobServices) queueCertificateRenewal(ctx context.Context) (int, error) {
	if services.certificates == nil {
		return 0, nil
	}
	return services.certificates.QueueRenewal(ctx)
}

// scheduledBackupCheckEvery is how often the scheduled backup's sweep asks whether it is due, so an
// hourly backup comes within five minutes of its hour; a look at the backup directory, nothing
// more (D305 §2).
const scheduledBackupCheckEvery = 5 * time.Minute

// sweepsAfterAPass are the sweeps an identification pass asks for: every one but the pass's own.
func sweepsAfterAPass() []string {
	return []string{sweepFirstMatches, sweepRefreshRequests, sweepRechecks, sweepRollingRefresh, sweepReadThroughs}
}

// sweepCheckEvery is how often the sweeping loop looks for a sweep whose interval has passed: a
// bound on how late one runs, not a choice about how often work is derived.
const sweepCheckEvery = time.Minute

// sweeps are every sweep that derives the server's work from persistent.db (D231 §3), each queuing
// what the library's state implies of one kind's work, as its name says.
func (services jobServices) sweeps(logger *slog.Logger) []jobs.Sweep {
	// The sweeps that derive first matches, the refreshes people asked for, rechecks and the
	// identification passes run as derived_work_sweep_every_seconds says, besides at start, after
	// each pass and when a destination comes back (D231 §3).
	derivedWorkEvery := func(ctx context.Context) time.Duration {
		return lengthOrStarting(ctx, services.settings, settings.DerivedWorkSweepEverySeconds, logger)
	}
	return []jobs.Sweep{
		// Before the first matches, which queue what it returns to them.
		{Name: sweepReconsidered, Derive: services.matcher.ReconsiderAmbiguous, Every: derivedWorkEvery},
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
		// The passes a match or a refresh left owed, one per library however many asked (D303, DL14).
		{Name: sweepOwedPasses, Derive: services.pass.IdentifyOwed, Every: derivedWorkEvery},
		{
			Name: sweepCertificate, Derive: services.queueCertificateRenewal,
			Every: func(context.Context) time.Duration { return certificateCheckEvery },
		},
		{
			Name: sweepScheduledBackup, Derive: services.backups.QueueScheduled,
			Every: func(context.Context) time.Duration { return scheduledBackupCheckEvery },
		},
		{
			Name: sweepScheduledScans, Derive: services.scanSchedule.QueueDue,
			Every: func(context.Context) time.Duration { return scheduledScansCheckEvery },
		},
		{
			Name: sweepEndedCredentials, Derive: purgeEndedCredentials(services.credentials),
			Every: func(context.Context) time.Duration { return retentionSweepEvery },
		},
		{
			Name:   sweepRemovedFiles,
			Derive: purgeRemovedFiles(services.removedFiles, services.settings, clock.System{}),
			Every:  func(context.Context) time.Duration { return retentionSweepEvery },
		},
		{
			Name: sweepInvariants, Derive: checkInvariants(services.invariants, logger),
			Every: func(context.Context) time.Duration { return invariantsCheckEvery },
		},
		{
			Name: sweepReadThroughs, Derive: services.readThrough.QueueReadThroughs,
			Every: func(ctx context.Context) time.Duration {
				return lengthOrStarting(ctx, services.settings, settings.ReadThroughSweepEveryHours, logger)
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
	if err := services.scans.Resume(ctx); err != nil {
		return nil, err
	}
	services.queue.OnCancel(services.exchanges.Cancelled)
	services.queue.OnOverdue(services.exchanges.Overdue)
	services.queue.SetReportedProgress(reportedProgressOf(services.exchanges))
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
		// for every silenceCheckEvery.
		ticker := time.NewTicker(silenceCheckEvery)
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
	loops.Go(func() {
		ticker := time.NewTicker(demoteUnwantedEvery)
		defer ticker.Stop()
		demoteUnwanted(loopsContext, services.queue, services.exchanges, ticker.C, logger)
	})
	loops.Go(func() {
		ticker := time.NewTicker(pruneFinishedEvery)
		defer ticker.Stop()
		pruneFinished(loopsContext, services.queue, ticker.C, logger)
	})
	services.startUpkeep(loopsContext, &loops, logger)
	loops.Go(func() { services.discovery.Run(loopsContext) })
	loops.Go(func() { services.portMapping.Run(loopsContext) })
	return func() { cancel(); loops.Wait() }, nil
}

// startUpkeep starts, on loops, the loops that keep things tidy until ctx ends: the artwork
// cache's ceiling, the sessions' remembered uses written in batches (D303 §4), and the players'
// share of the server's upload (D286 §5).
func (services jobServices) startUpkeep(ctx context.Context, loops *sync.WaitGroup, logger *slog.Logger) {
	loops.Go(func() { maintainArtworkCache(ctx, services.artwork, services.settings, logger) })
	loops.Go(func() { writeSessionUses(ctx, services.sessions, services.settings, logger) })
	loops.Go(func() {
		ticker := time.NewTicker(uploadLogEvery)
		defer ticker.Stop()
		services.playback.WatchUpload(ctx, ticker.C, logger)
	})
	loops.Go(func() {
		ticker := time.NewTicker(uploadKeptEvery)
		defer ticker.Stop()
		services.playback.KeepUnderShare(ctx, ticker.C, logger)
	})
}

// uploadLogEvery is how often what the server's players need of its upload is logged while anyone
// plays: the chunks' speed window, over which a few chunks of each playback arrive (D286 §5).
const uploadLogEvery = 20 * time.Second

// uploadKeptEvery is how often the server's players' qualities are lowered or raised to keep within
// its upload: about a chunk's time, so a player told at its next chunk has moved before the next look
// (D286 §5).
const uploadKeptEvery = 5 * time.Second

// silenceCheckEvery is how often the work given to nodes is looked at for a silence past its limit:
// a bound on how late a silence is noticed, against limits of minutes (node_silent_after_seconds,
// the slowest read); a look reads memory alone.
const silenceCheckEvery = time.Second

// demoteUnwantedEvery is how often the jobs no person waits for any more are looked for: a bound
// on how late, past its "wanted until", play-driven work falls to background priority. It is half
// play_priority_kept_seconds' shortest, so even at that a job is demoted at most half its keeping
// late; one cache.db query a tick, which finds nothing most of the time.
const demoteUnwantedEvery = 5 * time.Second

// demoteUnwanted gives the jobs no person waits for any more background priority at each tick, and
// tells the node holding each one given to it (D234 §3, S87), until ctx ends.
func demoteUnwanted(
	ctx context.Context, queue *jobs.Queue, exchanges *nodes.Exchanges, tick <-chan time.Time, logger *slog.Logger,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		demoted, err := queue.DemoteUnwanted(ctx)
		if err != nil {
			logger.WarnContext(ctx, "could not demote the jobs no one waits for; trying again", "err", err)
			continue
		}
		for _, job := range demoted {
			if job.IsAssigned {
				exchanges.Reprioritize(job.ID, jobs.PriorityBackground)
			}
		}
		if len(demoted) > 0 {
			logger.InfoContext(ctx, "jobs no one waits for any more fell to background priority",
				"jobCount", len(demoted))
		}
	}
}

// pruneFinishedEvery is how often the jobs that ended long enough ago are forgotten: hours against a
// month's keeping (finished_jobs_kept_days).
const pruneFinishedEvery = time.Hour

// pruneFinished forgets, at each tick, the jobs that ended longer ago than finished_jobs_kept_days
// (D233 §8), until ctx ends.
func pruneFinished(ctx context.Context, queue *jobs.Queue, tick <-chan time.Time, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		pruned, err := queue.PruneFinished(ctx)
		if err != nil {
			logger.WarnContext(ctx, "could not forget the jobs that ended; trying again", "err", err)
			continue
		}
		if pruned > 0 {
			logger.InfoContext(ctx, "forgot the jobs that ended long ago", "jobCount", pruned)
		}
	}
}

// reportedProgressOf has the job view say where the node holding each assigned job's work last said it is
// (D240 §6).
func reportedProgressOf(exchanges *nodes.Exchanges) jobs.ReportedProgress {
	return func(jobID int64) (string, bool) {
		word, isSaid := exchanges.ReportedProgressOf(jobID)
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
