// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ScheduleSettings are the settings the scan schedule reads: settings.Service.
type ScheduleSettings interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// ScheduledScans are how the schedule asks for a scan, of a whole root or of a folder in it, and sees
// those in progress: Scans.
type ScheduledScans interface {
	RequestUnder(ctx context.Context, nodeID int64, root, scope string, priority int64) error
	InProgress(ctx context.Context) (map[ScanRoot]ScanProgress, error)
}

// ScanSchedule asks for the scans that libraries noticing changes by themselves are due: one
// checked on an interval every check_every_hours, and a watched one every
// watched_library_scan_every_hours beneath its watching, which can miss a change and never says so
// (D27, D272 §1); and, when a watched library's node says something changed in a folder of its
// root, the scan of that folder (D272 §9). A library that changes only when asked is never scanned
// by it.
type ScanSchedule struct {
	database *store.Database
	scans    ScheduledScans
	settings ScheduleSettings
	clock    clock.Clock
	logger   *slog.Logger
}

// NewScanSchedule returns the scan schedule.
func NewScanSchedule(
	database *store.Database, scans ScheduledScans, scheduleSettings ScheduleSettings, clockSource clock.Clock,
	logger *slog.Logger,
) *ScanSchedule {
	return &ScanSchedule{
		database: database, scans: scans, settings: scheduleSettings, clock: clockSource, logger: logger,
	}
}

// QueueDue asks for a scan of every root that is due one, with no person waiting for it, and says
// how many it asked for. A root whose scan runs already is not asked again.
func (schedule *ScanSchedule) QueueDue(ctx context.Context) (int, error) {
	rows, err := schedule.database.Read().ListCheckedRoots(ctx)
	if err != nil {
		return 0, fmt.Errorf("list the roots libraries check: %w", err)
	}
	watchedEvery, err := schedule.settings.Duration(ctx, settings.WatchedLibraryScanEveryHours)
	if err != nil {
		return 0, fmt.Errorf("read watched_library_scan_every_hours: %w", err)
	}
	running, err := schedule.scans.InProgress(ctx)
	if err != nil {
		return 0, err
	}
	asked := 0
	for _, root := range dueRoots(rows, watchedEvery, schedule.clock.Now()) {
		if _, isRunning := running[root]; isRunning {
			continue
		}
		err := schedule.scans.RequestUnder(ctx, root.NodeID, root.RootPath, wholeRoot, jobs.PriorityBackground)
		if err != nil {
			return asked, err
		}
		asked++
	}
	if asked > 0 {
		schedule.logger.InfoContext(ctx, "asked for the scans libraries are due", "count", asked)
	}
	return asked, nil
}

// TakeChange is a node watcher's word that something changed under folder, relative to a root of
// the node's: the folder is scanned when a watched library covers the root, and the notice is
// dropped otherwise, since only a watched library asked to be told (D272 §9).
func (schedule *ScanSchedule) TakeChange(ctx context.Context, nodeID int64, root, folder string) error {
	rows, err := schedule.database.Read().ListCheckedRoots(ctx)
	if err != nil {
		return fmt.Errorf("list the roots libraries check: %w", err)
	}
	for _, row := range rows {
		if row.NodeID == nodeID && row.RootPath == root && row.ChangeDetection == "watch" {
			schedule.logger.InfoContext(ctx, "a node's watcher saw a change; scanning the folder", "nodeId", nodeID,
				"rootPath", root, "path", folder)
			return schedule.scans.RequestUnder(ctx, nodeID, root, folder, jobs.PriorityBackground)
		}
	}
	return nil
}

// dueRoots are the roots whose last scan is older than their libraries' interval, or that were
// never scanned, once each. A root two libraries check is due by the shorter interval.
func dueRoots(rows []sqlcgen.ListCheckedRootsRow, watchedEvery time.Duration, now time.Time) []ScanRoot {
	shortest := map[ScanRoot]time.Duration{}
	lastScan := map[ScanRoot]*int64{}
	var order []ScanRoot
	for _, row := range rows {
		root := ScanRoot{NodeID: row.NodeID, RootPath: row.RootPath}
		every := watchedEvery
		if row.ChangeDetection == "interval" && row.CheckEveryHours != nil {
			every = time.Duration(*row.CheckEveryHours) * time.Hour
		}
		known, isKnown := shortest[root]
		if !isKnown {
			order = append(order, root)
			lastScan[root] = row.LastScanAt
		}
		if !isKnown || every < known {
			shortest[root] = every
		}
	}
	var due []ScanRoot
	for _, root := range order {
		last := lastScan[root]
		if last == nil || !now.Before(time.UnixMilli(*last).Add(shortest[root])) {
			due = append(due, root)
		}
	}
	return due
}
