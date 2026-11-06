// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Refusals of the admin's word on one file, and on a root that moved.
var (
	// ErrFileReadable is a file an admin says is gone that can be read now: only what is missing
	// is gone on the admin's word (D241 §5).
	ErrFileReadable = errors.New("the file can be read now, so it is not gone")
	// ErrNoFile is a file that is not there to play: never seen, gone, not a video, or never probed.
	ErrNoFile = errors.New("no such file")
	// ErrRootCovered is a path the library already covers on the node, by a folder at or above it, or,
	// for a moved folder, one with others of the library below it (D363 §1).
	ErrRootCovered = errors.New("the library already covers that path on the node")
)

// RetireFile is the admin saying one file that cannot be read now is gone for good (D241 §5): it
// becomes a tombstone at every place it was found (D363 §2), its history staying on its title (D173),
// and its work, and that of a title no library lists any more, is cancelled. A file that can be read
// at any place is refused (ErrFileReadable).
func (admin *Admin) RetireFile(ctx context.Context, fileID int64) (Retired, error) {
	queries := admin.database.Read()
	file, err := store.OrNotFound(queries.GetPlayableFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return Retired{}, fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	if err != nil {
		return Retired{}, fmt.Errorf("read file %d: %w", fileID, err)
	}
	place, err := PlaceToRead(ctx, queries, admin.nodes, nil, auth.EveryLibrary(), file)
	if err != nil {
		return Retired{}, err
	}
	_, isUnreadable, err := UnreadableFile(ctx, queries, admin.nodes, place.NodeID, place.ReportedPathRaw)
	if err != nil {
		return Retired{}, err
	}
	if !isUnreadable {
		return Retired{}, fmt.Errorf("file %d: %w", fileID, ErrFileReadable)
	}
	return admin.retire(ctx, func(retire *retiring) error {
		titleIDs, listErr := retire.queries.ListWorksOfFileNoLibraryLists(ctx, fileID)
		if listErr != nil {
			return fmt.Errorf("list file %d's titles: %w", fileID, listErr)
		}
		removed, removeErr := removeFilesAndPlaces(ctx, retire.queries, retire.logger, []int64{fileID}, retire.nowMs)
		if removeErr != nil {
			return removeErr
		}
		retire.fileIDs = append(retire.fileIDs, removed...)
		retire.tombstonedCount += len(removed)
		// Its titles no library listed but by it are listed by none now.
		gone, goneErr := retire.queries.ListWorksOfFileNoLibraryLists(ctx, fileID)
		if goneErr != nil {
			return fmt.Errorf("list file %d's titles: %w", fileID, goneErr)
		}
		for _, titleID := range gone {
			if !slices.Contains(titleIDs, titleID) {
				retire.itemIDs = append(retire.itemIDs, titleID)
			}
		}
		return nil
	})
}

// forgetRootState forgets a folder that is no scan's root any more, retired or repointed: its scans,
// its facts, the disks mounted inside it and its places' availability.
func forgetRootState(ctx context.Context, queries *sqlcgen.Queries, nodeID int64, rootPath string) error {
	place := sqlcgen.ForgetRootStateParams{NodeID: nodeID, RootPath: rootPath}
	if err := queries.ForgetRootState(ctx, place); err != nil {
		return fmt.Errorf("forget %s's state: %w", rootPath, err)
	}
	if err := queries.ForgetRootMounts(ctx, sqlcgen.ForgetRootMountsParams(place)); err != nil {
		return fmt.Errorf("forget %s's mounts: %w", rootPath, err)
	}
	if err := queries.ForgetRootScan(ctx, sqlcgen.ForgetRootScanParams(place)); err != nil {
		return fmt.Errorf("forget %s's last scan: %w", rootPath, err)
	}
	return forgetRootPlaces(ctx, queries, nodeID, rootPath)
}
