// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// These tests are scenarios from plans/scenarios.md, named by their ids.

// films is n observed films, Film00.mkv onwards.
func films(n int) []nodeproto.Record {
	records := make([]nodeproto.Record, 0, n)
	for index := range n {
		name := fmt.Sprintf("Film%02d.mkv", index)
		records = append(records, observed(name, name))
	}
	return records
}

func (world *harness) rootState() (outcome, reason string) {
	world.t.Helper()
	row, err := world.database.Read().GetRootState(world.t.Context(), rootStateParams(world.nodeID))
	if err != nil {
		world.t.Fatal(err)
	}
	if row.WithheldReason != nil {
		reason = *row.WithheldReason
	}
	return row.Outcome, reason
}

// S7 and S8: the disk under a mount point is unplugged, leaving the empty directory beneath it,
// whose listing succeeds; nothing may be removed, and when the disk returns everything is as it
// was (D41, D208 §3).
func TestScenario_S7_S8_AnUnpluggedDiskRemovesNothingAndItsReturnRestoresEverything(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	mounted := nodeproto.ScanBegin{Root: root, FilesystemDevice: 7, IsMountPoint: true}
	world.scanOn(mounted, films(3), true)
	before := world.live()

	unplugged := nodeproto.ScanBegin{Root: root, FilesystemDevice: 1, IsMountPoint: false}
	world.scanOn(unplugged, nil, true)
	if got := len(world.live()); got != 3 {
		t.Fatalf("%d live files after a clean scan of the empty mount point, want all 3", got)
	}
	if outcome, reason := world.rootState(); outcome != "withheld" || reason == "" {
		t.Errorf("root_state %s %q, want the removals withheld with a reason for the admin", outcome, reason)
	}

	world.scanOn(mounted, films(3), true)
	for path, file := range world.live() {
		if before[path].ID != file.ID {
			t.Errorf("%s came back as file %d, want %d with its history", path, file.ID, before[path].ID)
		}
	}
	if outcome, _ := world.rootState(); outcome != "complete" {
		t.Errorf("root_state %s after the disk returned, want complete", outcome)
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
			world.scan(films(tc.had), true)
			world.scan(films(tc.kept), true)
			left := len(world.live())
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
	world.scan([]nodeproto.Record{
		observed("Metropolis.mkv", "heat"), observed("Wing.mkv", "alien"),
		observed("Nosferatu.mkv", "ronin"),
	}, true)
	heat := world.live()[root+"/Metropolis.mkv"].ID

	world.scan([]nodeproto.Record{observed("Wing.mkv", "alien"), observed("Nosferatu.mkv", "ronin")}, true)
	if _, live := world.live()[root+"/Metropolis.mkv"]; live {
		t.Fatal("the deleted film is still live; the test proves nothing")
	}
	world.scan([]nodeproto.Record{
		observed("Wing.mkv", "alien"), observed("Restored/Metropolis (1995).mkv", "heat"),
		observed("Nosferatu.mkv", "ronin"),
	}, true)
	if back := world.live()[root+"/Restored/Metropolis (1995).mkv"]; back.ID != heat {
		t.Errorf("the film came back as file %d, want %d with its history (D208 §4)", back.ID, heat)
	}
}

func mount(path string, device uint64) nodeproto.Record {
	return nodeproto.Record{Type: nodeproto.RecordMount, Mount: &nodeproto.Mount{
		Path: nodeproto.Path{Text: path}, FilesystemDevice: device,
	}}
}

// under is records with path prefixed by directory.
func inDirectory(directory string, records []nodeproto.Record) []nodeproto.Record {
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
	both := func(disk1, disk2 []nodeproto.Record, disk1Mounted bool) []nodeproto.Record {
		var records []nodeproto.Record
		if disk1Mounted {
			records = append(records, mount("disk1", 21))
		}
		records = append(records, inDirectory("disk1", disk1)...)
		records = append(records, mount("disk2", 22))
		return append(records, inDirectory("disk2", disk2)...)
	}
	world.scan(both(films(2), films(30), true), true)
	before := world.live()

	// disk1 unplugged: an empty directory, no boundary. On disk2 one film was deleted.
	world.scan(both(nil, films(29), false), true)
	live := world.live()
	for _, name := range []string{"disk1/Film00.mkv", "disk1/Film01.mkv"} {
		if _, found := live[root+"/"+name]; !found {
			t.Errorf("%s was removed while its disk was unplugged", name)
		}
	}
	if _, found := live[root+"/disk2/Film29.mkv"]; found {
		t.Error("the film deleted from the other disk was not removed")
	}

	world.scan(both(films(2), films(29), true), true)
	for _, name := range []string{"disk1/Film00.mkv", "disk1/Film01.mkv"} {
		if got := world.live()[root+"/"+name]; got.ID != before[root+"/"+name].ID {
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
	world.scan(append(append([]nodeproto.Record{mount("disk1", 21)}, inDirectory("disk1", films(2))...),
		inDirectory("local", films(10))...), true)
	world.scan(append([]nodeproto.Record{mount("disk1", 99)}, inDirectory("local", films(10))...), true)
	if got := len(world.live()); got != 12 {
		t.Errorf("%d live files, want all 12 kept: device 99 is not the disk that held two of them", got)
	}
}

// The negative sibling: a directory that was never a mount point and is now empty is just
// emptied, and its files go.
func TestScenario_S7b_AnEmptiedDirectoryThatWasNeverAMountIsRemovedAsUsual(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan(append(inDirectory("Old", films(2)), films(10)...), true)
	world.scan(films(10), true)
	if got := len(world.live()); got != 10 {
		t.Errorf("%d live files, want the 2 under Old/ removed", got)
	}
}
