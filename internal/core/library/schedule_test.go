// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// scheduledRequests are the scans the schedule asked for at the priority they are wanted at,
// background unless said, and the scans in progress it sees.
type scheduledRequests struct {
	asked    []ScanRoot
	scopes   []string
	running  map[ScanRoot]ScanProgress
	priority int64
}

func (requests *scheduledRequests) RequestUnder(
	_ context.Context, nodeID int64, root, scope string, priority int64,
) error {
	if priority == requests.priority {
		requests.asked = append(requests.asked, ScanRoot{NodeID: nodeID, RootPath: root})
		requests.scopes = append(requests.scopes, scope)
	}
	return nil
}

func (requests *scheduledRequests) InProgress(context.Context) (map[ScanRoot]ScanProgress, error) {
	return requests.running, nil
}

func TestDueRoots_ARootIsDueOnceItsLibrariesIntervalHasPassedSinceItsLastScan(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	hoursAgo := func(hours int) *int64 { return new(now.Add(-time.Duration(hours) * time.Hour).UnixMilli()) }
	every := func(hours int64) *int64 { return &hours }
	row := func(root, detection string, checkEvery, lastScan *int64) sqlcgen.ListCheckedRootsRow {
		return sqlcgen.ListCheckedRootsRow{
			NodeID: 1, RootPath: root, ChangeDetection: detection,
			CheckEveryHours: checkEvery, LastScanAt: lastScan,
		}
	}
	for _, tc := range []struct {
		name    string
		rows    []sqlcgen.ListCheckedRootsRow
		wantDue []string
	}{
		{"never scanned", []sqlcgen.ListCheckedRootsRow{row("/a", "interval", every(6), nil)}, []string{"/a"}},
		{
			"scanned within its interval",
			[]sqlcgen.ListCheckedRootsRow{row("/a", "interval", every(6), hoursAgo(5))},
			nil,
		},
		{
			"its interval just passed",
			[]sqlcgen.ListCheckedRootsRow{row("/a", "interval", every(6), hoursAgo(6))},
			[]string{"/a"},
		},
		{
			"watched, within the watched interval",
			[]sqlcgen.ListCheckedRootsRow{row("/a", "watch", nil, hoursAgo(23))},
			nil,
		},
		{"watched, past it", []sqlcgen.ListCheckedRootsRow{row("/a", "watch", nil, hoursAgo(24))}, []string{"/a"}},
		{"two libraries over one root, due by the shorter", []sqlcgen.ListCheckedRootsRow{
			row("/a", "watch", nil, hoursAgo(3)), row("/a", "interval", every(2), hoursAgo(3)),
		}, []string{"/a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var due []string
			for _, root := range dueRoots(tc.rows, 24*time.Hour, now) {
				due = append(due, root.RootPath)
			}
			if !slices.Equal(due, tc.wantDue) {
				t.Errorf("due roots %v, want %v", due, tc.wantDue)
			}
		})
	}
}

func TestScanSchedule_AsksForTheDueScansOfLibrariesThatCheckByThemselves(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	for _, made := range []NewLibrary{
		{
			Name: "Checked", ChangeDetection: "interval", CheckEveryHours: new(int64(6)),
			Roots: []string{"/media/checked"},
		},
		{Name: "Watched", ChangeDetection: "watch", Roots: []string{"/media/watched"}},
		{Name: "Running", ChangeDetection: "watch", Roots: []string{"/media/running"}},
	} {
		made.Kind, made.MetadataSource, made.NodeID = "movies", "nfo", world.nodeID
		if _, err := Create(t.Context(), world.database, world.clock, made); err != nil {
			t.Fatal(err)
		}
	}
	requests := &scheduledRequests{running: map[ScanRoot]ScanProgress{
		{NodeID: world.nodeID, RootPath: "/media/running"}: {},
	}}
	schedule := NewScanSchedule(world.database, requests, world.settings, world.clock, slog.New(slog.DiscardHandler))
	asked, err := schedule.QueueDue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []ScanRoot{
		{NodeID: world.nodeID, RootPath: "/media/checked"}, {NodeID: world.nodeID, RootPath: "/media/watched"},
	}
	if asked != len(want) || !slices.Equal(requests.asked, want) {
		t.Errorf("asked for %d scans, %v; want %v: never scanned, and not the manual library's %s nor the one "+
			"running", asked, requests.asked, want, root)
	}
}

func TestScanSchedule_AWatchersNoticeScansTheFolderOnlyForAWatchedLibrary(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	for _, made := range []NewLibrary{
		{Name: "Watched", ChangeDetection: "watch", Roots: []string{"/media/watched"}},
		{
			Name: "Checked", ChangeDetection: "interval", CheckEveryHours: new(int64(6)),
			Roots: []string{"/media/checked"},
		},
	} {
		made.Kind, made.MetadataSource, made.NodeID = "movies", "nfo", world.nodeID
		if _, err := Create(t.Context(), world.database, world.clock, made); err != nil {
			t.Fatal(err)
		}
	}
	requests := &scheduledRequests{}
	schedule := NewScanSchedule(world.database, requests, world.settings, world.clock, slog.New(slog.DiscardHandler))
	for _, notice := range []struct{ root, folder string }{
		{"/media/watched", "Shows/Dag"}, {"/media/checked", "Films"}, {root, "Films"}, {"/media/elsewhere", "x"},
	} {
		if err := schedule.TakeChange(t.Context(), world.nodeID, notice.root, notice.folder); err != nil {
			t.Fatal(err)
		}
	}
	want := []ScanRoot{{NodeID: world.nodeID, RootPath: "/media/watched"}}
	if !slices.Equal(requests.asked, want) || !slices.Equal(requests.scopes, []string{"Shows/Dag"}) {
		t.Errorf("the notices asked for %v, of %v; want the watched library's folder alone", requests.asked,
			requests.scopes)
	}
}

// A library root that is unavailable, its disk or share not answering, is scanned again once
// unavailable_root_rescan_seconds have passed since its last scan, until one reads it; one scanned
// lately, one being scanned and one available are not; and it goes before the queue's own scans,
// one of which held the disk's one reader for a minute (D233 §10; staging, 2026-10-21).
func TestScanSchedule_ScansAnUnavailableRootAgainUntilOneReadsIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	made := NewLibrary{
		Name: "Away", ChangeDetection: "manual", Kind: "movies", MetadataSource: "nfo", NodeID: world.nodeID,
		Roots: []string{"/media/away", "/media/lately", "/media/running", "/media/fine"},
	}
	if _, err := Create(t.Context(), world.database, world.clock, made); err != nil {
		t.Fatal(err)
	}
	now := world.clock.Now().UnixMilli()
	storetest.ExecRaw(t, world.database.Path(), fmt.Sprintf(`INSERT INTO root_state
		(node_id, root_path, last_scan_outcome, last_scan_at, unavailable_since) VALUES
		(%[1]d, '/media/away', 'unreadable', %[2]d, %[2]d), (%[1]d, '/media/lately', 'unreadable', %[3]d, %[2]d),
		(%[1]d, '/media/running', 'unreadable', %[2]d, %[2]d), (%[1]d, '/media/fine', 'complete', %[2]d, NULL)`,
		world.nodeID, now-2*60_000, now-2_000))
	requests := &scheduledRequests{priority: jobs.PriorityBringsBack, running: map[ScanRoot]ScanProgress{
		{NodeID: world.nodeID, RootPath: "/media/running"}: {},
	}}
	schedule := NewScanSchedule(world.database, requests, world.settings, world.clock, slog.New(slog.DiscardHandler))
	asked, err := schedule.RescanUnavailable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if want := []ScanRoot{{NodeID: world.nodeID, RootPath: "/media/away"}}; asked != 1 ||
		!slices.Equal(requests.asked, want) {
		t.Errorf("asked for %d scans, %v; want %v alone", asked, requests.asked, want)
	}
}
