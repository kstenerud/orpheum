// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// complete ends a scan: it concludes the moves it held, tombstones what the scan did not see
// outside the regions it could not read, and records the root as scanned — all in this batch's
// transaction, so a tombstone and "completed" are written together (schema.md, root_scan).
func (work *batch) complete(ctx context.Context, scan *scanState, complete *nodeproto.ScanComplete) error {
	delete(work.ingester.scans, work.nodeID)
	if complete.Root != scan.root {
		return fmt.Errorf("scan_complete names %s, but the scan in progress is of %s", complete.Root, scan.root)
	}
	if err := work.concludeMoves(ctx, scan); err != nil {
		return err
	}
	if complete.Stopped || scan.unreadAll {
		work.ingester.logger.WarnContext(ctx, "a scan could not read its whole root; nothing was removed",
			"nodeId", work.nodeID, "root", scan.root, "errors", complete.Errors)
		return nil
	}
	unseen, err := work.queries.ListUnseenLiveFiles(ctx, sqlcgen.ListUnseenLiveFilesParams{
		NodeID: work.nodeID, LibraryID: scan.libraryID, LastSeen: scan.startedAt,
	})
	if err != nil {
		return fmt.Errorf("list the files the scan of %s did not see: %w", scan.root, err)
	}
	for _, gone := range unseen {
		if !under(gone.Path, scan.root) || scan.hides(gone.Path) {
			continue
		}
		tombstoneErr := work.queries.TombstoneFile(ctx, sqlcgen.TombstoneFileParams{RemovedAt: &work.now, ID: gone.ID})
		if tombstoneErr != nil {
			return fmt.Errorf("tombstone %s: %w", gone.Path, tombstoneErr)
		}
	}
	err = work.queries.UpsertRootScan(ctx, sqlcgen.UpsertRootScanParams{
		NodeID: work.nodeID, Path: scan.root, CompletedAt: work.now,
	})
	if err != nil {
		return fmt.Errorf("record the scan of %s as complete: %w", scan.root, err)
	}
	return nil
}

// concludeMoves settles each held path: if the file whose payload it carries was not seen
// anywhere else in the scan, the file moved, and its row — with its history — follows it; if it
// was, the new path is a duplicate with a row of its own (D179).
func (work *batch) concludeMoves(ctx context.Context, scan *scanState) error {
	for _, move := range scan.pending {
		candidates, err := work.queries.ListLiveFilesByHash(ctx, sqlcgen.ListLiveFilesByHashParams{
			NodeID: work.nodeID, PayloadHash: move.seen.hash,
		})
		if err != nil {
			return fmt.Errorf("look for %s's payload again: %w", move.seen.path, err)
		}
		movedFrom := int64(0)
		for _, candidate := range candidates {
			if candidate.ID == move.from && candidate.LastSeen < scan.startedAt {
				movedFrom = candidate.ID
			}
		}
		if movedFrom != 0 {
			err = work.update(ctx, scan, movedFrom, move.seen)
		} else {
			err = work.insert(ctx, scan, move.seen)
		}
		if err != nil {
			return err
		}
	}
	scan.pending = nil
	return nil
}

// hides reports whether filePath lies in a region the scan could not read.
func (scan *scanState) hides(filePath string) bool {
	for _, unread := range scan.unread {
		if under(filePath, unread) {
			return true
		}
	}
	return false
}

// under reports whether filePath is directory or below it.
func under(filePath, directory string) bool {
	return filePath == directory || strings.HasPrefix(filePath, strings.TrimSuffix(directory, "/")+"/")
}
