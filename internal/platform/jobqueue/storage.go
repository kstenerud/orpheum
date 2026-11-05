// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue

import (
	"context"
	"time"
)

// Storage keeps the jobs. The queue decides; the storage only reads and writes rows, each call
// one transaction, so that a program's database stays in the one package allowed to touch it.
type Storage[T comparable] interface {
	Queueing[T]
	Cancelling[T]
	Working[T]
	Listing[T]
}

// Queueing is the part of Storage that takes requests and keeps their priorities.
type Queueing[T comparable] interface {
	// Enqueue reads kind's open jobs for target, asks plan what to do with them, and applies its
	// Change, all in one write, so two requests for one target never both insert.
	Enqueue(ctx context.Context, kind Kind, target T, plan func(open Open[T]) (Change, error)) error
	// EnqueueBatch is Enqueue for each of several requests, all in one write: plan is asked for the
	// change of each by its index. A sweep queues thousands, which each took the writer apart.
	EnqueueBatch(
		ctx context.Context, requested []Requested[T], plan func(index int, open Open[T]) (Change, error),
	) error
	// KeepWanted moves on, to until, the time a person waits for kind's open jobs on target until; a
	// later time already kept stands.
	KeepWanted(ctx context.Context, kind Kind, target T, until time.Time) error
	// ReleaseWanted ends now the time a person waits for kind's open jobs on target, so the next
	// DemoteUnwanted gives them background priority.
	ReleaseWanted(ctx context.Context, kind Kind, target T, now time.Time) error
	// DemoteUnwanted gives every open job whose wanted time is before now the given priority, forgets
	// its time, and returns it.
	DemoteUnwanted(ctx context.Context, now time.Time, priority int64) ([]Row[T], error)
	// PruneFinished forgets the jobs that ended, done, failed or cancelled, before a time, and says
	// how many.
	PruneFinished(ctx context.Context, before time.Time) (int64, error)
}

// Cancelling is the part of Storage that ends jobs as cancelled.
type Cancelling[T comparable] interface {
	// Cancel ends a queued, running or assigned job as cancelled at the given time, and says whether
	// one was open. A cancelled job stays cancelled whatever its handler records after.
	Cancel(ctx context.Context, id int64, at time.Time) (bool, error)
	// CancelOpenOf ends as cancelled, at the given time and in one write, kind's queued, running and
	// assigned jobs on target that match says, and returns their ids: reading only that target's
	// jobs, by the index an enqueue reads them by, not every open job.
	CancelOpenOf(ctx context.Context, kind Kind, target T, at time.Time, match func(Row[T]) bool) ([]int64, error)
}

// Working is the part of Storage that runners use.
type Working[T comparable] interface {
	// Claim marks the next queued job the claim allows running and counts its attempt: the row
	// it returns has the new count. The next is the highest priority whose RunnableAt has come and
	// whose destination is not one the claim passes over, then the earliest RunnableAt, then the
	// lowest id. It reports false when none is due.
	Claim(ctx context.Context, claim Claim) (Row[T], bool, error)
	// Record writes a claimed job's outcome, unless the job was cancelled meanwhile.
	Record(ctx context.Context, outcome Outcome) error
	// TakeAssigned returns an assigned job to running, so its outcome can be recorded, and reports
	// false when it is not assigned: cancelled, or already taken by its outcome or its time running
	// out. Only one of those takes it.
	TakeAssigned(ctx context.Context, id int64) (Row[T], bool, error)
	// TakeOverdueAssigned returns to running the assigned job of a kind whose time ran out first,
	// by now, and reports false when there is none.
	TakeOverdueAssigned(ctx context.Context, kind Kind, now time.Time) (Row[T], bool, error)
	// SetProgress records how far a running job is.
	SetProgress(ctx context.Context, id int64, fraction float64) error
	// RequeueRunning returns every running job to queued, keeping the attempt its claim counted,
	// and says how many there were: what a stop left running. An assigned job stays assigned: the
	// machine it was given to still has it.
	RequeueRunning(ctx context.Context) (int64, error)
}

// Listing is the part of Storage that reads jobs for a person, and the open ones for a cancel.
type Listing[T comparable] interface {
	// List is the newest jobs before beforeID (0 for the newest), newest first, at most limit: a page
	// of the job view.
	List(ctx context.Context, beforeID, limit int64) ([]Row[T], error)
	// ListOpen is every queued, running and assigned job, for a cancel of the work of what a program
	// retires. What became of a job that ended is never a fact a program derives work from
	// (D231 section 3).
	ListOpen(ctx context.Context) ([]Row[T], error)
	// Health is each kind's queued jobs, the oldest of them and its jobs failed since failedSince:
	// the queue's health for a person (D233 §8). A kind with no job is left out.
	Health(ctx context.Context, failedSince time.Time) ([]KindHealth, error)
}

// KindHealth is how one kind's jobs are doing.
type KindHealth struct {
	Kind        Kind
	QueuedCount int64
	// OldestQueuedAt is when the oldest of them was queued; zero when none is.
	OldestQueuedAt time.Time
	// FailedCount is how many failed since the time asked about.
	FailedCount int64
}

// Requested is one of a batch's requests, as the storage reads its open jobs by: a kind and a target.
type Requested[T comparable] struct {
	Kind   Kind
	Target T
}

// Open are a kind's open jobs for one target: at most one queued, and those running, where an
// assigned job counts as running. More than one runs when a job given to a node freed its runner to
// claim the follow-up queued beside it, as a player's index read beside a full read-through.
type Open[T comparable] struct {
	Queued  *Row[T]
	Running []Row[T]
}

// Action is what a request does to the jobs open for its target.
type Action string

// The actions a request may take.
const (
	// ActionInsert queues a new job: none was queued, and none running already covers it.
	ActionInsert Action = "insert"
	// ActionMerge folds the request into the one queued job.
	ActionMerge Action = "merge"
	// ActionDrop drops the request: the running job already does what it asks.
	ActionDrop Action = "drop"
)

// Change is what Storage.Enqueue writes.
type Change struct {
	Action Action
	// MergeIntoID is the queued job an ActionMerge changes.
	MergeIntoID int64
	// ArgsJSON, Priority, Reason, RunnableAt and Destination are the inserted job's, or the
	// merged job's after the merge.
	ArgsJSON    string
	Priority    int64
	Reason      string
	RunnableAt  time.Time
	Destination Destination
	// CreatedAt is an inserted job's.
	CreatedAt time.Time
}

// Claim says which queued job a runner may take.
type Claim struct {
	Kind Kind
	// Now is the time a job's RunnableAt must have come by.
	Now time.Time
	// Unclaimable are the parked destinations whose probe is not due (D231 §6): a job kept with
	// one of them is passed over, and spends nothing while it waits.
	Unclaimable []Destination
}

// Outcome is a claimed job's end, or its return to the queue.
type Outcome struct {
	ID int64
	// State is StateDone, StateFailed, StateQueued for a job that waits to run again, or
	// StateAssigned for one whose work another machine does.
	State State
	// At is when a done or failed job finished, when a queued one may run again, or when an
	// assigned one's time runs out.
	At time.Time
	// AttemptCount is the count the job ends or waits with: the claim's, or one less when the
	// attempt is given back.
	AttemptCount int64
	// IsSnoozed and ToolFailed are a queued job's Row.IsSnoozed and Row.LastAttemptToolFailed.
	IsSnoozed  bool
	ToolFailed bool
	// Destination is a queued job's destination: the one it was parked on, or the one it had.
	Destination Destination
	// LastFailure is what the attempt said when it did not succeed, or "".
	LastFailure string
}
