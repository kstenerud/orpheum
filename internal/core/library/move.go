// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

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
	// ErrRootCovered is a path another library, or this one, already covers on the node.
	ErrRootCovered = errors.New("a library already covers that path on the node")
)

// RetireFile is the admin saying one file that cannot be read now is gone for good (D241 §5): it
// becomes a tombstone, its history staying on its title (D173), and its work, and that of a title
// left with no live file, is cancelled. A file that can be read is refused (ErrFileReadable).
func (admin *Admin) RetireFile(ctx context.Context, fileID int64) (Retired, error) {
	queries := admin.database.Read()
	file, err := store.OrNotFound(queries.GetPlayableFile(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return Retired{}, fmt.Errorf("file %d: %w", fileID, ErrNoFile)
	}
	if err != nil {
		return Retired{}, fmt.Errorf("read file %d: %w", fileID, err)
	}
	_, isUnreadable, err := UnreadableFile(ctx, queries, admin.nodes, file.NodeID, file.ReportedPathRaw)
	if err != nil {
		return Retired{}, err
	}
	if !isUnreadable {
		return Retired{}, fmt.Errorf("file %d: %w", fileID, ErrFileReadable)
	}
	return admin.retire(ctx, func(retire *retiring) error {
		tombstone := sqlcgen.TombstoneFileParams{RemovedAt: &retire.nowMs, ID: fileID}
		if tombstoneErr := retire.queries.TombstoneFile(ctx, tombstone); tombstoneErr != nil {
			return fmt.Errorf("tombstone file %d: %w", fileID, tombstoneErr)
		}
		retire.fileIDs = append(retire.fileIDs, fileID)
		retire.libraryIDs = append(retire.libraryIDs, file.LibraryID)
		return retire.titlesLeftWithoutFiles(ctx)
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
// removal under D208. The new path must be one the new node names, and no library may cover it.
func (admin *Admin) RepointRoot(ctx context.Context, repoint Repoint) (Repointed, error) {
	root, err := admin.findRoot(ctx, repoint.LibraryID, repoint.NodeName, repoint.Path)
	if err != nil {
		return Repointed{}, err
	}
	newNodeID, roots, connected := admin.nodes.ConnectedNodeRoots(ctx, repoint.NewNodeName)
	if !connected {
		return Repointed{}, fmt.Errorf("node %q: %w", repoint.NewNodeName, ErrNodeNotConnected)
	}
	if !slices.Contains(roots, repoint.NewPath) {
		return Repointed{}, fmt.Errorf("%s on node %q: %w; it names %v", repoint.NewPath, repoint.NewNodeName,
			ErrNotANodeRoot, roots)
	}
	_, err = store.OrNotFound(admin.database.Read().GetLibraryForRoot(ctx, sqlcgen.GetLibraryForRootParams{
		NodeID: newNodeID, RootPath: repoint.NewPath,
	}))
	switch {
	case err == nil:
		return Repointed{}, fmt.Errorf("%s on node %q: %w", repoint.NewPath, repoint.NewNodeName, ErrRootCovered)
	case !errors.Is(err, store.ErrNotFound):
		return Repointed{}, fmt.Errorf("read the library over %s: %w", repoint.NewPath, err)
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

// moveRoot moves a library root, its live files and its state to the new place, in one
// transaction, and returns how many files moved.
func moveRoot(ctx context.Context, queries *sqlcgen.Queries, repoint Repoint, nodeID, newNodeID int64) (int, error) {
	rootPaths, err := queries.ListLibraryRootsOnNode(ctx, sqlcgen.ListLibraryRootsOnNodeParams{
		LibraryID: repoint.LibraryID, NodeID: nodeID,
	})
	if err != nil {
		return 0, fmt.Errorf("list library %d's roots: %w", repoint.LibraryID, err)
	}
	files, err := queries.ListLiveFilesOfLibraryOnNode(ctx, sqlcgen.ListLiveFilesOfLibraryOnNodeParams{
		NodeID: nodeID, LibraryID: repoint.LibraryID,
	})
	if err != nil {
		return 0, fmt.Errorf("list the files under %s: %w", repoint.Path, err)
	}
	moved := 0
	for _, file := range files {
		holding, relative, found := RootHolding(file.ReportedPathRaw, rootPaths)
		if !found || holding != repoint.Path {
			continue
		}
		newRaw := bytes.Join([][]byte{[]byte(repoint.NewPath), relative}, []byte("/"))
		newPath := PathText(newRaw)
		moveErr := queries.RepointFile(ctx, sqlcgen.RepointFileParams{
			NodeID: newNodeID, Path: newPath, ReportedPathRaw: newRaw, ID: file.ID,
		})
		if moveErr != nil {
			return 0, fmt.Errorf("move file %d to %s: %w", file.ID, newPath, moveErr)
		}
		moved++
	}
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
	place := sqlcgen.ForgetRootStateParams{NodeID: nodeID, RootPath: repoint.Path}
	if err := queries.ForgetRootState(ctx, place); err != nil {
		return 0, fmt.Errorf("forget %s's state: %w", repoint.Path, err)
	}
	if err := queries.ForgetRootMounts(ctx, sqlcgen.ForgetRootMountsParams(place)); err != nil {
		return 0, fmt.Errorf("forget %s's mounts: %w", repoint.Path, err)
	}
	if err := queries.ForgetRootScan(ctx, sqlcgen.ForgetRootScanParams(place)); err != nil {
		return 0, fmt.Errorf("forget %s's last scan: %w", repoint.Path, err)
	}
	return moved, nil
}
