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

// Handler runs one job. A nil error finishes it; a *RetryAtError defers it and Blocked holds its kind,
// neither costing an attempt; Permanent fails it; any other error retries it with back-off until
// the retry limit.
type Handler func(ctx context.Context, job Job) error

// Runner runs one kind of job, one at a time: one fetch per provider (cut-a §5.3).
type Runner struct {
	queue       *Queue
	kind        Kind
	handler     Handler
	schedule    []time.Duration
	pausedUntil time.Time
}

// RetryOn gives the runner's kind its own retry schedule in place of the settings' doubling
// back-off: the wait before each retry, after which the job fails (D213 §5).
func (runner *Runner) RetryOn(schedule []time.Duration) *Runner {
	runner.schedule = schedule
	return runner
}

// NewRunner returns the runner for one kind of job.
func (queue *Queue) NewRunner(kind Kind, handler Handler) *Runner {
	return &Runner{queue: queue, kind: kind, handler: handler}
}

// Run runs due jobs on every tick and whenever the queue is woken, until ctx ends.
func (runner *Runner) Run(ctx context.Context, tick <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
		case <-runner.queue.wake:
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
		if err := runner.finish(ctx, job, runner.handler(ctx, job)); err != nil {
			return err
		}
	}
	return nil
}

// claim marks the next due job running.
func (runner *Runner) claim(ctx context.Context, now time.Time) (Job, bool, error) {
	var job Job
	found := false
	err := runner.queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		row, nextErr := store.One(queries.NextQueuedJob(ctx, cachegen.NextQueuedJobParams{
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
			ID: row.ID, Kind: runner.kind, Subject: deref(row.Subject), Args: row.Args, Attempts: row.Attempts + 1,
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
func (runner *Runner) finish(ctx context.Context, job Job, outcome error) error {
	now := runner.queue.clock.Now()
	if outcome == nil {
		return runner.write(ctx, finishParams(job, stateDone, nil, now))
	}
	if retry, isRetry := errors.AsType[*RetryAtError](outcome); isRetry {
		runner.pausedUntil = retry.At
		return runner.deferUntil(ctx, job, retry.At, outcome, job.Attempts-1)
	}
	if failure, isPermanent := errors.AsType[*permanent](outcome); isPermanent {
		return runner.write(ctx, finishParams(job, stateFailed, failure, now))
	}
	limit, backoff, err := runner.retryPolicy(ctx)
	if err != nil {
		return err
	}
	if held, isBlocked := errors.AsType[*blocked](outcome); isBlocked {
		runner.pausedUntil = now.Add(backoff)
		return runner.deferUntil(ctx, job, runner.pausedUntil, held, job.Attempts-1)
	}
	if runner.schedule != nil {
		limit = int64(len(runner.schedule)) + 1
	}
	if job.Attempts >= limit {
		runner.queue.logger.WarnContext(ctx, "a job failed", "kind", job.Kind, "subject", job.Subject,
			"attempts", job.Attempts, "err", outcome)
		return runner.write(ctx, finishParams(job, stateFailed, outcome, now))
	}
	wait := backoff << (job.Attempts - 1)
	if runner.schedule != nil {
		wait = runner.schedule[job.Attempts-1]
	}
	return runner.deferUntil(ctx, job, now.Add(wait), outcome, job.Attempts)
}

func (runner *Runner) retryPolicy(ctx context.Context) (limit int64, backoff time.Duration, err error) {
	limit, err = runner.queue.settings.Int(ctx, settings.JobAttemptLimit)
	if err != nil {
		return 0, 0, fmt.Errorf("read the retry limit: %w", err)
	}
	seconds, err := runner.queue.settings.Int(ctx, settings.JobRetryBackoffS)
	if err != nil {
		return 0, 0, fmt.Errorf("read the retry back-off: %w", err)
	}
	return limit, time.Duration(seconds) * time.Second, nil
}

func (runner *Runner) deferUntil(ctx context.Context, job Job, until time.Time, why error, attempts int64) error {
	message := why.Error()
	return runner.queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		err := queries.DeferJob(ctx, cachegen.DeferJobParams{
			RunnableAt: until.UnixMilli(), LastError: &message, Attempts: attempts, ID: job.ID,
		})
		if err != nil {
			return fmt.Errorf("defer job %d: %w", job.ID, err)
		}
		return nil
	})
}

func (runner *Runner) write(ctx context.Context, params cachegen.FinishJobParams) error {
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
		params.Progress = new(1.0)
	}
	if why != nil {
		params.LastError = new(why.Error())
	}
	return params
}

func deref(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
