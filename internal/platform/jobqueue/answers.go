// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"fmt"
	"time"
)

// A handler answers with an error, or nil when the job is done. These wrap the error to say what
// the queue does next; any other error is the job's own failure, retried with back-off until the
// attempt limit. Only an error wrapped this way costs no attempt.

// RetryAtError is an answer that the job cannot run before At, for a reason that is not its own
// (a provider's Retry-After): it waits until then at no cost of an attempt, a repeat request
// does not bring it forward, and the runner claims nothing more before then (D170).
type RetryAtError struct {
	At  time.Time
	Err error
}

func (retry *RetryAtError) Error() string {
	return fmt.Sprintf("retry after %s: %v", retry.At.UTC().Format(time.RFC3339), retry.Err)
}

func (retry *RetryAtError) Unwrap() error { return retry.Err }

// Snooze answers that the job should wait until a time at no cost of an attempt, while the
// runner goes on with other jobs. A repeat request does not bring it forward.
func Snooze(err error, until time.Time) error { return &snoozed{err: err, until: until} }

type snoozed struct {
	err   error
	until time.Time
}

func (snooze *snoozed) Error() string { return snooze.err.Error() }
func (snooze *snoozed) Unwrap() error { return snooze.err }

// Blocked answers that something outside the queue holds every job of the kind (a provider's
// key unset or refused): the job waits the first retry delay, jittered, at no cost of an attempt,
// and the runner claims nothing more until then. Phase 2 of the jobs rebuild replaces it with a
// parked destination (D231 §5).
func Blocked(err error) error { return &blocked{err: err} }

type blocked struct{ err error }

func (failure *blocked) Error() string { return failure.err.Error() }
func (failure *blocked) Unwrap() error { return failure.err }

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
