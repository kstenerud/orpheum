// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// A handler answers with an error, or nil when the job is done. These wrap the error to say what
// the queue does next; any other error is the job's own failure, retried with back-off until the
// attempt limit. Only an error wrapped this way costs no attempt. The queue reads the first it
// finds of Assign, Park, Snooze, Permanent and ToolFailed, in that order.

// Park answers that the job's destination is down, a failure that says nothing of the job's own
// item (D231 §5): the job returns to the queue at no cost of an attempt, kept with that
// destination, and no job kept with it is claimed until its probe is due or it is unparked
// (D231 §6).
func Park(parking Parking, err error) error { return &parked{parking: parking, err: err} }

type parked struct {
	parking Parking
	err     error
}

func (failure *parked) Error() string { return failure.err.Error() }
func (failure *parked) Unwrap() error { return failure.err }

// ParkingOf is the destination an answer parks its job on, if it does.
func ParkingOf(answer error) (Parking, bool) {
	failure, isParked := errors.AsType[*parked](answer)
	if !isParked {
		return Parking{}, false
	}
	return failure.parking, true
}

// Assign answers that the job's work was given to another machine, a node, which says later how it
// ended (D234 §7): the job is assigned, its runner goes on with other jobs, and nothing waits for
// it. Runner.Resolve takes the node's outcome, as a handler's answer; past until, the job's time
// has run out, and that is its own failure.
func Assign(until time.Time) error { return &assigned{until: until} }

// AssignThen is Assign, with what to do once the job is recorded assigned: hand the work over, so the
// machine's word, however soon it comes, finds the job assigned (HandOver).
func AssignThen(until time.Time, then func(ctx context.Context)) error {
	return &assigned{until: until, then: then}
}

// HandOver does what an AssignThen answer does once its job is recorded assigned; any other answer does
// nothing. The runner calls it, and so does a test that runs a handler by itself.
func HandOver(ctx context.Context, answer error) {
	if handOff, isAssigned := errors.AsType[*assigned](answer); isAssigned && handOff.then != nil {
		handOff.then(ctx)
	}
}

// IsAssigned is whether a handler's answer is Assign: its job given to another machine.
func IsAssigned(answer error) bool {
	var handOff *assigned
	return errors.As(answer, &handOff)
}

type assigned struct {
	until time.Time
	then  func(ctx context.Context)
}

func (handOff *assigned) Error() string {
	return "assigned to another machine until " + handOff.until.UTC().Format(time.RFC3339)
}

// Snooze answers that the job should wait until a time at no cost of an attempt, while the
// runner goes on with other jobs. A repeat request does not bring it forward.
func Snooze(err error, until time.Time) error { return &snoozed{err: err, until: until} }

type snoozed struct {
	err   error
	until time.Time
}

func (snooze *snoozed) Error() string { return snooze.err.Error() }
func (snooze *snoozed) Unwrap() error { return snooze.err }

// Bump answers that work of a higher priority took the job's place: what it did is thrown away, and
// it goes back to the head of its priority's queue, claimed before every job of its priority that
// has waited, at no cost of an attempt (D255 §2).
func Bump(err error) error { return &bumped{err: err} }

type bumped struct{ err error }

func (bump *bumped) Error() string { return bump.err.Error() }
func (bump *bumped) Unwrap() error { return bump.err }

// headOfTheQueue is when a bumped job is due: before any other job's time, so of its priority's it
// is claimed first, as a claim takes the earliest time after the highest priority.
func headOfTheQueue() time.Time { return time.UnixMilli(0) }

// ToolFailed answers that the tool the job used failed in a way that says nothing of its target
// (a stall; a panic is answered so by the runner itself). It costs no attempt, and the job is
// tried again after the first retry delay, jittered, told that its last attempt's tool failed
// (Job.PreviousAttemptToolFailed). A second in a row is the job's own failure (D233 §6): a
// handler that answers ToolFailed decides that itself, from PreviousAttemptToolFailed, since it
// knows what its tool's failure says; a panic after a tool failure fails the job.
func ToolFailed(err error) error { return &toolError{err: err} }

// toolError is a tool's failure; isPanic marks one the runner made of a handler's panic.
type toolError struct {
	err     error
	isPanic bool
}

func (failure *toolError) Error() string { return failure.err.Error() }
func (failure *toolError) Unwrap() error { return failure.err }

// Permanent answers that no retry can fix the job: it fails now.
func Permanent(err error) error { return &permanent{err: err} }

type permanent struct{ err error }

func (failure *permanent) Error() string { return failure.err.Error() }
func (failure *permanent) Unwrap() error { return failure.err }

// PanicError is what a handler's panic becomes: the tool's failure, with the value it panicked
// with. The runner logs the stack.
type PanicError struct {
	Value any
}

func (failure *PanicError) Error() string {
	return fmt.Sprintf("the handler panicked: %v", failure.Value)
}
