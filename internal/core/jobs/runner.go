// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// Handler runs one job. A nil error finishes it; a *RetryAtError defers it and MarkBlocked holds
// its kind, neither costing an attempt; MarkPermanent fails it; any other error retries it with
// back-off until job_attempt_limit.
type Handler func(ctx context.Context, job Job) error

// Runner runs one kind of job, one at a time: one fetch per provider (cut-a §5.3).
type Runner struct {
	queue         *Queue
	kind          Kind
	handler       Handler
	retrySchedule []time.Duration
	pausedUntil   time.Time
	// wake is this runner's own wake-up, left there when a job of its kind is queued.
	wake chan struct{}
}

// WithRetrySchedule gives the runner's kind its own retry schedule in place of the settings' doubling
// back-off: the wait before each retry, after which the job fails (D213 §5).
func (runner *Runner) WithRetrySchedule(retrySchedule []time.Duration) *Runner {
	runner.retrySchedule = retrySchedule
	return runner
}

// NewRunner returns the runner for one kind of job.
func (queue *Queue) NewRunner(kind Kind, handler Handler) *Runner {
	return &Runner{queue: queue, kind: kind, handler: handler, wake: queue.registerWake(kind)}
}

// Run runs due jobs on every tick and whenever the queue is woken, until ctx ends.
func (runner *Runner) Run(ctx context.Context, tick <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
		case <-runner.wake:
		}
		if err := runner.RunDue(ctx); err != nil && ctx.Err() == nil {
			runner.queue.logger.ErrorContext(ctx, "the job queue could not run", "kind", runner.kind, "err", err)
		}
	}
}

// RunDue runs every job of the runner's kind whose time has come, highest priority first, and
// stops early when a handler says the kind must wait (a rate limit).
func (runner *Runner) RunDue(ctx context.Context) error {
	for ctx.Err() == nil {
		now := runner.queue.clock.Now()
		if now.Before(runner.pausedUntil) {
			return nil
		}
		job, found, err := runner.claim(ctx, now)
		if err != nil || !found {
			return err
		}
		if err := runner.finish(ctx, job, runner.handle(ctx, job)); err != nil {
			return err
		}
	}
	return nil
}

// handle runs one job's handler in a context a person's cancel ends (Queue.Cancel).
func (runner *Runner) handle(ctx context.Context, job Job) error {
	jobCtx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	runner.queue.trackCancel(job.ID, stop)
	defer runner.queue.untrackCancel(job.ID)
	return runner.handler(jobCtx, job)
}

// claim marks the next due job running.
func (runner *Runner) claim(ctx context.Context, now time.Time) (Job, bool, error) {
	var job Job
	found := false
	err := runner.queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		row, nextErr := store.OrNotFound(queries.NextQueuedJob(ctx, cachegen.NextQueuedJobParams{
			Kind: string(runner.kind), Now: now.UnixMilli(),
		}))
		if errors.Is(nextErr, store.ErrNotFound) {
			return nil
		}
		if nextErr != nil {
			return fmt.Errorf("find the next %s job: %w", runner.kind, nextErr)
		}
		if _, startErr := queries.StartJob(ctx, row.ID); startErr != nil {
			return fmt.Errorf("start job %d: %w", row.ID, startErr)
		}
		job = Job{
			ID: row.ID, Kind: runner.kind, ArgsJSON: row.ArgsJson, AttemptNumber: row.AttemptCount + 1,
			Target: targetOf(row.ItemID, row.RenditionID, row.StreamID, row.FetchedSubtitleID, row.FileID),
		}
		found = true
		return nil
	})
	if err != nil {
		return Job{}, false, fmt.Errorf("claim: %w", err)
	}
	return job, found, nil
}

// finish records a job's outcome.
func (runner *Runner) finish(ctx context.Context, job Job, handlerErr error) error {
	now := runner.queue.clock.Now()
	if handlerErr == nil {
		return runner.writeFinish(ctx, finishParams(job, stateDone, nil, now))
	}
	if retry, isRetry := errors.AsType[*RetryAtError](handlerErr); isRetry {
		runner.pausedUntil = retry.At
		return runner.deferUntil(ctx, job, retry.At, handlerErr, job.AttemptNumber-1)
	}
	if failure, isPermanent := errors.AsType[*permanent](handlerErr); isPermanent {
		return runner.writeFinish(ctx, finishParams(job, stateFailed, failure, now))
	}
	attemptLimit, firstRetryDelay, err := runner.retryPolicy(ctx)
	if err != nil {
		return err
	}
	if held, isBlocked := errors.AsType[*blocked](handlerErr); isBlocked {
		runner.pausedUntil = now.Add(firstRetryDelay)
		return runner.deferUntil(ctx, job, runner.pausedUntil, held, job.AttemptNumber-1)
	}
	if runner.retrySchedule != nil {
		attemptLimit = int64(len(runner.retrySchedule)) + 1
	}
	if job.AttemptNumber >= attemptLimit {
		runner.queue.logger.WarnContext(ctx, "a job failed", "kind", job.Kind, "target", job.Target,
			"attemptCount", job.AttemptNumber, "err", handlerErr)
		return runner.writeFinish(ctx, finishParams(job, stateFailed, handlerErr, now))
	}
	wait := doubledDelay(firstRetryDelay, job.AttemptNumber-1)
	if runner.retrySchedule != nil {
		wait = runner.retrySchedule[job.AttemptNumber-1]
	}
	return runner.deferUntil(ctx, job, now.Add(wait), handlerErr, job.AttemptNumber)
}

// longestRetryDelay is where the doubling back-off stops: a job that keeps failing retries once
// a day until job_attempt_limit. A day is a sane ceiling the owner chose; the attempt limit, not
// an ever-longer wait, is what makes a failure permanent.
const longestRetryDelay = 24 * time.Hour

// doubledDelay is first doubled doublings times, stopping at longestRetryDelay. Stopping also
// keeps it from overflowing: the settings allow 100 attempts, and a wrapped shift would retry at
// once.
func doubledDelay(first time.Duration, doublings int64) time.Duration {
	wait := first
	for range doublings {
		if wait >= longestRetryDelay/2 {
			return longestRetryDelay
		}
		wait *= 2
	}
	return wait
}

func (runner *Runner) retryPolicy(ctx context.Context) (attemptLimit int64, firstRetryDelay time.Duration, err error) {
	attemptLimit, err = runner.queue.settings.Int(ctx, settings.JobAttemptLimit)
	if err != nil {
		return 0, 0, fmt.Errorf("read job_attempt_limit: %w", err)
	}
	seconds, err := runner.queue.settings.Int(ctx, settings.JobFirstRetryDelayS)
	if err != nil {
		return 0, 0, fmt.Errorf("read job_first_retry_delay_s: %w", err)
	}
	return attemptLimit, time.Duration(seconds) * time.Second, nil
}

func (runner *Runner) deferUntil(ctx context.Context, job Job, until time.Time, why error, attempts int64) error {
	message := why.Error()
	return runner.queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		err := queries.DeferJob(ctx, cachegen.DeferJobParams{
			RunnableAt: until.UnixMilli(), LastFailure: &message, AttemptCount: attempts, ID: job.ID,
		})
		if err != nil {
			return fmt.Errorf("defer job %d: %w", job.ID, err)
		}
		return nil
	})
}

func (runner *Runner) writeFinish(ctx context.Context, params cachegen.FinishJobParams) error {
	return runner.queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if err := queries.FinishJob(ctx, params); err != nil {
			return fmt.Errorf("finish job %d: %w", params.ID, err)
		}
		return nil
	})
}

func finishParams(job Job, state string, why error, now time.Time) cachegen.FinishJobParams {
	params := cachegen.FinishJobParams{State: state, FinishedAt: new(now.UnixMilli()), ID: job.ID}
	if state == stateDone {
		params.ProgressFraction = new(1.0)
	}
	if why != nil {
		params.LastFailure = new(why.Error())
	}
	return params
}
