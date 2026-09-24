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
	mounted := nodeproto.ScanBegin{Root: root, Device: 7, MountPoint: true}
	world.scanOn(mounted, films(3), true)
	before := world.live()

	unplugged := nodeproto.ScanBegin{Root: root, Device: 1, MountPoint: false}
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
