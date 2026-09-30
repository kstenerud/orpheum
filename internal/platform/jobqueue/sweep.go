// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"
)

// Sweep derives work from a program's durable state and queues it (D231 §3): the queue is a cache
// of the work that state implies, so a job lost with its storage, cancelled or failed through no
// fault of its item is queued again by the next sweep, and a request to queue one is a hint to
// run it sooner. Queuing what is already queued merges into it, so a sweep may queue all it finds.
type Sweep struct {
	// Name says what it derives, in the log and to RequestSweeps.
	Name string
	// Derive queues the work the state implies now, and says how many jobs it added.
	Derive func(ctx context.Context) (int, error)
	// Every is how long after it runs it runs again on its own, read at each run.
	Every func(ctx context.Context) time.Duration
}

// sweeps are a queue's sweeps, when each last ran, and the ones asked for.
type sweeps struct {
	mutex     sync.Mutex
	all       []Sweep              // guarded by mutex
	lastRunBy map[string]time.Time // guarded by mutex
	requested map[string]bool      // guarded by mutex: nil when none; a "" key asks for them all
	wake      chan struct{}        // a request's nudge to the loop in Sweeping
}

// AddSweep adds a sweep. Add them before Sweeping runs.
func (queue *Queue[T]) AddSweep(sweep Sweep) {
	queue.sweeps.mutex.Lock()
	defer queue.sweeps.mutex.Unlock()
	queue.sweeps.all = append(queue.sweeps.all, sweep)
}

// RequestSweeps asks Sweeping to run the named sweeps soon, or all of them when none is named: after
// a pass over the state, or when a destination comes back (D231 §3). It returns at once.
func (queue *Queue[T]) RequestSweeps(names ...string) {
	queue.sweeps.mutex.Lock()
	if queue.sweeps.requested == nil {
		queue.sweeps.requested = map[string]bool{}
	}
	if len(names) == 0 {
		names = []string{""}
	}
	for _, name := range names {
		queue.sweeps.requested[name] = true
	}
	queue.sweeps.mutex.Unlock()
	nudge(queue.sweeps.wake)
}

// RunSweeps runs the named sweeps now, or all of them when none is named, in the order they were
// added, and says how many jobs they added. A sweep that fails is logged, and the others still run.
func (queue *Queue[T]) RunSweeps(ctx context.Context, names ...string) int {
	queue.sweeps.mutex.Lock()
	chosen := slices.DeleteFunc(slices.Clone(queue.sweeps.all), func(sweep Sweep) bool {
		return len(names) > 0 && !slices.Contains(names, sweep.Name)
	})
	queue.sweeps.mutex.Unlock()
	added := 0
	for _, sweep := range chosen {
		if ctx.Err() != nil {
			break
		}
		count, err := sweep.Derive(ctx)
		queue.sweeps.mutex.Lock()
		if queue.sweeps.lastRunBy == nil {
			queue.sweeps.lastRunBy = map[string]time.Time{}
		}
		queue.sweeps.lastRunBy[sweep.Name] = queue.clock.Now()
		queue.sweeps.mutex.Unlock()
		if err != nil && ctx.Err() == nil {
			queue.logger.ErrorContext(ctx, "a sweep could not derive its work", "sweep", sweep.Name, "err", err)
		}
		if count > 0 {
			queue.logger.InfoContext(ctx, "a sweep queued work", "sweep", sweep.Name, "queuedCount", count)
		}
		added += count
	}
	return added
}

// Sweeping runs every sweep at once, then each again when its Every has passed since it last ran,
// checked on every tick, and those RequestSweeps asks for, until ctx ends.
func (queue *Queue[T]) Sweeping(ctx context.Context, tick <-chan time.Time) {
	queue.RunSweeps(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			if due := queue.dueSweeps(ctx); len(due) > 0 {
				queue.RunSweeps(ctx, due...)
			}
		case <-queue.sweeps.wake:
			if requested, isAll := queue.takeRequested(); isAll {
				queue.RunSweeps(ctx)
			} else if len(requested) > 0 {
				queue.RunSweeps(ctx, requested...)
			}
		}
	}
}

// dueSweeps are the sweeps whose Every has passed since they last ran.
func (queue *Queue[T]) dueSweeps(ctx context.Context) []string {
	queue.sweeps.mutex.Lock()
	all := slices.Clone(queue.sweeps.all)
	lastRunBy := maps.Clone(queue.sweeps.lastRunBy)
	queue.sweeps.mutex.Unlock()
	now := queue.clock.Now()
	var due []string
	for _, sweep := range all {
		if !now.Before(lastRunBy[sweep.Name].Add(sweep.Every(ctx))) {
			due = append(due, sweep.Name)
		}
	}
	return due
}

// takeRequested takes the sweeps asked for since the last, and whether all were.
func (queue *Queue[T]) takeRequested() ([]string, bool) {
	queue.sweeps.mutex.Lock()
	defer queue.sweeps.mutex.Unlock()
	requested := queue.sweeps.requested
	queue.sweeps.requested = nil
	if requested[""] {
		return nil, true
	}
	names := make([]string, 0, len(requested))
	for name := range requested {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, false
}

// CancelWhere cancels every queued and running job that match says is the work of what a program
// retires (D231 §7, §9): a cancel stops a run, and it is the change of state that keeps the work
// from being derived again. It says how many it cancelled.
func (queue *Queue[T]) CancelWhere(ctx context.Context, match func(Row[T]) bool) (int, error) {
	open, err := queue.storage.ListOpen(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the open jobs: %w", err)
	}
	cancelled := 0
	for _, row := range open {
		if !match(row) {
			continue
		}
		switch err := queue.Cancel(ctx, row.ID); {
		case err == nil:
			cancelled++
		case errors.Is(err, ErrNotOpen): // it ended meanwhile
		default:
			return cancelled, err
		}
	}
	return cancelled, nil
}
