// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// HideTitlesOfDeletedLibrary tombstones a library's films, series and episodes as an admin
// deletes it (D231 §7): hidden with their history, and with no merged_into, so a film of it that
// reappears in another library brings its title back (D208 §2). It returns their ids, whose work
// the delete cancels.
func (pass *Pass) HideTitlesOfDeletedLibrary(ctx context.Context, libraryID int64) ([]int64, error) {
	var hidden []int64
	err := pass.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		nowMs := pass.clock.Now().UnixMilli()
		var err error
		hidden, err = queries.TombstoneItemsOfLibrary(ctx, sqlcgen.TombstoneItemsOfLibraryParams{
			RemovedAt: &nowMs, LibraryID: &libraryID,
		})
		if err != nil {
			return fmt.Errorf("tombstone the titles: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hide library %d's titles: %w", libraryID, err)
	}
	return hidden, nil
}
