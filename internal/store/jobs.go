// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
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
		rows, err := queries.ListOpenJobs(ctx, cachegen.ListOpenJobsParams{
			Kind: string(kind), ItemID: columns.item, RenditionID: columns.rendition, StreamID: columns.stream,
			FetchedSubtitleID: columns.fetchedSubtitle, FileID: columns.file,
		})
		if err != nil {
			return fmt.Errorf("list the open %s jobs for %+v: %w", kind, target, err)
		}
		var open jobqueue.Open[JobTarget]
		for _, row := range rows {
			converted := jobRow(row)
			if converted.State == jobqueue.StateRunning {
				open.Running = &converted
			} else {
				open.Queued = &converted
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
			FetchedSubtitleID: columns.fetchedSubtitle, FileID: columns.file, ArgsJSON: change.ArgsJSON,
			Priority: change.Priority, RunnableAt: change.RunnableAt.UnixMilli(), Reason: &change.Reason,
			CreatedAt: change.CreatedAt.UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("queue a %s job: %w", kind, err)
		}
	case jobqueue.ActionMerge:
		merged, err := queries.MergeQueuedJob(ctx, cachegen.MergeQueuedJobParams{
			ArgsJSON: change.ArgsJSON, Priority: change.Priority, Reason: &change.Reason,
			RunnableAt: change.RunnableAt.UnixMilli(), ID: change.MergeIntoID,
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

// Cancel cancels a queued or running job, and says whether one was open.
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

// Claim marks the next due job of the claim's kind running, counts its attempt, and returns it.
func (jobStorage *JobStorage) Claim(ctx context.Context, claim jobqueue.Claim) (jobqueue.Row[JobTarget], bool, error) {
	var claimed jobqueue.Row[JobTarget]
	found := false
	err := jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		id, nextErr := OrNotFound(queries.NextQueuedJob(ctx, cachegen.NextQueuedJobParams{
			Kind: string(claim.Kind), Now: claim.Now.UnixMilli(),
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
				ID: outcome.ID,
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
		case jobqueue.StateRunning, jobqueue.StateCancelled:
		}
		return fmt.Errorf("job %d: an outcome cannot leave it %s", outcome.ID, outcome.State)
	})
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

// EverQueued is whether a job of kind was ever queued for target.
func (jobStorage *JobStorage) EverQueued(ctx context.Context, kind jobqueue.Kind, target JobTarget) (bool, error) {
	columns := target.columns()
	found, err := jobStorage.cache.Read().WasJobEverQueued(ctx, cachegen.WasJobEverQueuedParams{
		Kind: string(kind), ItemID: columns.item, RenditionID: columns.rendition, StreamID: columns.stream,
		FetchedSubtitleID: columns.fetchedSubtitle, FileID: columns.file,
	})
	if err != nil {
		return false, fmt.Errorf("look for a %s job for %+v: %w", kind, target, err)
	}
	return found, nil
}

// jobTargetColumns are a target as the job table holds it: NULL where unset.
type jobTargetColumns struct {
	item, rendition, stream, fetchedSubtitle, file *int64
}

// columns are the target's columns.
func (target JobTarget) columns() jobTargetColumns {
	return jobTargetColumns{
		item: ptr.NilIfZero(target.ItemID), rendition: ptr.NilIfZero(target.RenditionID),
		stream: ptr.NilIfZero(target.StreamID), fetchedSubtitle: ptr.NilIfZero(target.FetchedSubtitleID),
		file: ptr.NilIfZero(target.FileID),
	}
}

// jobRow is a job table row as the queue reads it.
func jobRow(row cachegen.Job) jobqueue.Row[JobTarget] {
	converted := jobqueue.Row[JobTarget]{
		ID: row.ID, Kind: jobqueue.Kind(row.Kind), ArgsJSON: row.ArgsJSON, Priority: row.Priority,
		Target: JobTarget{
			ItemID: ptr.ValueOrZero(row.ItemID), RenditionID: ptr.ValueOrZero(row.RenditionID),
			StreamID: ptr.ValueOrZero(row.StreamID), FetchedSubtitleID: ptr.ValueOrZero(row.FetchedSubtitleID),
			FileID: ptr.ValueOrZero(row.FileID),
		},
		State: jobqueue.State(row.State), RunnableAt: time.UnixMilli(row.RunnableAt), IsSnoozed: row.IsSnoozed != 0,
		AttemptCount: row.AttemptCount, LastAttemptToolFailed: row.LastAttemptToolFailed != 0,
		ProgressFraction: row.ProgressFraction, Reason: ptr.ValueOrZero(row.Reason),
		LastFailure: ptr.ValueOrZero(row.LastFailure), CreatedAt: time.UnixMilli(row.CreatedAt),
	}
	if row.FinishedAt != nil {
		converted.FinishedAt = new(time.UnixMilli(*row.FinishedAt))
	}
	return converted
}

// columnFlag is a boolean as a column holds it.
func columnFlag(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
