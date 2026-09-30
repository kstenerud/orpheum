// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Handler runs one job. A nil error finishes it; a *RetryAtError defers it, MarkBlocked holds
// its kind and MarkStalled tries it again, none costing an attempt; MarkPermanent fails it; any
// other error retries it with jittered back-off until job_attempt_limit. A panic is recovered
// and counted as a stall is (D233 §6).
type Handler func(ctx context.Context, job Job) error

// Runner runs one kind of job, one at a time: one fetch per provider (cut-a §5.3).
type Runner struct {
	runner *jobqueue.Runner[Target]
}

// NewRunner returns the runner for one kind of job.
func (queue *Queue) NewRunner(kind Kind, handler Handler) *Runner {
	return &Runner{runner: queue.queue.NewRunner(kind, func(ctx context.Context, job jobqueue.Job[Target]) error {
		return handler(ctx, Job{
			ID: job.ID, Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON, AttemptNumber: job.AttemptNumber,
			PreviousAttemptToolFailed: job.PreviousAttemptToolFailed,
		})
	})}
}

// WithRetrySchedule gives the runner's kind its own retry schedule in place of the settings'
// doubling back-off: the wait before each retry, after which the job fails (D213 §5).
func (runner *Runner) WithRetrySchedule(retrySchedule []time.Duration) *Runner {
	runner.runner.WithRetrySchedule(retrySchedule)
	return runner
}

// Run runs due jobs on every tick and whenever the queue is woken, until ctx ends.
func (runner *Runner) Run(ctx context.Context, tick <-chan time.Time) error {
	return runner.runner.Run(ctx, tick)
}

// RunDue runs every job of the runner's kind whose time has come, highest priority first, and
// stops early when a handler says the kind must wait (a rate limit).
func (runner *Runner) RunDue(ctx context.Context) error {
	return runner.runner.RunDue(ctx)
}
