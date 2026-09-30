// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestUnavailableUnder_AFileUnderAMissingRootOrDiskWaitsAndAScanThatFindsThemSaysSo(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	var found [][]string
	world.ingester.SetOnPathsFound(func(_ context.Context, nodeID int64, paths []string) {
		if nodeID == world.nodeID {
			found = append(found, slices.Sorted(slices.Values(paths)))
		}
	})
	unavailable := func(path string) (Unavailable, bool) {
		t.Helper()
		missing, isMissing, err := UnavailableUnder(t.Context(), world.database.Read(), world.nodeID, []byte(path))
		if err != nil {
			t.Fatal(err)
		}
		return missing, isMissing
	}
	onDisk := func(isMounted bool) []nodeproto.Record {
		var records []nodeproto.Record
		if isMounted {
			records = append(records, mountRecord("disk1", 21))
		}
		return append(records, inDirectory("disk1", filmRecords(2))...)
	}
	world.scan(onDisk(true), true)
	if want := [][]string{{root, root + "/disk1"}}; !slices.EqualFunc(found, want, slices.Equal) {
		t.Errorf("a complete scan found %q, want the root and its disk", found)
	}
	if missing, isMissing := unavailable(root + "/disk1/Film00.mkv"); isMissing {
		t.Errorf("with everything there, %+v is missing", missing)
	}

	world.scan(onDisk(false), true) // the disk unplugged: its mount point lists empty
	unpluggedAt := world.clock.Now()
	missing, isMissing := unavailable(root + "/disk1/Film00.mkv")
	if !isMissing || missing.Path != root+"/disk1" || !missing.Since.Equal(unpluggedAt) {
		t.Errorf("a film on the unplugged disk: %+v (%v), want the disk missing since the scan", missing, isMissing)
	}
	if _, isMissing := unavailable(root + "/Elsewhere.mkv"); isMissing {
		t.Error("a file beside the unplugged disk, not on it, is missing")
	}

	world.scan([]nodeproto.Record{errorRecord(".", nodeproto.ScopeRoot)}, true)
	if missing, isMissing := unavailable(root + "/Elsewhere.mkv"); !isMissing || missing.Path != root {
		t.Errorf("a file under a root that could not be read: %+v (%v), want the root missing", missing, isMissing)
	}

	found = nil
	world.scan(onDisk(true), true)
	if want := [][]string{{root, root + "/disk1"}}; !slices.EqualFunc(found, want, slices.Equal) {
		t.Errorf("the scan after both came back found %q, want the root and its disk", found)
	}
	if missing, isMissing := unavailable(root + "/disk1/Film00.mkv"); isMissing {
		t.Errorf("after they came back, %+v is missing", missing)
	}
}
