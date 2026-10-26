// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// removedFilePurger deletes the files removed before a time: library.FilePurger.
type removedFilePurger interface {
	PurgeRemovedFiles(ctx context.Context, removedBefore time.Time, forgets ...library.PurgedFilesForget) (int, error)
}

// credentialPurger deletes the credentials ended past their keeping: auth.Service.
type credentialPurger interface {
	PurgeEndedCredentials(ctx context.Context) (int, error)
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

// derivedByAPlay names what derives the work a play asks for: a run of chunks, a subtitle's text. It
// needs no sweep, since the next play asks again (D231 §4).
func derivedByAPlay() []string { return []string{"the next play"} }

// derivedByAScanRequest names what derives a scan's work: a scan asked for by an admin, a node's
// hello or the scan schedule's sweep, which a lost scan is asked for again by (D240).
func derivedByAScanRequest() []string { return []string{"the next scan asked for"} }

// runners are the kinds of job the server works, each one at a time, the artwork retry schedule read
// by the caller: one fetch per provider; a node's job is given to its node and answered at once,
// the core holding each node to its slots and devices (D234 §7, D240 §5).
func (services *Services) runners(artworkRetrySchedule []time.Duration) jobRunners {
	return jobRunners{
		{
			jobs.KindFetchMetadata, services.matcher.Handle, services.matcher.RecordFailure, nil, 1,
			[]string{
				sweepFirstMatches, sweepRefreshRequests, sweepRechecks, sweepRollingRefresh, sweepPasses, sweepMissing,
			},
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
			jobs.KindScan, services.Scans.HandleScan, services.Scans.RecordScanFailure, nil, 1,
			derivedByAScanRequest(), services.Scans.ScanAssignee,
		},
		{
			jobs.KindExamine, services.Scans.HandleExamine, services.Scans.RecordExamineFailure, nil, 1,
			derivedByAScanRequest(), services.Scans.ExamineAssignee,
		},
		{jobs.KindBackup, services.backups.HandleBackup, nil, nil, 1, []string{sweepScheduledBackup}, nil},
		{jobs.KindRenewCertificate, services.renewCertificate, nil, nil, 1, []string{sweepCertificate}, nil},
	}
}

// KindRunner is one runner of the job queue's, of its kind, which the composition root runs on its
// ticker.
type KindRunner struct {
	Kind   jobs.Kind
	Runner *jobs.Runner
}

// KindRunners are the runners the queue is worked by: each kind's, one for each of its jobs that may
// run at once, the artwork retry schedule read by the caller. A node's word on a job given to it is
// taken by a runner of its kind, as its handler's answer, which the exchanges are given here.
func (services *Services) KindRunners(artworkRetrySchedule []time.Duration) []KindRunner {
	var running []KindRunner
	for _, runnerSpec := range services.runners(artworkRetrySchedule) {
		if runnerSpec.assignee != nil {
			services.Exchanges.Handle(runnerSpec.kind, runnerSpec.assignee(services.newRunner(runnerSpec)))
		}
		for range runnerSpec.concurrency {
			running = append(running, KindRunner{Kind: runnerSpec.kind, Runner: services.newRunner(runnerSpec)})
		}
	}
	return running
}

// newRunner is a runner of one kind, with its retry schedule and the recording of its failures.
func (services *Services) newRunner(runnerSpec jobRunner) *jobs.Runner {
	runner := services.Queue.NewRunner(runnerSpec.kind, runnerSpec.handler)
	if runnerSpec.retrySchedule != nil {
		runner.WithRetrySchedule(runnerSpec.retrySchedule)
	}
	if runnerSpec.recordFailure != nil {
		runner.WithFailureRecorder(runnerSpec.recordFailure)
	}
	return runner
}

// The sweeps, by what they derive (D231 §3): the first match of each film and series with no
// answer yet; the refreshes people asked for that no fetch has answered; the one recheck of each
// image its source said was missing (D213 §1); the refresh of each item whose data is due (D213
// §3); every library identified again from persistent.db, each item's season listings and images
// owed from what the pass reports, as after a scan (D209 §3, D212); and the measurement of each
// video through whose bytes lack their loudness or their picture's timeline (D226 §2, D247 §10).
const (
	sweepFirstMatches     = "first matches"
	sweepMissing          = "what should have been fetched"
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
	sweepUnavailableRoots = "the unavailable roots scanned again"
	sweepInvariants       = "the database's invariants"
	sweepEndedCredentials = "ended credentials past their keeping" //nolint:gosec // G101: a sweep's name
	sweepRemovedFiles     = "removed files past their keeping"
)

// scheduledScansCheckEvery is how often the scan schedule's sweep asks whether a root is due its
// scan: a bound on how late one starts, against intervals of hours, and a read of persistent.db's
// roots, nothing more.
const scheduledScansCheckEvery = 15 * time.Minute

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
func (services *Services) renewCertificate(ctx context.Context, job jobs.Job) error {
	if services.Certificates == nil {
		return jobs.MarkPermanent(ErrNoPublicName)
	}
	return services.Certificates.HandleRenew(ctx, job)
}

// queueCertificateRenewal asks for a renewal when one is due; none for a server with no public name.
func (services *Services) queueCertificateRenewal(ctx context.Context) (int, error) {
	if services.Certificates == nil {
		return 0, nil
	}
	return services.Certificates.QueueRenewal(ctx)
}

// scheduledBackupCheckEvery is how often the scheduled backup's sweep asks whether it is due, so an
// hourly backup comes within five minutes of its hour; a look at the backup directory, nothing
// more (D305 §2).
const scheduledBackupCheckEvery = 5 * time.Minute

// sweepsAfterAPass are the sweeps an identification pass asks for: every one but the pass's own.
func sweepsAfterAPass() []string {
	return []string{sweepFirstMatches, sweepRefreshRequests, sweepRechecks, sweepRollingRefresh, sweepReadThroughs}
}

// AddSweeps gives the queue every sweep that derives the server's work from persistent.db (D231 §3),
// which it runs at start, on their timers, after each pass and when a destination comes back.
func (services *Services) AddSweeps(logger *slog.Logger) {
	for _, sweep := range services.sweeps(logger) {
		services.Queue.AddSweep(sweep)
	}
}

// sweeps are every sweep that derives the server's work from persistent.db (D231 §3), each queuing
// what the library's state implies of one kind's work, as its name says.
func (services *Services) sweeps(logger *slog.Logger) []jobs.Sweep {
	// The sweeps that derive first matches, the refreshes people asked for, rechecks and the
	// identification passes run as derived_work_sweep_every_seconds says, besides at start, after
	// each pass and when a destination comes back (D231 §3).
	every := func(key settings.Key) func(ctx context.Context) time.Duration {
		return func(ctx context.Context) time.Duration {
			return settings.DurationOrStarting(ctx, services.settings, key, logger)
		}
	}
	derivedWorkEvery := every(settings.DerivedWorkSweepEverySeconds)
	return []jobs.Sweep{
		// Before the first matches, which queue what it returns to them.
		{Name: sweepReconsidered, Derive: services.matcher.ReconsiderAmbiguous, Every: derivedWorkEvery},
		{Name: sweepFirstMatches, Derive: services.refresher.QueueFirstMatches, Every: derivedWorkEvery},
		{Name: sweepRefreshRequests, Derive: services.refresher.QueueRequested, Every: derivedWorkEvery},
		// A little at a time, hourly and at start, never after each pass (D335 §3).
		{Name: sweepMissing, Derive: services.refresher.QueueMissing, Every: derivedWorkEvery},
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
			Every: every(settings.MetadataRefreshCheckEveryHours),
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
			Name: sweepUnavailableRoots, Derive: services.scanSchedule.RescanUnavailable,
			Every: every(settings.UnavailableRootRescanSeconds),
		},
		{
			Name: sweepEndedCredentials, Derive: purgeEndedCredentials(services.credentials),
			Every: every(settings.RetentionSweepEveryHours),
		},
		{
			Name:   sweepRemovedFiles,
			Derive: purgeRemovedFiles(services.removedFiles, services.settings, services.clock),
			Every:  every(settings.RetentionSweepEveryHours),
		},
		{
			Name: sweepInvariants, Derive: checkInvariants(services.invariants, logger),
			Every: every(settings.InvariantsCheckEveryHours),
		},
		{
			Name: sweepReadThroughs, Derive: services.readThrough.QueueReadThroughs,
			Every: every(settings.ReadThroughSweepEveryHours),
		},
	}
}
