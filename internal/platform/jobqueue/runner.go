// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
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
	// wake is this runner's own wake-up, left there when a job of its kind is queued.
	wake chan struct{}
	// failing is the runner's lasting failure to run, said once a minute while it lasts.
	failing slogx.Failing
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
		if err := runner.RunDue(ctx); ctx.Err() == nil {
			runner.sayRun(ctx, err)
		}
	}
}

// sayRun logs a tick's failure to run, while it lasts once a minute with how often it failed
// (slogx.Failing), and the tick that runs again after one: a runner ticks every second, and logged
// each tick's failure while cache.db could not be written (the audit of 2026-10-25, C12).
func (runner *Runner[T]) sayRun(ctx context.Context, err error) {
	now, logger := runner.queue.clock.Now(), runner.queue.logger
	if err == nil {
		if isEnded, lasted := runner.failing.Succeed(now); isEnded {
			logger.InfoContext(ctx, "the job queue runs again", append([]any{"kind", runner.kind}, lasted...)...)
		}
		return
	}
	if isSaid, lasting := runner.failing.Fail(now); isSaid {
		logger.ErrorContext(ctx, "the job queue could not run",
			append([]any{"kind", runner.kind, "err", err}, lasting...)...)
	}
}

// RunDue runs every job of the runner's kind whose time has come and whose destination is not
// parked, highest priority first, once every assigned job of its kind whose time ran out has
// failed for it. While the storage is down it claims nothing until its probe. It reads its retry
// policy before it claims, so nothing it claims is left running for want of it (D540).
func (runner *Runner[T]) RunDue(ctx context.Context) error {
	for ctx.Err() == nil {
		if isWritten, err := runner.queue.writeUnwritten(ctx); err != nil || !isWritten {
			return err
		}
		policy, policyErr := runner.readPolicy(ctx)
		if policyErr != nil {
			return policyErr
		}
		if isOverdue, err := runner.failOverdueAssigned(ctx, policy); err != nil || isOverdue {
			if err != nil {
				return err
			}
			continue
		}
		claimed, found, err := runner.claim(ctx)
		if err != nil || !found {
			return err
		}
		err = runner.runClaimed(ctx, jobOf(claimed.row), claimed, policy)
		runner.queue.release(claimed.row.ID)
		if err != nil {
			return err
		}
	}
	return nil
}

// policyRead is the retry policy as a runner read it before it claimed or took a job: nothing
// between a claim and its record reads it, so nothing there can fail to (D540).
type policyRead struct {
	attemptLimit    int64
	firstRetryDelay time.Duration
}

// readPolicy reads the runner's attempt limit and the policy's first retry delay.
func (runner *Runner[T]) readPolicy(ctx context.Context) (policyRead, error) {
	attemptLimit, err := runner.attemptLimit(ctx)
	if err != nil {
		return policyRead{}, err
	}
	firstRetryDelay, err := runner.queue.policy.FirstRetryDelay(ctx)
	if err != nil {
		return policyRead{}, fmt.Errorf("read the first retry delay: %w", err)
	}
	return policyRead{attemptLimit: attemptLimit, firstRetryDelay: firstRetryDelay}, nil
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
// would answer, knowing its attempt (Job.PreviousAttemptToolFailed and the rest). A job taken
// whose outcome then cannot be written is given back to the queue, never left running (D540).
func (runner *Runner[T]) ResolveBy(ctx context.Context, id int64, decide func(job Job[T]) error) (bool, error) {
	policy, err := runner.readPolicy(ctx)
	if err != nil {
		return false, err
	}
	queue := runner.queue
	taken := queue.clock.Now()
	row, found, err := queue.storage.TakeAssigned(ctx, id)
	if err != nil {
		return false, fmt.Errorf("take assigned job %d: %w", id, err)
	}
	if !found {
		return false, nil
	}
	queue.hold(id)
	defer queue.release(id)
	if row.Kind != runner.kind {
		err := fmt.Errorf("job %d is a %s job, not this runner's %s", id, row.Kind, runner.kind)
		return false, errors.Join(err, queue.giveBack(ctx, id, err))
	}
	job := jobOf(row)
	return true, runner.finish(ctx, job, claimedRow[T]{row: row, at: taken}, decide(job), policy)
}

// failOverdueAssigned fails one assigned job of the runner's kind whose time ran out, as its own
// failure: retried after its back-off, or failed at its attempt limit. It reports whether there
// was one. While the storage is parked it waits for the claims' probe, and a write the storage
// refuses parks it, as a claim's does.
func (runner *Runner[T]) failOverdueAssigned(ctx context.Context, policy policyRead) (bool, error) {
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
	queue.hold(row.ID)
	defer queue.release(row.ID)
	overdue := fmt.Errorf("%w by %s", ErrOverdue, row.AssignedUntil.UTC().Format(time.RFC3339))
	taken := claimedRow[T]{row: row, at: now}
	if finishErr := runner.finish(ctx, jobOf(row), taken, overdue, policy); finishErr != nil {
		return true, finishErr
	}
	queue.tellOverdue(ctx, row.ID)
	return true, nil
}

// tellOverdue tells the queue's overdue hook that a job's machine no longer has its work: its time
// ran out, or its assignment could not be written.
func (queue *Queue[T]) tellOverdue(ctx context.Context, id int64) {
	queue.mutex.Lock()
	tell := queue.overdue
	queue.mutex.Unlock()
	if tell != nil {
		tell(ctx, id)
	}
}

// claimedRow is a claimed job, with the parked destinations it is the probe of: its own, and the
// storage when the claim was the storage's probe, and when it was claimed: a signal its handler's
// parking awaits that came before then answers nothing the handler looked at.
type claimedRow[T comparable] struct {
	row      Row[T]
	probesOf []Destination
	at       time.Time
}

// claim takes the next due job of the runner's kind that no parked destination holds, and makes
// it the probe of its destination when that one's probe is due. While the storage is parked,
// only its probe claims; a claim the storage fails parks it. A job claimed is held (held.go)
// until its runner releases it.
func (runner *Runner[T]) claim(ctx context.Context) (claimedRow[T], bool, error) {
	queue := runner.queue
	queue.claimMutex.Lock()
	defer queue.claimMutex.Unlock()
	now := queue.clock.Now()
	claimed := claimedRow[T]{at: now}
	storage := queue.storageDestination
	if storage != "" && queue.isParked(storage) {
		if !queue.beginProbe(storage, now) {
			return claimed, false, nil
		}
		claimed.probesOf = append(claimed.probesOf, storage)
	}
	row, found, err := queue.storage.Claim(ctx, Claim{
		Kind: runner.kind, Now: now, Unclaimable: queue.unclaimable(now), ProbedLast: queue.probedLast(),
	})
	if err != nil && queue.isStorageFailure(err) {
		queue.Park(ctx, Parking{
			Destination: storage, Reason: storageReason(err), IsProbeFailure: len(claimed.probesOf) > 0,
		})
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
	queue.hold(row.ID)
	if row.Destination != "" && queue.beginProbe(row.Destination, now) {
		claimed.probesOf = append(claimed.probesOf, row.Destination)
	}
	return claimed, true, nil
}

// storageReason is why the storage failed, as a person is shown it.
func storageReason(err error) string {
	return "cannot write: " + err.Error()
}

// runClaimed runs a claimed job and records its outcome, unless the claim has taken it past its
// attempt limit, which a stop in the middle of every attempt can do: then it fails unrun
// (D233 §6).
func (runner *Runner[T]) runClaimed(ctx context.Context, job Job[T], claimed claimedRow[T], policy policyRead) error {
	if attemptLimit := policy.attemptLimit; job.AttemptNumber > attemptLimit {
		refusal := fmt.Sprintf("not run again: it has had its %d attempts", attemptLimit)
		if lastFailure := claimed.row.LastFailure; lastFailure != "" {
			refusal += "; the last ended: " + lastFailure
		}
		runner.queue.logger.WarnContext(ctx, "a job reached its attempt limit when claimed", "kind", job.Kind,
			"target", job.Target, "attemptLimit", attemptLimit)
		runner.endProbes(ctx, job.ID, claimed.probesOf, nil)
		isFailed, err := runner.fail(ctx, job, refusal, job.AttemptNumber-1)
		if isFailed {
			runner.tellUnknownFailure(ctx, job, refusal)
		}
		return err
	}
	isCancelled, answer := runner.handle(ctx, job, claimed)
	if isCancelled {
		// Cancelled: it stays so whatever it answered, and a cancel is no failure of its item.
		runner.endProbes(ctx, job.ID, claimed.probesOf, answer)
		_, err := runner.record(ctx, Outcome{
			ID: job.ID, State: StateFailed, At: runner.queue.clock.Now(), AttemptCount: job.AttemptNumber,
			LastFailure: errorText(answer),
		})
		return err
	}
	if runner.queue.isStorageFailure(answer) {
		answer = Park(Parking{Destination: runner.queue.storageDestination, Reason: storageReason(answer)}, answer)
	}
	runner.endProbes(ctx, job.ID, claimed.probesOf, answer)
	return runner.finish(ctx, job, claimed, answer, policy)
}

// park keeps a job whose answer parked its destination with it, at no cost of an attempt, unless
// the job has failed alone: its destination's last probes, or one another job's probe showed its
// own (D531 §2, §3). A job whose wait is not written, cancelled meanwhile, parks nothing (D540).
func (runner *Runner[T]) park(
	ctx context.Context, job Job[T], claimed claimedRow[T], parking Parking, answer error, policy policyRead,
) error {
	if runner.queue.hasFailedAlone(job.ID) {
		return runner.takeAsTheJobsFailure(ctx, job, parking, answer,
			"another job found the destination up after this one's probe failed", policy)
	}
	parking.IsProbeFailure = slices.Contains(claimed.probesOf, parking.Destination)
	if parking.IsProbeFailure &&
		runner.queue.probeFailedBy(parking.Destination, job.ID) >= ProbeFailuresThatAreTheJobs {
		return runner.takeAsTheJobsFailure(ctx, job, parking, answer,
			fmt.Sprintf("it failed %d of the destination's probes in a row", ProbeFailuresThatAreTheJobs), policy)
	}
	// A tool that failed in a way that parks the job, as a stall on a disk parks it on its root, is
	// still a tool failure: the next attempt knows it.
	_, isToolFailure := errors.AsType[*toolError](answer) //nolint:errcheck // AsType's second value is a bool
	isWaiting, err := runner.wait(ctx, job, Outcome{
		At: runner.queue.clock.Now(), AttemptCount: job.AttemptNumber - 1, ToolFailed: isToolFailure,
		Destination: cmp.Or(parking.Destination, claimed.row.Destination),
	}, answer)
	if isWaiting {
		runner.queue.park(ctx, parking, claimed.at)
	}
	return err
}

// takeAsTheJobsFailure takes the failure of a job that failed alone (park) as the job's own
// (D531 §2, §3): the destination is up, and the job spends an attempt and waits its back-off, as
// any failure of its own does.
func (runner *Runner[T]) takeAsTheJobsFailure(
	ctx context.Context, job Job[T], parking Parking, answer error, why string, policy policyRead,
) error {
	runner.queue.logger.WarnContext(ctx, "a job's failure parks its destination no more; it is taken as the job's",
		"destination", parking.Destination, "kind", job.Kind, "target", job.Target, "why", why, "err", answer)
	runner.queue.Unpark(ctx, parking.Destination)
	return runner.retryOrFail(ctx, job, parking.Destination, errors.Unwrap(answer), policy)
}

// endProbes says what a probe's answer found of each destination it probed: the destination
// failing again backs its next probe off (Park, from finish); the tool failing says nothing,
// and it is probed again at the same step; any other answer is the destination answering, and
// it is unparked.
func (runner *Runner[T]) endProbes(ctx context.Context, job int64, probesOf []Destination, answer error) {
	parking, isParked := ParkingOf(answer)
	var toolFailure *toolError
	isToolFailure := errors.As(answer, &toolFailure)
	for _, destination := range probesOf {
		switch {
		case isParked && parking.Destination == destination:
		case isToolFailure:
			runner.queue.endProbeUnanswered(ctx, destination)
		default:
			runner.queue.probeFoundUp(destination, job)
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
// kept with it, at no cost of an attempt; a probe that failed backs the next one off. An outcome
// that finds its job cancelled is not acted on: nothing is handed over, counted or parked (D540).
func (runner *Runner[T]) finish(
	ctx context.Context, job Job[T], claimed claimedRow[T], answer error, policy policyRead,
) error {
	now, destination := runner.queue.clock.Now(), claimed.row.Destination
	if answer == nil {
		recorded, err := runner.record(ctx, Outcome{
			ID: job.ID, State: StateDone, At: now, AttemptCount: job.AttemptNumber,
		})
		if recorded == writtenNever {
			return err
		}
		runner.queue.succeeded(job.ID)
		if unknownFailures := runner.queue.unknownFailuresTold(); unknownFailures != nil {
			unknownFailures.Done(ctx, job)
		}
		return nil
	}
	given := job.AttemptNumber - 1 // the attempt's count, given back
	if handOff, isAssigned := errors.AsType[*assigned](answer); isAssigned {
		return runner.assign(ctx, job, Outcome{
			ID: job.ID, State: StateAssigned, At: handOff.until, AttemptCount: job.AttemptNumber,
		}, destination, answer)
	}
	if parking, isParked := ParkingOf(answer); isParked {
		return runner.park(ctx, job, claimed, parking, answer, policy)
	}
	//nolint:errcheck // AsType's second value is a bool
	if _, isBumped := errors.AsType[*bumped](answer); isBumped {
		head := Outcome{At: headOfTheQueue(), AttemptCount: given, Destination: destination}
		_, err := runner.wait(ctx, job, head, answer)
		return err
	}
	if snooze, isSnooze := errors.AsType[*snoozed](answer); isSnooze {
		_, err := runner.wait(ctx, job, Outcome{
			At: snooze.until, AttemptCount: given, IsSnoozed: true, Destination: destination,
		}, answer)
		return err
	}
	if failure, isPermanent := errors.AsType[*permanent](answer); isPermanent {
		_, err := runner.fail(ctx, job, failure.Error(), job.AttemptNumber)
		return err
	}
	if failure, isToolFailure := errors.AsType[*toolError](answer); isToolFailure {
		if failure.isPanic && job.PreviousAttemptToolFailed {
			return runner.failUnknown(ctx, job, "the tool failed on two attempts in a row: "+failure.Error())
		}
		at := now.Add(runner.queue.jittered(ctx, policy.firstRetryDelay))
		_, err := runner.wait(ctx, job, Outcome{
			At: at, AttemptCount: given, ToolFailed: true, Destination: destination,
		}, failure)
		return err
	}
	return runner.retryOrFail(ctx, job, destination, answer, policy)
}

// assign records a job given to another machine assigned, and only then hands its work over, so
// the machine's word, however soon, finds it assigned. An assignment the storage refuses for want
// of room is kept as the job's return to the queue, at no cost of an attempt, and its work is not
// handed over: the machine's attempt is forgotten (the overdue hook), and the job is given again
// once there is room (D540).
func (runner *Runner[T]) assign(
	ctx context.Context, job Job[T], outcome Outcome, destination Destination, answer error,
) error {
	queue := runner.queue
	instead := Outcome{
		ID: job.ID, State: StateQueued, At: queue.clock.Now(), AttemptCount: job.AttemptNumber - 1,
		Destination: destination, LastFailure: "its assignment could not be written",
	}
	recorded, err := runner.recordOr(ctx, outcome, instead)
	switch recorded {
	case writtenNow:
		HandOver(ctx, answer)
	case writtenLater:
		queue.tellOverdue(ctx, job.ID)
	case writtenNever:
	}
	return err
}

// retryOrFail answers a job's own failure: a retry after its back-off, or, at its attempt limit, a
// failure of unknown cause, unless its cause is known to pass: then a retry after the longest retry
// delay, its attempts held under the limit, as long as it fails so (D511 §1).
func (runner *Runner[T]) retryOrFail(
	ctx context.Context, job Job[T], destination Destination, answer error, policy policyRead,
) error {
	attemptLimit := policy.attemptLimit
	if job.AttemptNumber >= attemptLimit && runner.queue.isPassingFailure(answer) {
		runner.queue.logger.WarnContext(ctx, "a job keeps failing for a cause that passes", "kind", job.Kind,
			"target", job.Target, "attemptCount", job.AttemptNumber, "err", answer)
		at := runner.queue.clock.Now().Add(runner.queue.jittered(ctx, runner.queue.longestRetryDelay(ctx)))
		last := Outcome{At: at, AttemptCount: attemptLimit - 1, Destination: destination}
		_, err := runner.wait(ctx, job, last, answer)
		return err
	}
	if job.AttemptNumber >= attemptLimit {
		runner.queue.logger.WarnContext(ctx, "a job failed", "kind", job.Kind, "target", job.Target,
			"attemptCount", job.AttemptNumber, "err", answer)
		return runner.failUnknown(ctx, job, answer.Error())
	}
	wait := runner.queue.doubledDelay(ctx, policy.firstRetryDelay, job.AttemptNumber-1)
	if runner.retrySchedule != nil {
		wait = runner.retrySchedule[job.AttemptNumber-1]
	}
	at := runner.queue.clock.Now().Add(runner.queue.jittered(ctx, wait))
	_, err := runner.wait(ctx, job, Outcome{At: at, AttemptCount: job.AttemptNumber, Destination: destination}, answer)
	return err
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

// wait returns a job to the queue, as outcome says, with its answer as its last failure, and says
// whether it waits: false is a job cancelled meanwhile.
func (runner *Runner[T]) wait(ctx context.Context, job Job[T], outcome Outcome, answer error) (bool, error) {
	outcome.ID, outcome.State, outcome.LastFailure = job.ID, StateQueued, answer.Error()
	recorded, err := runner.record(ctx, outcome)
	return recorded != writtenNever, err
}

// fail ends a job as failed, with why and the attempt count it ends with, and records it as its
// item's own failure. It says whether the job failed: false is one cancelled meanwhile, or whose
// end could not be written, which is no failure of its item.
func (runner *Runner[T]) fail(ctx context.Context, job Job[T], why string, attemptCount int64) (bool, error) {
	recorded, err := runner.record(ctx, Outcome{
		ID: job.ID, State: StateFailed, At: runner.queue.clock.Now(), AttemptCount: attemptCount, LastFailure: why,
	})
	if recorded == writtenNever {
		return false, err
	}
	runner.recordItemsFailure(ctx, job, why)
	return true, nil
}

// failUnknown fails a job for a cause neither known to block it nor known to pass, and tells the
// queue's UnknownFailures (D511 §2).
func (runner *Runner[T]) failUnknown(ctx context.Context, job Job[T], why string) error {
	isFailed, err := runner.fail(ctx, job, why, job.AttemptNumber)
	if isFailed {
		runner.tellUnknownFailure(ctx, job, why)
	}
	return err
}

func (runner *Runner[T]) tellUnknownFailure(ctx context.Context, job Job[T], why string) {
	if unknownFailures := runner.queue.unknownFailuresTold(); unknownFailures != nil {
		unknownFailures.Failed(ctx, job, why)
	}
}

// recorded is what became of an outcome a runner recorded.
type recorded int

const (
	// The storage wrote it.
	writtenNow recorded = iota
	// The storage refused it for want of room, and it is kept and written once the storage's
	// probe finds room (S77): it stands.
	writtenLater
	// It is not written and never will be: its job was cancelled meanwhile, and the queue's
	// cancel hook is told again, or the write failed, and the job is given back (D540).
	writtenNever
)

// record writes a job's outcome. One the storage refuses for want of room is kept and the storage
// parked; one that finds its job cancelled tells the cancel hook again, since the cancel's own word
// may have come while the handler still gave the job's work away; and on any other failure the job
// is given back to the queue, so no outcome lost leaves a job running (D540).
func (runner *Runner[T]) record(ctx context.Context, outcome Outcome) (recorded, error) {
	return runner.recordOr(ctx, outcome, outcome)
}

// recordOr is record, keeping instead in place of outcome when the storage refuses it for want of
// room.
func (runner *Runner[T]) recordOr(ctx context.Context, outcome, instead Outcome) (recorded, error) {
	queue := runner.queue
	isWritten, err := queue.storage.Record(ctx, outcome)
	switch {
	case err != nil && queue.isStorageFailure(err):
		queue.keepUnwritten(instead)
		queue.Park(ctx, Parking{Destination: queue.storageDestination, Reason: storageReason(err)})
		return writtenLater, nil
	case err != nil:
		err = fmt.Errorf("record job %d's outcome: %w", outcome.ID, err)
		return writtenNever, errors.Join(err, queue.giveBack(ctx, outcome.ID, err))
	case !isWritten:
		queue.tellCancelled(ctx, outcome.ID)
		return writtenNever, nil
	}
	return writtenNow, nil
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
