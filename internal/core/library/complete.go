// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// complete ends a scan: it concludes the moves it held, then either tombstones what the scan did
// not see — outside the regions it could not read — or withholds that and asks, when the removal
// looks like a lost disk rather than deleted files (D41, D208 §3). Tombstones, root_scan and
// root_state are written in this batch's transaction, together.
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
		return work.recordNotComplete(ctx, scan, outcomeUnreadable, "")
	}
	missing, err := work.missingMounts(ctx, scan)
	if err != nil {
		return err
	}
	unseen, total, err := work.unseenUnder(ctx, scan)
	if err != nil {
		return err
	}
	reason, err := work.withholdReason(ctx, scan, len(unseen), total)
	if err != nil {
		return err
	}
	if reason != "" {
		work.ingester.logger.WarnContext(ctx, "a scan's removals wait for the admin", "nodeId", work.nodeID,
			"root", scan.root, "wouldRemove", len(unseen), "of", total, "reason", reason)
		return work.recordNotComplete(ctx, scan, outcomeWithheld, reason)
	}
	for _, gone := range unseen {
		tombstone := sqlcgen.TombstoneFileParams{RemovedAt: &work.now, ID: gone}
		if tombstoneErr := work.queries.TombstoneFile(ctx, tombstone); tombstoneErr != nil {
			return fmt.Errorf("tombstone file %d: %w", gone, tombstoneErr)
		}
	}
	if mountsErr := work.recordMounts(ctx, scan, missing); mountsErr != nil {
		return mountsErr
	}
	err = work.queries.UpsertRootScan(ctx, sqlcgen.UpsertRootScanParams{
		NodeID: work.nodeID, Path: scan.root, CompletedAt: work.now,
	})
	if err != nil {
		return fmt.Errorf("record the scan of %s as complete: %w", scan.root, err)
	}
	device, mountPoint := int64(scan.facts.Device), boolInt(scan.facts.MountPoint) //nolint:gosec // a device id fits
	err = work.queries.RecordRootComplete(ctx, sqlcgen.RecordRootCompleteParams{
		NodeID: work.nodeID, Path: scan.root, Device: &device, MountPoint: &mountPoint, At: work.now,
	})
	if err != nil {
		return fmt.Errorf("record %s's facts: %w", scan.root, err)
	}
	return nil
}

// missingMounts compares the mount boundaries this scan crossed with those the root's last
// complete scan recorded. A boundary not seen where it was, or seen on another device, is a disk
// that is not there — an unplugged disk under /mnt lists as an empty directory — so nothing
// under it may be removed: it joins the regions the scan could not read (D208).
func (work *batch) missingMounts(ctx context.Context, scan *scanState) (map[string]bool, error) {
	known, err := work.queries.ListRootMounts(ctx, sqlcgen.ListRootMountsParams{NodeID: work.nodeID, Root: scan.root})
	if err != nil {
		return nil, fmt.Errorf("list the mounts under %s: %w", scan.root, err)
	}
	missing := map[string]bool{}
	for _, mount := range known {
		device, seen := scan.mounts[mount.Path]
		if seen && int64(device) == mount.Device { //nolint:gosec // a device id fits
			continue
		}
		missing[mount.Path] = true
		scan.unread = append(scan.unread, mount.Path)
		work.ingester.logger.WarnContext(ctx, "a disk mounted inside a root is not there; nothing under it was removed",
			"nodeId", work.nodeID, "root", scan.root, "mount", mount.Path)
	}
	return missing, nil
}

// recordMounts keeps a complete scan's mount boundaries as the known-good ones, and marks those
// it did not find as missing, keeping the device they were on.
func (work *batch) recordMounts(ctx context.Context, scan *scanState, missing map[string]bool) error {
	for mountPath := range missing {
		err := work.queries.RecordRootMountMissing(ctx, sqlcgen.RecordRootMountMissingParams{
			NodeID: work.nodeID, Path: mountPath,
		})
		if err != nil {
			return fmt.Errorf("record %s as missing: %w", mountPath, err)
		}
	}
	for mountPath, device := range scan.mounts {
		if missing[mountPath] {
			continue // on another device than it was: not the disk we knew
		}
		err := work.queries.RecordRootMountPresent(ctx, sqlcgen.RecordRootMountPresentParams{
			NodeID: work.nodeID, Root: scan.root, Path: mountPath, Device: int64(device), //nolint:gosec // fits
		})
		if err != nil {
			return fmt.Errorf("record the mount %s: %w", mountPath, err)
		}
	}
	return nil
}

// unseenUnder returns the live files under the root that the scan did not see, outside what it
// could not read, and how many live files the root holds.
func (work *batch) unseenUnder(ctx context.Context, scan *scanState) ([]int64, int, error) {
	files, err := work.queries.ListLiveFilesOfLibrary(ctx, sqlcgen.ListLiveFilesOfLibraryParams{
		NodeID: work.nodeID, LibraryID: scan.libraryID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list the files under %s: %w", scan.root, err)
	}
	var unseen []int64
	total := 0
	for _, file := range files {
		if !under(file.Path, scan.root) {
			continue
		}
		total++
		if file.LastSeen < scan.startedAt && !scan.hides(file.Path) {
			unseen = append(unseen, file.ID)
		}
	}
	return unseen, total, nil
}

// withholdReason says why a scan's removals should wait for the admin, or "" when they should
// not: the root's device or mount changed since its last complete scan, or it would lose all of
// its files, or more than removal_withhold_percent of them and more than
// removal_withhold_min_files (D208 §3).
func (work *batch) withholdReason(ctx context.Context, scan *scanState, removing, total int) (string, error) {
	prior, err := store.One(work.queries.GetRootState(ctx, sqlcgen.GetRootStateParams{
		NodeID: work.nodeID, Path: scan.root,
	}))
	switch {
	case err == nil && prior.Device != nil && *prior.Device != int64(scan.facts.Device): //nolint:gosec // fits
		return "the root is on another device than at its last complete scan: an unmounted disk?", nil
	case err == nil && prior.MountPoint != nil && *prior.MountPoint != boolInt(scan.facts.MountPoint):
		return "the root was a mount point and is not now, or the other way round", nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return "", err
	}
	if removing == 0 {
		return "", nil
	}
	if removing == total {
		return fmt.Sprintf("the scan saw none of the root's %d files", total), nil
	}
	percent, err := work.ingester.settings.Int(ctx, settings.RemovalWithholdPercent)
	if err != nil {
		return "", fmt.Errorf("read removal_withhold_percent: %w", err)
	}
	minimum, err := work.ingester.settings.Int(ctx, settings.RemovalWithholdMinFiles)
	if err != nil {
		return "", fmt.Errorf("read removal_withhold_min_files: %w", err)
	}
	if int64(removing)*100 > percent*int64(total) && int64(removing) > minimum {
		return fmt.Sprintf("the scan would remove %d of the root's %d files", removing, total), nil
	}
	return "", nil
}

// recordNotComplete records a scan that could not read its root, or whose removals wait.
func (work *batch) recordNotComplete(ctx context.Context, scan *scanState, outcome, reason string) error {
	var since *int64
	var why *string
	if outcome == outcomeWithheld {
		since, why = &scan.startedAt, &reason
	}
	err := work.queries.RecordRootNotComplete(ctx, sqlcgen.RecordRootNotCompleteParams{
		NodeID: work.nodeID, Path: scan.root, Outcome: outcome, At: work.now, WithheldSince: since, WithheldReason: why,
	})
	if err != nil {
		return fmt.Errorf("record the scan of %s as %s: %w", scan.root, outcome, err)
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

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
