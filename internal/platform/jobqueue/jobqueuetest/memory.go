// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueuetest

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// Memory is a jobqueue.Storage held in memory, keeping the promises internal/store's storage
// keeps on cache.db. It is safe for concurrent use.
type Memory[T comparable] struct {
	mutex  sync.Mutex
	rows   []jobqueue.Row[T] // guarded by mutex; by id, oldest first
	nextID int64             // guarded by mutex
	// writeFailure is what every write answers while it is set: a full disk (guarded by mutex).
	writeFailure error
}

// FailWrites makes every write answer failure until it is called with nil, as a storage out of
// space does: a claim, a record, an enqueue.
func (memory *Memory[T]) FailWrites(failure error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	memory.writeFailure = failure
}

// Compile-time proof that *Memory satisfies jobqueue.Storage.
var _ jobqueue.Storage[int64] = (*Memory[int64])(nil)

// Rows are a copy of every job, oldest first.
func (memory *Memory[T]) Rows() []jobqueue.Row[T] {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	return slices.Clone(memory.rows)
}

// Put adds a row as it is, for a test that needs a state no request makes, and returns its id.
func (memory *Memory[T]) Put(row jobqueue.Row[T]) int64 {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	memory.nextID++
	row.ID = memory.nextID
	memory.rows = append(memory.rows, row)
	return row.ID
}

// Enqueue applies plan's change to the kind's open jobs for target.
func (memory *Memory[T]) Enqueue(
	_ context.Context, kind jobqueue.Kind, target T, plan func(open jobqueue.Open[T]) (jobqueue.Change, error),
) error {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	if memory.writeFailure != nil {
		return memory.writeFailure
	}
	var open jobqueue.Open[T]
	for index := range memory.rows {
		row := memory.rows[index]
		if row.Kind != kind || row.Target != target {
			continue
		}
		switch row.State {
		case jobqueue.StateQueued:
			open.Queued = &row
		case jobqueue.StateRunning:
			open.Running = &row
		case jobqueue.StateDone, jobqueue.StateFailed, jobqueue.StateCancelled:
		}
	}
	change, err := plan(open)
	if err != nil {
		return err
	}
	switch change.Action {
	case jobqueue.ActionInsert:
		memory.nextID++
		memory.rows = append(memory.rows, jobqueue.Row[T]{
			ID: memory.nextID, Kind: kind, Target: target, ArgsJSON: change.ArgsJSON, Priority: change.Priority,
			State: jobqueue.StateQueued, Destination: change.Destination, RunnableAt: change.RunnableAt,
			Reason: change.Reason, CreatedAt: change.CreatedAt,
		})
	case jobqueue.ActionMerge:
		row := memory.find(change.MergeIntoID)
		if row == nil || row.State != jobqueue.StateQueued {
			return fmt.Errorf("merge into job %d: not queued", change.MergeIntoID)
		}
		row.ArgsJSON, row.Priority, row.Reason, row.RunnableAt = change.ArgsJSON, change.Priority, change.Reason,
			change.RunnableAt
		row.Destination = change.Destination
	case jobqueue.ActionDrop:
	}
	return nil
}

// Cancel cancels an open job.
func (memory *Memory[T]) Cancel(_ context.Context, id int64, cancelledAt time.Time) (bool, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	row := memory.find(id)
	if row == nil || (row.State != jobqueue.StateQueued && row.State != jobqueue.StateRunning) {
		return false, nil
	}
	row.State, row.FinishedAt = jobqueue.StateCancelled, &cancelledAt
	return true, nil
}

// Claim marks the next due job of the claim's kind that no parked destination holds running and
// counts its attempt.
func (memory *Memory[T]) Claim(_ context.Context, claim jobqueue.Claim) (jobqueue.Row[T], bool, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	if memory.writeFailure != nil {
		return jobqueue.Row[T]{}, false, memory.writeFailure
	}
	var next *jobqueue.Row[T]
	for index := range memory.rows {
		row := &memory.rows[index]
		if row.Kind != claim.Kind || row.State != jobqueue.StateQueued || row.RunnableAt.After(claim.Now) ||
			slices.Contains(claim.Unclaimable, row.Destination) {
			continue
		}
		if next == nil || claimsFirst(*row, *next) {
			next = row
		}
	}
	if next == nil {
		return jobqueue.Row[T]{}, false, nil
	}
	next.State = jobqueue.StateRunning
	next.AttemptCount++
	return *next, true, nil
}

// claimsFirst is whether a is claimed before b: the higher priority, then the earlier
// RunnableAt, then the lower id.
func claimsFirst[T comparable](a, b jobqueue.Row[T]) bool {
	return cmp.Or(
		cmp.Compare(b.Priority, a.Priority), a.RunnableAt.Compare(b.RunnableAt), cmp.Compare(a.ID, b.ID),
	) < 0
}

// Record writes a claimed job's outcome unless it was cancelled.
func (memory *Memory[T]) Record(_ context.Context, outcome jobqueue.Outcome) error {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	if memory.writeFailure != nil {
		return memory.writeFailure
	}
	row := memory.find(outcome.ID)
	if row == nil || row.State == jobqueue.StateCancelled {
		return nil
	}
	row.State, row.LastFailure, row.AttemptCount = outcome.State, outcome.LastFailure, outcome.AttemptCount
	switch outcome.State {
	case jobqueue.StateQueued:
		row.RunnableAt, row.Destination = outcome.At, outcome.Destination
		row.IsSnoozed, row.LastAttemptToolFailed = outcome.IsSnoozed, outcome.ToolFailed
	case jobqueue.StateDone:
		row.ProgressFraction, row.FinishedAt = new(1.0), &outcome.At
	case jobqueue.StateFailed, jobqueue.StateRunning, jobqueue.StateCancelled:
		row.FinishedAt = &outcome.At
	}
	return nil
}

// SetProgress records a running job's progress.
func (memory *Memory[T]) SetProgress(_ context.Context, id int64, fraction float64) error {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	if row := memory.find(id); row != nil && row.State == jobqueue.StateRunning {
		row.ProgressFraction = &fraction
	}
	return nil
}

// RequeueRunning returns every running job to queued.
func (memory *Memory[T]) RequeueRunning(context.Context) (int64, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	var count int64
	for index := range memory.rows {
		if memory.rows[index].State == jobqueue.StateRunning {
			memory.rows[index].State = jobqueue.StateQueued
			count++
		}
	}
	return count, nil
}

// List is the newest jobs first.
func (memory *Memory[T]) List(_ context.Context, limit int64) ([]jobqueue.Row[T], error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	rows := slices.Clone(memory.rows)
	slices.Reverse(rows)
	return rows[:min(int64(len(rows)), limit)], nil
}

// EverQueued is whether any job of kind was queued for target.
func (memory *Memory[T]) EverQueued(_ context.Context, kind jobqueue.Kind, target T) (bool, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	return slices.ContainsFunc(memory.rows, func(row jobqueue.Row[T]) bool {
		return row.Kind == kind && row.Target == target
	}), nil
}

// find is the row with id, or nil. The caller holds the mutex.
func (memory *Memory[T]) find(id int64) *jobqueue.Row[T] {
	for index := range memory.rows {
		if memory.rows[index].ID == id {
			return &memory.rows[index]
		}
	}
	return nil
}
