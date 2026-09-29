// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeNodes is the node channel with one connected node, "local", that named its roots.
type fakeNodes struct {
	id     int64
	roots  []string
	queued []nodeproto.Command
}

func (nodes *fakeNodes) Roots(name string) (int64, []string, bool) {
	if name != "local" {
		return 0, nil, false
	}
	return nodes.id, nodes.roots, true
}

func (nodes *fakeNodes) Enqueue(nodeID int64, command nodeproto.Command) {
	if nodeID == nodes.id {
		nodes.queued = append(nodes.queued, command)
	}
}

func scannedRoots(commands []nodeproto.Command) []string {
	var roots []string
	for _, command := range commands {
		if command.Type == nodeproto.CommandScan {
			roots = append(roots, command.Scan.Root)
		}
	}
	return roots
}

func showsRequest(node string, roots ...string) Request {
	return Request{Node: node, NewLibrary: NewLibrary{
		Name: "Shows", Kind: "shows", MetadataSource: "tmdb", ChangeDetection: "manual", Roots: roots,
	}}
}

func TestAdmin_CreatesALibraryOverTheNodesRootsAndScansIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root, "/media/shows"}}
	admin := NewAdmin(world.database, world.clock, nodes)
	_, absent := admin.Create(t.Context(), showsRequest("elsewhere", "/media/shows"))
	if !errors.Is(absent, ErrNodeNotConnected) {
		t.Errorf("a node that has not said hello: %v, want ErrNodeNotConnected", absent)
	}
	if _, err := admin.Create(t.Context(), showsRequest("local", "/etc")); !errors.Is(err, ErrNotANodeRoot) {
		t.Errorf("a root the node did not configure: %v, want ErrNotANodeRoot (D107)", err)
	}
	if len(nodes.queued) != 0 {
		t.Fatalf("a refused request queued %v", nodes.queued)
	}
	if _, err := admin.Create(t.Context(), showsRequest("local", "/media/shows")); err != nil {
		t.Fatal(err)
	}
	if got := scannedRoots(nodes.queued); !slices.Equal(got, []string{"/media/shows"}) {
		t.Errorf("scans queued for %v, want the new library's root", got)
	}
	libraries, err := admin.List(t.Context())
	if err != nil || len(libraries) != 2 || libraries[1].Name != "Shows" || len(libraries[1].Roots) != 1 {
		t.Errorf("List = %+v, %v", libraries, err)
	}
}

func TestAdmin_ListsEachLibrarysRootStateAndFilings(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.Record{observed("a.mkv", "a"), skipped("notes.txt", nodeproto.SkipExtension)}, true)
	admin := NewAdmin(world.database, world.clock, &fakeNodes{id: world.nodeID, roots: []string{root}})
	libraries, err := admin.List(t.Context())
	if err != nil || len(libraries) != 1 {
		t.Fatalf("List = %+v, %v", libraries, err)
	}
	films := libraries[0]
	if len(films.Roots) != 1 || films.Roots[0].LastScanOutcome == nil || *films.Roots[0].LastScanOutcome != "complete" {
		t.Errorf("roots %+v, want the root complete", films.Roots)
	}
	if films.Filings["skipped"] != 1 || len(films.Filings) != 2 {
		t.Errorf("filings %v, want one skipped and one other", films.Filings)
	}
}

func TestAdmin_ScanAsksEachConnectedRootsNode(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root}}
	admin := NewAdmin(world.database, world.clock, nodes)
	libraries, err := admin.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	asked, err := admin.Scan(t.Context(), libraries[0].ID)
	if err != nil || asked != 1 || !slices.Equal(scannedRoots(nodes.queued), []string{root}) {
		t.Errorf("Scan = %d, %v; queued %v", asked, err, nodes.queued)
	}
	if _, err := admin.Scan(t.Context(), 999); !errors.Is(err, ErrNoLibrary) {
		t.Errorf("Scan(999) = %v, want ErrNoLibrary", err)
	}
}

func TestAdmin_FilesPagesByIDAndFilters(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.Record{
		observed("a.mkv", "a"), observed("b.mkv", "b"), skipped("notes.txt", nodeproto.SkipExtension),
	}, true)
	admin := NewAdmin(world.database, world.clock, &fakeNodes{})
	first, err := admin.Files(t.Context(), FilesQuery{Limit: 2})
	if err != nil || len(first) != 2 {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	rest, err := admin.Files(t.Context(), FilesQuery{After: first[1].ID, Limit: 2})
	if err != nil || len(rest) != 1 {
		t.Errorf("second page = %+v, %v", rest, err)
	}
	skipped := "skipped"
	only, err := admin.Files(t.Context(), FilesQuery{Filing: &skipped, Limit: 10})
	if err != nil || len(only) != 1 || only[0].Filing != skipped {
		t.Errorf("filing=skipped = %+v, %v", only, err)
	}
}
