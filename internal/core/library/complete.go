// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"path"

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

// outcomeComplete is a root's last scan that completed, as RecordRootComplete writes it.
const outcomeComplete = "complete"

// completeScan ends a scan: it concludes the moves it held, then either tombstones what the scan did
// not see — outside the regions it could not read — or withholds that and asks, when the removal
// looks like a lost disk rather than deleted files (D41, D208 §3). Tombstones, root_scan and
// root_state are written in this batch's transaction, together.
func (batch *ingestBatch) completeScan(ctx context.Context, scan *scanState, complete *nodeproto.ScanComplete) error {
	delete(batch.ingester.scans, batch.key)
	if complete.RootPath != scan.root {
		return fmt.Errorf("scan_complete names %s, but the scan in progress is of %s", complete.RootPath, scan.root)
	}
	if err := batch.concludeMoves(ctx, scan); err != nil {
		return err
	}
	batch.endedLibraryIDs = append(batch.endedLibraryIDs, scan.libraryID)
	if scan.isScoped() {
		if err := batch.checkScopeMayJudge(ctx, scan, complete); err != nil {
			return err
		}
	}
	if complete.IsStoppedByErrors || scan.isEverythingUnread {
		batch.ingester.logger.WarnContext(ctx, "a scan could not read its whole root; nothing was removed",
			"nodeId", batch.nodeID, "rootPath", scan.root, "errorCount", complete.ErrorCount)
		return batch.recordNotComplete(ctx, scan, outcomeUnreadable, "", true)
	}
	judged, err := batch.judgeRemovals(ctx, scan)
	if err != nil {
		return err
	}
	if judged.withheldReason != "" {
		batch.ingester.logger.WarnContext(ctx, "a scan's removals wait for the admin", "nodeId", batch.nodeID,
			"rootPath", scan.root, "wouldRemove", len(judged.unseenFileIDs), "rootFileCount", judged.liveFileCount,
			"reason", judged.withheldReason)
		// A scan that read the root and only holds back removals leaves it playable; one that saw
		// none of its files is a lost disk (D277 §1).
		sawNone := judged.liveFileCount > 0 && judged.seenFileCount == 0
		return batch.recordNotComplete(ctx, scan, outcomeWithheld, judged.withheldReason, sawNone)
	}
	for _, gone := range judged.unseenFileIDs {
		tombstone := sqlcgen.TombstoneFileParams{RemovedAt: &batch.nowMs, ID: gone}
		if tombstoneErr := batch.queries.TombstoneFile(ctx, tombstone); tombstoneErr != nil {
			return fmt.Errorf("tombstone file %d: %w", gone, tombstoneErr)
		}
	}
	if mountsErr := batch.recordMounts(ctx, scan, judged.missingMounts); mountsErr != nil {
		return mountsErr
	}
	if attachErr := batch.attachSidecars(ctx, scan); attachErr != nil {
		return attachErr
	}
	return batch.recordComplete(ctx, scan)
}

// judgedRemovals are what a scan would remove: the live files it covers and did not see, the disks
// mounted inside the root it did not find, how many live files the root holds, and why the removals
// wait for the admin, if they do.
type judgedRemovals struct {
	unseenFileIDs  []int64
	missingMounts  map[string]bool
	liveFileCount  int
	seenFileCount  int
	withheldReason string
}

// judgeRemovals is what a scan would remove, and whether it must wait. A scan of a folder that would
// lose a disk or wait for the admin may not judge it: errScopeNeedsTheRoot (D272 §9).
func (batch *ingestBatch) judgeRemovals(ctx context.Context, scan *scanState) (judgedRemovals, error) {
	var judged judgedRemovals
	var err error
	if judged.missingMounts, err = batch.missingMounts(ctx, scan); err != nil {
		return judged, err
	}
	if len(judged.missingMounts) > 0 && scan.isScoped() {
		return judged, fmt.Errorf("%w: a disk mounted below it is not there", errScopeNeedsTheRoot)
	}
	counted, err := batch.unseenUnder(ctx, scan)
	if err != nil {
		return judged, err
	}
	judged.unseenFileIDs, judged.liveFileCount = counted.unseenIDs, counted.liveCount
	judged.seenFileCount = counted.seenCount
	judged.withheldReason, err = batch.withholdReason(ctx, scan, len(judged.unseenFileIDs), judged.liveFileCount)
	if err == nil && judged.withheldReason != "" && scan.isScoped() {
		err = fmt.Errorf("%w: its removals would wait for the admin: %s", errScopeNeedsTheRoot, judged.withheldReason)
	}
	return judged, err
}

// recordComplete records a whole scan as the root's last, with its facts as the known-good ones. A
// folder's scan is not the root's: its last scan and facts stay the last whole scan's, whose
// schedule a watcher's notices never put off (D272 §9).
func (batch *ingestBatch) recordComplete(ctx context.Context, scan *scanState) error {
	if scan.isScoped() {
		batch.foundPaths = append(batch.foundPaths, path.Join(scan.root, scan.scope))
		return nil
	}
	err := batch.queries.UpsertRootScan(ctx, sqlcgen.UpsertRootScanParams{
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

// checkScopeMayJudge refuses, with errScopeNeedsTheRoot, a scan of a folder that may not judge what
// is gone: one that could not read all of it, or under a root whose last scan was not complete or
// that is unavailable, which only a scan of the whole root brings back (D208, D241, D272 §9).
func (batch *ingestBatch) checkScopeMayJudge(
	ctx context.Context, scan *scanState, complete *nodeproto.ScanComplete,
) error {
	if complete.IsStoppedByErrors || scan.isEverythingUnread {
		return fmt.Errorf("%w: it could not read %s", errScopeNeedsTheRoot, scan.scope)
	}
	prior, err := store.OrNotFound(batch.queries.GetRootState(ctx, sqlcgen.GetRootStateParams{
		NodeID: batch.nodeID, RootPath: scan.root,
	}))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("%w: the root has had no complete scan", errScopeNeedsTheRoot)
	case err != nil:
		return err
	case prior.LastScanOutcome != outcomeComplete || prior.UnavailableSince != nil:
		return fmt.Errorf("%w: the root's last scan was %s, or it is unavailable", errScopeNeedsTheRoot,
			prior.LastScanOutcome)
	}
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
		_, isBelowScope := RelativeToRoot([]byte(mount.MountPath), path.Join(scan.root, scan.scope))
		if scan.isScoped() && !isBelowScope {
			continue // not below the folder this scan listed: not looked at as a mount
		}
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
// it did not find as missing since the first scan that did not, keeping the device they were on.
func (batch *ingestBatch) recordMounts(ctx context.Context, scan *scanState, missing map[string]bool) error {
	for mountPath := range missing {
		err := batch.queries.RecordRootMountMissing(ctx, sqlcgen.RecordRootMountMissingParams{
			NodeID: batch.nodeID, MountPath: mountPath, MissingSince: &batch.nowMs,
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

// unseenUnder counts a root's live files as the scan found them: a scan of a folder
// is judged against the whole root, so a deleted folder is not most of what its scan looked at
// (D208 §3, D272 §9).
func (batch *ingestBatch) unseenUnder(ctx context.Context, scan *scanState) (rootFiles, error) {
	files, err := batch.queries.ListLiveFilesOfLibraryOnNode(ctx, sqlcgen.ListLiveFilesOfLibraryOnNodeParams{
		NodeID: batch.nodeID, LibraryID: scan.libraryID,
	})
	if err != nil {
		return rootFiles{}, fmt.Errorf("list the files under %s: %w", scan.root, err)
	}
	var counted rootFiles
	for _, file := range files {
		if _, under := RelativeToRoot(file.ReportedPathRaw, scan.root); !under {
			continue
		}
		counted.liveCount++
		if file.LastSeenAt >= scan.startedAt {
			counted.seenCount++
			continue
		}
		if scan.covers(file.ReportedPathRaw) && !scan.isUnread(file.ReportedPathRaw) {
			counted.unseenIDs = append(counted.unseenIDs, file.ID)
		}
	}
	return counted, nil
}

// rootFiles is a root's live files as a scan found them: those it covers and did not see, outside
// what it could not read; how many there are; and how many of them it saw.
type rootFiles struct {
	unseenIDs []int64
	liveCount int
	seenCount int
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
// A scan that removes nothing never waits: a root that moved to another device with every file
// still in it is not an unmounted disk, and its new device is recorded (D276).
func (batch *ingestBatch) withholdReason(ctx context.Context, scan *scanState, removing, total int) (string, error) {
	if removing == 0 {
		return "", nil
	}
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

// recordNotComplete records a scan that could not read its root, or whose removals wait, and
// whether its root is unavailable for it.
func (batch *ingestBatch) recordNotComplete(
	ctx context.Context, scan *scanState, outcome, reason string, isUnavailable bool,
) error {
	var unavailableSince *int64
	if isUnavailable {
		unavailableSince = &batch.nowMs
	}
	var withheldSince *int64
	var withheldReason *string
	if outcome == outcomeWithheld {
		withheldSince, withheldReason = &scan.startedAt, &reason
	}
	err := batch.queries.RecordRootNotComplete(ctx, sqlcgen.RecordRootNotCompleteParams{
		NodeID: batch.nodeID, RootPath: scan.root, LastScanOutcome: outcome, LastScanAt: batch.nowMs,
		WithheldScanStartedAt: withheldSince, WithheldReason: withheldReason, UnavailableSince: unavailableSince,
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
