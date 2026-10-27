// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// FoldWatchingOfPurgedFiles keeps what was watched of removed files about to be deleted on their
// versions, how often each person finished one and how long they watched, then deletes their
// spans, events and places in them (D306 §8): what was watched follows the title and its edition,
// and a place in a file names a moment in its timeline alone. It is given the files' ids as a JSON
// array, and writes through queries, the transaction that deletes them
// (library.FilePurger.PurgeRemovedFiles).
func FoldWatchingOfPurgedFiles(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) error {
	if err := queries.FoldWatchingOfPurgedFiles(ctx, fileIDsJSON); err != nil {
		return fmt.Errorf("keep what was watched of the files: %w", err)
	}
	for _, step := range []struct {
		what     string
		deleteOf func(context.Context, string) error
	}{
		{"spans", queries.DeletePlaySpansOfPurgedFiles},
		{"events", queries.DeletePlayEventsOfPurgedFiles},
		{"positions", queries.DeletePlayPositionsOfPurgedFiles},
	} {
		if err := step.deleteOf(ctx, fileIDsJSON); err != nil {
			return fmt.Errorf("delete the files' %s: %w", step.what, err)
		}
	}
	return nil
}

// MoveWatchingToCopysFile moves what was watched of a file confirmed to be a copy of another onto that
// file, in the copy's confirmation (library.CopyFold, D363 §2): every span and event, and each person's
// position, the later of theirs in the two where they have one in both.
func MoveWatchingToCopysFile(ctx context.Context, queries *sqlcgen.Queries, fromFileID, toFileID int64) error {
	err := queries.MovePlayPositionsToFile(ctx, sqlcgen.MovePlayPositionsToFileParams{
		ToFile: toFileID, FromFile: fromFileID,
	})
	if err == nil {
		err = queries.DeletePlayPositionsOfFile(ctx, fromFileID)
	}
	if err == nil {
		err = queries.MovePlayEventsToFile(ctx,
			sqlcgen.MovePlayEventsToFileParams{ToFile: toFileID, FromFile: fromFileID})
	}
	if err == nil {
		err = queries.MovePlaySpansToFile(ctx,
			sqlcgen.MovePlaySpansToFileParams{ToFile: toFileID, FromFile: fromFileID})
	}
	if err != nil {
		return fmt.Errorf("move what was watched of file %d to file %d: %w", fromFileID, toFileID, err)
	}
	return nil
}
