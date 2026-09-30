// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Outcomes of a root's scan that did not complete, as root_state keeps them; RecordRootComplete
// writes the third, "complete".
const (
	outcomeUnreadable = "unreadable"
	outcomeWithheld   = "withheld"
)

// completeScan ends a scan: it concludes the moves it held, then either tombstones what the scan did
// not see — outside the regions it could not read — or withholds that and asks, when the removal
// looks like a lost disk rather than deleted files (D41, D208 §3). Tombstones, root_scan and
// root_state are written in this batch's transaction, together.
func (batch *ingestBatch) completeScan(ctx context.Context, scan *scanState, complete *nodeproto.ScanComplete) error {
	delete(batch.ingester.scanByNodeID, batch.nodeID)
	if complete.RootPath != scan.root {
		return fmt.Errorf("scan_complete names %s, but the scan in progress is of %s", complete.RootPath, scan.root)
	}
	if err := batch.concludeMoves(ctx, scan); err != nil {
		return err
	}
	batch.endedLibraryIDs = append(batch.endedLibraryIDs, scan.libraryID)
	if complete.IsStoppedByErrors || scan.isEverythingUnread {
		batch.ingester.logger.WarnContext(ctx, "a scan could not read its whole root; nothing was removed",
			"nodeId", batch.nodeID, "rootPath", scan.root, "errorCount", complete.ErrorCount)
		return batch.recordNotComplete(ctx, scan, outcomeUnreadable, "")
	}
	missing, err := batch.missingMounts(ctx, scan)
	if err != nil {
		return err
	}
	unseenFileIDs, liveFileCount, err := batch.unseenUnder(ctx, scan)
	if err != nil {
		return err
	}
	reason, err := batch.withholdReason(ctx, scan, len(unseenFileIDs), liveFileCount)
	if err != nil {
		return err
	}
	if reason != "" {
		batch.ingester.logger.WarnContext(ctx, "a scan's removals wait for the admin", "nodeId", batch.nodeID,
			"rootPath", scan.root, "wouldRemove", len(unseenFileIDs), "rootFileCount", liveFileCount, "reason", reason)
		return batch.recordNotComplete(ctx, scan, outcomeWithheld, reason)
	}
	for _, gone := range unseenFileIDs {
		tombstone := sqlcgen.TombstoneFileParams{RemovedAt: &batch.nowMs, ID: gone}
		if tombstoneErr := batch.queries.TombstoneFile(ctx, tombstone); tombstoneErr != nil {
			return fmt.Errorf("tombstone file %d: %w", gone, tombstoneErr)
		}
	}
	if mountsErr := batch.recordMounts(ctx, scan, missing); mountsErr != nil {
		return mountsErr
	}
	if attachErr := batch.attachSidecars(ctx, scan); attachErr != nil {
		return attachErr
	}
	err = batch.queries.UpsertRootScan(ctx, sqlcgen.UpsertRootScanParams{
		NodeID: batch.nodeID, RootPath: scan.root, CompletedAt: batch.nowMs,
	})
	if err != nil {
		return fmt.Errorf("record the scan of %s as complete: %w", scan.root, err)
	}
	device := int64(scan.facts.FilesystemDevice) //nolint:gosec // a device id fits
	mountPoint := boolInt(scan.facts.IsMountPoint)
	err = batch.queries.RecordRootComplete(ctx, sqlcgen.RecordRootCompleteParams{
		NodeID: batch.nodeID, RootPath: scan.root, FilesystemDevice: &device, IsMountPoint: &mountPoint,
		LastScanAt: batch.nowMs,
	})
	if err != nil {
		return fmt.Errorf("record %s's facts: %w", scan.root, err)
	}
	batch.foundPaths = append(batch.foundPaths, scan.root)
	return nil
}

// missingMounts compares the mount boundaries this scan crossed with those the root's last
// complete scan recorded. A boundary not seen where it was, or seen on another device, is a disk
// that is not there — an unplugged disk under /mnt lists as an empty directory — so nothing
// under it may be removed: it joins the regions the scan could not read (D208).
func (batch *ingestBatch) missingMounts(ctx context.Context, scan *scanState) (map[string]bool, error) {
	known, err := batch.queries.ListRootMounts(ctx, sqlcgen.ListRootMountsParams{
		NodeID:   batch.nodeID,
		RootPath: scan.root,
	})
	if err != nil {
		return nil, fmt.Errorf("list the mounts under %s: %w", scan.root, err)
	}
	missing := map[string]bool{}
	for _, mount := range known {
		device, wasSeen := scan.mounts[mount.MountPath]
		if wasSeen && int64(device) == mount.FilesystemDevice { //nolint:gosec // a device id fits
			continue
		}
		missing[mount.MountPath] = true
		scan.unreadPaths = append(scan.unreadPaths, mount.MountPath)
		batch.ingester.logger.WarnContext(ctx,
			"a disk mounted inside a root is not there; nothing under it was removed",
			"nodeId", batch.nodeID, "rootPath", scan.root, "mountPath", mount.MountPath)
	}
	return missing, nil
}

// recordMounts keeps a complete scan's mount boundaries as the known-good ones, and marks those
// it did not find as missing, keeping the device they were on.
func (batch *ingestBatch) recordMounts(ctx context.Context, scan *scanState, missing map[string]bool) error {
	for mountPath := range missing {
		err := batch.queries.RecordRootMountMissing(ctx, sqlcgen.RecordRootMountMissingParams{
			NodeID: batch.nodeID, MountPath: mountPath,
		})
		if err != nil {
			return fmt.Errorf("record %s as missing: %w", mountPath, err)
		}
	}
	for mountPath, device := range scan.mounts {
		if missing[mountPath] {
			continue // on another device than it was: not the disk we knew
		}
		err := batch.queries.RecordRootMountPresent(ctx, sqlcgen.RecordRootMountPresentParams{
			NodeID: batch.nodeID, RootPath: scan.root, MountPath: mountPath,
			FilesystemDevice: int64(device), //nolint:gosec // fits
		})
		if err != nil {
			return fmt.Errorf("record the mount %s: %w", mountPath, err)
		}
		batch.foundPaths = append(batch.foundPaths, mountPath)
	}
	return nil
}

// unseenUnder returns the live files under the root that the scan did not see, outside what it
// could not read, and how many live files the root holds.
func (batch *ingestBatch) unseenUnder(ctx context.Context, scan *scanState) ([]int64, int, error) {
	files, err := batch.queries.ListLiveFilesOfLibraryOnNode(ctx, sqlcgen.ListLiveFilesOfLibraryOnNodeParams{
		NodeID: batch.nodeID, LibraryID: scan.libraryID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list the files under %s: %w", scan.root, err)
	}
	var unseenFileIDs []int64
	liveFileCount := 0
	for _, file := range files {
		if _, under := RelativeToRoot(file.ReportedPathRaw, scan.root); !under {
			continue
		}
		liveFileCount++
		if file.LastSeenAt < scan.startedAt && !scan.isUnread(file.ReportedPathRaw) {
			unseenFileIDs = append(unseenFileIDs, file.ID)
		}
	}
	return unseenFileIDs, liveFileCount, nil
}

// changedFactsReason says how a root's device or mount differs from its last complete scan's, if it
// does: an unmounted disk leaves its empty mount point behind (D41).
func changedFactsReason(prior sqlcgen.GetRootStateRow, scan *scanState) string {
	switch {
	//nolint:gosec // a device id fits
	case prior.FilesystemDevice != nil && *prior.FilesystemDevice != int64(scan.facts.FilesystemDevice):
		return "the root is on another device than at its last complete scan: an unmounted disk?"
	case prior.IsMountPoint != nil && *prior.IsMountPoint != boolInt(scan.facts.IsMountPoint):
		return "the root was a mount point and is not now, or the other way round"
	}
	return ""
}

// withholdReason says why a scan's removals should wait for the admin, or "" when they should
// not: the root's device or mount changed since its last complete scan, or it would lose all of
// its files, or more than scan_withhold_removals_over_percent of them and more than
// scan_withhold_removals_over_files (D208 §3) — unless the admin approved this root's removals.
func (batch *ingestBatch) withholdReason(ctx context.Context, scan *scanState, removing, total int) (string, error) {
	prior, err := store.OrNotFound(batch.queries.GetRootState(ctx, sqlcgen.GetRootStateParams{
		NodeID: batch.nodeID, RootPath: scan.root,
	}))
	switch {
	case err == nil && prior.AreRemovalsApproved == 1:
		return "", nil // the admin said the files are gone
	case err == nil:
		if changed := changedFactsReason(prior, scan); changed != "" {
			return changed, nil
		}
	case !errors.Is(err, store.ErrNotFound):
		return "", err
	}
	if removing == 0 {
		return "", nil
	}
	if removing == total {
		return fmt.Sprintf("the scan saw none of the root's %d files", total), nil
	}
	overPercent, err := batch.ingester.settings.Int(ctx, settings.ScanWithholdRemovalsOverPercent)
	if err != nil {
		return "", fmt.Errorf("read scan_withhold_removals_over_percent: %w", err)
	}
	overFilesThreshold, err := batch.ingester.settings.Int(ctx, settings.ScanWithholdRemovalsOverFiles)
	if err != nil {
		return "", fmt.Errorf("read scan_withhold_removals_over_files: %w", err)
	}
	if int64(removing)*100 > overPercent*int64(total) && int64(removing) > overFilesThreshold {
		return fmt.Sprintf("the scan would remove %d of the root's %d files", removing, total), nil
	}
	return "", nil
}

// recordNotComplete records a scan that could not read its root, or whose removals wait.
func (batch *ingestBatch) recordNotComplete(ctx context.Context, scan *scanState, outcome, reason string) error {
	var withheldSince *int64
	var withheldReason *string
	if outcome == outcomeWithheld {
		withheldSince, withheldReason = &scan.startedAt, &reason
	}
	err := batch.queries.RecordRootNotComplete(ctx, sqlcgen.RecordRootNotCompleteParams{
		NodeID: batch.nodeID, RootPath: scan.root, LastScanOutcome: outcome, LastScanAt: batch.nowMs,
		WithheldScanStartedAt: withheldSince, WithheldReason: withheldReason,
	})
	if err != nil {
		return fmt.Errorf("record the scan of %s as %s: %w", scan.root, outcome, err)
	}
	return nil
}

// concludeMoves settles each held path: if the file whose payload it carries was not seen
// anywhere else in the scan, the file moved, and its row — with its history — follows it; if it
// was, the new path is a duplicate with a row of its own (D179).
func (batch *ingestBatch) concludeMoves(ctx context.Context, scan *scanState) error {
	for _, move := range scan.pendingMoves {
		candidates, err := batch.queries.ListLiveFilesByPayloadHash(ctx, sqlcgen.ListLiveFilesByPayloadHashParams{
			NodeID: batch.nodeID, PayloadHash: move.seenFile.hash,
		})
		if err != nil {
			return fmt.Errorf("look for %s's payload again: %w", move.seenFile.path, err)
		}
		movedFrom := int64(0)
		for _, candidate := range candidates {
			if candidate.ID == move.fromFileID && candidate.LastSeenAt < scan.startedAt {
				movedFrom = candidate.ID
			}
		}
		if movedFrom != 0 {
			err = batch.updateObservedFile(ctx, scan, movedFrom, move.seenFile)
		} else {
			err = batch.insertFile(ctx, scan, move.seenFile)
		}
		if err != nil {
			return err
		}
	}
	scan.pendingMoves = nil
	return nil
}

// isUnread reports whether a file, by the bytes its node reported, lies in a region the scan
// could not read.
func (scan *scanState) isUnread(reportedPath []byte) bool {
	for _, unread := range scan.unreadPaths {
		if isAtOrUnder(reportedPath, unread) {
			return true
		}
	}
	return false
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
