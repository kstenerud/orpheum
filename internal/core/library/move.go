// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
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

// Repoint is a library root an admin says is now at another path, on the same node or another.
type Repoint struct {
	LibraryID   int64
	NodeName    string
	Path        string
	NewNodeName string
	NewPath     string
}

// Repointed is what a repoint moved: how many live files it took to the new place.
type Repointed struct {
	MovedFileCount int
}

// RepointRoot is the admin saying a library root moved (D241 §6): the root and its live files take
// the new path, keeping their history, and a scan of it is asked for, which reads each file again
// and confirms it by its payload hash before it counts as there, one it cannot confirm being a
// removal under D208. A file another library's folder still covers at the old path stays there, that
// library's (D363 §1). The new path must be one the new node names, and the library may not cover it
// already, by it or a folder above it.
func (admin *Admin) RepointRoot(ctx context.Context, repoint Repoint) (Repointed, error) {
	root, err := admin.findRoot(ctx, repoint.LibraryID, repoint.NodeName, repoint.Path)
	if err != nil {
		return Repointed{}, err
	}
	newNodeID, err := admin.libraryFolderOn(ctx, repoint.NewNodeName, repoint.NewPath)
	if err != nil {
		return Repointed{}, err
	}
	// The folder moved away from covers nothing once it has moved.
	except := ""
	if newNodeID == root.NodeID {
		except = repoint.Path
	}
	replaced, coveredErr := admin.refuseCovered(ctx, repoint.LibraryID, newNodeID, repoint.NewNodeName,
		repoint.NewPath, except)
	if coveredErr != nil {
		return Repointed{}, coveredErr
	}
	if len(replaced) > 0 {
		// A folder moved above others of its library would take their places: that is an add, not a move.
		return Repointed{}, fmt.Errorf("%s on node %q: %w: the library has %v below it", repoint.NewPath,
			repoint.NewNodeName, ErrRootCovered, replaced)
	}
	var moved int
	err = admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var moveErr error
		moved, moveErr = moveRoot(ctx, queries, repoint, root.NodeID, newNodeID)
		return moveErr
	})
	if err != nil {
		return Repointed{}, err
	}
	admin.watchedFoldersChanged() // the old folder is watched no more, the new one from now
	// The work parked on the old place waits on a destination that is no more; the sweeps derive
	// it again for the new one.
	forgotten := jobs.RetiredWork{Destinations: []jobs.Destination{jobs.RootDestination(root.NodeID, repoint.Path)}}
	if _, err := admin.retirement.Work.CancelRetired(ctx, forgotten); err != nil {
		return Repointed{}, fmt.Errorf("forget the old place's waiting work: %w", err)
	}
	if err := admin.scans.Request(ctx, newNodeID, repoint.NewPath, jobs.PriorityPersonsRequest); err != nil {
		return Repointed{}, err
	}
	return Repointed{MovedFileCount: moved}, nil
}

// moveRoot moves a library root, the live places under it that no other library's folder covers,
// and, when no live folder is left at the old path, its state, to the new place, in one transaction,
// and returns how many files moved.
func moveRoot(ctx context.Context, queries *sqlcgen.Queries, repoint Repoint, nodeID, newNodeID int64) (int, error) {
	count, err := queries.RepointLibraryRoot(ctx, sqlcgen.RepointLibraryRootParams{
		NewNodeID: newNodeID, NewRootPath: repoint.NewPath, LibraryID: repoint.LibraryID, NodeID: nodeID,
		RootPath: repoint.Path,
	})
	if err != nil {
		return 0, fmt.Errorf("move root %s: %w", repoint.Path, err)
	}
	if count == 0 {
		return 0, fmt.Errorf("%s: %w", repoint.Path, ErrNoSuchRoot)
	}
	after, before := PathRange(repoint.Path)
	left, err := queries.ListUncoveredFilesUnder(ctx, sqlcgen.ListUncoveredFilesUnderParams{
		NodeID: nodeID, After: after, Before: before,
	})
	if err != nil {
		return 0, fmt.Errorf("list the files under %s: %w", repoint.Path, err)
	}
	files, err := queries.ListLiveFilesUnder(ctx, sqlcgen.ListLiveFilesUnderParams{
		NodeID: nodeID, After: after, Before: before,
	})
	if err != nil {
		return 0, fmt.Errorf("list the files under %s: %w", repoint.Path, err)
	}
	moved := 0
	for _, file := range files {
		relative, under := RelativeToRoot(file.ReportedPathRaw, repoint.Path)
		if !under || !slices.Contains(left, file.ID) {
			continue
		}
		newRaw := bytes.Join([][]byte{[]byte(repoint.NewPath), relative}, []byte("/"))
		newPath := PathText(newRaw)
		// The path it was identified from first, read from the place before it moves.
		moveErr := queries.RepointFileFilings(ctx, sqlcgen.RepointFileFilingsParams{Path: &newPath, ID: file.ID})
		if moveErr == nil {
			moveErr = queries.RepointFile(ctx, sqlcgen.RepointFileParams{
				NodeID: newNodeID, Path: newPath, ReportedPathRaw: newRaw, ID: file.ID,
			})
		}
		if moveErr != nil {
			return 0, fmt.Errorf("move file %d to %s: %w", file.ID, newPath, moveErr)
		}
		moved++
	}
	folders, err := queries.ListLiveRootPathsOnNode(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("list node %d's library folders: %w", nodeID, err)
	}
	if slices.Contains(folders, repoint.Path) {
		return moved, nil // another library's folder is at it: still a scan's root
	}
	return moved, forgetRootState(ctx, queries, nodeID, repoint.Path)
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
