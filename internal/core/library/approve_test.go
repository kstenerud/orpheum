// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// adminOf is the admin service over a harness whose node, "local", is connected.
func adminOf(world *harness) (*Admin, *fakeNodes, int64) {
	world.t.Helper()
	nodes := &fakeNodes{id: world.nodeID, roots: []string{root}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	libraries, err := admin.List(world.t.Context(), auth.EveryLibrary())
	if err != nil || len(libraries) != 1 {
		world.t.Fatalf("List = %+v, %v", libraries, err)
	}
	return admin, nodes, libraries[0].ID
}

// S7, D208 §3: the admin says a withheld removal is real; the next scan removes, once.
func TestScenario_S7_AnApprovedRemovalGoesAheadOnTheNextScanOnly(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	admin, nodes, libraryID := adminOf(world)
	world.scan(filmRecords(40), true)
	world.scan(filmRecords(19), true)
	if outcome, _ := world.rootState(); outcome != "withheld" {
		t.Fatalf("root is %s, want withheld", outcome)
	}
	if _, err := admin.ApproveRemovals(t.Context(), libraryID, "local", "/elsewhere"); !errors.Is(err, ErrNoSuchRoot) {
		t.Errorf("approving a root the library lacks: %v", err)
	}
	scanned, err := admin.ApproveRemovals(t.Context(), libraryID, "local", root)
	if err != nil || !scanned || !slices.Equal(nodes.scanned, []string{root}) {
		t.Fatalf("ApproveRemovals = %v, %v; asked for scans of %v", scanned, err, nodes.scanned)
	}
	world.scan(filmRecords(19), true)
	if live := len(world.liveFilesByPath()); live != 19 {
		t.Errorf("%d live files after the approved scan, want 19", live)
	}
	if outcome, _ := world.rootState(); outcome != "complete" {
		t.Errorf("root is %s after the approved scan, want complete", outcome)
	}
	if _, err := admin.ApproveRemovals(t.Context(), libraryID, "local", root); !errors.Is(err, ErrNothingWithheld) {
		t.Errorf("approving again with nothing withheld: %v", err)
	}
	world.scan(nil, true)
	if live := len(world.liveFilesByPath()); live != 19 {
		t.Errorf("%d live files: the approval should have been spent by the scan it was for", live)
	}
}

// S7b, D208: the admin says a disk missing from a mount point inside the root is gone for good.
func TestScenario_S7b_AGoneDiskIsForgottenAndItsFilesRemoved(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	admin, _, libraryID := adminOf(world)
	withDisk := func(mounted bool) []nodeproto.ScanRecord {
		var records []nodeproto.ScanRecord
		if mounted {
			records = append(records, mountRecord("disk1", 21))
			records = append(records, inDirectory("disk1", filmRecords(2))...)
		}
		return append(records, filmRecords(30)...)
	}
	world.scan(withDisk(true), true)
	world.scan(withDisk(false), true)
	if live := len(world.liveFilesByPath()); live != 32 {
		t.Fatalf("%d live files with the disk away, want all 32 kept", live)
	}
	_, notMount := admin.ForgetMount(t.Context(), libraryID, "local", root+"/Film00.mkv")
	if !errors.Is(notMount, ErrMountNotMissing) {
		t.Errorf("forgetting what is no mount: %v", notMount)
	}
	if _, err := admin.ForgetMount(t.Context(), libraryID, "local", root+"/disk1"); err != nil {
		t.Fatal(err)
	}
	world.scan(withDisk(false), true)
	if live := len(world.liveFilesByPath()); live != 30 {
		t.Errorf("%d live files after forgetting the disk, want its 2 removed", live)
	}
}
