// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"errors"
	"fmt"
	"time"
)

// A handler answers with an error, or nil when the job is done. These wrap the error to say what
// the queue does next; any other error is the job's own failure, retried with back-off until the
// attempt limit. Only an error wrapped this way costs no attempt. The queue reads the first it
// finds of Park, Snooze, Permanent and ToolFailed, in that order.

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

// Snooze answers that the job should wait until a time at no cost of an attempt, while the
// runner goes on with other jobs. A repeat request does not bring it forward.
func Snooze(err error, until time.Time) error { return &snoozed{err: err, until: until} }

type snoozed struct {
	err   error
	until time.Time
}

func (snooze *snoozed) Error() string { return snooze.err.Error() }
func (snooze *snoozed) Unwrap() error { return snooze.err }

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
