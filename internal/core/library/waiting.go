// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// AdminSettings are the settings the admin service reads: how many of a held-back scan's removals
// are named, scan_withheld_removals_named.
type AdminSettings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// UseSettings has the admin service read its settings from reader; without, each is its starting
// value. Set once, before the server serves.
func (admin *Admin) UseSettings(reader AdminSettings) { admin.settings = reader }

// namedRemovals is how many of a held-back scan's removals are named: scan_withheld_removals_named,
// enough to recognise a season or a folder, few enough to read at a glance; the rest are counted (the
// constants sweep, 2026-10-20).
func (admin *Admin) namedRemovals(ctx context.Context) (int, error) {
	if admin.settings == nil {
		return int(settings.StartingInt(settings.ScanWithheldRemovalsNamed)), nil
	}
	named, err := admin.settings.Int(ctx, settings.ScanWithheldRemovalsNamed)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", settings.ScanWithheldRemovalsNamed, err)
	}
	return int(named), nil
}

// WaitingRemovals is a root whose last scan's removals wait for the admin (D208 §3): why, since
// when, how many files it would remove and the first of them by path (D277 §2).
type WaitingRemovals struct {
	LibraryID   int64
	LibraryName string
	NodeName    string
	RootPath    string
	Reason      string
	Since       time.Time
	FileCount   int
	// FirstPaths are the first scan_withheld_removals_named of the files, by path.
	FirstPaths []string
}

// Waiting is every root of the live libraries whose removals wait for the admin, the longest
// waiting first.
func (admin *Admin) Waiting(ctx context.Context) ([]WaitingRemovals, error) {
	named, err := admin.namedRemovals(ctx)
	if err != nil {
		return nil, err
	}
	queries := admin.database.Read()
	roots, err := queries.ListWithheldRoots(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the roots whose removals wait: %w", err)
	}
	waiting := make([]WaitingRemovals, 0, len(roots))
	for _, root := range roots {
		after, before := PathRange(root.RootPath)
		paths, err := queries.ListFilesAHeldBackScanDidNotSee(ctx, sqlcgen.ListFilesAHeldBackScanDidNotSeeParams{
			NodeID: root.NodeID, After: after, Before: before, ScanStartedAt: root.WithheldScanStartedAt,
		})
		if err != nil {
			return nil, fmt.Errorf("list what %s's scan would remove: %w", root.RootPath, err)
		}
		waiting = append(waiting, WaitingRemovals{
			LibraryID: root.LibraryID, LibraryName: root.LibraryName, NodeName: root.NodeName,
			RootPath: root.RootPath, Reason: ptr.ValueOrZero(root.WithheldReason),
			Since: time.UnixMilli(root.WithheldScanStartedAt), FileCount: len(paths),
			FirstPaths: paths[:min(len(paths), named)],
		})
	}
	return waiting, nil
}

// UnreadPath is a file or folder a scan could not read (D163 §8): where, and the platform's
// account of why, for the admin to fix (D277 §3).
type UnreadPath struct {
	LibraryID   int64
	LibraryName string
	NodeName    string
	Path        string
	// Code is the scan's error code: permissionDenied, say.
	Code   string
	Detail ErrorDetail
	// Since is when a scan first found it.
	Since time.Time
}

// UnreadPaths is everything the scans of the live libraries could not read, by library and path.
func (admin *Admin) UnreadPaths(ctx context.Context) ([]UnreadPath, error) {
	rows, err := admin.database.Read().ListUnreadablePaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("list what could not be read: %w", err)
	}
	unread := make([]UnreadPath, 0, len(rows))
	for _, row := range rows {
		path := UnreadPath{
			LibraryID: row.LibraryID, LibraryName: row.LibraryName, NodeName: row.NodeName, Path: row.Path,
			Code: ptr.ValueOrZero(row.FilingReason), Since: time.UnixMilli(row.FirstSeenAt),
		}
		if row.ErrorDetailJSON != nil {
			if path.Detail, err = DecodeErrorDetail(*row.ErrorDetailJSON); err != nil {
				return nil, err
			}
		}
		unread = append(unread, path)
	}
	return unread, nil
}
