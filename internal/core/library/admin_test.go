// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// fakeNodes is the node channel with one connected node, "local", that named its roots, and any others.
type fakeNodes struct {
	id    int64
	roots []string
	// others are other connected nodes by name, each with roots, and what was asked to scan on them.
	others map[string]*fakeNodes
	// scanned are the roots a person asked to scan.
	scanned []string
	// scopes are the folders a person asked to scan, as root:folder.
	scopes []string
	// readAgain are what scans asked for read again, as root:path, each path its own.
	readAgain []string
	// scanning are the scans in progress.
	scanning map[ScanRoot]ScanProgress
	// removed are the nodes an admin retired.
	removed []int64
	// database is where a retired node is removed, as the node service does, at clock's time.
	database *store.Database
	clock    clock.Clock
	// isAway is whether the node is not connected.
	isAway bool
	// watches are the node's library folders' watches, as its status said.
	watches map[string]nodeproto.RootWatch
	// watchedFoldersChanges are how many times the nodes were told their watched folders changed.
	watchedFoldersChanges int
}

func (nodes *fakeNodes) RootWatches(int64) map[string]nodeproto.RootWatch { return nodes.watches }

func (nodes *fakeNodes) WatchedFoldersChanged() { nodes.watchedFoldersChanges++ }

// NodeConnection is the node's: connected unless it is away.
func (nodes *fakeNodes) NodeConnection(context.Context, int64) (corenodes.Connection, time.Time) {
	if nodes.isAway {
		return corenodes.Silent, time.Time{}
	}
	return corenodes.Connected, time.Time{}
}

func (nodes *fakeNodes) RemoveNode(ctx context.Context, nodeID int64) error {
	nodes.removed = append(nodes.removed, nodeID)
	if err := corenodes.Remove(ctx, nodes.database, nodes.clock, nodeID); err != nil {
		return fmt.Errorf("remove node %d: %w", nodeID, err)
	}
	return nil
}

// fakeWork is the job queue, keeping the work each retire asked it to cancel.
type fakeWork struct {
	cancelled []jobs.RetiredWork
}

// retirementOf is what a retire asks of other packages, with work as the job queue.
func retirementOf(work *fakeWork) Retirement { return Retirement{Work: work} }

func (work *fakeWork) CancelRetired(_ context.Context, retired jobs.RetiredWork) (int, error) {
	work.cancelled = append(work.cancelled, retired)
	return len(retired.FileIDs), nil
}

func (nodes *fakeNodes) ConnectedNodeRoots(_ context.Context, name string) (int64, []string, bool) {
	if other, isOther := nodes.others[name]; isOther {
		return other.id, other.roots, true
	}
	if name != "local" {
		return 0, nil, false
	}
	return nodes.id, nodes.roots, true
}

func (nodes *fakeNodes) InProgress(context.Context) (map[ScanRoot]ScanProgress, error) {
	return nodes.scanning, nil
}

func (nodes *fakeNodes) Request(ctx context.Context, nodeID int64, root string, priority int64) error {
	for _, other := range nodes.others {
		if other.id == nodeID {
			return other.Request(ctx, nodeID, root, priority)
		}
	}
	if nodeID == nodes.id && priority == jobs.PriorityPersonsRequest {
		nodes.scanned = append(nodes.scanned, root)
	}
	return nil
}

func (nodes *fakeNodes) RequestUnder(_ context.Context, nodeID int64, root, scope string, priority int64) error {
	if nodeID == nodes.id && priority == jobs.PriorityPersonsRequest {
		nodes.scopes = append(nodes.scopes, root+":"+scope)
	}
	return nil
}

func (nodes *fakeNodes) RequestReadingAgain(
	ctx context.Context, nodeID int64, root, scope string, readAgain ReadAgain, priority int64,
) error {
	if nodeID == nodes.id && priority == jobs.PriorityPersonsRequest {
		for _, path := range readAgain {
			nodes.readAgain = append(nodes.readAgain, root+":"+path)
		}
	}
	return nodes.RequestUnder(ctx, nodeID, root, scope, priority)
}

func showsRequest(node string, roots ...string) CreateRequest {
	return CreateRequest{NodeName: node, NewLibrary: NewLibrary{
		Name: "Shows", Kind: "series", MetadataSource: "tmdb", ChangeDetection: "manual", Roots: roots,
	}}
}

func TestAdmin_CreatesALibraryOverTheNodesRootsAndScansIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root, "/media/shows"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	_, absent := admin.Create(t.Context(), showsRequest("elsewhere", "/media/shows"))
	if !errors.Is(absent, ErrNodeNotConnected) {
		t.Errorf("a node that has not said hello: %v, want ErrNodeNotConnected", absent)
	}
	if _, err := admin.Create(t.Context(), showsRequest("local", "/etc")); !errors.Is(err, ErrNotANodeRoot) {
		t.Errorf("a root the node did not configure: %v, want ErrNotANodeRoot (D107)", err)
	}
	if len(nodes.scanned) != 0 {
		t.Fatalf("a refused request asked for scans of %v", nodes.scanned)
	}
	if _, err := admin.Create(t.Context(), showsRequest("local", "/media/shows")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(nodes.scanned, []string{"/media/shows"}) {
		t.Errorf("scans asked for of %v, want the new library's root, as a person's request", nodes.scanned)
	}
	libraries, err := admin.List(t.Context(), auth.EveryLibrary())
	if err != nil || len(libraries) != 2 || libraries[1].Name != "Shows" || len(libraries[1].Roots) != 1 {
		t.Errorf("List = %+v, %v", libraries, err)
	}
}

func TestAdmin_ListsEachLibrarysRootStateAndFilings(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("a.mkv", "a"), skipRecord("notes.txt", nodeproto.SkipExtension),
	}, true)
	watch := nodeproto.RootWatch{RootPath: root, Cause: nodeproto.WatchCauseWatchLimit, Reason: "too many folders"}
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root}, watches: map[string]nodeproto.RootWatch{root: watch}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	libraries, err := admin.List(t.Context(), auth.EveryLibrary())
	if err != nil || len(libraries) != 1 {
		t.Fatalf("List = %+v, %v", libraries, err)
	}
	films := libraries[0]
	if len(films.Roots) != 1 || films.Roots[0].LastScanOutcome == nil || *films.Roots[0].LastScanOutcome != "complete" {
		t.Errorf("roots %+v, want the root complete", films.Roots)
	}
	if got := films.Roots[0].Watch; got == nil || *got != watch {
		t.Errorf("the root's watch is %+v, want %+v, as its node's status said (D272 §9)", got, watch)
	}
	if films.FileCountByFiling["skipped"] != 1 || len(films.FileCountByFiling) != 2 {
		t.Errorf("filings %v, want one skipped and one other", films.FileCountByFiling)
	}
}

// A root's scan in progress is listed with it, how far it is and whether one more follows (D236).
func TestAdmin_ListsARootsScanInProgress(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	progress := ScanProgress{
		IsListed: true, ListedCount: 40, ToExamineCount: 3, ExaminedCount: 1, IsAnotherScanQueued: true,
	}
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root}, scanning: map[ScanRoot]ScanProgress{
		{NodeID: world.nodeID, RootPath: root}: progress, {NodeID: world.nodeID, RootPath: "/media/other"}: {},
	}}
	libraries, err := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{})).
		List(t.Context(), auth.EveryLibrary())
	if err != nil || len(libraries) != 1 || len(libraries[0].Roots) != 1 {
		t.Fatalf("List = %+v, %v", libraries, err)
	}
	if got := libraries[0].Roots[0].Scan; got == nil || *got != progress {
		t.Errorf("the root's scan is %+v, want %+v", got, progress)
	}
	nodes.scanning = nil
	libraries, err = NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{})).
		List(t.Context(), auth.EveryLibrary())
	if err != nil || libraries[0].Roots[0].Scan != nil {
		t.Errorf("with no scan in progress the root lists %+v (%v), want none", libraries[0].Roots[0].Scan, err)
	}
}

func TestAdmin_ScanAsksEachConnectedRootsNode(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	libraries, err := admin.List(t.Context(), auth.EveryLibrary())
	if err != nil {
		t.Fatal(err)
	}
	asked, err := admin.Scan(t.Context(), libraries[0].ID)
	if err != nil || asked != 1 || !slices.Equal(nodes.scanned, []string{root}) {
		t.Errorf("Scan = %d, %v; asked for scans of %v", asked, err, nodes.scanned)
	}
	if _, err := admin.Scan(t.Context(), 999); !errors.Is(err, ErrNoLibrary) {
		t.Errorf("Scan(999) = %v, want ErrNoLibrary", err)
	}
}

func TestAdmin_FilesPagesByIDAndFilters(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("a.mkv", "a"), observedRecord("b.mkv", "b"), skipRecord("notes.txt", nodeproto.SkipExtension),
	}, true)
	admin := NewAdmin(world.database, world.clock, &fakeNodes{}, &fakeNodes{}, retirementOf(&fakeWork{}))
	first, err := admin.Files(t.Context(), FilesQuery{Limit: 2})
	if err != nil || len(first.Files) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	rest, err := admin.Files(t.Context(), FilesQuery{Cursor: first.NextCursor, Limit: 2})
	if err != nil || len(rest.Files) != 1 || rest.NextCursor != "" {
		t.Errorf("second page = %+v, %v", rest, err)
	}
	skipped := "skipped"
	only, err := admin.Files(t.Context(), FilesQuery{Filing: &skipped, Limit: 10})
	if err != nil || len(only.Files) != 1 || only.Files[0].Filing != skipped {
		t.Errorf("filing=skipped = %+v, %v", only, err)
	}
	if _, err = admin.Files(t.Context(), FilesQuery{Cursor: "12", Limit: 2}); !errors.Is(err, ErrBadCursor) {
		t.Errorf("a cursor this server did not give: %v, want ErrBadCursor", err)
	}
}
