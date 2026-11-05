// SPDX-License-Identifier: AGPL-3.0-or-later

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
		case jobqueue.StateRunning, jobqueue.StateAssigned:
			open.Running = append(open.Running, row)
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
	if row == nil || !isOpen(row.State) {
		return false, nil
	}
	row.State, row.FinishedAt, row.AssignedUntil = jobqueue.StateCancelled, &cancelledAt, nil
	return true, nil
}

// KeepWanted is jobqueue.Storage's.
func (memory *Memory[T]) KeepWanted(_ context.Context, kind jobqueue.Kind, target T, until time.Time) error {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	for index := range memory.rows {
		row := &memory.rows[index]
		if row.Kind == kind && row.Target == target && isOpen(row.State) &&
			(row.WantedUntil == nil || row.WantedUntil.Before(until)) {
			row.WantedUntil = &until
		}
	}
	return nil
}

// ReleaseWanted is jobqueue.Storage's.
func (memory *Memory[T]) ReleaseWanted(_ context.Context, kind jobqueue.Kind, target T, now time.Time) error {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	released := now.Add(-time.Millisecond)
	for index := range memory.rows {
		row := &memory.rows[index]
		if row.Kind == kind && row.Target == target && isOpen(row.State) && row.WantedUntil != nil {
			row.WantedUntil = &released
		}
	}
	return nil
}

// DemoteUnwanted is jobqueue.Storage's.
func (memory *Memory[T]) DemoteUnwanted(_ context.Context, now time.Time, priority int64) ([]jobqueue.Row[T], error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	var demoted []jobqueue.Row[T]
	for index := range memory.rows {
		row := &memory.rows[index]
		if row.WantedUntil != nil && row.WantedUntil.Before(now) && isOpen(row.State) {
			row.Priority, row.WantedUntil = priority, nil
			demoted = append(demoted, *row)
		}
	}
	return demoted, nil
}

// PruneFinished is jobqueue.Storage's.
func (memory *Memory[T]) PruneFinished(_ context.Context, before time.Time) (int64, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	kept := memory.rows[:0]
	var pruned int64
	for _, row := range memory.rows {
		if !isOpen(row.State) && row.FinishedAt != nil && row.FinishedAt.Before(before) {
			pruned++
			continue
		}
		kept = append(kept, row)
	}
	memory.rows = kept
	return pruned, nil
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
	row.AssignedUntil = nil
	switch outcome.State {
	case jobqueue.StateAssigned:
		row.AssignedUntil = &outcome.At
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

// TakeAssigned returns an assigned job to running.
func (memory *Memory[T]) TakeAssigned(_ context.Context, id int64) (jobqueue.Row[T], bool, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	if memory.writeFailure != nil {
		return jobqueue.Row[T]{}, false, memory.writeFailure
	}
	row := memory.find(id)
	if row == nil || row.State != jobqueue.StateAssigned {
		return jobqueue.Row[T]{}, false, nil
	}
	row.State = jobqueue.StateRunning
	return *row, true, nil
}

// TakeOverdueAssigned returns to running the assigned job of a kind whose time ran out first.
func (memory *Memory[T]) TakeOverdueAssigned(
	_ context.Context, kind jobqueue.Kind, now time.Time,
) (jobqueue.Row[T], bool, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	if memory.writeFailure != nil {
		return jobqueue.Row[T]{}, false, memory.writeFailure
	}
	var first *jobqueue.Row[T]
	for index := range memory.rows {
		row := &memory.rows[index]
		if row.Kind != kind || row.State != jobqueue.StateAssigned || row.AssignedUntil.After(now) {
			continue
		}
		if first == nil || row.AssignedUntil.Before(*first.AssignedUntil) {
			first = row
		}
	}
	if first == nil {
		return jobqueue.Row[T]{}, false, nil
	}
	first.State = jobqueue.StateRunning
	return *first, true, nil
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
func (memory *Memory[T]) List(_ context.Context, beforeID, limit int64) ([]jobqueue.Row[T], error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	rows := slices.DeleteFunc(slices.Clone(memory.rows), func(row jobqueue.Row[T]) bool {
		return beforeID != 0 && row.ID >= beforeID
	})
	slices.Reverse(rows)
	return rows[:min(int64(len(rows)), limit)], nil
}

// Health is jobqueue.Storage's.
func (memory *Memory[T]) Health(_ context.Context, failedSince time.Time) ([]jobqueue.KindHealth, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	byKind := map[jobqueue.Kind]*jobqueue.KindHealth{}
	var kinds []jobqueue.Kind
	for _, row := range memory.rows {
		kind, isKnown := byKind[row.Kind]
		if !isKnown {
			kind = &jobqueue.KindHealth{Kind: row.Kind}
			byKind[row.Kind] = kind
			kinds = append(kinds, row.Kind)
		}
		if row.State == jobqueue.StateQueued {
			kind.QueuedCount++
			if kind.OldestQueuedAt.IsZero() || row.CreatedAt.Before(kind.OldestQueuedAt) {
				kind.OldestQueuedAt = row.CreatedAt
			}
		}
		if row.State == jobqueue.StateFailed && row.FinishedAt != nil && !row.FinishedAt.Before(failedSince) {
			kind.FailedCount++
		}
	}
	slices.Sort(kinds)
	health := make([]jobqueue.KindHealth, 0, len(kinds))
	for _, kind := range kinds {
		health = append(health, *byKind[kind])
	}
	return health, nil
}

// EnqueueBatch is Enqueue for each request, in order.
func (memory *Memory[T]) EnqueueBatch(
	ctx context.Context, requested []jobqueue.Requested[T],
	plan func(index int, open jobqueue.Open[T]) (jobqueue.Change, error),
) error {
	for index, each := range requested {
		err := memory.Enqueue(ctx, each.Kind, each.Target, func(open jobqueue.Open[T]) (jobqueue.Change, error) {
			return plan(index, open)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// CancelOpenOf cancels kind's open jobs on target that match says, and returns their ids.
func (memory *Memory[T]) CancelOpenOf(
	_ context.Context, kind jobqueue.Kind, target T, cancelledAt time.Time, match func(jobqueue.Row[T]) bool,
) ([]int64, error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	var cancelled []int64
	for index := range memory.rows {
		row := &memory.rows[index]
		if row.Kind != kind || row.Target != target || !isOpen(row.State) || !match(*row) {
			continue
		}
		row.State, row.FinishedAt, row.AssignedUntil = jobqueue.StateCancelled, &cancelledAt, nil
		cancelled = append(cancelled, row.ID)
	}
	return cancelled, nil
}

// ListOpen is every queued, running and assigned job, oldest first.
func (memory *Memory[T]) ListOpen(context.Context) ([]jobqueue.Row[T], error) {
	memory.mutex.Lock()
	defer memory.mutex.Unlock()
	var open []jobqueue.Row[T]
	for _, row := range memory.rows {
		if isOpen(row.State) {
			open = append(open, row)
		}
	}
	return open, nil
}

// isOpen is whether a job in state is open: queued, running or assigned.
func isOpen(state jobqueue.State) bool {
	return state == jobqueue.StateQueued || state == jobqueue.StateRunning || state == jobqueue.StateAssigned
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
