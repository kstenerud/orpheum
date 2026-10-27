// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ForgetSubtitlesOfPurgedFiles deletes the subtitles fetched for removed files about to be deleted
// (D306 §8): each was chosen for and timed to its file. It is given the files' ids as a JSON
// array, and writes through queries, the transaction that deletes them
// (library.FilePurger.PurgeRemovedFiles).
func ForgetSubtitlesOfPurgedFiles(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) error {
	if err := queries.DeleteFetchedSubtitlesOfPurgedFiles(ctx, fileIDsJSON); err != nil {
		return fmt.Errorf("delete the subtitles fetched for the files: %w", err)
	}
	return nil
}

// MoveFetchedToCopysFile moves the subtitles fetched for a file confirmed to be a copy of another onto
// that file, one fetched for both from one provider's file kept once, in the copy's confirmation
// (library.CopyFold, D363 §2).
func MoveFetchedToCopysFile(ctx context.Context, queries *sqlcgen.Queries, fromFileID, toFileID int64) error {
	err := queries.MoveFetchedSubtitlesToFile(ctx, sqlcgen.MoveFetchedSubtitlesToFileParams{
		ToFile: toFileID, FromFile: fromFileID,
	})
	if err == nil {
		err = queries.DeleteFetchedSubtitlesOfFile(ctx, fromFileID)
	}
	if err != nil {
		return fmt.Errorf("move the subtitles fetched for file %d to file %d: %w", fromFileID, toFileID, err)
	}
	return nil
}
