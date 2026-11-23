// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// systemLibraryName is the system library's name, which no listing shows (D435 §1).
const systemLibraryName = "System videos"

// CreateSystemLibrary makes the hidden system library over a node's system folder, unless one was
// made first (D435 §1, §5): an other library, read from NFO alone and scanned only when asked, found
// by its system_library row. The system videos make it through this, for this package alone writes
// the library tables.
func CreateSystemLibrary(
	ctx context.Context, database *store.Database, clockSource clock.Clock, nodeID int64, folder string,
) error {
	return database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := store.OrNotFound(queries.GetSystemLibrary(ctx))
		if err == nil {
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("read the system library: %w", err)
		}
		// Kept as every library's hours are, and never used: it is scanned only when asked (D541 §1).
		hours, err := hoursToStart(ctx, nil, nil)
		if err != nil {
			return err
		}
		id, err := queries.InsertLibrary(ctx, sqlcgen.InsertLibraryParams{
			Name: systemLibraryName, Kind: LibraryKindOther, MetadataSource: "nfo", ChangeDetection: "manual",
			CheckEveryHours: hours, CreatedAt: clockSource.Now().UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("create the system library: %w", err)
		}
		err = queries.InsertLibraryRoot(ctx, sqlcgen.InsertLibraryRootParams{
			LibraryID: id, NodeID: nodeID, Path: folder,
		})
		if err != nil {
			return fmt.Errorf("add the system library's folder: %w", err)
		}
		return queries.InsertSystemLibrary(ctx, id)
	})
}
