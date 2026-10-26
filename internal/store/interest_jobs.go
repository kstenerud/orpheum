// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// RaisedJob is an open job a person's interest is in, with its priority now, its state, the
// destination it last waited on, "" for none, and whether this raise is what put it there.
type RaisedJob struct {
	ID          int64
	Priority    int64
	State       string
	Destination string
	HasRisen    bool
}

// RaiseJobsOfInterest gives every open job about the items or the files at least priority, wanted
// until untilMs, or for good when it is zero, and returns each, saying whether its priority rose
// (D324 §2). It reads them first and takes the writer only when one would change: a raise for good
// repeated each minute, or a tick whose interest has no open job, writes nothing (the audit of
// 2026-10-25, C4).
func (jobStorage *JobStorage) RaiseJobsOfInterest(
	ctx context.Context, itemIDs, fileIDs []int64, priority, untilMs int64,
) ([]RaisedJob, error) {
	items, files, err := idArraysJSON(itemIDs, fileIDs)
	if err != nil {
		return nil, err
	}
	open, err := jobStorage.cache.Read().ListOpenJobsOfItemsAndFiles(ctx, cachegen.ListOpenJobsOfItemsAndFilesParams{
		ItemIdsJSON: items, FileIdsJSON: files,
	})
	if err != nil {
		return nil, fmt.Errorf("list the interest's jobs: %w", err)
	}
	raised := make([]RaisedJob, 0, len(open))
	var due []cachegen.ListOpenJobsOfItemsAndFilesRow
	for _, job := range open {
		if isRaisedBy(job, priority, untilMs) {
			due = append(due, job)
			continue
		}
		raised = append(raised, RaisedJob{
			ID: job.ID, Priority: job.Priority, State: job.State, Destination: ptr.ValueOrZero(job.Destination),
		})
	}
	if len(due) == 0 {
		return raised, nil
	}
	err = jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		for _, job := range due {
			row, raiseErr := OrNotFound(queries.RaiseJob(ctx, cachegen.RaiseJobParams{
				ID: job.ID, Priority: priority, Until: untilMs,
			}))
			if errors.Is(raiseErr, ErrNotFound) {
				continue // it ended since it was read
			}
			if raiseErr != nil {
				return fmt.Errorf("raise job %d: %w", job.ID, raiseErr)
			}
			raised = append(raised, RaisedJob{
				ID: row.ID, Priority: row.Priority, State: row.State, Destination: ptr.ValueOrZero(row.Destination),
				HasRisen: row.Priority > job.Priority,
			})
		}
		return nil
	})
	slices.SortFunc(raised, func(first, second RaisedJob) int { return cmp.Compare(first.ID, second.ID) })
	return raised, err
}

// isRaisedBy is whether a raise to priority, wanted until untilMs (zero for good), changes the job:
// a priority below it, or a wanted time short of it.
func isRaisedBy(job cachegen.ListOpenJobsOfItemsAndFilesRow, priority, untilMs int64) bool {
	if job.Priority < priority {
		return true
	}
	return untilMs != 0 && (job.WantedUntil == nil || *job.WantedUntil < untilMs)
}

// ReleaseJobsOfInterest ends at nowMs the wanted time of the open jobs about the items or the files
// below a play's priority, playBelow: the next demotion takes them (D324 §2).
func (jobStorage *JobStorage) ReleaseJobsOfInterest(
	ctx context.Context, itemIDs, fileIDs []int64, playBelow, nowMs int64,
) error {
	items, files, err := idArraysJSON(itemIDs, fileIDs)
	if err != nil {
		return err
	}
	return jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		err := queries.ReleaseJobsOfItemsAndFiles(ctx, cachegen.ReleaseJobsOfItemsAndFilesParams{
			Now: nowMs, Below: playBelow, ItemIdsJSON: items, FileIdsJSON: files,
		})
		if err != nil {
			return fmt.Errorf("release the interest's jobs: %w", err)
		}
		return nil
	})
}

// idArraysJSON are the items and the files as the queries' JSON arrays.
func idArraysJSON(itemIDs, fileIDs []int64) (items, files string, err error) {
	itemsJSON, err := json.Marshal(append([]int64{}, itemIDs...))
	if err != nil {
		return "", "", fmt.Errorf("encode the items: %w", err)
	}
	filesJSON, err := json.Marshal(append([]int64{}, fileIDs...))
	if err != nil {
		return "", "", fmt.Errorf("encode the files: %w", err)
	}
	return string(itemsJSON), string(filesJSON), nil
}
