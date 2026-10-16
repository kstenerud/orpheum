// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// RetireUnreadFile retires what a pass filed for a row a listing stored unread, unreadID, whose read
// found it to be fileID, a file stored before that moved or came back (library.UnreadRetirer): its
// version is removed, and the title the pass made from its path, left with no live version, is
// merged into fileID's, which holds the work's history (D304 §5, D173).
func RetireUnreadFile(ctx context.Context, queries *sqlcgen.Queries, unreadID, fileID, nowMs int64) error {
	unread, err := store.OrNotFound(queries.GetFileTitle(ctx, unreadID))
	if errors.Is(err, store.ErrNotFound) {
		return nil // never filed
	}
	if err != nil {
		return fmt.Errorf("read the unread file %d's title: %w", unreadID, err)
	}
	removed := sqlcgen.RemoveVersionParams{RemovedAt: &nowMs, ID: ptr.ValueOrZero(unread.VersionID)}
	err = queries.RemoveVersion(ctx, removed)
	if err != nil {
		return fmt.Errorf("remove the unread file %d's version: %w", unreadID, err)
	}
	stored, err := store.OrNotFound(queries.GetFileTitle(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && stored.TitleID == unread.TitleID) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read file %d's title: %w", fileID, err)
	}
	err = queries.MergeTitleWithNoLiveVersion(ctx, sqlcgen.MergeTitleWithNoLiveVersionParams{
		MergedInto: &stored.TitleID, RemovedAt: &nowMs, TitleID: unread.TitleID,
	})
	if err != nil {
		return fmt.Errorf("merge title %d into %d: %w", unread.TitleID, stored.TitleID, err)
	}
	return nil
}
