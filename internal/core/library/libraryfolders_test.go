// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A library folder is one of its node's configured roots or a folder inside one, by a clean path; any
// other is refused, and the refusal logged at warn (D107, D359 §1, §5).
func TestAdmin_ALibraryFolderIsAConfiguredRootOrAFolderInsideOneByACleanPath(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	logged := &testutil.Log{}
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	admin.UseLogger(slog.New(slog.NewTextHandler(logged, nil)))
	for _, outside := range []string{
		"/etc", "/mediaX/shows", "/media/../etc", "/media/shows/", "/media//shows", "/media/./shows", "media/shows",
	} {
		if _, err := admin.Create(t.Context(), showsRequest("local", outside)); !errors.Is(err, ErrNotANodeRoot) {
			t.Errorf("a new library over %s: %v, want ErrNotANodeRoot (D107)", outside, err)
		}
	}
	logged.AwaitLine(t, "refused a library folder that no configured root of its node holds", "path=/media/../etc")
	if len(nodes.scanned) != 0 {
		t.Errorf("refused requests asked for scans of %v", nodes.scanned)
	}
	if _, err := admin.Create(t.Context(), showsRequest("local", "/media/video/shows")); err != nil {
		t.Errorf("a new library over a folder inside a configured root: %v", err)
	}
}

// The folders a library had when they were the configured roots keep working once the configured
// roots are their parents, each with its own watch, as its node's status says it, never the
// configured root's (D272 §9, D359 §4 and its amendment of 2026-10-27).
func TestAdmin_AFolderInsideAConfiguredRootIsListedWithItsOwnWatch(t *testing.T) {
	t.Parallel()
	world := newHarness(t) // its library, Films, has /media/films, once a configured root
	rootsWatch := nodeproto.RootWatch{RootPath: "/media", Cause: nodeproto.WatchCauseNetworkFilesystem}
	watch := nodeproto.RootWatch{RootPath: root, IsWatching: true}
	nodes := &fakeNodes{
		id: world.nodeID, roots: []string{"/media"},
		watches: map[string]nodeproto.RootWatch{"/media": rootsWatch, root: watch},
	}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	libraries, err := admin.List(t.Context(), auth.EveryLibrary())
	if err != nil || len(libraries) != 1 || len(libraries[0].Roots) != 1 || libraries[0].Roots[0].RootPath != root {
		t.Fatalf("List = %+v, %v; want Films over %s", libraries, err, root)
	}
	if got := libraries[0].Roots[0].Watch; got == nil || *got != watch {
		t.Errorf("a folder inside /media is watched %+v; want its own watch, %+v", got, watch)
	}
	delete(nodes.watches, root)
	libraries, err = admin.List(t.Context(), auth.EveryLibrary())
	if err != nil || libraries[0].Roots[0].Watch != nil {
		t.Errorf("a folder its node said nothing of is watched %+v, %v; want no watch, not /media's",
			libraries[0].Roots[0].Watch, err)
	}
}

// Libraries may overlap: a library's folder may be at, above or below another library's (D363 §1).
// Within one library, a folder beneath another of its folders adds nothing, refused as covered and
// logged, and one above some of its folders takes their place; a new library's folders beneath others
// of its own are left out. A folder added is scanned (D359 §3).
func TestAdmin_LibrariesMayOverlapAndWithinOneAFolderBeneathAnotherAddsNothing(t *testing.T) {
	t.Parallel()
	world := newHarness(t) // its library, Films, has /media/films
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	logged := &testutil.Log{}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	admin.UseLogger(slog.New(slog.NewTextHandler(logged, nil)))
	for index, overlapping := range []string{"/media/films", "/media/films/Wing", "/media"} {
		request := showsRequest("local", overlapping)
		request.Name = fmt.Sprintf("Shows %d", index)
		if _, err := admin.Create(t.Context(), request); err != nil {
			t.Errorf("a new library over %s, at, below or above Films' /media/films: %v, want it made",
				overlapping, err)
		}
	}
	nested := showsRequest("local", "/media/video/shows/Dark", "/media/video/shows")
	shows, createErr := admin.Create(t.Context(), nested)
	if createErr != nil {
		t.Fatal(createErr)
	}
	logged.AwaitLine(t, "a new library's folder beneath another of its folders adds nothing",
		"path=/media/video/shows/Dark")
	if err := admin.AddRoot(t.Context(), shows, "local", "/media/video/anime/Nosferatu"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]error{
		"/media/video/shows": ErrRootCovered, "/media/video/shows/Dark": ErrRootCovered, "/srv": ErrNotANodeRoot,
	} {
		if err := admin.AddRoot(t.Context(), shows, "local", path); !errors.Is(err, want) {
			t.Errorf("adding %s: %v, want %v", path, err, want)
		}
	}
	logged.AwaitLine(t, "refused a library folder its library covers already", "path=/media/video/shows/Dark")
	if err := admin.AddRoot(t.Context(), shows, "local", "/media/video/anime"); err != nil {
		t.Fatal(err)
	}
	roots, err := admin.libraryRoots(t.Context(), shows)
	var paths []string
	for _, each := range roots {
		paths = append(paths, each.RootPath)
	}
	if want := []string{"/media/video/anime", "/media/video/shows"}; err != nil || !slices.Equal(paths, want) {
		t.Errorf("the library's folders are %v, %v; want %v: the parent took Nosferatu's place", paths, err, want)
	}
	if err := admin.AddRoot(t.Context(), shows+100, "local", "/media/music"); !errors.Is(err, ErrNoLibrary) {
		t.Errorf("adding to a library that does not exist: %v, want ErrNoLibrary", err)
	}
}

// A folder a repoint moves to may be inside a configured root, inside the folder it moves from, which
// covers nothing once it has moved, and inside another library's folder (D241 §6, D359 §3, D363 §1);
// not inside or above another folder of its own library.
func TestRepointRoot_MovesAFolderToAFolderInsideAConfiguredRoot(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	if _, err := admin.Create(t.Context(), showsRequest("local", "/media/shows")); err != nil {
		t.Fatal(err)
	}
	libraries, listErr := admin.List(t.Context(), auth.EveryLibrary())
	if listErr != nil || len(libraries) != 2 {
		t.Fatalf("List = %+v, %v", libraries, listErr)
	}
	libraryID := libraries[0].ID // Films
	if err := admin.AddRoot(t.Context(), libraryID, "local", "/media/more"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.RepointRoot(t.Context(), Repoint{
		LibraryID: libraryID, NodeName: "local", Path: root, NewNodeName: "local", NewPath: "/media/more/films",
	}); !errors.Is(err, ErrRootCovered) {
		t.Errorf("a repoint into another of its library's folders: %v, want ErrRootCovered", err)
	}
	if _, err := admin.RepointRoot(t.Context(), Repoint{
		LibraryID: libraryID, NodeName: "local", Path: root, NewNodeName: "local", NewPath: "/media/films/all",
	}); err != nil {
		t.Errorf("a repoint into a folder inside the one it moves from: %v, want it moved", err)
	}
	if _, err := admin.RepointRoot(t.Context(), Repoint{
		LibraryID: libraryID, NodeName: "local", Path: "/media/films/all", NewNodeName: "local",
		NewPath: "/media/shows/films",
	}); err != nil {
		t.Errorf("a repoint into Shows' folder: %v, want it moved: libraries may overlap", err)
	}
}

// fakeFolders lists a node's folders as its byte socket would, or waits as a share that has gone away
// does.
type fakeFolders struct {
	listing nodeproto.FolderListing
	isGone  bool
	asked   []string
}

func (folders *fakeFolders) ListFolders(ctx context.Context, _ int64, path string) (nodeproto.FolderListing, error) {
	folders.asked = append(folders.asked, path)
	if folders.isGone {
		<-ctx.Done()
		return nodeproto.FolderListing{}, ctx.Err()
	}
	return folders.listing, nil
}

// The folders in a folder are asked of its node only for a folder a configured root holds, a refusal
// logged at warn; each listing is logged at debug with its node, path, count and time; a node
// that does not answer within folderListingWait is not answering (D359 §2, §5).
func TestAdmin_ListsANodesFoldersInsideItsRootsWithinTheWait(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	logged := &testutil.Log{}
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	admin.UseLogger(slog.New(slog.NewTextHandler(logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	folders := &fakeFolders{listing: nodeproto.FolderListing{
		Path: "/media/video", State: nodeproto.ProbeReadable, Folders: []string{"movies", "shows"},
	}}
	admin.ListFoldersThrough(folders)
	listing, listErr := admin.Folders(t.Context(), "local", "/media/video")
	if listErr != nil || !slices.Equal(listing.Folders, []string{"movies", "shows"}) {
		t.Errorf("the folders in /media/video: %+v, %v", listing, listErr)
	}
	logged.AwaitLine(t, "listed the folders in a folder on a node", "nodeName=local", "path=/media/video",
		"folderCount=2", "tookMs=")
	for _, outside := range []string{"/etc", "/media/../etc"} {
		if _, err := admin.Folders(t.Context(), "local", outside); !errors.Is(err, ErrNotANodeRoot) {
			t.Errorf("the folders in %s: %v, want ErrNotANodeRoot", outside, err)
		}
	}
	if _, err := admin.Folders(t.Context(), "away", "/media"); !errors.Is(err, ErrNodeNotConnected) {
		t.Errorf("a node not connected: %v, want ErrNodeNotConnected", err)
	}
	if !slices.Equal(folders.asked, []string{"/media/video"}) {
		t.Errorf("the node was asked for %v; want only the folder inside its root", folders.asked)
	}
	logged.AwaitLine(t, "refused a library folder that no configured root of its node holds", "path=/etc")
	folders.isGone = true
	admin.folderListingWait = time.Millisecond // the fake waits until the wait ends it
	gone, err := admin.Folders(t.Context(), "local", "/media/video")
	if err != nil || gone.State != nodeproto.ProbeNotAnswering {
		t.Errorf("a share that has gone away: %+v, %v; want not answering", gone, err)
	}
}

// A node watches each live folder of each live library on it that watches for changes, and nothing
// else; every change of a library's folders, or of whether it watches, tells the nodes at once (D272
// §9, D359's amendment of 2026-10-27).
func TestScanSchedule_ANodeWatchesTheFoldersOfItsWatchedLibrariesAndEachChangeIsSaid(t *testing.T) {
	t.Parallel()
	world := newHarness(t) // Films, over /media/films, which watches nothing
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	schedule := NewScanSchedule(world.database, &scheduledRequests{}, world.settings, world.clock,
		slog.New(slog.DiscardHandler))
	watched := func(t *testing.T, want ...string) {
		t.Helper()
		got, err := schedule.WatchedFolders(t.Context(), world.nodeID)
		if err != nil || !slices.Equal(got, append([]string{}, want...)) {
			t.Errorf("the node watches %v, %v; want %v", got, err, want)
		}
		if other, err := schedule.WatchedFolders(t.Context(), world.nodeID+1); err != nil || len(other) != 0 {
			t.Errorf("another node watches %v, %v; want nothing", other, err)
		}
	}
	said := func(t *testing.T, what string) {
		t.Helper()
		if nodes.watchedFoldersChanges != 1 {
			t.Errorf("%s told the nodes %d times their folders changed; want once", what, nodes.watchedFoldersChanges)
		}
		nodes.watchedFoldersChanges = 0
	}
	watched(t)
	libraries, err := admin.List(t.Context(), auth.EveryLibrary())
	if err != nil || len(libraries) != 1 {
		t.Fatalf("List = %+v, %v; want Films", libraries, err)
	}
	films, shows := libraries[0].ID, int64(0)
	ctx := t.Context()
	for _, step := range []struct {
		what   string
		change func() error
		want   []string
	}{
		{"a new library", func() error {
			request := showsRequest("local", "/media/video/shows")
			request.ChangeDetection = "watch"
			var createErr error
			shows, createErr = admin.Create(ctx, request)
			return createErr
		}, []string{"/media/video/shows"}},
		{"a folder added", func() error {
			return admin.AddRoot(ctx, shows, "local", "/media/video/anime")
		}, []string{"/media/video/anime", "/media/video/shows"}},
		{"a folder moved", func() error {
			_, repointErr := admin.RepointRoot(ctx, Repoint{
				LibraryID: shows, NodeName: "local", Path: "/media/video/anime", NewNodeName: "local",
				NewPath: "/media/anime",
			})
			return repointErr
		}, []string{"/media/anime", "/media/video/shows"}},
		{"a folder retired", func() error {
			_, retireErr := admin.RetireRoot(ctx, shows, "local", "/media/anime")
			return retireErr
		}, []string{"/media/video/shows"}},
		{"a library set to watch", func() error {
			return admin.SetChangeDetection(ctx, films, "watch", nil)
		}, []string{root, "/media/video/shows"}},
		{"a library deleted", func() error {
			_, deleteErr := admin.Delete(ctx, shows)
			return deleteErr
		}, []string{root}},
	} {
		if changeErr := step.change(); changeErr != nil {
			t.Fatalf("%s: %v", step.what, changeErr)
		}
		said(t, step.what)
		watched(t, step.want...)
	}
}
