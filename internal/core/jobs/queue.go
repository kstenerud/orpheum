// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// Kind is what a job does; the job table's CHECK names the set.
type Kind string

// The kinds of job (cut-a §5.3).
const (
	KindFetch   Kind = "fetch"
	KindArtwork Kind = "artwork"
	KindPrepare Kind = "prepare"
	// KindExtractSubtitle extracts one text subtitle track; its subject is the stream (D217 §3).
	KindExtractSubtitle Kind = "extract_subtitle"
	// KindConvertFetchedSubtitle converts a subtitle fetched on request; its subject is the
	// fetched subtitle (D224 §5).
	KindConvertFetchedSubtitle Kind = "convert_fetched_subtitle"
	KindScan                   Kind = "scan"
	KindBackup                 Kind = "backup"
)

// The states a job moves through.
const (
	stateDone   = "done"
	stateFailed = "failed"
)

// Job is one queued piece of work, as a runner receives it.
type Job struct {
	ID       int64
	Kind     Kind
	Subject  int64
	Args     string
	Attempts int64
}

// PriorityPlayNow is the highest priority: a job someone is waiting on to watch (cut-a §5.3).
const PriorityPlayNow int64 = 1000

// New is a job to queue.
type New struct {
	Kind     Kind
	Subject  int64
	Args     string // JSON; "{}" when empty
	Priority int64
	Reason   string // why it exists, shown to the user
	// RunnableAt is when it may first run; now when zero.
	RunnableAt time.Time
}

// Settings are the values the queue reads when a job fails.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Queue is the one work queue in cache.db (schema.md, cache.db).
type Queue struct {
	cache    *store.Cache
	clock    clock.Clock
	settings Settings
	logger   *slog.Logger

	mutex   sync.Mutex
	running map[int64]context.CancelCauseFunc // guarded by mutex: each running job's cancel
	// wakes are each kind's runners' wake-up: one each, so a job of one kind never wakes a
	// runner of another in its place.
	wakes map[Kind][]chan struct{} // guarded by mutex
}

// NewQueue returns the queue.
func NewQueue(cache *store.Cache, clockSource clock.Clock, config Settings, logger *slog.Logger) *Queue {
	return &Queue{
		cache: cache, clock: clockSource, settings: config, logger: logger,
		running: map[int64]context.CancelCauseFunc{}, wakes: map[Kind][]chan struct{}{},
	}
}

// Enqueue adds a job unless one of its kind is already queued or running for its subject, and
// reports whether it added one.
func (queue *Queue) Enqueue(ctx context.Context, job New) (bool, error) {
	added := false
	err := queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		subject := job.Subject
		open := cachegen.FindOpenJobParams{Kind: string(job.Kind), Subject: &subject}
		_, findErr := store.One(queries.FindOpenJob(ctx, open))
		if findErr == nil {
			return nil
		}
		if !errors.Is(findErr, store.ErrNotFound) {
			return fmt.Errorf("look for an open %s job: %w", job.Kind, findErr)
		}
		args := job.Args
		if args == "" {
			args = "{}"
		}
		reason, runnableAt := job.Reason, job.RunnableAt
		if runnableAt.IsZero() {
			runnableAt = queue.clock.Now()
		}
		_, insertErr := queries.InsertJob(ctx, cachegen.InsertJobParams{
			Kind: string(job.Kind), Subject: &subject, Args: args, Priority: job.Priority,
			RunnableAt: runnableAt.UnixMilli(), Reason: &reason, CreatedAt: queue.clock.Now().UnixMilli(),
		})
		if insertErr != nil {
			return fmt.Errorf("queue a %s job: %w", job.Kind, insertErr)
		}
		added = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("enqueue: %w", err)
	}
	if added {
		queue.wakeKind(job.Kind)
	}
	return added, nil
}

// EverQueued reports whether a job of this kind was ever queued for the subject.
func (queue *Queue) EverQueued(ctx context.Context, kind Kind, subject int64) (bool, error) {
	found, err := queue.cache.Read().HasJob(ctx, cachegen.HasJobParams{Kind: string(kind), Subject: &subject})
	if err != nil {
		return false, fmt.Errorf("look for a %s job for %d: %w", kind, subject, err)
	}
	return found, nil
}

// RequeueRunning returns the jobs a crash left running to the queue: their output is a cache, so
// they restart from zero (cut-a §5.3).
func (queue *Queue) RequeueRunning(ctx context.Context) (int64, error) {
	var count int64
	err := queue.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var requeueErr error
		count, requeueErr = queries.RequeueRunningJobs(ctx)
		return requeueErr
	})
	if err != nil {
		return 0, fmt.Errorf("requeue running jobs: %w", err)
	}
	return count, nil
}

// Wake tells every runner there may be work before its next tick.
func (queue *Queue) Wake() {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	for _, wakes := range queue.wakes {
		for _, wake := range wakes {
			nudge(wake)
		}
	}
}

// wakeKind tells the runners of one kind there may be work before their next tick.
func (queue *Queue) wakeKind(kind Kind) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	for _, wake := range queue.wakes[kind] {
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

// wakeFor is a new runner's own wake-up, for its kind.
func (queue *Queue) wakeFor(kind Kind) chan struct{} {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	wake := make(chan struct{}, 1)
	queue.wakes[kind] = append(queue.wakes[kind], wake)
	return wake
}

// RetryAtError is a handler's answer that the job cannot run before a time that is not its fault — a
// provider's Retry-After, or a missing key — so the wait costs it no attempt (D170).
type RetryAtError struct {
	At  time.Time
	Err error
}

func (retry *RetryAtError) Error() string {
	return fmt.Sprintf("retry after %s: %v", retry.At.UTC().Format(time.RFC3339), retry.Err)
}

func (retry *RetryAtError) Unwrap() error { return retry.Err }

// Blocked marks a handler's error as one that holds every job of its kind until something
// outside the queue changes — a provider's key unset or refused — so the kind waits the retry
// back-off, and the wait costs the job no attempt.
func Blocked(err error) error { return &blocked{err: err} }

type blocked struct{ err error }

func (failure *blocked) Error() string { return failure.err.Error() }
func (failure *blocked) Unwrap() error { return failure.err }

// Permanent marks a handler's error as one no retry can fix.
func Permanent(err error) error { return &permanent{err: err} }

type permanent struct{ err error }

func (failure *permanent) Error() string { return failure.err.Error() }
func (failure *permanent) Unwrap() error { return failure.err }
