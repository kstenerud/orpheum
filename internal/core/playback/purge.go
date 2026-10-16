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
