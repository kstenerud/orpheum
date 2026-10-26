// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// OpenJob is an open job someone may be waiting on, as how long it will take reads it (D324 §6).
type OpenJob struct {
	ID       int64
	Priority int64
	// IsAssigned is whether a node holds it, running it or waiting there to.
	IsAssigned bool
	// FileID is a read-through's file.
	FileID int64
	// ArgsJSON are its arguments.
	ArgsJSON string
	// DoneFraction is how far it is, 0 to 1, where its node said; 0 before.
	DoneFraction float64
}

// OpenReadThroughs are every open read-through of a file: what is ahead of a file's own on its node.
func (jobStorage *JobStorage) OpenReadThroughs(ctx context.Context) ([]OpenJob, error) {
	rows, err := jobStorage.cache.Read().ListOpenReadThroughs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the open read-throughs: %w", err)
	}
	open := make([]OpenJob, 0, len(rows))
	for _, row := range rows {
		open = append(open, OpenJob{
			ID: row.ID, Priority: row.Priority, IsAssigned: row.State == "assigned",
			FileID: ptr.ValueOrZero(row.FileID), ArgsJSON: row.ArgsJSON,
			DoneFraction: ptr.ValueOrZero(row.ProgressFraction),
		})
	}
	return open, nil
}

// OpenJobsOfFile are kind's open jobs of one file, by job_open_target: whether a file has a
// read-through open, asked before the waits estimate reads every other's (audit 2026-10-25, S5).
func (jobStorage *JobStorage) OpenJobsOfFile(ctx context.Context, kind jobqueue.Kind, fileID int64) ([]OpenJob, error) {
	rows, err := jobStorage.cache.Read().ListOpenJobsOfTarget(ctx, openOfTargetParams(kind, JobTarget{FileID: fileID}))
	if err != nil {
		return nil, fmt.Errorf("list the open %s jobs of file %d: %w", kind, fileID, err)
	}
	open := make([]OpenJob, 0, len(rows))
	for _, row := range rows {
		open = append(open, OpenJob{
			ID: row.ID, Priority: row.Priority, IsAssigned: row.State == string(jobqueue.StateAssigned), FileID: fileID,
			ArgsJSON: row.ArgsJSON, DoneFraction: ptr.ValueOrZero(row.ProgressFraction),
		})
	}
	return open, nil
}
