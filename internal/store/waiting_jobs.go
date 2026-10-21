// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"

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
	// NodeID and Profile are a prepare's rendition's node and profile.
	NodeID  int64
	Profile string
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
		})
	}
	return open, nil
}

// OpenPreparesOfFile are the open prepares of a file's renditions, the highest first: what a player
// waiting for a prepared copy waits on.
func (jobStorage *JobStorage) OpenPreparesOfFile(ctx context.Context, fileID int64) ([]OpenJob, error) {
	rows, err := jobStorage.cache.Read().ListOpenPreparesOfFile(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("list file %d's open prepares: %w", fileID, err)
	}
	open := make([]OpenJob, 0, len(rows))
	for _, row := range rows {
		open = append(open, OpenJob{
			ID: row.ID, IsAssigned: row.State == "assigned", DoneFraction: ptr.ValueOrZero(row.ProgressFraction),
			NodeID: row.NodeID, Profile: row.Profile,
		})
	}
	return open, nil
}
