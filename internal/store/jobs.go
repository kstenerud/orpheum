// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// JobTarget is what a job in cache.db's job table works on: one column per kind of target
// (D227 §2), zero when unset. Which kind of job takes which is the caller's; the table holds at
// most one.
type JobTarget struct {
	ItemID            int64
	RenditionID       int64
	StreamID          int64
	FetchedSubtitleID int64
	FileID            int64
	ScanID            int64
	ScanBatchID       int64
	ChunkStoreID      int64
}

// JobStorage is the job queue's storage on cache.db's job table (D231 §2): the queue in
// internal/platform/jobqueue decides, and this reads and writes the rows.
type JobStorage struct {
	cache *Cache
}

// Compile-time proof that *JobStorage satisfies jobqueue.Storage.
var _ jobqueue.Storage[JobTarget] = (*JobStorage)(nil)

// Jobs is the job queue's storage on this cache.db.
func (cache *Cache) Jobs() *JobStorage { return &JobStorage{cache: cache} }

// Enqueue reads kind's open jobs for target and applies plan's change, in one write.
func (jobStorage *JobStorage) Enqueue(
	ctx context.Context, kind jobqueue.Kind, target JobTarget,
	plan func(open jobqueue.Open[JobTarget]) (jobqueue.Change, error),
) error {
	return jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		columns := target.columns()
		rows, err := queries.ListOpenJobsOfTarget(ctx, cachegen.ListOpenJobsOfTargetParams{
			Kind: string(kind), ItemID: columns.item, RenditionID: columns.rendition, StreamID: columns.stream,
			FetchedSubtitleID: columns.fetchedSubtitle, FileID: columns.file,
			ScanID: columns.scan, ScanBatchID: columns.scanBatch, ChunkStoreID: columns.chunkStore,
		})
		if err != nil {
			return fmt.Errorf("list the open %s jobs for %+v: %w", kind, target, err)
		}
		var open jobqueue.Open[JobTarget]
		for _, row := range rows {
			converted := jobRow(row)
			if converted.State == jobqueue.StateQueued {
				open.Queued = &converted
			} else {
				open.Running = &converted // running, or assigned, which counts as running
			}
		}
		change, err := plan(open)
		if err != nil {
			return err
		}
		return applyChange(ctx, queries, kind, target, change)
	})
}

// applyChange writes an enqueue's change.
func applyChange(
	ctx context.Context, queries *cachegen.Queries, kind jobqueue.Kind, target JobTarget, change jobqueue.Change,
) error {
	switch change.Action {
	case jobqueue.ActionInsert:
		columns := target.columns()
		_, err := queries.InsertJob(ctx, cachegen.InsertJobParams{
			Kind: string(kind), ItemID: columns.item, RenditionID: columns.rendition, StreamID: columns.stream,
			FetchedSubtitleID: columns.fetchedSubtitle, FileID: columns.file,
			ScanID: columns.scan, ScanBatchID: columns.scanBatch, ChunkStoreID: columns.chunkStore,
			ArgsJSON: change.ArgsJSON,
			Priority: change.Priority, RunnableAt: change.RunnableAt.UnixMilli(), Reason: &change.Reason,
			Destination: destinationColumn(change.Destination), CreatedAt: change.CreatedAt.UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("queue a %s job: %w", kind, err)
		}
	case jobqueue.ActionMerge:
		merged, err := queries.MergeQueuedJob(ctx, cachegen.MergeQueuedJobParams{
			ArgsJSON: change.ArgsJSON, Priority: change.Priority, Reason: &change.Reason,
			RunnableAt: change.RunnableAt.UnixMilli(), Destination: destinationColumn(change.Destination),
			ID: change.MergeIntoID,
		})
		if err != nil {
			return fmt.Errorf("merge a request into job %d: %w", change.MergeIntoID, err)
		}
		if merged != 1 {
			return fmt.Errorf("merge a request into job %d: it is not queued", change.MergeIntoID)
		}
	case jobqueue.ActionDrop:
	}
	return nil
}

// Cancel cancels a queued, running or assigned job, and says whether one was open.
func (jobStorage *JobStorage) Cancel(ctx context.Context, id int64, at time.Time) (bool, error) {
	var cancelledCount int64
	err := jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var cancelErr error
		cancelledCount, cancelErr = queries.CancelJob(ctx, cachegen.CancelJobParams{
			FinishedAt: new(at.UnixMilli()), ID: id,
		})
		if cancelErr != nil {
			return fmt.Errorf("mark it cancelled: %w", cancelErr)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("cancel job %d: %w", id, err)
	}
	return cancelledCount > 0, nil
}

// KeepWanted moves on, to until, the time a person waits for kind's open jobs on target until.
func (jobStorage *JobStorage) KeepWanted(
	ctx context.Context, kind jobqueue.Kind, target JobTarget, until time.Time,
) error {
	return jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		columns := target.columns()
		return queries.KeepJobWanted(ctx, cachegen.KeepJobWantedParams{
			Until: until.UnixMilli(), Kind: string(kind), ItemID: columns.item, RenditionID: columns.rendition,
			StreamID: columns.stream, FetchedSubtitleID: columns.fetchedSubtitle, FileID: columns.file,
			ScanID: columns.scan, ScanBatchID: columns.scanBatch, ChunkStoreID: columns.chunkStore,
		})
	})
}

// DemoteUnwanted gives every open job whose wanted time is before now the priority, and returns
// each, by its id and state.
func (jobStorage *JobStorage) DemoteUnwanted(
	ctx context.Context, now time.Time, priority int64,
) ([]jobqueue.Row[JobTarget], error) {
	var demoted []jobqueue.Row[JobTarget]
	err := jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		rows, err := queries.DemoteUnwantedJobs(ctx, cachegen.DemoteUnwantedJobsParams{
			Priority: priority, Now: now.UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("demote the jobs no one waits for: %w", err)
		}
		for _, row := range rows {
			demoted = append(demoted, jobqueue.Row[JobTarget]{
				ID: row.ID, State: jobqueue.State(row.State), Priority: priority,
			})
		}
		return nil
	})
	return demoted, err
}

// PruneFinished forgets the jobs that ended before a time.
func (jobStorage *JobStorage) PruneFinished(ctx context.Context, before time.Time) (int64, error) {
	var pruned int64
	err := jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var deleteErr error
		pruned, deleteErr = queries.DeleteFinishedJobs(ctx, before.UnixMilli())
		if deleteErr != nil {
			return fmt.Errorf("delete the jobs that ended: %w", deleteErr)
		}
		return nil
	})
	return pruned, err
}

// Claim marks the next due job of the claim's kind running, counts its attempt, and returns it.
func (jobStorage *JobStorage) Claim(ctx context.Context, claim jobqueue.Claim) (jobqueue.Row[JobTarget], bool, error) {
	var claimed jobqueue.Row[JobTarget]
	found := false
	unclaimable, err := json.Marshal(append([]jobqueue.Destination{}, claim.Unclaimable...))
	if err != nil {
		return claimed, false, fmt.Errorf("claim: encode the parked destinations: %w", err)
	}
	err = jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		id, nextErr := OrNotFound(queries.NextQueuedJob(ctx, cachegen.NextQueuedJobParams{
			Kind: string(claim.Kind), Now: claim.Now.UnixMilli(), UnclaimableJSON: string(unclaimable),
		}))
		if errors.Is(nextErr, ErrNotFound) {
			return nil
		}
		if nextErr != nil {
			return fmt.Errorf("find the next %s job: %w", claim.Kind, nextErr)
		}
		row, startErr := queries.StartJob(ctx, id)
		if startErr != nil {
			return fmt.Errorf("start job %d: %w", id, startErr)
		}
		claimed, found = jobRow(row), true
		return nil
	})
	if err != nil {
		return jobqueue.Row[JobTarget]{}, false, fmt.Errorf("claim: %w", err)
	}
	return claimed, found, nil
}

// Record writes a claimed job's outcome, unless it was cancelled meanwhile.
func (jobStorage *JobStorage) Record(ctx context.Context, outcome jobqueue.Outcome) error {
	lastFailure := ptr.NilIfZero(outcome.LastFailure)
	return jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		switch outcome.State {
		case jobqueue.StateQueued:
			err := queries.DeferJob(ctx, cachegen.DeferJobParams{
				RunnableAt: outcome.At.UnixMilli(), LastFailure: lastFailure, AttemptCount: outcome.AttemptCount,
				IsSnoozed: columnFlag(outcome.IsSnoozed), LastAttemptToolFailed: columnFlag(outcome.ToolFailed),
				Destination: destinationColumn(outcome.Destination), ID: outcome.ID,
			})
			if err != nil {
				return fmt.Errorf("return job %d to the queue: %w", outcome.ID, err)
			}
			return nil
		case jobqueue.StateDone, jobqueue.StateFailed:
			params := cachegen.FinishJobParams{
				State: string(outcome.State), LastFailure: lastFailure, AttemptCount: outcome.AttemptCount,
				FinishedAt: new(outcome.At.UnixMilli()), ID: outcome.ID,
			}
			if outcome.State == jobqueue.StateDone {
				params.ProgressFraction = new(1.0)
			}
			if err := queries.FinishJob(ctx, params); err != nil {
				return fmt.Errorf("finish job %d: %w", outcome.ID, err)
			}
			return nil
		case jobqueue.StateAssigned:
			err := queries.AssignJob(ctx, cachegen.AssignJobParams{
				AssignedUntil: new(outcome.At.UnixMilli()), AttemptCount: outcome.AttemptCount, ID: outcome.ID,
			})
			if err != nil {
				return fmt.Errorf("assign job %d: %w", outcome.ID, err)
			}
			return nil
		case jobqueue.StateRunning, jobqueue.StateCancelled:
		}
		return fmt.Errorf("job %d: an outcome cannot leave it %s", outcome.ID, outcome.State)
	})
}

// TakeAssigned returns an assigned job to running, and reports false when it is not assigned.
func (jobStorage *JobStorage) TakeAssigned(ctx context.Context, id int64) (jobqueue.Row[JobTarget], bool, error) {
	return jobStorage.take(ctx, "take assigned job", func(queries *cachegen.Queries) (cachegen.Job, error) {
		return queries.TakeAssignedJob(ctx, id)
	})
}

// TakeOverdueAssigned returns to running the assigned job of kind whose time ran out first, by now.
func (jobStorage *JobStorage) TakeOverdueAssigned(
	ctx context.Context, kind jobqueue.Kind, now time.Time,
) (jobqueue.Row[JobTarget], bool, error) {
	return jobStorage.take(ctx, "take an overdue assigned job", func(queries *cachegen.Queries) (cachegen.Job, error) {
		return queries.TakeOverdueAssignedJob(ctx, cachegen.TakeOverdueAssignedJobParams{
			Kind: string(kind), Now: new(now.UnixMilli()),
		})
	})
}

// take writes what update does to one job and returns the job, or false when it found none.
func (jobStorage *JobStorage) take(
	ctx context.Context, action string, update func(*cachegen.Queries) (cachegen.Job, error),
) (jobqueue.Row[JobTarget], bool, error) {
	var taken jobqueue.Row[JobTarget]
	found := false
	err := jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		row, updateErr := OrNotFound(update(queries))
		if errors.Is(updateErr, ErrNotFound) {
			return nil
		}
		if updateErr != nil {
			return updateErr
		}
		taken, found = jobRow(row), true
		return nil
	})
	if err != nil {
		return jobqueue.Row[JobTarget]{}, false, fmt.Errorf("%s: %w", action, err)
	}
	return taken, found, nil
}

// SetProgress records how far a running job is.
func (jobStorage *JobStorage) SetProgress(ctx context.Context, id int64, fraction float64) error {
	return jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.SetJobProgress(ctx, cachegen.SetJobProgressParams{ProgressFraction: &fraction, ID: id})
	})
}

// RequeueRunning returns every running job to queued, each keeping its counted attempt.
func (jobStorage *JobStorage) RequeueRunning(ctx context.Context) (int64, error) {
	var count int64
	err := jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var requeueErr error
		count, requeueErr = queries.RequeueRunningJobs(ctx)
		if requeueErr != nil {
			return fmt.Errorf("requeue running jobs: %w", requeueErr)
		}
		return nil
	})
	return count, err
}

// List is the newest jobs, newest first, at most limit.
func (jobStorage *JobStorage) List(ctx context.Context, limit int64) ([]jobqueue.Row[JobTarget], error) {
	rows, err := jobStorage.cache.Read().ListJobs(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	listed := make([]jobqueue.Row[JobTarget], 0, len(rows))
	for _, row := range rows {
		listed = append(listed, jobRow(row))
	}
	return listed, nil
}

// Health is each kind's queued jobs, the oldest of them and its jobs failed since failedSince.
func (jobStorage *JobStorage) Health(ctx context.Context, failedSince time.Time) ([]jobqueue.KindHealth, error) {
	rows, err := jobStorage.cache.Read().CountJobsByKind(ctx, new(failedSince.UnixMilli()))
	if err != nil {
		return nil, fmt.Errorf("count the jobs by kind: %w", err)
	}
	health := make([]jobqueue.KindHealth, 0, len(rows))
	for _, row := range rows {
		kind := jobqueue.KindHealth{
			Kind: jobqueue.Kind(row.Kind), QueuedCount: row.QueuedCount, FailedCount: row.FailedCount,
		}
		if row.OldestQueuedAt != 0 {
			kind.OldestQueuedAt = time.UnixMilli(row.OldestQueuedAt)
		}
		health = append(health, kind)
	}
	return health, nil
}

// ListOpen is every queued, running and assigned job.
func (jobStorage *JobStorage) ListOpen(ctx context.Context) ([]jobqueue.Row[JobTarget], error) {
	rows, err := jobStorage.cache.Read().ListEveryOpenJob(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the open jobs: %w", err)
	}
	listed := make([]jobqueue.Row[JobTarget], 0, len(rows))
	for _, row := range rows {
		listed = append(listed, jobRow(row))
	}
	return listed, nil
}

// jobTargetColumns are a target as the job table holds it: NULL where unset.
type jobTargetColumns struct {
	item, rendition, stream, fetchedSubtitle, file, scan, scanBatch, chunkStore *int64
}

// columns are the target's columns.
func (target JobTarget) columns() jobTargetColumns {
	return jobTargetColumns{
		item: ptr.NilIfZero(target.ItemID), rendition: ptr.NilIfZero(target.RenditionID),
		stream: ptr.NilIfZero(target.StreamID), fetchedSubtitle: ptr.NilIfZero(target.FetchedSubtitleID),
		file: ptr.NilIfZero(target.FileID), scan: ptr.NilIfZero(target.ScanID),
		scanBatch: ptr.NilIfZero(target.ScanBatchID), chunkStore: ptr.NilIfZero(target.ChunkStoreID),
	}
}

// jobRow is a job table row as the queue reads it.
func jobRow(row cachegen.Job) jobqueue.Row[JobTarget] {
	converted := jobqueue.Row[JobTarget]{
		ID: row.ID, Kind: jobqueue.Kind(row.Kind), ArgsJSON: row.ArgsJSON, Priority: row.Priority,
		Target: JobTarget{
			ItemID: ptr.ValueOrZero(row.ItemID), RenditionID: ptr.ValueOrZero(row.RenditionID),
			StreamID: ptr.ValueOrZero(row.StreamID), FetchedSubtitleID: ptr.ValueOrZero(row.FetchedSubtitleID),
			FileID: ptr.ValueOrZero(row.FileID), ScanID: ptr.ValueOrZero(row.ScanID),
			ScanBatchID: ptr.ValueOrZero(row.ScanBatchID), ChunkStoreID: ptr.ValueOrZero(row.ChunkStoreID),
		},
		State: jobqueue.State(row.State), Destination: jobqueue.Destination(ptr.ValueOrZero(row.Destination)),
		RunnableAt: time.UnixMilli(row.RunnableAt), IsSnoozed: row.IsSnoozed != 0,
		AttemptCount: row.AttemptCount, LastAttemptToolFailed: row.LastAttemptToolFailed != 0,
		ProgressFraction: row.ProgressFraction, Reason: ptr.ValueOrZero(row.Reason),
		LastFailure: ptr.ValueOrZero(row.LastFailure), CreatedAt: time.UnixMilli(row.CreatedAt),
	}
	if row.FinishedAt != nil {
		converted.FinishedAt = new(time.UnixMilli(*row.FinishedAt))
	}
	if row.AssignedUntil != nil {
		converted.AssignedUntil = new(time.UnixMilli(*row.AssignedUntil))
	}
	if row.WantedUntil != nil {
		converted.WantedUntil = new(time.UnixMilli(*row.WantedUntil))
	}
	return converted
}

// destinationColumn is a job's destination as its column holds it: NULL when it names none.
func destinationColumn(destination jobqueue.Destination) *string {
	return ptr.NilIfZero(string(destination))
}

// columnFlag is a boolean as a column holds it.
func columnFlag(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
