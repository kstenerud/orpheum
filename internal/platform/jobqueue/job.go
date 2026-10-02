// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import "time"

// Kind is what a job does. The queue treats it as a name: it runs a kind's jobs on that kind's
// runners and merges a repeat request by its kind and target, and knows nothing else of it.
type Kind string

// State is where a job is in its life.
type State string

// The states a job moves through: queued until a runner claims it, running while its handler
// works, then done, failed or cancelled. A retry or a snooze returns a running job to queued. A
// job whose handler gave its work to another machine (Assign) is assigned until that machine's
// outcome is taken (Runner.Resolve) or its time runs out (D234 §7).
const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateAssigned  State = "assigned"
	StateDone      State = "done"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Request is a job to queue, or a repeat request for one already open (Queue.Enqueue). T is
// what a job works on, opaque to the queue: it only compares two targets.
type Request[T comparable] struct {
	Kind   Kind
	Target T
	// ArgsJSON are the kind's own arguments, as JSON; "{}" when empty.
	ArgsJSON string
	// Priority orders claiming: the higher runs first.
	Priority int64
	// Reason is why the job exists, shown to a person.
	Reason string
	// RunnableAt is when it may first run; now when zero.
	RunnableAt time.Time
	// Destination is what it needs that can be down, when known before it runs; a job that finds
	// its destination down while it runs is parked on it then (Park).
	Destination Destination
	// IsDerived is a request a sweep made, deriving again work that state implies (Sweep): it looks
	// at no parked destination, whatever its priority, since no one is waiting on it now.
	IsDerived bool
}

// Job is one claimed job, as its handler receives it.
type Job[T comparable] struct {
	ID       int64
	Kind     Kind
	Target   T
	ArgsJSON string
	// Priority is how urgent it is: a handler may pass it on to a provider, which weighs a
	// person's request above the background (D231 §5).
	Priority int64
	// AttemptNumber counts this attempt: 1 the first time, and not raised by an attempt that was
	// given back (a snooze, a hold, a tool that failed).
	AttemptNumber int64
	// PreviousAttemptToolFailed is whether the attempt before this one ended with the tool failing
	// (ToolFailed, or a panic in the handler): a second in a row is the job's own failure (D233 §6).
	PreviousAttemptToolFailed bool
}

// Row is one job as the storage keeps it.
type Row[T comparable] struct {
	ID       int64
	Kind     Kind
	Target   T
	ArgsJSON string
	Priority int64
	State    State
	// Destination is what it needs that can be down: named when it was queued, or the one it was
	// last parked on.
	Destination Destination
	// RunnableAt is when a queued job may next be claimed.
	RunnableAt time.Time
	// IsSnoozed is whether a queued job waits for a time it was given (a rate limit, a hold), which
	// a repeat request never brings forward; otherwise its wait is its own start or back-off, which
	// a repeat request may.
	IsSnoozed bool
	// AttemptCount is how many attempts it has spent; a claim counts one.
	AttemptCount int64
	// LastAttemptToolFailed is whether its last attempt ended with the tool failing.
	LastAttemptToolFailed bool
	// AssignedUntil is when an assigned job's time runs out, kept while its outcome is taken; nil
	// once that is written, and for a job never assigned.
	AssignedUntil *time.Time
	// WantedUntil is until when a person waits for it, play-driven work; nil for other work, and once
	// past (Queue.DemoteUnwanted).
	WantedUntil *time.Time
	// ProgressFraction is how far a running job is, 0 to 1, or nil when not known.
	ProgressFraction *float64
	Reason           string
	// LastFailure is what its last failed attempt said, or "".
	LastFailure string
	CreatedAt   time.Time
	// FinishedAt is when it was done, failed or cancelled; nil while it is open.
	FinishedAt *time.Time
}
