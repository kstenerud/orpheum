// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"
)

// Handler runs one job. Its answer says what happens next (answers.go): nil finishes it.
type Handler[T comparable] func(ctx context.Context, job Job[T]) error

// Runner runs one kind of job, one at a time; a kind may have several runners.
type Runner[T comparable] struct {
	queue         *Queue[T]
	kind          Kind
	handler       Handler[T]
	retrySchedule []time.Duration
	// heldUntil is when this runner may claim again after an answer that held it
	// (RetryAtError, Blocked).
	heldUntil time.Time
	// wake is this runner's own wake-up, left there when a job of its kind is queued.
	wake chan struct{}
}

// NewRunner returns a runner for one kind of job.
func (queue *Queue[T]) NewRunner(kind Kind, handler Handler[T]) *Runner[T] {
	return &Runner[T]{queue: queue, kind: kind, handler: handler, wake: queue.registerWake(kind)}
}

// WithRetrySchedule gives the runner's kind its own retry schedule in place of the policy's
// doubling back-off: the wait before each retry, after which the job fails. Each wait is jittered
// as the back-off's is.
func (runner *Runner[T]) WithRetrySchedule(retrySchedule []time.Duration) *Runner[T] {
	runner.retrySchedule = retrySchedule
	return runner
}

// Run runs due jobs on every tick and whenever the queue is woken, until ctx ends.
func (runner *Runner[T]) Run(ctx context.Context, tick <-chan time.Time) error {
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
// stops early when an answer holds the runner.
func (runner *Runner[T]) RunDue(ctx context.Context) error {
	for ctx.Err() == nil {
		now := runner.queue.clock.Now()
		if now.Before(runner.heldUntil) {
			return nil
		}
		row, found, err := runner.queue.storage.Claim(ctx, Claim{Kind: runner.kind, Now: now})
		if err != nil {
			return fmt.Errorf("claim a %s job: %w", runner.kind, err)
		}
		if !found {
			return nil
		}
		job := Job[T]{
			ID: row.ID, Kind: row.Kind, Target: row.Target, ArgsJSON: row.ArgsJSON, AttemptNumber: row.AttemptCount,
			PreviousAttemptToolFailed: row.LastAttemptToolFailed,
		}
		if err := runner.runClaimed(ctx, job, row.LastFailure); err != nil {
			return err
		}
	}
	return nil
}

// runClaimed runs a claimed job and records its outcome, unless the claim has taken it past its
// attempt limit, which a stop in the middle of every attempt can do: then it fails unrun
// (D233 §6).
func (runner *Runner[T]) runClaimed(ctx context.Context, job Job[T], lastFailure string) error {
	attemptLimit, err := runner.attemptLimit(ctx)
	if err != nil {
		return err
	}
	if job.AttemptNumber > attemptLimit {
		refusal := fmt.Sprintf("not run again: it has had its %d attempts", attemptLimit)
		if lastFailure != "" {
			refusal += "; the last ended: " + lastFailure
		}
		runner.queue.logger.WarnContext(ctx, "a job reached its attempt limit when claimed", "kind", job.Kind,
			"target", job.Target, "attemptLimit", attemptLimit)
		return runner.record(ctx, Outcome{
			ID: job.ID, State: StateFailed, At: runner.queue.clock.Now(), AttemptCount: job.AttemptNumber - 1,
			LastFailure: refusal,
		})
	}
	return runner.finish(ctx, job, runner.handle(ctx, job))
}

// handle runs one job's handler in a context a cancel ends (Queue.Cancel). A panic in the handler
// is recovered and answered as the tool's failure (D233 §6), so one job cannot bring the program
// down.
func (runner *Runner[T]) handle(ctx context.Context, job Job[T]) (answer error) {
	jobCtx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	runner.queue.trackCancel(job.ID, stop)
	defer runner.queue.untrackCancel(job.ID)
	defer func() {
		if recovered := recover(); recovered != nil {
			runner.queue.logger.ErrorContext(ctx, "a job's handler panicked", "kind", job.Kind, "target", job.Target,
				"panicValue", fmt.Sprint(recovered), "stack", string(debug.Stack()))
			answer = &toolError{err: &PanicError{Value: recovered}, isPanic: true}
		}
	}()
	return runner.handler(jobCtx, job)
}

// finish records a job's outcome from its handler's answer.
func (runner *Runner[T]) finish(ctx context.Context, job Job[T], answer error) error {
	now := runner.queue.clock.Now()
	if answer == nil {
		return runner.record(ctx, Outcome{ID: job.ID, State: StateDone, At: now, AttemptCount: job.AttemptNumber})
	}
	given := job.AttemptNumber - 1 // the attempt's count, given back
	if retry, isRetry := errors.AsType[*RetryAtError](answer); isRetry {
		runner.heldUntil = retry.At
		return runner.wait(ctx, job, Outcome{At: retry.At, AttemptCount: given, IsSnoozed: true}, answer)
	}
	if snooze, isSnooze := errors.AsType[*snoozed](answer); isSnooze {
		return runner.wait(ctx, job, Outcome{At: snooze.until, AttemptCount: given, IsSnoozed: true}, answer)
	}
	if failure, isPermanent := errors.AsType[*permanent](answer); isPermanent {
		return runner.fail(ctx, job, failure.Error())
	}
	firstRetryDelay, err := runner.queue.policy.FirstRetryDelay(ctx)
	if err != nil {
		return fmt.Errorf("read the first retry delay: %w", err)
	}
	if held, isBlocked := errors.AsType[*blocked](answer); isBlocked {
		runner.heldUntil = now.Add(runner.queue.jittered(ctx, firstRetryDelay))
		return runner.wait(ctx, job, Outcome{At: runner.heldUntil, AttemptCount: given, IsSnoozed: true}, held)
	}
	if failure, isToolFailure := errors.AsType[*toolError](answer); isToolFailure {
		if failure.isPanic && job.PreviousAttemptToolFailed {
			return runner.fail(ctx, job, "the tool failed on two attempts in a row: "+failure.Error())
		}
		at := now.Add(runner.queue.jittered(ctx, firstRetryDelay))
		return runner.wait(ctx, job, Outcome{At: at, AttemptCount: given, ToolFailed: true}, failure)
	}
	return runner.retryOrFail(ctx, job, answer, firstRetryDelay)
}

// retryOrFail answers a job's own failure: a retry after its back-off, or, at its attempt limit, a
// failure.
func (runner *Runner[T]) retryOrFail(
	ctx context.Context, job Job[T], answer error, firstRetryDelay time.Duration,
) error {
	attemptLimit, err := runner.attemptLimit(ctx)
	if err != nil {
		return err
	}
	if job.AttemptNumber >= attemptLimit {
		runner.queue.logger.WarnContext(ctx, "a job failed", "kind", job.Kind, "target", job.Target,
			"attemptCount", job.AttemptNumber, "err", answer)
		return runner.fail(ctx, job, answer.Error())
	}
	wait := doubledDelay(firstRetryDelay, job.AttemptNumber-1)
	if runner.retrySchedule != nil {
		wait = runner.retrySchedule[job.AttemptNumber-1]
	}
	at := runner.queue.clock.Now().Add(runner.queue.jittered(ctx, wait))
	return runner.wait(ctx, job, Outcome{At: at, AttemptCount: job.AttemptNumber}, answer)
}

// attemptLimit is how many attempts a job of the runner's kind may spend: its schedule's retries
// and the first attempt, or the policy's limit.
func (runner *Runner[T]) attemptLimit(ctx context.Context) (int64, error) {
	if runner.retrySchedule != nil {
		return int64(len(runner.retrySchedule)) + 1, nil
	}
	attemptLimit, err := runner.queue.policy.AttemptLimit(ctx)
	if err != nil {
		return 0, fmt.Errorf("read the attempt limit: %w", err)
	}
	return attemptLimit, nil
}

// wait returns a job to the queue, as outcome says, with its answer as its last failure.
func (runner *Runner[T]) wait(ctx context.Context, job Job[T], outcome Outcome, answer error) error {
	outcome.ID, outcome.State, outcome.LastFailure = job.ID, StateQueued, answer.Error()
	return runner.record(ctx, outcome)
}

// fail ends a job as failed, with why.
func (runner *Runner[T]) fail(ctx context.Context, job Job[T], why string) error {
	return runner.record(ctx, Outcome{
		ID: job.ID, State: StateFailed, At: runner.queue.clock.Now(), AttemptCount: job.AttemptNumber, LastFailure: why,
	})
}

func (runner *Runner[T]) record(ctx context.Context, outcome Outcome) error {
	if err := runner.queue.storage.Record(ctx, outcome); err != nil {
		return fmt.Errorf("record job %d's outcome: %w", outcome.ID, err)
	}
	return nil
}
