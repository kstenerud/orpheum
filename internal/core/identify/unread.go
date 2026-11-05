// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// RetireVersionInto retires the version a row found to be another file left (library.VersionRetirer):
// a row a listing stored unread whose read found a file stored before that moved or came back, or a
// file a whole read confirmed to be a copy of one found before it. The version, holding no file any
// more, is removed, and the title the pass made it in, left with no live version, is merged into
// intoTitleID, the other file's title of the same kind, which holds the work's history (D304 §5, D363
// §2, D173).
func RetireVersionInto(ctx context.Context, queries *sqlcgen.Queries, versionID, intoTitleID, nowMs int64) error {
	holding, err := queries.CountFilingsOfVersion(ctx, &versionID)
	if err != nil {
		return fmt.Errorf("count the files of version %d: %w", versionID, err)
	}
	if holding > 0 {
		return nil // another part of it stays
	}
	version, err := queries.GetVersionWork(ctx, versionID)
	if err != nil {
		return fmt.Errorf("read version %d's title: %w", versionID, err)
	}
	err = queries.RemoveVersion(ctx, sqlcgen.RemoveVersionParams{RemovedAt: &nowMs, ID: versionID})
	if err != nil {
		return fmt.Errorf("remove version %d: %w", versionID, err)
	}
	if version.WorkID == intoTitleID {
		return nil
	}
	err = queries.MergeWorkWithNoLiveVersion(ctx, sqlcgen.MergeWorkWithNoLiveVersionParams{
		MergedInto: &intoTitleID, RemovedAt: &nowMs, WorkID: version.WorkID,
	})
	if err != nil {
		return fmt.Errorf("merge title %d into %d: %w", version.WorkID, intoTitleID, err)
	}
	return nil
}
