// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
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
	// recordFailure records a job's failure as its item's own, when the kind gives one.
	recordFailure FailureRecorder[T]
	// unrecorded is an outcome the storage failed to write, written before anything is claimed.
	unrecorded *Outcome
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

// FailureRecorder records that a job failed as its item's own failure, a fact in the program's
// state that its sweep passes over and a person may clear (D231 §4, §10), with what the job's last
// attempt said: the attempt limit reached, a permanent answer, or a claim past the limit. A
// destination's being down is never one: that job parked.
type FailureRecorder[T comparable] func(ctx context.Context, job Job[T], failure string) error

// WithFailureRecorder has the runner's kind record each failed job as its item's own failure.
func (runner *Runner[T]) WithFailureRecorder(record FailureRecorder[T]) *Runner[T] {
	runner.recordFailure = record
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

// RunDue runs every job of the runner's kind whose time has come and whose destination is not
// parked, highest priority first, once every assigned job of its kind whose time ran out has
// failed for it. While the storage is down it claims nothing until its probe.
func (runner *Runner[T]) RunDue(ctx context.Context) error {
	for ctx.Err() == nil {
		if isWritten, err := runner.recordUnrecorded(ctx); err != nil || !isWritten {
			return err
		}
		if isOverdue, err := runner.failOverdueAssigned(ctx); err != nil || isOverdue {
			if err != nil {
				return err
			}
			continue
		}
		claimed, found, err := runner.claim(ctx)
		if err != nil || !found {
			return err
		}
		if err := runner.runClaimed(ctx, jobOf(claimed.row), claimed); err != nil {
			return err
		}
	}
	return nil
}

// jobOf is a job as its handler receives it, from its row.
func jobOf[T comparable](row Row[T]) Job[T] {
	return Job[T]{
		ID: row.ID, Kind: row.Kind, Target: row.Target, ArgsJSON: row.ArgsJSON, Priority: row.Priority,
		AttemptNumber: row.AttemptCount, PreviousAttemptToolFailed: row.LastAttemptToolFailed,
	}
}

// Resolve takes the outcome of an assigned job of the runner's kind, which the machine it was given
// to said (Assign), as though its handler had answered it: nil is done, and the answers.go answers
// say what else happens. It reports false, and records nothing, for a job that is not assigned:
// cancelled meanwhile, or failed when its time ran out.
func (runner *Runner[T]) Resolve(ctx context.Context, id int64, answer error) (bool, error) {
	return runner.ResolveBy(ctx, id, func(Job[T]) error { return answer })
}

// ResolveBy is Resolve with the answer decide gives the job as it was assigned: what its handler
// would answer, knowing its attempt (Job.PreviousAttemptToolFailed and the rest).
func (runner *Runner[T]) ResolveBy(ctx context.Context, id int64, decide func(job Job[T]) error) (bool, error) {
	row, found, err := runner.queue.storage.TakeAssigned(ctx, id)
	if err != nil {
		return false, fmt.Errorf("take assigned job %d: %w", id, err)
	}
	if !found {
		return false, nil
	}
	if row.Kind != runner.kind {
		return false, fmt.Errorf("job %d is a %s job, not this runner's %s", id, row.Kind, runner.kind)
	}
	job := jobOf(row)
	return true, runner.finish(ctx, job, claimedRow[T]{row: row}, decide(job))
}

// failOverdueAssigned fails one assigned job of the runner's kind whose time ran out, as its own
// failure: retried after its back-off, or failed at its attempt limit. It reports whether there
// was one. While the storage is parked it waits for the claims' probe, and a write the storage
// refuses parks it, as a claim's does.
func (runner *Runner[T]) failOverdueAssigned(ctx context.Context) (bool, error) {
	queue := runner.queue
	if storage := queue.storageDestination; storage != "" && queue.isParked(storage) {
		return false, nil
	}
	now := queue.clock.Now()
	row, found, err := queue.storage.TakeOverdueAssigned(ctx, runner.kind, now)
	if err != nil && queue.isStorageFailure(err) {
		queue.Park(ctx, Parking{Destination: queue.storageDestination, Reason: storageReason(err)})
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("take an overdue assigned %s job: %w", runner.kind, err)
	}
	if !found {
		return false, nil
	}
	overdue := fmt.Errorf("its machine did not say how it ended by %s", row.AssignedUntil.UTC().Format(time.RFC3339))
	return true, runner.finish(ctx, jobOf(row), claimedRow[T]{row: row}, overdue)
}

// claimedRow is a claimed job, with the parked destinations it is the probe of: its own, and the
// storage when the claim was the storage's probe.
type claimedRow[T comparable] struct {
	row      Row[T]
	probesOf []Destination
}

// claim takes the next due job of the runner's kind that no parked destination holds, and makes
// it the probe of its destination when that one's probe is due. While the storage is parked,
// only its probe claims; a claim the storage fails parks it.
func (runner *Runner[T]) claim(ctx context.Context) (claimedRow[T], bool, error) {
	queue := runner.queue
	queue.claimMutex.Lock()
	defer queue.claimMutex.Unlock()
	now := queue.clock.Now()
	var claimed claimedRow[T]
	storage := queue.storageDestination
	if storage != "" && queue.isParked(storage) {
		if !queue.beginProbe(storage, now) {
			return claimed, false, nil
		}
		claimed.probesOf = append(claimed.probesOf, storage)
	}
	row, found, err := queue.storage.Claim(ctx, Claim{Kind: runner.kind, Now: now, Unclaimable: queue.unclaimable(now)})
	if err != nil && queue.isStorageFailure(err) {
		queue.Park(ctx, Parking{Destination: storage, Reason: storageReason(err), IsProbe: len(claimed.probesOf) > 0})
		return claimed, false, nil
	}
	if err != nil {
		for _, destination := range claimed.probesOf {
			queue.endProbeUnanswered(ctx, destination)
		}
		return claimed, false, fmt.Errorf("claim a %s job: %w", runner.kind, err)
	}
	if !found {
		// The claim wrote nothing to be refused, and nothing waits to be written: the storage is
		// taken to be back, and the next write that fails parks it again.
		for _, destination := range claimed.probesOf {
			queue.Unpark(ctx, destination)
		}
		return claimed, false, nil
	}
	claimed.row = row
	if row.Destination != "" && queue.beginProbe(row.Destination, now) {
		claimed.probesOf = append(claimed.probesOf, row.Destination)
	}
	return claimed, true, nil
}

// storageReason is why the storage failed, as a person is shown it.
func storageReason(err error) string {
	return "cannot write: " + err.Error()
}

// recordUnrecorded writes the outcome the storage failed to write before, when the storage's probe
// is due, and says whether nothing is left unwritten.
func (runner *Runner[T]) recordUnrecorded(ctx context.Context) (bool, error) {
	if runner.unrecorded == nil {
		return true, nil
	}
	queue := runner.queue
	storage := queue.storageDestination
	queue.claimMutex.Lock()
	isProbe := queue.beginProbe(storage, queue.clock.Now())
	queue.claimMutex.Unlock()
	if !isProbe && queue.isParked(storage) {
		return false, nil
	}
	err := queue.storage.Record(ctx, *runner.unrecorded)
	if err != nil && queue.isStorageFailure(err) {
		queue.Park(ctx, Parking{Destination: storage, Reason: storageReason(err), IsProbe: isProbe})
		return false, nil
	}
	if err != nil {
		if isProbe {
			queue.endProbeUnanswered(ctx, storage)
		}
		return false, fmt.Errorf("record job %d's outcome: %w", runner.unrecorded.ID, err)
	}
	runner.unrecorded = nil
	queue.Unpark(ctx, storage)
	return true, nil
}

// runClaimed runs a claimed job and records its outcome, unless the claim has taken it past its
// attempt limit, which a stop in the middle of every attempt can do: then it fails unrun
// (D233 §6).
func (runner *Runner[T]) runClaimed(ctx context.Context, job Job[T], claimed claimedRow[T]) error {
	attemptLimit, err := runner.attemptLimit(ctx)
	if err != nil {
		return err
	}
	if job.AttemptNumber > attemptLimit {
		refusal := fmt.Sprintf("not run again: it has had its %d attempts", attemptLimit)
		if lastFailure := claimed.row.LastFailure; lastFailure != "" {
			refusal += "; the last ended: " + lastFailure
		}
		runner.queue.logger.WarnContext(ctx, "a job reached its attempt limit when claimed", "kind", job.Kind,
			"target", job.Target, "attemptLimit", attemptLimit)
		runner.endProbes(ctx, claimed.probesOf, nil)
		runner.recordItemsFailure(ctx, job, refusal)
		return runner.record(ctx, Outcome{
			ID: job.ID, State: StateFailed, At: runner.queue.clock.Now(), AttemptCount: job.AttemptNumber - 1,
			LastFailure: refusal,
		})
	}
	isCancelled, answer := runner.handle(ctx, job, claimed)
	if isCancelled {
		// Cancelled: it stays so whatever it answered, and a cancel is no failure of its item.
		runner.endProbes(ctx, claimed.probesOf, answer)
		return runner.record(ctx, Outcome{
			ID: job.ID, State: StateFailed, At: runner.queue.clock.Now(), AttemptCount: job.AttemptNumber,
			LastFailure: errorText(answer),
		})
	}
	if runner.queue.isStorageFailure(answer) {
		answer = Park(Parking{Destination: runner.queue.storageDestination, Reason: storageReason(answer)}, answer)
	}
	runner.endProbes(ctx, claimed.probesOf, answer)
	return runner.finish(ctx, job, claimed, answer)
}

// endProbes says what a probe's answer found of each destination it probed: the destination
// failing again backs its next probe off (Park, from finish); the tool failing says nothing,
// and it is probed again at the same step; any other answer is the destination answering, and
// it is unparked.
func (runner *Runner[T]) endProbes(ctx context.Context, probesOf []Destination, answer error) {
	parking, isParked := ParkingOf(answer)
	var toolFailure *toolError
	isToolFailure := errors.As(answer, &toolFailure)
	for _, destination := range probesOf {
		switch {
		case isParked && parking.Destination == destination:
		case isToolFailure:
			runner.queue.endProbeUnanswered(ctx, destination)
		default:
			runner.queue.Unpark(ctx, destination)
		}
	}
}

// handle runs one job's handler in a context a cancel ends (Queue.Cancel), which says it is a
// job's (IsInJob) and which destination it probes (ProbeOf). A panic in the handler is recovered
// and answered as the tool's failure (D233 §6), so one job cannot bring the program down.
func (runner *Runner[T]) handle(
	ctx context.Context, job Job[T], claimed claimedRow[T],
) (isCancelled bool, answer error) {
	jobCtx, stop := context.WithCancelCause(context.WithValue(ctx, jobContextKey{}, true))
	defer stop(nil)
	defer func() { isCancelled = errors.Is(context.Cause(jobCtx), ErrCancelled) }()
	if destination := claimed.row.Destination; slices.Contains(claimed.probesOf, destination) {
		jobCtx = context.WithValue(jobCtx, probeContextKey{}, destination)
	}
	runner.queue.trackCancel(job.ID, stop)
	defer runner.queue.untrackCancel(job.ID)
	defer func() {
		if recovered := recover(); recovered != nil {
			runner.queue.logger.ErrorContext(ctx, "a job's handler panicked", "kind", job.Kind, "target", job.Target,
				"panicValue", fmt.Sprint(recovered), "stack", string(debug.Stack()))
			answer = &toolError{err: &PanicError{Value: recovered}, isPanic: true}
		}
	}()
	return false, runner.handler(jobCtx, job)
}

// errorText is an answer's words, or "" for none.
func errorText(answer error) string {
	if answer == nil {
		return ""
	}
	return answer.Error()
}

// finish records a job's outcome from its handler's answer. A job parked on a destination is
// kept with it, at no cost of an attempt; a probe that failed backs the next one off.
func (runner *Runner[T]) finish(ctx context.Context, job Job[T], claimed claimedRow[T], answer error) error {
	now, destination := runner.queue.clock.Now(), claimed.row.Destination
	if answer == nil {
		return runner.record(ctx, Outcome{ID: job.ID, State: StateDone, At: now, AttemptCount: job.AttemptNumber})
	}
	if handOff, isAssigned := errors.AsType[*assigned](answer); isAssigned {
		return runner.record(ctx, Outcome{
			ID: job.ID, State: StateAssigned, At: handOff.until, AttemptCount: job.AttemptNumber,
		})
	}
	given := job.AttemptNumber - 1 // the attempt's count, given back
	if parking, isParked := ParkingOf(answer); isParked {
		parking.IsProbe = slices.Contains(claimed.probesOf, parking.Destination)
		runner.queue.Park(ctx, parking)
		return runner.wait(ctx, job, Outcome{
			At: now, AttemptCount: given, Destination: cmp.Or(parking.Destination, destination),
		}, answer)
	}
	if snooze, isSnooze := errors.AsType[*snoozed](answer); isSnooze {
		return runner.wait(ctx, job, Outcome{
			At: snooze.until, AttemptCount: given, IsSnoozed: true, Destination: destination,
		}, answer)
	}
	if failure, isPermanent := errors.AsType[*permanent](answer); isPermanent {
		return runner.fail(ctx, job, failure.Error())
	}
	firstRetryDelay, err := runner.queue.policy.FirstRetryDelay(ctx)
	if err != nil {
		return fmt.Errorf("read the first retry delay: %w", err)
	}
	if failure, isToolFailure := errors.AsType[*toolError](answer); isToolFailure {
		if failure.isPanic && job.PreviousAttemptToolFailed {
			return runner.fail(ctx, job, "the tool failed on two attempts in a row: "+failure.Error())
		}
		at := now.Add(runner.queue.jittered(ctx, firstRetryDelay))
		return runner.wait(ctx, job, Outcome{
			At: at, AttemptCount: given, ToolFailed: true, Destination: destination,
		}, failure)
	}
	return runner.retryOrFail(ctx, job, destination, answer, firstRetryDelay)
}

// retryOrFail answers a job's own failure: a retry after its back-off, or, at its attempt limit, a
// failure.
func (runner *Runner[T]) retryOrFail(
	ctx context.Context, job Job[T], destination Destination, answer error, firstRetryDelay time.Duration,
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
	return runner.wait(ctx, job, Outcome{At: at, AttemptCount: job.AttemptNumber, Destination: destination}, answer)
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

// fail ends a job as failed, with why, and records it as its item's own failure.
func (runner *Runner[T]) fail(ctx context.Context, job Job[T], why string) error {
	runner.recordItemsFailure(ctx, job, why)
	return runner.record(ctx, Outcome{
		ID: job.ID, State: StateFailed, At: runner.queue.clock.Now(), AttemptCount: job.AttemptNumber, LastFailure: why,
	})
}

func (runner *Runner[T]) record(ctx context.Context, outcome Outcome) error {
	err := runner.queue.storage.Record(ctx, outcome)
	if err != nil && runner.queue.isStorageFailure(err) {
		// Kept, and written first once the storage's probe finds it writable: the job is not left
		// running for want of a write (S77).
		runner.unrecorded = &outcome
		runner.queue.Park(ctx, Parking{Destination: runner.queue.storageDestination, Reason: storageReason(err)})
		return nil
	}
	if err != nil {
		return fmt.Errorf("record job %d's outcome: %w", outcome.ID, err)
	}
	return nil
}

// recordItemsFailure records a failed job as its item's own failure, when the kind records one. A
// failure to record it is logged, and the job still fails: the fact not written, its sweep derives
// the work again, and it is recorded when that fails.
func (runner *Runner[T]) recordItemsFailure(ctx context.Context, job Job[T], why string) {
	if runner.recordFailure == nil {
		return
	}
	if err := runner.recordFailure(ctx, job, why); err != nil {
		runner.queue.logger.ErrorContext(ctx, "could not record a job's failure as its item's", "kind", job.Kind,
			"target", job.Target, "err", err)
	}
}
