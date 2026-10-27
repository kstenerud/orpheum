// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoNode is a node that is not enrolled, or was retired.
var ErrNoNode = errors.New("no such node")

// RetiredWorkCanceller cancels the work of what a retire took out of the library and forgets its
// destinations: the job queue (D231 §7, §9).
type RetiredWorkCanceller interface {
	CancelRetired(ctx context.Context, work jobs.RetiredWork) (int, error)
}

// Retirement is what a retire asks of the packages that own what it does not: the job queue, which
// cancels the work.
type Retirement struct {
	Work RetiredWorkCanceller
}

// Retired is what a retire action did: how many live places became tombstones, how many no library
// covers any more, kept with their history (D363 §10), and how many jobs of theirs, and of their
// titles, were cancelled.
type Retired struct {
	TombstonedFileCount int
	UncoveredFileCount  int
	CancelledJobCount   int
}

// retiring gathers, inside a retire's transaction, what it took out of the library.
type retiring struct {
	queries *sqlcgen.Queries
	logger  *slog.Logger
	nowMs   int64
	// fileIDs are the places whose work goes: tombstoned, or no library's any more.
	fileIDs         []int64
	tombstonedCount int
	uncoveredCount  int
	itemIDs         []int64
	// libraryIDs are the libraries whose titles may be listed by no library any more.
	libraryIDs   []int64
	destinations []jobs.Destination
}

// RetireRoot is the admin taking a folder out of a library, one that is gone for good among them (D231
// §7, S22): the library scans it no more. A place under it that no other library's folder covers is
// kept, live, with its history, its data reapable as the budgets need room, and found as it was by a
// library that covers it again (D363 §10); its work, and that of titles no library lists any more, is
// cancelled. Offline never ends without this, or a scan that finds it (D41).
func (admin *Admin) RetireRoot(ctx context.Context, libraryID int64, nodeName, path string) (Retired, error) {
	root, err := admin.findRoot(ctx, libraryID, nodeName, path)
	if err != nil {
		return Retired{}, err
	}
	return admin.retire(ctx, func(retire *retiring) error {
		if rootErr := retire.root(ctx, libraryID, root.NodeID, path); rootErr != nil {
			return rootErr
		}
		return retire.titlesNoLibraryLists(ctx)
	})
}

// RetireNode is the admin saying a node is gone for good (D231 §7, S23): its folders, in every
// library, are retired, every place on it is tombstoned, a file found first there moving to its copy
// elsewhere (D363 §2), its work and that of its prepared directory cancelled, and the node service
// removes it, so its name, should it say hello again, enrols a new node.
func (admin *Admin) RetireNode(ctx context.Context, nodeName string) (Retired, error) {
	node, err := store.OrNotFound(admin.database.Read().GetLiveNodeByName(ctx, nodeName))
	if errors.Is(err, store.ErrNotFound) {
		return Retired{}, fmt.Errorf("node %q: %w", nodeName, ErrNoNode)
	}
	if err != nil {
		return Retired{}, fmt.Errorf("read node %q: %w", nodeName, err)
	}
	retired, err := admin.retire(ctx, func(retire *retiring) error {
		roots, listErr := retire.queries.ListLiveRootsOfNode(ctx, node.ID)
		if listErr != nil {
			return fmt.Errorf("list node %q's roots: %w", nodeName, listErr)
		}
		for _, root := range roots {
			if rootErr := retire.root(ctx, root.LibraryID, node.ID, root.RootPath); rootErr != nil {
				return rootErr
			}
		}
		if placesErr := retire.placesOfNode(ctx, node.ID); placesErr != nil {
			return placesErr
		}
		retire.destinations = append(retire.destinations,
			jobs.NodeDestination(node.ID), jobs.PreparedDirectoryDestination(node.ID))
		return retire.titlesNoLibraryLists(ctx)
	})
	if err != nil {
		return Retired{}, err
	}
	if err := admin.nodes.RemoveNode(ctx, node.ID); err != nil {
		return Retired{}, err
	}
	return retired, nil
}

// Delete is the admin deleting a library (D231 §7, S24): its folders are retired as RetireRoot retires
// one, and it is gone. It deletes no item and no file: what no other library covers is kept, with every
// person's progress, its data reapable as the budgets need room, and a library that covers it again
// finds it as it was (D363 §10). The work of what no library covers is cancelled.
func (admin *Admin) Delete(ctx context.Context, libraryID int64) (Retired, error) {
	if _, err := admin.libraryRoots(ctx, libraryID); err != nil {
		return Retired{}, err
	}
	return admin.retire(ctx, func(retire *retiring) error {
		removed, removeErr := retire.queries.RemoveLibrary(ctx, sqlcgen.RemoveLibraryParams{
			RemovedAt: &retire.nowMs, ID: libraryID,
		})
		if removeErr != nil {
			return fmt.Errorf("remove library %d: %w", libraryID, removeErr)
		}
		if removed == 0 {
			return fmt.Errorf("library %d: %w", libraryID, ErrNoLibrary)
		}
		roots, rootsErr := retire.queries.RetireRootsOfLibrary(ctx, sqlcgen.RetireRootsOfLibraryParams{
			RetiredAt: &retire.nowMs, LibraryID: libraryID,
		})
		if rootsErr != nil {
			return fmt.Errorf("retire library %d's roots: %w", libraryID, rootsErr)
		}
		for _, root := range roots {
			if leftErr := retire.leftUncovered(ctx, root.NodeID, root.RootPath); leftErr != nil {
				return leftErr
			}
		}
		retire.libraryIDs = append(retire.libraryIDs, libraryID)
		return retire.titlesNoLibraryLists(ctx)
	})
}

// retire runs a retire's changes in one transaction, then cancels the work of what it took out.
func (admin *Admin) retire(ctx context.Context, change func(retire *retiring) error) (Retired, error) {
	var work jobs.RetiredWork
	var done retiring
	err := admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		retire := &retiring{queries: queries, logger: admin.log(), nowMs: admin.clock.Now().UnixMilli()}
		if changeErr := change(retire); changeErr != nil {
			return changeErr
		}
		var workErr error
		work, workErr = retire.work(ctx)
		done = *retire
		return workErr
	})
	if err != nil {
		return Retired{}, err
	}
	admin.watchedFoldersChanged() // a retired folder is watched no more
	cancelled, err := admin.retirement.Work.CancelRetired(ctx, work)
	if err != nil {
		return Retired{}, fmt.Errorf("cancel the retired work: %w", err)
	}
	return Retired{
		TombstonedFileCount: done.tombstonedCount, UncoveredFileCount: done.uncoveredCount,
		CancelledJobCount: cancelled,
	}, nil
}

// root retires one folder of a library on a node: what no library covers any more under it is left
// to no one (leftUncovered).
func (retire *retiring) root(ctx context.Context, libraryID, nodeID int64, rootPath string) error {
	retiredCount, err := retire.queries.RetireLibraryRoot(ctx, sqlcgen.RetireLibraryRootParams{
		RetiredAt: &retire.nowMs, LibraryID: libraryID, NodeID: nodeID, RootPath: rootPath,
	})
	if err != nil {
		return fmt.Errorf("retire %s: %w", rootPath, err)
	}
	if retiredCount == 0 {
		return fmt.Errorf("%s: %w", rootPath, ErrNoSuchRoot)
	}
	retire.libraryIDs = append(retire.libraryIDs, libraryID)
	return retire.leftUncovered(ctx, nodeID, rootPath)
}

// leftUncovered counts the places under a retired folder that no live library's live folder covers
// any more, kept with their history, and adds them to those whose work goes (D363 §10). When no live
// folder is at its path any more, it is no scan's root: its state, its disks and its places'
// availability are forgotten, and their destinations with them.
func (retire *retiring) leftUncovered(ctx context.Context, nodeID int64, rootPath string) error {
	after, before := PathRange(rootPath)
	uncovered, err := retire.queries.ListUncoveredFilesUnder(ctx, sqlcgen.ListUncoveredFilesUnderParams{
		NodeID: nodeID, After: after, Before: before,
	})
	if err != nil {
		return fmt.Errorf("list the places under %s no library covers: %w", rootPath, err)
	}
	retire.fileIDs = append(retire.fileIDs, uncovered...)
	retire.uncoveredCount += len(uncovered)
	if len(uncovered) > 0 {
		retire.logger.InfoContext(ctx, "a library's folder was retired; what no library covers any more is kept",
			"nodeId", nodeID, "rootPath", rootPath, "uncoveredFileCount", len(uncovered))
	}
	folders, err := retire.queries.ListLiveRootPathsOnNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list node %d's library folders: %w", nodeID, err)
	}
	if slices.Contains(folders, rootPath) {
		return nil // another library's folder is at it: still a scan's root
	}
	return retire.forgetScanRoot(ctx, nodeID, rootPath)
}

// forgetScanRoot forgets a folder that is no scan's root any more: its destinations, and those of the
// disks mounted inside it, its state, its disks and its places' availability.
func (retire *retiring) forgetScanRoot(ctx context.Context, nodeID int64, rootPath string) error {
	if err := retire.forgetRootAndMounts(ctx, nodeID, rootPath); err != nil {
		return err
	}
	return forgetRootState(ctx, retire.queries, nodeID, rootPath)
}

// placesOfNode tombstones every live place on a node that is gone: a file found first there moves to
// a copy of it elsewhere, when it has one (removePlace).
func (retire *retiring) placesOfNode(ctx context.Context, nodeID int64) error {
	places, err := retire.queries.ListLiveFilesByNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list node %d's files: %w", nodeID, err)
	}
	for _, isCopies := range []bool{true, false} {
		for _, place := range places {
			if (place.CopyOf != nil) != isCopies {
				continue
			}
			isGone, removeErr := removePlace(ctx, retire.queries, retire.logger, place.ID, retire.nowMs)
			if removeErr != nil {
				return removeErr
			}
			retire.tombstonedCount++
			if isGone {
				retire.fileIDs = append(retire.fileIDs, place.ID)
			}
		}
	}
	return nil
}

// forgetRootAndMounts adds a retired root's destination, and those of the disks mounted inside
// it, to those forgotten.
func (retire *retiring) forgetRootAndMounts(ctx context.Context, nodeID int64, rootPath string) error {
	mountPaths, err := retire.queries.ListRootMountPaths(ctx, sqlcgen.ListRootMountPathsParams{
		NodeID: nodeID, RootPath: rootPath,
	})
	if err != nil {
		return fmt.Errorf("list the disks mounted inside %s: %w", rootPath, err)
	}
	retire.destinations = append(retire.destinations, jobs.RootDestination(nodeID, rootPath))
	for _, mountPath := range mountPaths {
		retire.destinations = append(retire.destinations, jobs.RootDestination(nodeID, mountPath))
	}
	return nil
}

// titlesNoLibraryLists adds the titles the retired folders' libraries filed that no library lists any
// more: their fetches and artwork go with their files' work (D363 §10).
func (retire *retiring) titlesNoLibraryLists(ctx context.Context) error {
	for _, libraryID := range retire.libraryIDs {
		titleIDs, err := retire.queries.ListTitlesNoLibraryLists(ctx, &libraryID)
		if err != nil {
			return fmt.Errorf("list library %d's titles no library lists: %w", libraryID, err)
		}
		retire.itemIDs = append(retire.itemIDs, titleIDs...)
	}
	return nil
}

// work is the work of what the retire took out: its titles, files and their streams and fetched
// subtitles, and its destinations.
func (retire *retiring) work(ctx context.Context) (jobs.RetiredWork, error) {
	fileIDList, err := json.Marshal(retire.fileIDs)
	if err != nil {
		return jobs.RetiredWork{}, fmt.Errorf("list the retired files: %w", err)
	}
	streamIDs, err := retire.queries.ListStreamIDsOfFiles(ctx, string(fileIDList))
	if err != nil {
		return jobs.RetiredWork{}, fmt.Errorf("list the retired files' streams: %w", err)
	}
	fetchedSubtitleIDs, err := retire.queries.ListFetchedSubtitleIDsOfFiles(ctx, string(fileIDList))
	if err != nil {
		return jobs.RetiredWork{}, fmt.Errorf("list the retired files' fetched subtitles: %w", err)
	}
	return jobs.RetiredWork{
		ItemIDs: retire.itemIDs, FileIDs: retire.fileIDs, StreamIDs: streamIDs,
		FetchedSubtitleIDs: fetchedSubtitleIDs, Destinations: retire.destinations,
	}, nil
}

// Offline is a place, a root or a disk mounted inside one, that cannot be read now, and since the
// first time in a row it was found so: offline ends when an admin retires it, or it comes back (D41,
// D231 §7, D328).
type Offline struct {
	LibraryID   int64
	LibraryName string
	NodeName    string
	RootPath    string
	// MountPath is the disk's mount point inside the root; empty when the root itself is offline.
	MountPath string
	Since     time.Time
}

// Offline is every offline place of the live libraries, the longest offline first.
func (admin *Admin) Offline(ctx context.Context) ([]Offline, error) {
	places, err := admin.database.Read().ListOfflinePlaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the offline places: %w", err)
	}
	offline := make([]Offline, 0, len(places))
	for _, place := range places {
		var mountPath string
		if place.Path != place.RootPath {
			mountPath = place.Path
		}
		offline = append(offline, Offline{
			LibraryID: place.LibraryID, LibraryName: place.LibraryName, NodeName: place.NodeName,
			RootPath: place.RootPath, MountPath: mountPath, Since: time.UnixMilli(place.Since),
		})
	}
	return offline, nil
}
