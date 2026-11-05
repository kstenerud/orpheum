// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// showsInside makes a second library, of series, over the folder Shows inside the bench's library's
// folder: two libraries over one place (D363 §1).
func showsInside(t *testing.T, world *harness) {
	t.Helper()
	librarytest.LibraryOn(t, world.database, world.nodeID,
		librarytest.Library{Name: "Shows", Kind: "series", MetadataSource: "nfo", Root: root + "/Shows"})
}

// D363 §1, §2: a folder of one library inside another's is scanned within the outer folder, of its
// scope, so a place both cover is listed and read once; an ask for both joins one scan.
func TestScans_AFolderInsideAnotherLibrarysFolderIsScannedOnceWithinIt(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	showsInside(t, bench.harness)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Shows/Dag/1.mkv", "dag 1")}, true)
	err := bench.scans.RequestUnder(t.Context(), bench.nodeID, root+"/Shows", wholeRoot, jobs.PriorityBackground)
	if err != nil {
		t.Fatal(err)
	}
	bench.requestUnder(wholeRoot)
	scanIDs := map[int64]bool{}
	for _, request := range bench.jobs.requests {
		if request.Kind == jobs.KindScan {
			scanIDs[request.ScanID] = true
		}
	}
	if len(scanIDs) != 1 {
		t.Fatalf("scans %v queued; want one, the Shows library's ask joined by the root's", scanIDs)
	}
	bench.listedIn(wholeRoot, listedDirectory("Shows"), listedFile("Shows/Dag/1.mkv", 5, 1))
	if sent := bench.sent.commands[len(bench.sent.commands)-1]; sent.List.RootPath != root {
		t.Errorf("the listing was of %s; want the outer folder %s", sent.List.RootPath, root)
	}
	places := storetest.QueryRaw(t, bench.database.Path(),
		"SELECT path FROM file WHERE removed_at IS NULL ORDER BY path")
	if !slices.Equal(places, []string{root + "/Shows/Dag/1.mkv"}) {
		t.Errorf("the live places are %v; want the episode once", places)
	}
}

// D363 §1: an ask for a scan of a folder inside another library's folder alone is that folder's
// scope within the outer one.
func TestScans_AnAskForAnInnerFolderIsItsScopeWithinTheOuter(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	showsInside(t, bench.harness)
	bench.scan([]nodeproto.ScanRecord{observedRecord("Films/Metropolis.mkv", "heat")}, true)
	err := bench.scans.RequestUnder(t.Context(), bench.nodeID, root+"/Shows", "Dag", jobs.PriorityBackground)
	if err != nil {
		t.Fatal(err)
	}
	bench.listedIn("Shows/Dag", listedDirectory("Shows/Dag"))
	if sent := bench.sent.commands[len(bench.sent.commands)-1]; sent.List.RootPath != root {
		t.Errorf("the listing was under %s; want the outer folder %s", sent.List.RootPath, root)
	}
	if _, isLive := bench.liveFilesByPath()[root+"/Films/Metropolis.mkv"]; !isLive {
		t.Error("a scan of a folder in Shows removed a film outside it")
	}
}

// D363 §2: a node watches the outermost of its watched library folders, of whichever library, each
// place once.
func TestScanSchedule_ANodeWatchesAFolderInsideAnotherOnlyThroughIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	showsInside(t, world)
	storetest.ExecRaw(t, world.database.Path(), "UPDATE library SET change_detection = 'watch'")
	schedule := NewScanSchedule(world.database, &scheduledRequests{}, world.settings, world.clock,
		slog.New(slog.DiscardHandler))
	watched, err := schedule.WatchedFolders(t.Context(), world.nodeID)
	if err != nil || !slices.Equal(watched, []string{root}) {
		t.Errorf("the node watches %v, %v; want only the outer folder %s", watched, err, root)
	}
}

func TestScanRootOf_IsTheOutermostFolderAtOrAboveWithTheScopeWithinIt(t *testing.T) {
	t.Parallel()
	folders := []string{"/m", "/m/shows", "/m/shows/anime", "/n"}
	for _, tc := range []struct{ folder, scope, root, rootScope string }{
		{"/m/shows/anime", wholeRoot, "/m", "shows/anime"},
		{"/m/shows", "Dag", "/m", "shows/Dag"},
		{"/m", "series", "/m", "series"},
		{"/n", wholeRoot, "/n", wholeRoot},
		{"/other", wholeRoot, "/other", wholeRoot},
	} {
		root, scope := scanRootOf(tc.folder, tc.scope, folders)
		if root != tc.root || scope != tc.rootScope {
			t.Errorf("scanRootOf(%q, %q) = %q, %q; want %q, %q", tc.folder, tc.scope, root, scope, tc.root,
				tc.rootScope)
		}
	}
	if got := outermostFolders(folders); !slices.Equal(got, []string{"/m", "/n"}) {
		t.Errorf("outermostFolders = %v; want /m and /n", got)
	}
}
