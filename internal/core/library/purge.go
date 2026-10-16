// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// FilePurger deletes the removed files past their keeping, daily (D304 §6).
type FilePurger struct {
	database *store.Database
}

// NewFilePurger returns the purger of removed files.
func NewFilePurger(database *store.Database) *FilePurger {
	return &FilePurger{database: database}
}

// PurgedFilesForget deletes, or keeps elsewhere, what another package holds of removed files about
// to be deleted, in the transaction that deletes them: playback.FoldWatchingOfPurgedFiles, say.
// It is given the files' ids as a JSON array.
type PurgedFilesForget func(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) error

// filesPurgedInOneWrite is how many removed files one write deletes, so a purge of a library
// removed whole holds the writer a short while at a time (D303, DL1).
const filesPurgedInOneWrite = 200

// PurgeRemovedFiles deletes each file removed before removedBefore, with its streams, probe and
// NFO, after each of forgets has dealt with what its package holds of it, and says how many it
// deleted (D304 §6, D306 §8): every file ever removed was kept. A removed sidecar goes with its
// video or before it, and a video goes only once no sidecar of it stays. Its title and version
// stay: what was watched of it is kept on the version, and a new copy of it finds its title.
func (purger *FilePurger) PurgeRemovedFiles(
	ctx context.Context, removedBefore time.Time, forgets ...PurgedFilesForget,
) (int, error) {
	cutOff := removedBefore.UnixMilli()
	deleted := 0
	for {
		var deletedNow int64
		err := purger.database.Write(ctx, func(queries *sqlcgen.Queries) error {
			var err error
			deletedNow, err = purgeFiles(ctx, queries, cutOff, forgets)
			return err
		})
		if err != nil {
			return deleted, fmt.Errorf("delete the files removed before %s: %w",
				removedBefore.Format(time.RFC3339), err)
		}
		deleted += int(deletedNow)
		if deletedNow < filesPurgedInOneWrite {
			return deleted, nil
		}
	}
}

// purgeFiles deletes one batch of the files removed before cutOff, and says how many.
func purgeFiles(
	ctx context.Context, queries *sqlcgen.Queries, cutOff int64, forgets []PurgedFilesForget,
) (int64, error) {
	fileIDs, err := queries.ListFilesToPurge(ctx, sqlcgen.ListFilesToPurgeParams{
		RemovedBefore: &cutOff, MaxRows: filesPurgedInOneWrite,
	})
	if err != nil {
		return 0, fmt.Errorf("list the files: %w", err)
	}
	if len(fileIDs) == 0 {
		return 0, nil
	}
	encoded, err := json.Marshal(fileIDs)
	if err != nil {
		return 0, fmt.Errorf("encode the files' ids: %w", err)
	}
	fileIDsJSON := string(encoded)
	for _, forget := range forgets {
		if forgetErr := forget(ctx, queries, fileIDsJSON); forgetErr != nil {
			return 0, forgetErr
		}
	}
	for _, step := range []struct {
		what     string
		deleteOf func(context.Context, string) error
	}{
		{"streams", queries.DeleteStreamsOfPurgedFiles},
		{"probes", queries.DeleteProbesOfPurgedFiles},
		{"NFOs", queries.DeleteNFOsOfPurgedFiles},
	} {
		if err = step.deleteOf(ctx, fileIDsJSON); err != nil {
			return 0, fmt.Errorf("delete the files' %s: %w", step.what, err)
		}
	}
	deleted, err := queries.DeletePurgedFiles(ctx, fileIDsJSON)
	if err != nil {
		return 0, fmt.Errorf("delete the files: %w", err)
	}
	return deleted, nil
}
