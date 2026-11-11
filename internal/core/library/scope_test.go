// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestWiderScope_IsTheFolderBothAreIn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ first, second, want string }{
		{"Shows/Dag/Season 1", "Shows/Dag/Season 2", "Shows/Dag"},
		{"Shows/Dag", "Shows/Dag/Season 1", "Shows/Dag"},
		{"Shows", "Films", wholeRoot},
		{"Shows", wholeRoot, wholeRoot},
		{"Shows/Dag", "Shows/Dagger", "Shows"},
		{"/etc", "Shows", wholeRoot},
		{"../elsewhere", "../elsewhere", wholeRoot},
		{"Shows/./Dag/", "Shows/Dag", "Shows/Dag"},
	} {
		if got := widerScope(tc.first, tc.second); got != tc.want {
			t.Errorf("widerScope(%q, %q) = %q, want %q", tc.first, tc.second, got, tc.want)
		}
	}
}

// listedIn is the listing of the last scan job, which must be of scope; it answers with records, as
// bench.list does.
func (bench *scanBench) listedIn(scope string, records ...nodeproto.ScanRecord) {
	bench.t.Helper()
	ctx, job := bench.t.Context(), bench.jobs.last(bench.t, jobs.KindScan)
	bench.clock.Advance(time.Minute)
	if err := bench.scans.HandleScan(ctx, job); err != nil {
		bench.t.Fatal(err)
	}
	if sent := bench.sent.commands[len(bench.sent.commands)-1]; sent.List == nil || sent.List.Path != scope {
		bench.t.Fatalf("the scan job sent %+v, want a listing of %q", sent.List, scope)
	}
	listing := append([]nodeproto.ScanRecord{{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{
		RootPath: root, FilesystemDevice: 7, IsMountPoint: true,
	}}}, records...)
	listing = append(listing, nodeproto.ScanRecord{
		Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{RootPath: root},
	})
	err := bench.scans.ScanAssignee(nil).Answer(ctx, job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Listing: &nodeproto.Listing{Records: listing}})
	if err != nil {
		bench.t.Fatal(err)
	}
}

// examineAs answers the last examine job with records.
func (bench *scanBench) examineAs(records ...nodeproto.ScanRecord) {
	bench.t.Helper()
	job, _ := bench.examine()
	err := bench.scans.ExamineAssignee(nil).Answer(bench.t.Context(), job, nodes.Attempt{NodeID: bench.nodeID},
		nodeproto.Result{Examined: &nodeproto.Examined{Records: records}})
	if err != nil {
		bench.t.Fatal(err)
	}
}

// requestUnder asks for a scan of scope under the root.
func (bench *scanBench) requestUnder(scope string) {
	bench.t.Helper()
	err := bench.scans.RequestUnder(bench.t.Context(), bench.nodeID, root, scope, jobs.PriorityBackground)
	if err != nil {
		bench.t.Fatal(err)
	}
}

// A scan of a folder lists that folder and removes what it no longer holds, and nothing outside it;
// it is not the root's scan, so the root's last scan stays the whole one (D272 §9).
func TestScans_AScanOfAFolderRemovesOnlyWhatItCovers(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{
		observedRecord("Shows/Dag/1.mkv", "dag 1"), observedRecord("Films/Metropolis.mkv", "metropolis"),
	}, true)
	before, err := bench.database.Read().GetRootState(t.Context(), rootStateParams(bench.nodeID))
	if err != nil {
		t.Fatal(err)
	}
	bench.requestUnder("Shows")
	bench.listedIn("Shows", listedDirectory("Shows"))
	live := bench.liveFilesByPath()
	if _, isLive := live[root+"/Shows/Dag/1.mkv"]; isLive {
		t.Error("the file gone from the folder scanned is still live")
	}
	if _, isLive := live[root+"/Films/Metropolis.mkv"]; !isLive {
		t.Error("a scan of Shows removed a film outside it")
	}
	after, err := bench.database.Read().GetRootState(t.Context(), rootStateParams(bench.nodeID))
	if err != nil || after.LastScanOutcome != "complete" || bench.isScanning() {
		t.Errorf("after the folder's scan the root's state is %+v (%v), scanning %v; want it as it was, and "+
			"the scan ended", after, err, bench.isScanning())
	}
	if *after.FilesystemDevice != *before.FilesystemDevice {
		t.Errorf("the root's facts changed from %+v to %+v", before, after)
	}
}

// A copy put in the folder scanned is a second file, never the file outside it moved: that one was
// not looked at, so it cannot be judged gone (D179, D272 §9).
func TestScans_ACopyInTheFolderIsADuplicateNotAMoveOfTheFileOutsideIt(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Films/Metropolis.mkv", "metropolis")}, true)
	bench.requestUnder("Shows")
	bench.listedIn("Shows", listedDirectory("Shows"), listedFile("Shows/Metropolis.mkv", 4, 1))
	bench.examineAs(observedRecord("Shows/Metropolis.mkv", "metropolis"))
	live := bench.liveFilesByPath()
	_, isOriginal := live[root+"/Films/Metropolis.mkv"]
	_, isCopy := live[root+"/Shows/Metropolis.mkv"]
	if !isOriginal || !isCopy {
		t.Errorf("the original is live: %t, the copy: %t; want both", isOriginal, isCopy)
	}
	if count := bench.jobs.count(jobs.KindExamine); count != 1 {
		t.Errorf("%d batches were read, want the one holding the copy", count)
	}
}

// A scan of a folder under a root that has had no complete scan may judge nothing gone: the whole root
// is scanned instead, and nothing is removed meanwhile.
func TestScans_AScanOfAFolderThatMayNotJudgeBecomesTheWholeRoots(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.requestUnder("Shows")
	bench.listedIn("Shows", listedDirectory("Shows"))
	if count := bench.jobs.count(jobs.KindScan); count != 2 {
		t.Fatalf("%d scans queued, want the folder's, then the root's", count)
	}
	bench.listedIn(wholeRoot)
	if _, err := bench.database.Read().GetRootState(t.Context(), rootStateParams(bench.nodeID)); err != nil {
		t.Errorf("after the root's scan its state is not recorded: %v", err)
	}
}

// Folders asked for while a scan runs are scanned once after it, by the folder that holds them all,
// and a request for the whole root widens it to the root (D236 §3, D272 §9).
func TestScans_FoldersAskedForWhileOneRunsAreScannedOnceByTheFolderHoldingThem(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan(nil, true)
	bench.requestUnder("Films")
	bench.hand()
	bench.requestUnder("Shows/Dag/Season 1")
	bench.requestUnder("Shows/Dag/Season 2")
	bench.listedIn("Films")
	bench.listedIn("Shows/Dag")
	bench.requestUnder("Shows")
	bench.hand()
	if err := bench.scans.Request(t.Context(), bench.nodeID, root, jobs.PriorityBackground); err != nil {
		t.Fatal(err)
	}
	bench.listedIn("Shows")
	bench.listedIn(wholeRoot)
	if count := bench.jobs.count(jobs.KindScan); count != 4 || bench.isScanning() {
		t.Errorf("%d scans queued, one in progress: %v; want four and none", count, bench.isScanning())
	}
}
