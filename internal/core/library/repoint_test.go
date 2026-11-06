// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A repointed file keeps its modified time and is marked unconfirmed instead, so its film's found
// time does not move, and the path it was identified from moves with it, so identification does not
// take it for a file renamed (D432 §6, D208 §1). The next listing reads it again though its size and
// modified time are as stored, and the scan's read confirms it.
func TestRepoint_AMovedFileIsUnconfirmedAndKeepsItsIdentification(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("Arkon (1979)/Arkon.mkv", "arkon")}, true)
	ctx := t.Context()
	titleID := librarytest.Title(t, world.database, librarytest.TitleSpec{Library: world.libraryID})
	versionID := librarytest.Version(t, world.database, librarytest.VersionSpec{TitleID: titleID})
	arkon := world.liveFilesByPath()[root+"/Arkon (1979)/Arkon.mkv"]
	err := world.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return PutInVersion(ctx, queries, arkon.ID, LibraryKindMovie, versionID)
	})
	if err != nil {
		t.Fatal(err)
	}
	path := world.database.Path()
	found := storetest.QueryRaw(t, path, "SELECT found_at, found_file_modified_at FROM item WHERE id = "+itoa(titleID))
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	admin.ListFoldersThrough(readableFolders())
	if _, err = admin.Repoint(ctx, localMove(root, "/media/moved"), 1); err != nil {
		t.Fatal(err)
	}
	const moved = "/media/moved/Arkon (1979)/Arkon.mkv"
	checks := []struct{ what, query, want string }{
		{"the place", "SELECT path, modified_at, is_unconfirmed FROM place", moved + "|1|1"},
		{"the path identified from", "SELECT identified_from_path FROM file_filing", moved},
		{
			"the film's found time", "SELECT found_at, found_file_modified_at FROM item WHERE id = " + itoa(titleID),
			found[0],
		},
	}
	for _, check := range checks {
		if got := storetest.QueryRaw(t, path, check.query); !slices.Equal(got, []string{check.want}) {
			t.Errorf("%s after the repoint = %v; want %q", check.what, got, check.want)
		}
	}
	world.rootPath = "/media/moved"
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedDirectory("Arkon (1979)"), listedFile("Arkon (1979)/Arkon.mkv", int64(len("arkon")), 1),
	}, true)
	if !slices.ContainsFunc(taken.ToExamine, func(file nodeproto.ExamineFile) bool {
		return file.Path.Text == "Arkon (1979)/Arkon.mkv"
	}) {
		t.Errorf("to examine = %+v; want the unconfirmed file read again, though listed as stored", taken.ToExamine)
	}
	world.scan([]nodeproto.ScanRecord{observedRecord("Arkon (1979)/Arkon.mkv", "arkon")}, true)
	if got := storetest.QueryRaw(t, path, "SELECT is_unconfirmed FROM place"); !slices.Equal(got, []string{"0"}) {
		t.Errorf("the place after the scan read it = %v; want it confirmed", got)
	}
}

// repointWorld is a library, Films, over root on node "local", with a/a.mkv and a/b.mkv scanned and
// b.mkv removed by a later scan, and an admin whose nodes list every folder as readable.
func repointWorld(t *testing.T) (*harness, *fakeNodes, *Admin) {
	t.Helper()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("a/a.mkv", "a"), observedRecord("a/b.mkv", "b")}, true)
	world.scan([]nodeproto.ScanRecord{observedRecord("a/a.mkv", "a")}, true)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	admin.ListFoldersThrough(readableFolders())
	return world, nodes, admin
}

// A repoint moves every place at or under the folder, live and removed, and every library folder at
// or under it, forgets the old folder's scan state, tells the nodes their watched folders changed and
// asks for a scan of the new place (D429 §2; repoint-library.md, "The spec").
func TestRepoint_MovesEveryPlaceLiveAndRemovedAndForgetsTheOldFoldersState(t *testing.T) {
	t.Parallel()
	world, nodes, admin := repointWorld(t)
	path := world.database.Path()
	rootStates := storetest.QueryRaw(t, path, "SELECT count(*) FROM root_state WHERE root_path = '"+root+"'")
	if slices.Equal(rootStates, []string{"0"}) {
		t.Fatalf("no state for %s before the repoint; the test cannot show it forgotten", root)
	}
	repointed, err := admin.Repoint(t.Context(), localMove(root, "/media/new"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if repointed.RemovedPlaceCount != 1 || repointed.LivePlaceCount < 1 || repointed.RootCount != 1 {
		t.Errorf("repointed %+v; want b.mkv's removed place, the live ones and Films' folder", repointed)
	}
	checks := []struct{ what, query, want string }{
		{"places left at the old folder", "SELECT count(*) FROM place WHERE path LIKE '/media/films%'", "0"},
		{
			"the removed place",
			"SELECT count(*) FROM place WHERE removed_at IS NOT NULL AND path = '/media/new/a/b.mkv'", "1",
		},
		{"the live place", "SELECT count(*) FROM place WHERE removed_at IS NULL AND path = '/media/new/a/a.mkv'", "1"},
		{"Films' folder", "SELECT root_path FROM library_root", "/media/new"},
		{"the old folder's state", "SELECT count(*) FROM root_state WHERE root_path = '" + root + "'", "0"},
		{"the old folder's last scan", "SELECT count(*) FROM root_scan WHERE root_path = '" + root + "'", "0"},
	}
	for _, check := range checks {
		if got := storetest.QueryRaw(t, path, check.query); !slices.Equal(got, []string{check.want}) {
			t.Errorf("%s after the repoint = %v; want %q", check.what, got, check.want)
		}
	}
	if !slices.Equal(nodes.scanned, []string{"/media/new"}) || nodes.watchedFoldersChanges != 1 {
		t.Errorf("asked to scan %v, told of watched folders %d times; want the new folder, once",
			nodes.scanned, nodes.watchedFoldersChanges)
	}
}

// A folder moved into a folder inside itself goes there whole, though a file's new path is another's
// old one until that one moves too: every place goes aside first, as SQLite checks a unique index row
// by row (repoint-library.md, "The spec").
func TestRepoint_AFolderMovedInsideItselfGoesAsideFirst(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("sub/f.mkv", "a"), observedRecord("sub/sub/f.mkv", "b")}, true)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	admin.ListFoldersThrough(readableFolders())
	if _, err := admin.Repoint(t.Context(), localMove(root+"/sub", root+"/sub/sub"), 1); err != nil {
		t.Fatalf("a folder moved into one inside itself: %v", err)
	}
	got := storetest.QueryRaw(t, world.database.Path(), "SELECT path FROM place WHERE path LIKE '%f.mkv' ORDER BY path")
	if want := []string{root + "/sub/sub/f.mkv", root + "/sub/sub/sub/f.mkv"}; !slices.Equal(got, want) {
		t.Errorf("the files are at %v; want %v", got, want)
	}
}

// A repoint never merges into what is known at the new folder: a place or a library folder there
// refuses it, and the dry run shows the conflict, its count and examples, before anyone confirms (the
// owner, 2026-11-06). Nothing moves.
func TestRepoint_RefusesANewFolderWhereFilesAreAlreadyKnown(t *testing.T) {
	t.Parallel()
	world, _, admin := repointWorld(t)
	if _, err := admin.Create(t.Context(), showsRequest("local", "/media/shows")); err != nil {
		t.Fatal(err)
	}
	shows := []nodeproto.ScanRecord{observedRecord("s.mkv", "s")}
	world.scanOn(nodeproto.ScanBegin{RootPath: "/media/shows"}, shows, true)
	preview, err := admin.PreviewRepoint(t.Context(), localMove(root, "/media/shows"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.Refusals, RefusedTargetHoldsKnown) || preview.ConflictCount != 2 ||
		!slices.Contains(preview.ConflictExamples, "/media/shows/s.mkv") ||
		!slices.Contains(preview.ConflictExamples, "/media/shows") {
		t.Errorf("the dry run = %+v; want refused, with Shows' folder and s.mkv in the way", preview)
	}
	_, err = admin.Repoint(t.Context(), localMove(root, "/media/shows"), 1)
	if !slices.Contains(refusalsOf(err), RefusedTargetHoldsKnown) {
		t.Errorf("the repoint: %v; want it refused, the target holding known files", err)
	}
	got := storetest.QueryRaw(t, world.database.Path(), "SELECT count(*) FROM place WHERE path LIKE '"+root+"/%'")
	if slices.Equal(got, []string{"0"}) {
		t.Errorf("Films' places moved, though the repoint was refused")
	}
}

// A repoint is refused, and its dry run says why, for a folder under which nothing is known, a folder
// said to be where it is, a new folder its node cannot read, and a scan in progress at, under or
// above either folder (repoint-library.md, "The spec").
func TestRepoint_RefusesWhatTheDryRunSaysIsRefused(t *testing.T) {
	t.Parallel()
	for _, refused := range []struct {
		what    string
		move    FolderMove
		listing nodeproto.ProbeState
		scan    string
		want    RepointRefusal
	}{
		{"nothing known", localMove("/media/empty", "/media/new"), nodeproto.ProbeReadable, "", RefusedNothingToMove},
		{"the same folder", localMove(root, root), nodeproto.ProbeReadable, "", RefusedSameFolder},
		{"a missing new folder", localMove(root, "/media/new"), nodeproto.ProbeMissing, "", RefusedTargetNotReadable},
		{"a scan of the folder", localMove(root, "/media/new"), nodeproto.ProbeReadable, root, RefusedScanInProgress},
		{
			"a scan above the new folder", localMove(root, "/media/new/x"), nodeproto.ProbeReadable, "/media/new",
			RefusedScanInProgress,
		},
	} {
		t.Run(refused.what, func(t *testing.T) {
			t.Parallel()
			world, nodes, admin := repointWorld(t)
			admin.ListFoldersThrough(&fakeFolders{listing: nodeproto.FolderListing{State: refused.listing}})
			if refused.scan != "" {
				nodes.scanning = map[ScanRoot]ScanProgress{{NodeID: world.nodeID, RootPath: refused.scan}: {}}
			}
			preview, err := admin.PreviewRepoint(t.Context(), refused.move)
			if err != nil || !slices.Contains(preview.Refusals, refused.want) {
				t.Errorf("the dry run: %+v, %v; want %s", preview.Refusals, err, refused.want)
			}
			if _, err := admin.Repoint(t.Context(), refused.move, 1); !slices.Contains(refusalsOf(err), refused.want) {
				t.Errorf("the repoint: %v; want it refused: %s", err, refused.want)
			}
		})
	}
}

// The dry run says, for each library, how many live files it lists now and would not after the move,
// and the reverse: a folder moved out of Films into Shows' folder is Films' loss and Shows' gain.
func TestRepoint_TheDryRunSaysWhatEachLibraryGainsAndLoses(t *testing.T) {
	t.Parallel()
	world, _, admin := repointWorld(t)
	if _, err := admin.Create(t.Context(), showsRequest("local", "/media/shows")); err != nil {
		t.Fatal(err)
	}
	preview, err := admin.PreviewRepoint(t.Context(), localMove(root+"/a", "/media/shows/a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Libraries) != 2 || preview.Libraries[0].LibraryID != world.libraryID {
		t.Fatalf("the libraries changed: %+v; want Films and Shows", preview.Libraries)
	}
	films, shows := preview.Libraries[0], preview.Libraries[1]
	if films.LosesCount != preview.LivePlaceCount || films.GainsCount != 0 ||
		shows.GainsCount != preview.LivePlaceCount || shows.LosesCount != 0 || preview.LivePlaceCount == 0 {
		t.Errorf("Films %+v, Shows %+v, of %d live places; want Films to lose them all and Shows to gain them",
			films, shows, preview.LivePlaceCount)
	}
	if len(preview.Roots) != 0 || len(preview.Refusals) != 0 {
		t.Errorf("roots %+v, refusals %v; want no library folder moved and nothing refused",
			preview.Roots, preview.Refusals)
	}
}

// A folder moved to another node takes its places there, forgetting their device and inode, which
// name nothing on the new node, and the scan asked for is the new node's (D429 §2).
func TestRepoint_AFolderMovedToAnotherNodeForgetsWhereOnDiskItsFilesWere(t *testing.T) {
	t.Parallel()
	world, nodes, admin := repointWorld(t)
	path := world.database.Path()
	storetest.ExecRaw(t, path, "UPDATE place SET filesystem_device = 7, inode = id")
	otherID := librarytest.Node(t, world.database, "other")
	other := &fakeNodes{id: otherID, roots: []string{"/srv"}}
	nodes.others = map[string]*fakeNodes{"other": other}
	move := FolderMove{NodeName: "local", Path: root, NewNodeName: "other", NewPath: "/srv/films"}
	if _, err := admin.Repoint(t.Context(), move, 1); err != nil {
		t.Fatal(err)
	}
	checks := []struct{ what, query, want string }{
		{"places left on the old node", "SELECT count(*) FROM place WHERE node_id = " + itoa(world.nodeID), "0"},
		{
			"places that kept a device or inode",
			"SELECT count(*) FROM place WHERE filesystem_device IS NOT NULL OR inode IS NOT NULL", "0",
		},
		{"Films' folder", "SELECT node_id || ':' || root_path FROM library_root", itoa(otherID) + ":/srv/films"},
	}
	for _, check := range checks {
		if got := storetest.QueryRaw(t, path, check.query); !slices.Equal(got, []string{check.want}) {
			t.Errorf("%s after the repoint = %v; want %q", check.what, got, check.want)
		}
	}
	if !slices.Equal(other.scanned, []string{"/srv/films"}) {
		t.Errorf("asked the other node to scan %v; want the new folder", other.scanned)
	}
}

// A folder's places are moved by their bytes; a place whose text is under the folder but whose bytes
// are not, a folder spelled in another Unicode form, is another folder on disk, which stays, and the
// dry run counts it (repoint-library.md, "Raw and NFC prefixes").
func TestRepoint_ThePlacesSpelledLikeTheFolderInAnotherFormStayAndAreCounted(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	const nfc, nfd = "Café", "Café"
	spelledApart := observedRecord(nfc+"/b.mkv", "b")
	spelledApart.Observed.Path.Raw = []byte(nfd + "/b.mkv")
	world.scan([]nodeproto.ScanRecord{observedRecord(nfc+"/a.mkv", "a"), spelledApart}, true)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	admin.ListFoldersThrough(readableFolders())
	preview, err := admin.PreviewRepoint(t.Context(), localMove(root+"/"+nfc, "/media/new"))
	if err != nil {
		t.Fatal(err)
	}
	if preview.LookAlikeCount != 1 || preview.LivePlaceCount != 1 {
		t.Errorf("look-alikes %d, live places %d; want b.mkv's folder counted apart and a.mkv moving",
			preview.LookAlikeCount, preview.LivePlaceCount)
	}
}
