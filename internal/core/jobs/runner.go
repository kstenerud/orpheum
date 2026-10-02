// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Handler runs one job. A nil error finishes it; Park waits for its destination and MarkStalled
// tries it again, neither costing an attempt; MarkPermanent fails it; any other error retries it
// with jittered back-off until job_attempt_limit. A panic is recovered and counted as a stall is
// (D233 §6).
type Handler func(ctx context.Context, job Job) error

// Runner runs one kind of job, one at a time: one fetch per provider (cut-a §5.3).
type Runner struct {
	runner *jobqueue.Runner[Target]
}

// NewRunner returns the runner for one kind of job.
func (queue *Queue) NewRunner(kind Kind, handler Handler) *Runner {
	return &Runner{runner: queue.queue.NewRunner(kind, func(ctx context.Context, job jobqueue.Job[Target]) error {
		return handler(ctx, jobOf(job))
	})}
}

// jobOf is a claimed job as a handler receives it.
func jobOf(job jobqueue.Job[Target]) Job {
	return Job{
		ID: job.ID, Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON, Priority: job.Priority,
		AttemptNumber: job.AttemptNumber, PreviousAttemptToolFailed: job.PreviousAttemptToolFailed,
	}
}

// WithRetrySchedule gives the runner's kind its own retry schedule in place of the settings'
// doubling back-off: the wait before each retry, after which the job fails (D213 §5).
func (runner *Runner) WithRetrySchedule(retrySchedule []time.Duration) *Runner {
	runner.runner.WithRetrySchedule(retrySchedule)
	return runner
}

// FailureRecorder records a failed job as its item's own failure, a fact in persistent.db that the
// kind's sweep passes over and a person may clear (D231 §4, §10), with what its last attempt said.
// A job whose destination was down never failed: it parked.
type FailureRecorder func(ctx context.Context, job Job, failure string) error

// WithFailureRecorder has the runner's kind record each failed job as its item's own failure.
func (runner *Runner) WithFailureRecorder(record FailureRecorder) *Runner {
	runner.runner.WithFailureRecorder(func(ctx context.Context, job jobqueue.Job[Target], failure string) error {
		return record(ctx, jobOf(job), failure)
	})
	return runner
}

// Run runs due jobs on every tick and whenever the queue is woken, until ctx ends.
func (runner *Runner) Run(ctx context.Context, tick <-chan time.Time) error {
	return runner.runner.Run(ctx, tick)
}

// RunDue runs every job of the runner's kind whose time has come and whose destination is not
// parked, highest priority first.
func (runner *Runner) RunDue(ctx context.Context) error {
	return runner.runner.RunDue(ctx)
}

// ResolveBy takes the outcome of an assigned job of the runner's kind, which a node said (Assign),
// as decide answers it for the job as it was assigned, as its handler would have. It reports false,
// recording nothing, for a job no longer assigned: cancelled, or failed when its time ran out.
func (runner *Runner) ResolveBy(ctx context.Context, id int64, decide func(job Job) error) (bool, error) {
	resolved, err := runner.runner.ResolveBy(ctx, id, func(job jobqueue.Job[Target]) error {
		return decide(jobOf(job))
	})
	if err != nil {
		return false, fmt.Errorf("resolve job %d: %w", id, err)
	}
	return resolved, nil
}

// Assign is a handler's answer that its job's work was given to a node, which says later how it
// ended: the job is assigned until then, or until until, past which it is its own failure (D234 §7).
func Assign(until time.Time) error { return jobqueue.Assign(until) }
