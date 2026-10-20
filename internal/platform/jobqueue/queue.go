// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// ErrCancelled is the cause a running job's context ends with when it is cancelled: its handler
// stops what it started.
var ErrCancelled = errors.New("the job was cancelled")

// ErrNotOpen is a cancel of a job that is no longer queued or running, or was never there.
var ErrNotOpen = errors.New("no such job queued or running")

// RetryPolicy is what the queue reads when a job fails: how many attempts a job may spend, how long
// the first retry waits before the back-off doubles it, and the longest any retry waits.
type RetryPolicy interface {
	AttemptLimit(ctx context.Context) (int64, error)
	FirstRetryDelay(ctx context.Context) (time.Duration, error)
	LongestRetryDelay(ctx context.Context) (time.Duration, error)
}

// ArgsMerge is how a kind folds a repeat request's arguments into the job already open for its
// target (D231 §8). A kind that gives none keeps the queued job's arguments, and counts a running
// job as covering a request whose arguments are the same.
type ArgsMerge struct {
	// Merge is the queued job's arguments once the request's are folded in.
	Merge func(queued, requested string) (string, error)
	// Covers is whether a running job with these arguments already does what the request asks,
	// so the request is dropped rather than queued to follow it.
	Covers func(running, requested string) (bool, error)
}

// Config is what a queue is built with besides its storage and clock.
type Config struct {
	Policy RetryPolicy
	// Random is where the jitter on a retry's wait comes from: crypto/rand's Reader.
	Random io.Reader
	Logger *slog.Logger
	// ArgsMerges are the kinds that merge their arguments their own way.
	ArgsMerges map[Kind]ArgsMerge
	// Probes is the probe back-off of a parked destination; nil keeps FirstProbeDelay and
	// LongestProbeDelay.
	Probes ProbePolicy
	// PersonsRequestPriority is the lowest priority a person's request has: one at it or above
	// for a job whose destination is parked looks at the destination at once (D231 §6). Zero or
	// less, and no request probes.
	PersonsRequestPriority int64
	// StorageDestination is the program's own storage, where the queue keeps its jobs (D231 §5):
	// a claim, a record or a handler's answer that IsStorageFailure says is it failing parks it,
	// and no runner claims until its probe, one claim, finds it writable again. "" when the
	// program names none.
	StorageDestination Destination
	// IsStorageFailure is whether an error is the storage failing for want of space or with an
	// I/O error; nil when StorageDestination is "".
	IsStorageFailure func(err error) bool
}

// Queue is a job queue over a Storage. T is what a job works on.
type Queue[T comparable] struct {
	storage    Storage[T]
	clock      clock.Clock
	policy     RetryPolicy
	random     io.Reader
	logger     *slog.Logger
	argsMerges map[Kind]ArgsMerge

	probes                 ProbePolicy
	personsRequestPriority int64
	storageDestination     Destination
	isStorageFailure       func(err error) bool

	destinations destinations
	sweeps       sweeps
	// claimMutex makes each claim and the probe it may begin one step, so a destination has one
	// probe at a time however many runners claim.
	claimMutex sync.Mutex

	mutex                sync.Mutex
	cancelByRunningJobID map[int64]context.CancelCauseFunc // guarded by mutex: each running job's cancel
	// wakeupsByKind are each kind's runners' wake-ups: one each, so a job of one kind never wakes a
	// runner of another in its place.
	wakeupsByKind map[Kind][]chan struct{} // guarded by mutex
	// cancelled is told each job a cancel ended, by its id: an assigned job's machine is told to
	// stop its work there (D234 §6). Guarded by mutex.
	cancelled func(ctx context.Context, id int64)
	// overdue is told each assigned job whose time ran out, by its id, once its end is recorded: its
	// machine is told to stop the work it still holds. Guarded by mutex.
	overdue func(ctx context.Context, id int64)
}

// OnOverdue has the queue tell overdue each assigned job whose time ran out, after its end is
// recorded.
func (queue *Queue[T]) OnOverdue(overdue func(ctx context.Context, id int64)) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.overdue = overdue
}

// OnCancel has the queue tell cancelled each job a cancel ended, after it is recorded.
func (queue *Queue[T]) OnCancel(cancelled func(ctx context.Context, id int64)) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.cancelled = cancelled
}

// New returns a queue over storage.
func New[T comparable](storage Storage[T], clockSource clock.Clock, config Config) *Queue[T] {
	isStorageFailure := config.IsStorageFailure
	if isStorageFailure == nil {
		isStorageFailure = func(error) bool { return false }
	}
	return &Queue[T]{
		storage: storage, clock: clockSource, policy: config.Policy, random: config.Random, logger: config.Logger,
		argsMerges: config.ArgsMerges, probes: config.Probes, personsRequestPriority: config.PersonsRequestPriority,
		storageDestination: config.StorageDestination, isStorageFailure: isStorageFailure,
		destinations:         destinations{parkedBy: map[Destination]*parkedDestination{}},
		sweeps:               sweeps{wake: make(chan struct{}, 1)},
		cancelByRunningJobID: map[int64]context.CancelCauseFunc{}, wakeupsByKind: map[Kind][]chan struct{}{},
	}
}

// Enqueue queues a job, or folds the request into the job already open for its kind and target,
// so a target has at most one queued and one running job (D231 §8). A request for a target with
// a queued job merges into it: the higher priority wins, with its reason; a job waiting on its
// start or its back-off runs no later than the request asks, and a snoozed one keeps its time;
// the arguments merge as the kind says. A request for a target whose job is running is dropped
// when that job covers it, and otherwise queued to follow it. A person's request for a job whose
// destination is parked looks at the destination at once (D231 §6), unless a sweep derived it
// again (IsDerived). It says which it did.
func (queue *Queue[T]) Enqueue(ctx context.Context, request Request[T]) (Action, error) {
	now := queue.clock.Now()
	request.ArgsJSON = cmp.Or(request.ArgsJSON, "{}")
	if request.RunnableAt.IsZero() {
		request.RunnableAt = now
	}
	var change Change
	err := queue.storage.Enqueue(ctx, request.Kind, request.Target, func(open Open[T]) (Change, error) {
		var planErr error
		change, planErr = queue.plan(request, open, now)
		return change, planErr
	})
	if err != nil {
		return "", fmt.Errorf("enqueue a %s job: %w", request.Kind, err)
	}
	if change.Action != ActionDrop {
		queue.wakeKind(request.Kind)
	}
	if queue.personsRequestPriority > 0 && request.Priority >= queue.personsRequestPriority && !request.IsDerived {
		queue.ProbeNow(change.Destination)
	}
	return change.Action, nil
}

// KeepWanted moves on, to until, the time a person waits for kind's open jobs on target until
// (D234 §3): each play that asks for the work does.
func (queue *Queue[T]) KeepWanted(ctx context.Context, kind Kind, target T, until time.Time) error {
	if err := queue.storage.KeepWanted(ctx, kind, target, until); err != nil {
		return fmt.Errorf("keep a %s job wanted: %w", kind, err)
	}
	return nil
}

// ReleaseWanted says no person waits for kind's open jobs on target any more: their wanted time
// ends now, and the next DemoteUnwanted gives them background priority (D258 §2). A person still
// waiting asks again, and keeps them wanted.
func (queue *Queue[T]) ReleaseWanted(ctx context.Context, kind Kind, target T) error {
	if err := queue.storage.ReleaseWanted(ctx, kind, target, queue.clock.Now()); err != nil {
		return fmt.Errorf("release a %s job no one waits for: %w", kind, err)
	}
	return nil
}

// PruneFinished forgets the jobs that ended longer ago than keptFor, and says how many (D233 §8).
func (queue *Queue[T]) PruneFinished(ctx context.Context, keptFor time.Duration) (int64, error) {
	pruned, err := queue.storage.PruneFinished(ctx, queue.clock.Now().Add(-keptFor))
	if err != nil {
		return 0, fmt.Errorf("forget the jobs that ended: %w", err)
	}
	return pruned, nil
}

// DemoteUnwanted gives the open jobs no person waits for any more the given priority, background
// work's (D234 §3), and returns them: a job given to another machine is that machine's to be told.
func (queue *Queue[T]) DemoteUnwanted(ctx context.Context, priority int64) ([]Row[T], error) {
	demoted, err := queue.storage.DemoteUnwanted(ctx, queue.clock.Now(), priority)
	if err != nil {
		return nil, fmt.Errorf("demote the jobs no one waits for: %w", err)
	}
	return demoted, nil
}

// plan is what a request does to its target's open jobs.
func (queue *Queue[T]) plan(request Request[T], open Open[T], now time.Time) (Change, error) {
	argsMerge := queue.argsMerge(request.Kind)
	if queued := open.Queued; queued != nil {
		args, err := argsMerge.Merge(queued.ArgsJSON, request.ArgsJSON)
		if err != nil {
			return Change{}, fmt.Errorf("merge a request into job %d: %w", queued.ID, err)
		}
		change := Change{
			Action: ActionMerge, MergeIntoID: queued.ID, ArgsJSON: args, Priority: queued.Priority,
			Reason: queued.Reason, RunnableAt: queued.RunnableAt,
			Destination: cmp.Or(queued.Destination, request.Destination),
		}
		if request.Priority > queued.Priority {
			change.Priority, change.Reason = request.Priority, request.Reason
		}
		if !queued.IsSnoozed && request.RunnableAt.Before(queued.RunnableAt) {
			change.RunnableAt = request.RunnableAt
		}
		return change, nil
	}
	if running := open.Running; running != nil {
		covered, err := argsMerge.Covers(running.ArgsJSON, request.ArgsJSON)
		if err != nil {
			return Change{}, fmt.Errorf("compare a request with running job %d: %w", running.ID, err)
		}
		if covered {
			return Change{Action: ActionDrop, Destination: cmp.Or(running.Destination, request.Destination)}, nil
		}
	}
	return Change{
		Action: ActionInsert, ArgsJSON: request.ArgsJSON, Priority: request.Priority, Reason: request.Reason,
		RunnableAt: request.RunnableAt, Destination: request.Destination, CreatedAt: now,
	}, nil
}

// argsMerge is a kind's merge, with the default for what it leaves out.
func (queue *Queue[T]) argsMerge(kind Kind) ArgsMerge {
	argsMerge := queue.argsMerges[kind]
	if argsMerge.Merge == nil {
		argsMerge.Merge = func(queued, _ string) (string, error) { return queued, nil }
	}
	if argsMerge.Covers == nil {
		argsMerge.Covers = func(running, requested string) (bool, error) { return running == requested, nil }
	}
	return argsMerge
}

// RequeueRunning returns the jobs a stop left running to the queue, to start again from zero.
// Each keeps the attempt its claim counted, so a job that brings the program down every time it
// runs reaches its attempt limit, and the claim then refuses it (S86).
func (queue *Queue[T]) RequeueRunning(ctx context.Context) (int64, error) {
	count, err := queue.storage.RequeueRunning(ctx)
	if err != nil {
		return 0, fmt.Errorf("requeue running jobs: %w", err)
	}
	return count, nil
}

// Health is each kind's queued jobs, the oldest of them and its jobs failed since failedSince.
func (queue *Queue[T]) Health(ctx context.Context, failedSince time.Time) ([]KindHealth, error) {
	health, err := queue.storage.Health(ctx, failedSince)
	if err != nil {
		return nil, fmt.Errorf("read the queue's health: %w", err)
	}
	return health, nil
}

// List is the newest jobs, newest first, at most limit of them.
func (queue *Queue[T]) List(ctx context.Context, limit int64) ([]Row[T], error) {
	rows, err := queue.storage.List(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return rows, nil
}

// Cancel cancels a queued or running job: a queued one never runs, and a running one's handler is
// told through its context, whose cause is ErrCancelled, so it can stop the work it started.
// Whatever the handler answers after, the job stays cancelled. A job not open is ErrNotOpen.
func (queue *Queue[T]) Cancel(ctx context.Context, id int64) error {
	wasOpen, err := queue.storage.Cancel(ctx, id, queue.clock.Now())
	if err != nil {
		return fmt.Errorf("cancel job %d: %w", id, err)
	}
	if !wasOpen {
		return ErrNotOpen
	}
	queue.mutex.Lock()
	stop, isRunning := queue.cancelByRunningJobID[id]
	cancelled := queue.cancelled
	queue.mutex.Unlock()
	if isRunning {
		stop(ErrCancelled)
	}
	if cancelled != nil {
		cancelled(ctx, id)
	}
	return nil
}

// SetProgress records how far a running job is, from 0 to 1.
func (queue *Queue[T]) SetProgress(ctx context.Context, id int64, fraction float64) error {
	if err := queue.storage.SetProgress(ctx, id, fraction); err != nil {
		return fmt.Errorf("record job %d's progress: %w", id, err)
	}
	return nil
}

// Wake tells every runner there may be work before its next tick.
func (queue *Queue[T]) Wake() {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	for _, wakes := range queue.wakeupsByKind {
		for _, wake := range wakes {
			nudge(wake)
		}
	}
}

// wakeKind tells the runners of one kind there may be work before their next tick.
func (queue *Queue[T]) wakeKind(kind Kind) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	for _, wake := range queue.wakeupsByKind[kind] {
		nudge(wake)
	}
}

// nudge leaves a wake-up on a runner's channel unless one is already waiting there.
func nudge(wake chan struct{}) {
	select {
	case wake <- struct{}{}:
	default:
	}
}

// registerWake makes a new runner's own wake-up, for its kind, and keeps it with the kind's others.
func (queue *Queue[T]) registerWake(kind Kind) chan struct{} {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	wake := make(chan struct{}, 1)
	queue.wakeupsByKind[kind] = append(queue.wakeupsByKind[kind], wake)
	return wake
}

// trackCancel keeps a running job's cancel until its handler returns.
func (queue *Queue[T]) trackCancel(id int64, stop context.CancelCauseFunc) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.cancelByRunningJobID[id] = stop
}

func (queue *Queue[T]) untrackCancel(id int64) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	delete(queue.cancelByRunningJobID, id)
}
