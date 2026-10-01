// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// DeletedLibraryTitles hides a deleted library's films, series and episodes, returning their
// ids: identification's, which writes the titles.
type DeletedLibraryTitles interface {
	HideTitlesOfDeletedLibrary(ctx context.Context, libraryID int64) ([]int64, error)
}

// Retirement is what a retire asks of the packages that own what it does not: the job queue,
// which cancels the work, and identification, which hides a deleted library's titles.
type Retirement struct {
	Work   RetiredWorkCanceller
	Titles DeletedLibraryTitles
}

// Retired is what a retire action did: how many live files became tombstones, and how many jobs
// of theirs, and of their titles, were cancelled.
type Retired struct {
	TombstonedFileCount int
	CancelledJobCount   int
}

// retiring gathers, inside a retire's transaction, what it took out of the library.
type retiring struct {
	queries *sqlcgen.Queries
	nowMs   int64
	fileIDs []int64
	itemIDs []int64
	// libraryIDs are the libraries whose titles may have lost their last live file.
	libraryIDs   []int64
	destinations []jobs.Destination
}

// RetireRoot is the admin saying a root of a library is gone for good (D231 §7, S22): it is
// scanned no more, its files and those on the disks mounted inside it become tombstones, their
// history staying on their titles (D173), and the work of those files, and of titles left with
// no live file, is cancelled. Offline never ends without this (D41).
func (admin *Admin) RetireRoot(ctx context.Context, libraryID int64, nodeName, path string) (Retired, error) {
	root, err := admin.findRoot(ctx, libraryID, nodeName, path)
	if err != nil {
		return Retired{}, err
	}
	return admin.retire(ctx, func(retire *retiring) error {
		if rootErr := retire.root(ctx, libraryID, root.NodeID, path); rootErr != nil {
			return rootErr
		}
		return retire.titlesLeftWithoutFiles(ctx)
	})
}

// RetireNode is the admin saying a node is gone for good (D231 §7, S23): its roots, in every
// library, are retired, its work and that of its prepared directory cancelled, and the node
// service removes it, so its name, should it say hello again, enrols a new node.
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
		retire.destinations = append(retire.destinations,
			jobs.NodeDestination(node.ID), jobs.PreparedDirectoryDestination(node.ID))
		return retire.titlesLeftWithoutFiles(ctx)
	})
	if err != nil {
		return Retired{}, err
	}
	if err := admin.nodes.RemoveNode(ctx, node.ID); err != nil {
		return Retired{}, err
	}
	return retired, nil
}

// Delete is the admin deleting a library (D231 §7, S24): its films, series and episodes are
// hidden with their history, so a film of it that reappears in another library brings its title
// back (D208 §2); its roots are retired and its files become tombstones. Each person's watch
// history stays. Its work is cancelled.
func (admin *Admin) Delete(ctx context.Context, libraryID int64) (Retired, error) {
	if _, err := admin.libraryRoots(ctx, libraryID); err != nil {
		return Retired{}, err
	}
	hiddenItemIDs, err := admin.retirement.Titles.HideTitlesOfDeletedLibrary(ctx, libraryID)
	if err != nil {
		return Retired{}, err
	}
	return admin.retire(ctx, func(retire *retiring) error {
		retire.itemIDs = append(retire.itemIDs, hiddenItemIDs...)
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
			if forgetErr := retire.forgetRootAndMounts(ctx, root.NodeID, root.RootPath); forgetErr != nil {
				return forgetErr
			}
		}
		fileIDs, tombstoneErr := retire.queries.TombstoneLiveFilesOfLibrary(ctx,
			sqlcgen.TombstoneLiveFilesOfLibraryParams{RemovedAt: &retire.nowMs, LibraryID: libraryID})
		if tombstoneErr != nil {
			return fmt.Errorf("tombstone library %d's files: %w", libraryID, tombstoneErr)
		}
		retire.fileIDs = append(retire.fileIDs, fileIDs...)
		return nil
	})
}

// retire runs a retire's changes in one transaction, then cancels the work of what it took out.
func (admin *Admin) retire(ctx context.Context, change func(retire *retiring) error) (Retired, error) {
	var work jobs.RetiredWork
	tombstonedFileCount := 0
	err := admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		retire := &retiring{queries: queries, nowMs: admin.clock.Now().UnixMilli()}
		if changeErr := change(retire); changeErr != nil {
			return changeErr
		}
		var workErr error
		work, workErr = retire.work(ctx)
		tombstonedFileCount = len(retire.fileIDs)
		return workErr
	})
	if err != nil {
		return Retired{}, err
	}
	cancelled, err := admin.retirement.Work.CancelRetired(ctx, work)
	if err != nil {
		return Retired{}, fmt.Errorf("cancel the retired work: %w", err)
	}
	return Retired{TombstonedFileCount: tombstonedFileCount, CancelledJobCount: cancelled}, nil
}

// root retires one root of a library on a node and tombstones the live files it holds: those
// under it, and not under a deeper root of the library (RootHolding).
func (retire *retiring) root(ctx context.Context, libraryID, nodeID int64, rootPath string) error {
	rootPaths, err := retire.queries.ListLibraryRootsOnNode(ctx, sqlcgen.ListLibraryRootsOnNodeParams{
		LibraryID: libraryID, NodeID: nodeID,
	})
	if err != nil {
		return fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	files, err := retire.queries.ListLiveFilesOfLibraryOnNode(ctx, sqlcgen.ListLiveFilesOfLibraryOnNodeParams{
		NodeID: nodeID, LibraryID: libraryID,
	})
	if err != nil {
		return fmt.Errorf("list the files under %s: %w", rootPath, err)
	}
	for _, file := range files {
		if holding, _, found := RootHolding(file.ReportedPathRaw, rootPaths); !found || holding != rootPath {
			continue
		}
		tombstone := sqlcgen.TombstoneFileParams{RemovedAt: &retire.nowMs, ID: file.ID}
		if tombstoneErr := retire.queries.TombstoneFile(ctx, tombstone); tombstoneErr != nil {
			return fmt.Errorf("tombstone file %d: %w", file.ID, tombstoneErr)
		}
		retire.fileIDs = append(retire.fileIDs, file.ID)
	}
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
	return retire.forgetRootAndMounts(ctx, nodeID, rootPath)
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

// titlesLeftWithoutFiles adds the titles of the retired roots' libraries that no live file is
// left to: their fetches and artwork go with their files.
func (retire *retiring) titlesLeftWithoutFiles(ctx context.Context) error {
	for _, libraryID := range retire.libraryIDs {
		titleIDs, err := retire.queries.ListTitlesWithoutLiveFiles(ctx, &libraryID)
		if err != nil {
			return fmt.Errorf("list library %d's titles with no live file: %w", libraryID, err)
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

// Offline is a root, or a disk mounted inside one, that scans have found missing, and since the
// first that did: offline never ends without an admin, who retires it, or a scan that finds it
// (D41, D231 §7).
type Offline struct {
	LibraryID   int64
	LibraryName string
	NodeName    string
	RootPath    string
	// MountPath is the disk's mount point inside the root; empty when the root itself is offline.
	MountPath string
	Since     time.Time
}

// Offline is every offline root and disk of the live libraries, the longest offline first.
func (admin *Admin) Offline(ctx context.Context) ([]Offline, error) {
	queries := admin.database.Read()
	roots, err := queries.ListOfflineRoots(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the offline roots: %w", err)
	}
	disks, err := queries.ListOfflineDisks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the offline disks: %w", err)
	}
	offline := make([]Offline, 0, len(roots)+len(disks))
	for _, root := range roots {
		offline = append(offline, Offline{
			LibraryID: root.LibraryID, LibraryName: root.LibraryName, NodeName: root.NodeName, RootPath: root.RootPath,
			Since: time.UnixMilli(root.UnavailableSince),
		})
	}
	for _, disk := range disks {
		offline = append(offline, Offline{
			LibraryID: disk.LibraryID, LibraryName: disk.LibraryName, NodeName: disk.NodeName, RootPath: disk.RootPath,
			MountPath: disk.MountPath, Since: time.UnixMilli(disk.MissingSince),
		})
	}
	slices.SortStableFunc(offline, func(one, other Offline) int { return one.Since.Compare(other.Since) })
	return offline, nil
}
