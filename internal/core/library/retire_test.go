// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// withDisk is thirty films in the root, and two more on disk1, mounted inside it, when it is.
func withDisk(isMounted bool) []nodeproto.ScanRecord {
	var records []nodeproto.ScanRecord
	if isMounted {
		records = append(records, mountRecord("disk1", 21))
		records = append(records, inDirectory("disk1", filmRecords(2))...)
	}
	return append(records, filmRecords(30)...)
}

// retiringAdmin is the admin service over a harness, with its node channel, the work it
// cancels and its one library.
type retiringAdmin struct {
	*Admin
	nodes     *fakeNodes
	work      *fakeWork
	libraryID int64
}

func newRetiringAdmin(world *harness) retiringAdmin {
	world.t.Helper()
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root}, database: world.database, clock: world.clock}
	work := &fakeWork{}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(work))
	libraries, err := admin.List(world.t.Context())
	if err != nil || len(libraries) != 1 {
		world.t.Fatalf("List = %+v, %v", libraries, err)
	}
	return retiringAdmin{Admin: admin, nodes: nodes, work: work, libraryID: libraries[0].ID}
}

func TestRetireRoot_TombstonesItsFilesCancelsTheirWorkForgetsItsDisksAndScansItNoMore(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	admin := newRetiringAdmin(world)
	work, libraryID := admin.work, admin.libraryID
	world.scan(withDisk(true), true)
	world.scan(withDisk(false), true)
	if _, err := admin.RetireRoot(t.Context(), libraryID, "local", "/elsewhere"); !errors.Is(err, ErrNoSuchRoot) {
		t.Errorf("retiring a root the library lacks: %v, want ErrNoSuchRoot", err)
	}
	retired, err := admin.RetireRoot(t.Context(), libraryID, "local", root)
	if err != nil || retired.TombstonedFileCount != 32 {
		t.Fatalf("RetireRoot = %+v, %v; want all 32 files, the missing disk's too, removed", retired, err)
	}
	if live := len(world.liveFilesByPath()); live != 0 {
		t.Errorf("%d live files after the root was retired, want none", live)
	}
	if len(work.cancelled) != 1 || len(work.cancelled[0].FileIDs) != 32 || len(work.cancelled[0].StreamIDs) == 0 {
		t.Fatalf("the work cancelled: %+v; want the 32 files' and their streams'", work.cancelled)
	}
	wantDestinations := []jobs.Destination{
		jobs.RootDestination(world.nodeID, root), jobs.RootDestination(world.nodeID, root+"/disk1"),
	}
	if got := work.cancelled[0].Destinations; !slices.Equal(got, wantDestinations) {
		t.Errorf("destinations forgotten: %v, want the root's and its disk's %v", got, wantDestinations)
	}
	libraries, err := admin.List(t.Context())
	if err != nil || len(libraries) != 1 || len(libraries[0].Roots) != 0 {
		t.Errorf("libraries after the retire: %+v, %v; want the library with no roots", libraries, err)
	}
	world.scan(withDisk(true), true)
	if live := len(world.liveFilesByPath()); live != 0 {
		t.Errorf("%d live files after a scan of the retired root, want none: it is scanned no more", live)
	}
	if _, err := admin.RetireRoot(t.Context(), libraryID, "local", root); !errors.Is(err, ErrNoSuchRoot) {
		t.Errorf("retiring it again: %v, want ErrNoSuchRoot", err)
	}
}

func TestRetireNode_RetiresItsRootsCancelsItsWorkAndForgetsIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	admin := newRetiringAdmin(world)
	nodes, work := admin.nodes, admin.work
	world.scan(filmRecords(3), true)
	if _, err := admin.RetireNode(t.Context(), "elsewhere"); !errors.Is(err, ErrNoNode) {
		t.Errorf("retiring a node that is not enrolled: %v, want ErrNoNode", err)
	}
	retired, err := admin.RetireNode(t.Context(), "local")
	if err != nil || retired.TombstonedFileCount != 3 {
		t.Fatalf("RetireNode = %+v, %v; want its 3 files removed", retired, err)
	}
	if !slices.Equal(nodes.removed, []int64{world.nodeID}) {
		t.Errorf("nodes removed: %v, want the retired one", nodes.removed)
	}
	for _, want := range []jobs.Destination{
		jobs.RootDestination(world.nodeID, root), jobs.NodeDestination(world.nodeID),
		jobs.PreparedDirectoryDestination(world.nodeID),
	} {
		if !slices.Contains(work.cancelled[0].Destinations, want) {
			t.Errorf("destinations forgotten: %v, want %s among them", work.cancelled[0].Destinations, want)
		}
	}
	listed, err := admin.NodeRoots(t.Context())
	if err != nil || len(listed) != 0 {
		t.Errorf("nodes after the retire: %+v, %v; want none", listed, err)
	}
	if _, err := admin.RetireNode(t.Context(), "local"); !errors.Is(err, ErrNoNode) {
		t.Errorf("retiring it again: %v, want ErrNoNode", err)
	}
}

func TestDelete_RemovesTheLibraryItsRootsAndFilesAndCancelsItsWork(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	admin := newRetiringAdmin(world)
	work, libraryID := admin.work, admin.libraryID
	world.scan(filmRecords(3), true)
	retired, err := admin.Delete(t.Context(), libraryID)
	if err != nil || retired.TombstonedFileCount != 3 {
		t.Fatalf("Delete = %+v, %v; want its 3 files removed", retired, err)
	}
	if libraries, err := admin.List(t.Context()); err != nil || len(libraries) != 0 {
		t.Errorf("libraries after the delete: %+v, %v; want none", libraries, err)
	}
	wantDestinations := []jobs.Destination{jobs.RootDestination(world.nodeID, root)}
	if got := work.cancelled[0].Destinations; !slices.Equal(got, wantDestinations) {
		t.Errorf("destinations forgotten: %v, want its root's", got)
	}
	world.scan(filmRecords(3), true)
	if live := len(world.liveFilesByPath()); live != 0 {
		t.Errorf("%d live files after a scan of a deleted library's root, want none", live)
	}
	if _, err := admin.Delete(t.Context(), libraryID); !errors.Is(err, ErrNoLibrary) {
		t.Errorf("deleting it again: %v, want ErrNoLibrary", err)
	}
}

func TestOffline_IsSinceTheFirstScanThatFoundARootOrDiskMissingUntilOneFindsIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	admin := newRetiringAdmin(world)
	world.scan(withDisk(true), true)
	world.scan(withDisk(false), true)
	diskMissingAt := world.clock.Now()
	world.scanOn(nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 1}, nil, true) // another device: withheld
	rootMissingAt := world.clock.Now()
	world.scanOn(nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 1}, nil, true)
	offline, err := admin.Offline(t.Context())
	if err != nil || len(offline) != 2 {
		t.Fatalf("Offline = %+v, %v; want the disk and then the root", offline, err)
	}
	disk, wholeRoot := offline[0], offline[1]
	if disk.MountPath != root+"/disk1" || !disk.Since.Equal(diskMissingAt.Truncate(time.Millisecond)) ||
		disk.NodeName != "local" || disk.LibraryName != "Films" {
		t.Errorf("the disk: %+v, want missing since %v", disk, diskMissingAt)
	}
	if wholeRoot.MountPath != "" || wholeRoot.RootPath != root || !wholeRoot.Since.Equal(rootMissingAt) {
		t.Errorf("the root: %+v, want unavailable since the first scan that withheld, %v", wholeRoot, rootMissingAt)
	}
	world.scan(withDisk(true), true)
	if offline, err := admin.Offline(t.Context()); err != nil || len(offline) != 0 {
		t.Errorf("Offline after a scan found both = %+v, %v; want none", offline, err)
	}
}
