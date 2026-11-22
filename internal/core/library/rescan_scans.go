// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"path"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// What a rescan asks of the scans (D357 §2, D392 §2): its scope's files read again, changed on disk
// or not, and whether that scan is over.

// scanWorkFilesAtMost bounds the files ScanWork reads of an item to find the folders that hold them.
const scanWorkFilesAtMost = 10_000

// ScanWork asks, at a person's priority, for a scan of the folders that hold the live files of a
// film, a series (its every episode's), an episode or an other video in a library: under each of its
// roots that holds any, the one folder holding all of them there, whose scan removes nothing outside
// it (D272 §9) and reads those files again, changed on disk or not, as a rescan of the one item reads
// its files (D392 §2, D535 §3, §5). A root whose node is away is not asked. It says how many it asked.
func (admin *Admin) ScanWork(ctx context.Context, libraryID, itemID int64) (int, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	files, err := admin.database.Read().ListFilesOfWork(ctx, sqlcgen.ListFilesOfWorkParams{
		ItemID: itemID, MaxRows: scanWorkFilesAtMost,
	})
	if err != nil {
		return 0, fmt.Errorf("list item %d's files: %w", itemID, err)
	}
	places := make([]filePlace, 0, len(files))
	for _, file := range files {
		places = append(places, filePlace{nodeID: file.NodeID, path: file.Path})
	}
	asked := 0
	for _, root := range roots {
		scope, readAgain, holdsAny := filesUnder(root.NodeID, root.RootPath, places)
		nodeID, _, connected := admin.nodes.ConnectedNodeRoots(ctx, root.NodeName)
		if !holdsAny || !connected || nodeID != root.NodeID {
			continue
		}
		err := admin.scans.RequestReadingAgain(ctx, nodeID, root.RootPath, scope, readAgain,
			jobs.PriorityPersonsRequest)
		if err != nil {
			return asked, err
		}
		asked++
	}
	return asked, nil
}

// filePlace is a file's place: its node, and its path there.
type filePlace struct {
	nodeID int64
	path   string
}

// filesUnder is the one folder under a node's root holding all the files there of those given, its
// scope, and those files, relative to the root; and whether the root holds any: a rescan's of a film's
// or series' files (ScanWork), or those an older probe read (ReadAgainOlderProbes).
func filesUnder(nodeID int64, rootPath string, files []filePlace) (scope string, readAgain ReadAgain, holdsAny bool) {
	for _, file := range files {
		relative, under := RelativeToRoot([]byte(file.path), rootPath)
		if file.nodeID != nodeID || !under {
			continue
		}
		readAgain = append(readAgain, PathText(relative))
		folder := path.Dir(PathText(relative))
		if holdsAny {
			scope = widerScope(scope, folder)
		} else {
			scope, holdsAny = cleanScope(folder), true
		}
	}
	return scope, readAgain, holdsAny
}

// IsScanning is whether a scan of any of a library's roots is in progress.
func (admin *Admin) IsScanning(ctx context.Context, libraryID int64) (bool, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return false, err
	}
	progress, err := admin.scans.InProgress(ctx)
	if err != nil {
		return false, err
	}
	for _, root := range roots {
		if _, isScanning := progress[ScanRoot{NodeID: root.NodeID, RootPath: root.RootPath}]; isScanning {
			return true, nil
		}
	}
	return false, nil
}
