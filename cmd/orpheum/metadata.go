// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/resourceuse"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// raiseInterestEvery is how often the work of interest is raised and released: a bound on how late
// a page opened raises its work, short against a person's look of tens of seconds; a few cache.db
// writes a tick, for the few films people play and look at.
const raiseInterestEvery = 5 * time.Second

// sweepCheckEvery is how often the sweeping loop looks for a sweep whose interval has passed: a
// bound on how late one runs, not a choice about how often work is derived.
const sweepCheckEvery = time.Minute

// settingReader reads a length-of-time setting: settings.Service.
type settingReader interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// lengthOrStarting is a loop's wait, read afresh each time so a change takes effect at the next
// one; when it cannot be read, the loop goes on at the setting's starting value and says so.
func lengthOrStarting(ctx context.Context, config settingReader, key settings.Key, logger *slog.Logger) time.Duration {
	return settings.DurationOrStarting(ctx, config, key, logger)
}

// startBackgroundLoops starts a runner for every kind of job, the sweeps that derive their work
// (at start, on their timers, after each pass and when a destination comes back, D231 §3), and
// the artwork cache's upkeep. They run until the returned stop is called.
func startBackgroundLoops(ctx context.Context, opened openedServices, logger *slog.Logger) (stop func(), err error) {
	core := opened.core
	artworkRetrySchedule, err := artwork.RetrySchedule(ctx, opened.settings)
	if err != nil {
		return nil, err
	}
	if err := core.Exchanges.Load(ctx); err != nil {
		return nil, err
	}
	if err := core.Scans.Resume(ctx); err != nil {
		return nil, err
	}
	core.Queue.OnCancel(core.Exchanges.Cancelled)
	core.Queue.OnOverdue(core.Exchanges.Overdue)
	core.Queue.SetReportedProgress(reportedProgressOf(core.Exchanges))
	loopsContext, cancel := context.WithCancel(ctx)
	var loops sync.WaitGroup
	for _, running := range core.KindRunners(artworkRetrySchedule) {
		loops.Go(func() {
			// A runner looks for due jobs each second, and at once when the queue wakes it (a new
			// job, an unparked destination), so the tick only catches a retry coming due.
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			if err := running.Runner.Run(loopsContext, ticker.C); err != nil {
				logger.ErrorContext(ctx, "a job queue stopped", "kind", string(running.Kind), "err", err)
			}
		})
	}
	loops.Go(func() {
		// A node says where the work it holds is at each of its status ticks; a silence is looked
		// for every silenceCheckEvery.
		ticker := time.NewTicker(silenceCheckEvery)
		defer ticker.Stop()
		core.Exchanges.Watch(loopsContext, ticker.C)
	})
	core.AddSweeps(logger)
	loops.Go(func() {
		ticker := time.NewTicker(sweepCheckEvery)
		defer ticker.Stop()
		core.Queue.Sweeping(loopsContext, ticker.C)
	})
	loops.Go(func() {
		ticker := time.NewTicker(demoteUnwantedEvery)
		defer ticker.Stop()
		demoteUnwanted(loopsContext, core.Queue, core.Exchanges, ticker.C, logger)
	})
	if core.Interest != nil {
		loops.Go(func() {
			ticker := time.NewTicker(raiseInterestEvery)
			defer ticker.Stop()
			core.Interest.Run(loopsContext, ticker.C)
		})
	}
	loops.Go(func() {
		ticker := time.NewTicker(pruneFinishedEvery)
		defer ticker.Stop()
		pruneFinished(loopsContext, core.Queue, ticker.C, logger)
	})
	startUpkeep(loopsContext, opened, &loops, logger)
	loops.Go(func() { opened.discovery.Run(loopsContext) })
	loops.Go(func() { opened.portMapping.Run(loopsContext) })
	return func() { cancel(); loops.Wait() }, nil
}

// startUpkeep starts, on loops, the loops that keep things tidy until ctx ends: the artwork
// cache's ceiling, the sessions' remembered uses written in batches (D303 §4), the players' share of
// the server's upload (D286 §5), Watch Together's links and sessions past their lifetimes (D346),
// the casts whose receiver is gone (D364 §6; the two minutes seen on devices, VQ90), and the
// record of what the core and each node use of their machines, each minute (internal/core/resourceuse;
// a minute's ticker no test waits out, VQ257).
func startUpkeep(ctx context.Context, opened openedServices, loops *sync.WaitGroup, logger *slog.Logger) {
	dependencies := opened.core.API
	loops.Go(func() { maintainArtworkCache(ctx, dependencies.Artwork, opened.settings, logger) })
	loops.Go(func() { writeSessionUses(ctx, dependencies.Auth, opened.settings, logger) })
	loops.Go(func() {
		ticker := time.NewTicker(uploadLogEvery)
		defer ticker.Stop()
		dependencies.Playback.WatchUpload(ctx, ticker.C, logger)
	})
	loops.Go(func() {
		ticker := time.NewTicker(uploadKeptEvery)
		defer ticker.Stop()
		dependencies.Playback.KeepUnderShare(ctx, ticker.C, logger)
	})
	loops.Go(func() {
		ticker := time.NewTicker(togetherStaleEvery)
		defer ticker.Stop()
		endStaleTogether(ctx, dependencies.Together, opened.settings, ticker.C, logger)
	})
	loops.Go(func() {
		ticker := time.NewTicker(castsGoneEvery)
		defer ticker.Stop()
		endGoneCasts(ctx, dependencies.Live, dependencies.Together, ticker.C, logger)
	})
	loops.Go(func() {
		ticker := time.NewTicker(resourceuse.Every)
		defer ticker.Stop()
		dependencies.ResourceUse.Run(ctx, ticker.C)
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
// a bound on how late a silence is noticed, against limits of minutes (nodeSilentAfterSeconds,
// the slowest read); a look reads memory alone.
const silenceCheckEvery = time.Second

// demoteUnwantedEvery is how often the jobs no person waits for any more are looked for: a bound
// on how late, past its "wanted until", play-driven work falls to background priority. It is half
// playPriorityKeptSeconds' shortest, so even at that a job is demoted at most half its keeping
// late; one cache.db query a tick, which finds nothing most of the time.
const demoteUnwantedEvery = 5 * time.Second

// demoteUnwanted gives the jobs no person waits for any more background priority at each tick, and
// tells the node holding each one given to it (D234 §3, S87), until ctx ends.
func demoteUnwanted(
	ctx context.Context, queue *jobs.Queue, exchanges *nodes.Exchanges, tick <-chan time.Time, logger *slog.Logger,
) {
	var failing slogx.Failing // said once a minute while it lasts, not every 5 s (the audit of 2026-10-25, C12)
	for {
		var tickedAt time.Time
		select {
		case <-ctx.Done():
			return
		case tickedAt = <-tick:
		}
		demoted, err := queue.DemoteUnwanted(ctx)
		if err != nil {
			if isSaid, lasting := failing.Fail(tickedAt); isSaid {
				logger.WarnContext(ctx, "could not demote the jobs no one waits for; trying again",
					append([]any{"err", err, slogx.EventKey, "demoteFailed"}, lasting...)...)
			}
			continue
		}
		if isEnded, lasted := failing.Succeed(tickedAt); isEnded {
			logger.InfoContext(ctx, "the jobs no one waits for are demoted again", lasted...)
		}
		for _, job := range demoted {
			if job.IsAssigned {
				exchanges.Reprioritize(job.ID, jobs.PriorityShownInterest)
			}
		}
		if len(demoted) > 0 {
			logger.InfoContext(ctx, "jobs no one waits for any more fell to the shown-interest priority",
				"jobCount", len(demoted))
		}
	}
}

// pruneFinishedEvery is how often the jobs that ended long enough ago are forgotten: hours against a
// month's keeping (finishedJobsKeptDays).
const pruneFinishedEvery = time.Hour

// pruneFinished forgets, at each tick, the jobs that ended longer ago than finishedJobsKeptDays
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
