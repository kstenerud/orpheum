// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// RaisedJob is an open job a person's interest raised, with its new priority and its state.
type RaisedJob struct {
	ID       int64
	Priority int64
	State    string
}

// RaiseJobsOfInterest gives every open job about the items or the files at least priority, wanted
// until untilMs, or for good when it is zero, and returns those whose priority rose (D324 §2).
func (jobStorage *JobStorage) RaiseJobsOfInterest(
	ctx context.Context, itemIDs, fileIDs []int64, priority, untilMs int64,
) ([]RaisedJob, error) {
	items, files, err := idArraysJSON(itemIDs, fileIDs)
	if err != nil {
		return nil, err
	}
	var raised []RaisedJob
	err = jobStorage.cache.Write(ctx, func(queries *cachegen.Queries) error {
		open, listErr := queries.ListOpenJobsOfItemsAndFiles(ctx, cachegen.ListOpenJobsOfItemsAndFilesParams{
			ItemIdsJSON: items, FileIdsJSON: files,
		})
		if listErr != nil {
			return fmt.Errorf("list the interest's jobs: %w", listErr)
		}
		for _, job := range open {
			row, raiseErr := queries.RaiseJob(ctx, cachegen.RaiseJobParams{
				ID: job.ID, Priority: priority, Until: untilMs,
			})
			if raiseErr != nil {
				return fmt.Errorf("raise job %d: %w", job.ID, raiseErr)
			}
			if row.Priority > job.Priority {
				raised = append(raised, RaisedJob{ID: row.ID, Priority: row.Priority, State: row.State})
			}
		}
		return nil
	})
	return raised, err
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
