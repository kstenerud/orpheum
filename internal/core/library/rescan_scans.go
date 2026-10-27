// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"path"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// What a rescan asks of the scans (D357 §2): its scope's files scanned as a manual scan scans them,
// and whether that scan is over.

// scanWorkFilesAtMost bounds the files ScanWork reads of a film or series to find the folders that
// hold them.
const scanWorkFilesAtMost = 10_000

// ScanWork asks, at a person's priority, for a scan of the folders that hold the live files of a
// film or a series in a library: under each of its roots that holds any, the one folder holding
// all of them there, whose scan removes nothing outside it (D272 §9), as a rescan of the one item
// scans its files (D357 §2). A root whose node is away is not asked. It says how many it asked.
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
	asked := 0
	for _, root := range roots {
		scope, holdsAny := "", false
		for _, file := range files {
			relative, under := RelativeToRoot([]byte(file.Path), root.RootPath)
			if file.NodeID != root.NodeID || !under {
				continue
			}
			folder := path.Dir(PathText(relative))
			if holdsAny {
				scope = widerScope(scope, folder)
			} else {
				scope, holdsAny = cleanScope(folder), true
			}
		}
		nodeID, _, connected := admin.nodes.ConnectedNodeRoots(ctx, root.NodeName)
		if !holdsAny || !connected || nodeID != root.NodeID {
			continue
		}
		err := admin.scans.RequestUnder(ctx, nodeID, root.RootPath, scope, jobs.PriorityPersonsRequest)
		if err != nil {
			return asked, err
		}
		asked++
	}
	return asked, nil
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
