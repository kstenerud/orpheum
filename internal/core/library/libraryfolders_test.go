// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
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
// roots are their parents, a folder inside one watched as its root is (D272 §9, D359 §4).
func TestAdmin_AFolderInsideAConfiguredRootIsListedAndWatchedAsItsRootIs(t *testing.T) {
	t.Parallel()
	world := newHarness(t) // its library, Films, has /media/films, once a configured root
	watch := nodeproto.RootWatch{RootPath: "/media", Cause: nodeproto.WatchCauseNetworkFilesystem}
	nodes := &fakeNodes{
		id: world.nodeID, roots: []string{"/media"}, watches: map[string]nodeproto.RootWatch{"/media": watch},
	}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	libraries, err := admin.List(t.Context(), auth.EveryLibrary())
	if err != nil || len(libraries) != 1 || len(libraries[0].Roots) != 1 || libraries[0].Roots[0].RootPath != root {
		t.Fatalf("List = %+v, %v; want Films over %s", libraries, err, root)
	}
	if got := libraries[0].Roots[0].Watch; got == nil || *got != watch {
		t.Errorf("a folder inside /media is watched %+v; want /media's watch, %+v", got, watch)
	}
}

// No library's folder is at, above or below another's on its node: a new library's and an added one
// alike; a folder is added to a library and scanned (D359 §1, §3).
func TestAdmin_NoLibraryFolderIsAtAboveOrBelowAnothersOnItsNode(t *testing.T) {
	t.Parallel()
	world := newHarness(t) // its library, Films, has /media/films
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	for _, covered := range []string{"/media/films", "/media/films/Wing", "/media"} {
		if _, err := admin.Create(t.Context(), showsRequest("local", covered)); !errors.Is(err, ErrRootCovered) {
			t.Errorf("a new library over %s, at, below or above Films' /media/films: %v, want ErrRootCovered",
				covered, err)
		}
	}
	overlapping := showsRequest("local", "/media/video/shows", "/media/video/shows/Dark")
	if _, err := admin.Create(t.Context(), overlapping); !errors.Is(err, ErrRootCovered) {
		t.Errorf("a new library over a folder and one inside it: %v, want ErrRootCovered", err)
	}
	if len(nodes.scanned) != 0 {
		t.Fatalf("refused requests asked for scans of %v", nodes.scanned)
	}
	shows, createErr := admin.Create(t.Context(), showsRequest("local", "/media/video/shows"))
	if createErr != nil {
		t.Fatal(createErr)
	}
	if err := admin.AddRoot(t.Context(), shows, "local", "/media/video/anime"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/media/video/shows", "/media/video/anime"}; !slices.Equal(nodes.scanned, want) {
		t.Errorf("scans asked for of %v, want %v: the new library's folder, then the one added", nodes.scanned, want)
	}
	for path, want := range map[string]error{
		"/media/video": ErrRootCovered, "/media/video/anime/Nosferatu": ErrRootCovered, "/srv": ErrNotANodeRoot,
	} {
		if err := admin.AddRoot(t.Context(), shows, "local", path); !errors.Is(err, want) {
			t.Errorf("adding %s: %v, want %v", path, err, want)
		}
	}
	if err := admin.AddRoot(t.Context(), shows+100, "local", "/media/music"); !errors.Is(err, ErrNoLibrary) {
		t.Errorf("adding to a library that does not exist: %v, want ErrNoLibrary", err)
	}
}

// A folder a repoint moves to may be inside a configured root, and inside the folder it moves from,
// which covers nothing once it has moved; not one another library covers (D241 §6, D359 §3).
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
	if _, err := admin.RepointRoot(t.Context(), Repoint{
		LibraryID: libraryID, NodeName: "local", Path: root, NewNodeName: "local", NewPath: "/media/shows/films",
	}); !errors.Is(err, ErrRootCovered) {
		t.Errorf("a repoint into Shows' folder: %v, want ErrRootCovered", err)
	}
	if _, err := admin.RepointRoot(t.Context(), Repoint{
		LibraryID: libraryID, NodeName: "local", Path: root, NewNodeName: "local", NewPath: "/media/films/all",
	}); err != nil {
		t.Errorf("a repoint into a folder inside the one it moves from: %v, want it moved", err)
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
