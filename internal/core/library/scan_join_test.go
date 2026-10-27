// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// listingsSent is how many listings the bench's scans handed to the node.
func (bench *scanBench) listingsSent() int {
	count := 0
	for _, command := range bench.sent.commands {
		if command.List != nil {
			count++
		}
	}
	return count
}

// isAnotherScanQueued is whether the root's scan in progress has one more to follow it.
func (bench *scanBench) isAnotherScanQueued() bool {
	bench.t.Helper()
	scan, isScanning := bench.inProgressOf()
	if !isScanning {
		bench.t.Fatal("no scan of the root is in progress")
	}
	return scan.IsAnotherScanQueued
}

// Two watcher's notices of folders, the second while the first's listing waits to go to the node,
// make one listing, of the folder holding both, and no scan follows.
func TestScans_FoldersAskedForWhileTheListingWaitsAreListedOnceByTheFolderHoldingThem(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan(nil, true)
	ctx := t.Context()
	for _, folder := range []string{"Shows/Dag/Season 1", "Shows/Dag/Season 2"} { // a watcher's notices
		if err := bench.scans.RequestUnder(ctx, bench.nodeID, root, folder, jobs.PriorityBackground); err != nil {
			t.Fatal(err)
		}
	}
	if bench.isAnotherScanQueued() {
		t.Error("a folder's ask while the scan of another waited marked one more to follow; want it joined")
	}
	bench.listedIn("Shows/Dag")
	if bench.isScanning() || bench.listingsSent() != 1 {
		t.Fatalf("two folders' asks made %d listings, a scan in progress %v; want one of the folder holding both",
			bench.listingsSent(), bench.isScanning())
	}
}

// Every way of asking for a scan, made while a root's scan waits for its listing to go to the node,
// joins it: the node lists the root once, of the scope covering every ask, at the highest priority
// asked, and no scan follows. Work is per absolute path, done once (the owner, 2026-10-27).
func TestScans_AnAskForARootWhoseListingIsNotWithItsNodeJoinsIt(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan(nil, true)
	ctx := t.Context()
	for name, ask := range map[string]func() error{
		"a node's hello": func() error { return bench.scans.RequestOnHello(ctx, bench.nodeID, root) },
		"its disk answering again": func() error {
			return bench.scans.RequestOnReturn(ctx, bench.nodeID, root)
		},
		"the schedule": func() error {
			return bench.scans.RequestUnder(ctx, bench.nodeID, root, wholeRoot, jobs.PriorityBackground)
		},
		"a person's Scan button": func() error {
			return bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityPersonsRequest)
		},
		"a rescan of one film": func() error {
			return bench.scans.RequestUnder(ctx, bench.nodeID, root, "Films/Metropolis", jobs.PriorityPersonsRequest)
		},
	} {
		if err := bench.scans.RequestUnder(ctx, bench.nodeID, root, "Films", jobs.PriorityBackground); err != nil {
			t.Fatal(err)
		}
		if err := ask(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bench.isAnotherScanQueued() {
			t.Errorf("%s while the listing waited marked one more scan to follow; want it joined", name)
		}
		scope := "Films"
		if name != "a rescan of one film" {
			scope = wholeRoot
		}
		before := bench.listingsSent()
		bench.listedIn(scope)
		if bench.isScanning() || bench.listingsSent() != before+1 {
			t.Errorf("%s made %d listings, a scan in progress %v; want one", name, bench.listingsSent()-before,
				bench.isScanning())
		}
	}

	if err := bench.scans.RequestUnder(ctx, bench.nodeID, root, "Films", jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityPersonsRequest); err != nil {
		t.Fatal(err)
	}
	if raised := bench.jobs.last(t, jobs.KindScan); raised.Priority != jobs.PriorityPersonsRequest {
		t.Errorf("a person's ask joined a background listing at priority %d; want it raised to a person's %d",
			raised.Priority, jobs.PriorityPersonsRequest)
	}
}

// After a restart of core and node, the listing in flight is retried on the node's new start, and
// the node's hello asks for a scan of every root: the hello joins the retried listing rather than
// following it with a second, 45 s over a slow share (staging, 2026-10-27). An ask a listing with
// the node may have passed still has one more follow it (D236 §3): a watcher's notice, a disk
// answering again, a hello for more than the listing covers.
func TestScans_AHellosAskJoinsTheListingWithItsNodeAndOtherAsksFollowIt(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan(nil, true)
	ctx := t.Context()
	if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	bench.hand() // the listing goes to the node, and core and node restart before it answers
	bench.scans = bench.restart()
	if err := bench.scans.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	bench.hand() // the node's new start is given the listing again, before its hello's asks
	if err := bench.scans.RequestOnHello(ctx, bench.nodeID, root); err != nil {
		t.Fatal(err)
	}
	if bench.isAnotherScanQueued() {
		t.Error("the hello's ask marked one more scan to follow the listing its start was given; want it joined")
	}
	bench.list()
	if bench.isScanning() {
		t.Error("a scan follows the listing the hello joined; want the root listed once")
	}

	for _, tc := range []struct {
		name, listing string
		ask           func() error
	}{
		{"a watcher's notice of a folder inside the listing", "Shows", func() error {
			return bench.scans.RequestUnder(ctx, bench.nodeID, root, "Shows/Dag", jobs.PriorityBackground)
		}},
		{"its disk answering again", wholeRoot, func() error {
			return bench.scans.RequestOnReturn(ctx, bench.nodeID, root)
		}},
		{"a hello for more than the listing covers", "Shows", func() error {
			return bench.scans.RequestOnHello(ctx, bench.nodeID, root)
		}},
	} {
		if err := bench.scans.RequestUnder(ctx, bench.nodeID, root, tc.listing, jobs.PriorityBackground); err != nil {
			t.Fatal(err)
		}
		bench.hand()
		if err := tc.ask(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !bench.isAnotherScanQueued() {
			t.Errorf("%s while a listing was with the node joined it; want one more scan to follow", tc.name)
		}
		bench.listedIn(tc.listing)
		if !bench.isScanning() {
			t.Fatalf("after %s, no scan followed the listing it came during", tc.name)
		}
		bench.list()
	}
}

// Two libraries over one root path, as staging's "Series" and "shows" are (made before an admin was
// refused one, ErrRootCovered), share its scan: their asks make one listing and one reading of each
// file, which has one row, of one library.
func TestScans_TwoLibrariesOverOneRootPathMakeOneListingAndReadEachFileOnce(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	ctx := t.Context()
	librarytest.LibraryOn(t, bench.database, bench.nodeID,
		librarytest.Library{Name: "Series", Kind: "shows", MetadataSource: "nfo", Root: root})
	for range 2 { // each library's ask: one by the schedule, one by a person's Scan button
		if err := bench.scans.Request(ctx, bench.nodeID, root, jobs.PriorityBackground); err != nil {
			t.Fatal(err)
		}
	}
	bench.list(listedFile("Metropolis.mkv", int64(len("heat")), 1))
	job, files := bench.examine()
	if len(files) != 1 || bench.listingsSent() != 1 {
		t.Fatalf("two libraries' asks made %d listings, the batch reads %d files; want one listing reading one",
			bench.listingsSent(), len(files))
	}
	err := bench.scans.ExamineAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Examined: &nodeproto.Examined{Records: []nodeproto.ScanRecord{
			observedRecord("Metropolis.mkv", "heat"),
		}}})
	if err != nil {
		t.Fatal(err)
	}
	if bench.isScanning() || bench.jobs.count(jobs.KindExamine) != 1 {
		t.Errorf("a scan in progress %v, %d batches read; want the one scan over, its file read once",
			bench.isScanning(), bench.jobs.count(jobs.KindExamine))
	}
	rows, err := bench.database.Read().ListLiveFilesByNode(ctx, bench.nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Path != root+"/Metropolis.mkv" {
		t.Errorf("the live files are %+v; want Metropolis.mkv once, of one library", rows)
	}
}
