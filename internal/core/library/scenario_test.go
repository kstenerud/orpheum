// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// These tests are scenarios from plans/scenarios.md, named by their ids.

// filmRecords is n observed films, Film00.mkv onwards.
func filmRecords(n int) []nodeproto.ScanRecord {
	records := make([]nodeproto.ScanRecord, 0, n)
	for index := range n {
		name := fmt.Sprintf("Film%02d.mkv", index)
		records = append(records, observedRecord(name, name))
	}
	return records
}

func (world *harness) rootState() (outcome, reason string) {
	world.t.Helper()
	row, err := world.database.Read().GetRootState(world.t.Context(), rootStateParams(world.nodeID))
	if err != nil {
		world.t.Fatal(err)
	}
	return row.LastScanOutcome, ptr.ValueOrZero(row.WithheldReason)
}

// S7 and S8: the disk under a mount point is unplugged, leaving the empty directory beneath it,
// whose listing succeeds; nothing may be removed, and when the disk returns everything is as it
// was (D41, D208 §3).
func TestScenario_S7_S8_AnUnpluggedDiskRemovesNothingAndItsReturnRestoresEverything(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	mounted := nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 7, IsMountPoint: true}
	world.scanOn(mounted, filmRecords(3), true)
	before := world.liveFilesByPath()

	unplugged := nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 1, IsMountPoint: false}
	world.scanOn(unplugged, nil, true)
	if got := len(world.liveFilesByPath()); got != 3 {
		t.Fatalf("%d live files after a clean scan of the empty mount point, want all 3", got)
	}
	if outcome, reason := world.rootState(); outcome != "withheld" || reason == "" {
		t.Errorf("root_state %s %q, want the removals withheld with a reason for the admin", outcome, reason)
	}

	world.scanOn(mounted, filmRecords(3), true)
	for path, file := range world.liveFilesByPath() {
		if before[path].ID != file.ID {
			t.Errorf("%s came back as file %d, want %d with its history", path, file.ID, before[path].ID)
		}
	}
	if outcome, _ := world.rootState(); outcome != "complete" {
		t.Errorf("root_state %s after the disk returned, want complete", outcome)
	}
}

// D276: a root found on another device with every file still in it, as after a rebuilt machine
// mounts it again, is not an unmounted disk: the scan completes, nothing waits for the admin, and
// the new device is the one a later unplugging is judged against.
func TestScenario_ARootOnANewDeviceWithEveryFileCompletesAndKeepsTheNewDevice(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scanOn(nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 7, IsMountPoint: true}, filmRecords(3), true)
	moved := nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 8, IsMountPoint: true}
	world.scanOn(moved, filmRecords(3), true)
	if outcome, reason := world.rootState(); outcome != "complete" {
		t.Errorf("root_state %s %q on the new device with every file, want complete", outcome, reason)
	}
	world.scanOn(nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 8, IsMountPoint: true}, filmRecords(2), true)
	if left := len(world.liveFilesByPath()); left != 2 {
		t.Errorf("%d live files after a film was deleted on the new device, want 2: it is the known device now", left)
	}
	world.scanOn(nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 1, IsMountPoint: false}, filmRecords(1), true)
	if outcome, _ := world.rootState(); outcome != "withheld" {
		t.Errorf("root_state %s when the new device went missing and a film with it, want withheld", outcome)
	}
}

// D277 §1: a scan held back for the admin that read the root leaves it playable, and only its
// removals wait; one that saw none of the root's files is a lost disk, and the root is offline.
func TestScenario_AHeldBackScanThatReadTheRootLeavesItPlayableAndOneThatSawNoneDoesNot(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	admin := newRetiringAdmin(world)
	world.scanOn(nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 7, IsMountPoint: true}, filmRecords(3), true)
	moved := nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 8, IsMountPoint: true}
	world.scanOn(moved, filmRecords(2), true)
	if outcome, _ := world.rootState(); outcome != "withheld" {
		t.Fatalf("root_state %s after a film went missing on a new device, want withheld", outcome)
	}
	if offline, err := admin.Offline(t.Context()); err != nil || len(offline) != 0 {
		t.Errorf("Offline = %+v, %v while only removals wait; want nothing offline", offline, err)
	}
	world.scanOn(nodeproto.ScanBegin{RootPath: root, FilesystemDevice: 1, IsMountPoint: false}, nil, true)
	if offline, err := admin.Offline(t.Context()); err != nil || len(offline) != 1 {
		t.Errorf("Offline = %+v, %v after a scan saw none of the root; want the root", offline, err)
	}
}

// S7, D208 §3: removals wait when a scan would remove all of a root's files, or more than half
// of them and more than twenty; fewer go ahead.
func TestScenario_S7_RemovalsPastTheThresholdWaitAndFewerGoAhead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		had, kept int
		waits     bool
	}{
		{"21 of 40, over half and over twenty", 40, 19, true},
		{"20 of 40, not over half", 40, 20, false},
		{"all 3", 3, 0, true},
		{"2 of 3, over half but not over twenty", 3, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			world := newHarness(t)
			world.scan(filmRecords(tc.had), true)
			world.scan(filmRecords(tc.kept), true)
			left := len(world.liveFilesByPath())
			if tc.waits && left != tc.had {
				t.Errorf("%d live files, want all %d: the removal should wait for the admin", left, tc.had)
			}
			if !tc.waits && left != tc.kept {
				t.Errorf("%d live files, want %d: the removal should go ahead", left, tc.kept)
			}
		})
	}
}

// S10, D208 §4: a film deleted by mistake is tombstoned; copied back, anywhere in the library,
// it is the same file with its history.
func TestScenario_S10_AFilmPutBackRevivesItsTombstone(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("Metropolis.mkv", "heat"), observedRecord("Wing.mkv", "alien"),
		observedRecord("Nosferatu.mkv", "ronin"),
	}, true)
	heat := world.liveFilesByPath()[root+"/Metropolis.mkv"].ID

	world.scan([]nodeproto.ScanRecord{observedRecord("Wing.mkv", "alien"), observedRecord("Nosferatu.mkv", "ronin")}, true)
	if _, live := world.liveFilesByPath()[root+"/Metropolis.mkv"]; live {
		t.Fatal("the deleted film is still live; the test proves nothing")
	}
	world.scan([]nodeproto.ScanRecord{
		observedRecord("Wing.mkv", "alien"), observedRecord("Restored/Metropolis (1995).mkv", "heat"),
		observedRecord("Nosferatu.mkv", "ronin"),
	}, true)
	if back := world.liveFilesByPath()[root+"/Restored/Metropolis (1995).mkv"]; back.ID != heat {
		t.Errorf("the film came back as file %d, want %d with its history (D208 §4)", back.ID, heat)
	}
}

func mountRecord(path string, device uint64) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordMount, Mount: &nodeproto.Mount{
		Path: nodeproto.Path{Text: path}, FilesystemDevice: device,
	}}
}

// inDirectory is records with path prefixed by directory.
func inDirectory(directory string, records []nodeproto.ScanRecord) []nodeproto.ScanRecord {
	for index := range records {
		if observed := records[index].Observed; observed != nil {
			observed.Path.Text = directory + "/" + observed.Path.Text
		}
	}
	return records
}

// S7b: the root is /mnt-like, with disks mounted inside it. The root itself never changes when
// one of them is unplugged — its mount point lists as an empty directory — so the boundary is
// what tells the core that subtree is missing, not emptied (D208). Changes on the other disk
// still apply.
func TestScenario_S7b_ADiskUnpluggedInsideARootRemovesNothingUnderIt(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	both := func(disk1, disk2 []nodeproto.ScanRecord, disk1Mounted bool) []nodeproto.ScanRecord {
		var records []nodeproto.ScanRecord
		if disk1Mounted {
			records = append(records, mountRecord("disk1", 21))
		}
		records = append(records, inDirectory("disk1", disk1)...)
		records = append(records, mountRecord("disk2", 22))
		return append(records, inDirectory("disk2", disk2)...)
	}
	world.scan(both(filmRecords(2), filmRecords(30), true), true)
	before := world.liveFilesByPath()

	// disk1 unplugged: an empty directory, no boundary. On disk2 one film was deleted.
	world.scan(both(nil, filmRecords(29), false), true)
	live := world.liveFilesByPath()
	for _, name := range []string{"disk1/Film00.mkv", "disk1/Film01.mkv"} {
		if _, found := live[root+"/"+name]; !found {
			t.Errorf("%s was removed while its disk was unplugged", name)
		}
	}
	if _, found := live[root+"/disk2/Film29.mkv"]; found {
		t.Error("the film deleted from the other disk was not removed")
	}

	world.scan(both(filmRecords(2), filmRecords(29), true), true)
	for _, name := range []string{"disk1/Film00.mkv", "disk1/Film01.mkv"} {
		if got := world.liveFilesByPath()[root+"/"+name]; got.ID != before[root+"/"+name].ID {
			t.Errorf("%s came back as file %d, want %d", name, got.ID, before[root+"/"+name].ID)
		}
	}
}

// S7b: another disk at the same mount point is not the disk that was there.
func TestScenario_S7b_ADifferentDiskAtAMountPointHidesTheOldOnesFiles(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	// Ten other films on the root, so the removal is small enough to go ahead on its own: only
	// the boundary can keep the old disk's films.
	world.scan(append(append([]nodeproto.ScanRecord{mountRecord("disk1", 21)}, inDirectory("disk1", filmRecords(2))...),
		inDirectory("local", filmRecords(10))...), true)
	world.scan(append([]nodeproto.ScanRecord{mountRecord("disk1", 99)}, inDirectory("local", filmRecords(10))...), true)
	if got := len(world.liveFilesByPath()); got != 12 {
		t.Errorf("%d live files, want all 12 kept: device 99 is not the disk that held two of them", got)
	}
}

// The negative sibling: a directory that was never a mount point and is now empty is just
// emptied, and its files go.
func TestScenario_S7b_AnEmptiedDirectoryThatWasNeverAMountIsRemovedAsUsual(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan(append(inDirectory("Old", filmRecords(2)), filmRecords(10)...), true)
	world.scan(filmRecords(10), true)
	if got := len(world.liveFilesByPath()); got != 10 {
		t.Errorf("%d live files, want the 2 under Old/ removed", got)
	}
}
