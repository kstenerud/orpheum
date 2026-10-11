// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// shelve writes a manifest for each of dirs, which are on the shelf, as a finished making does.
func shelve(t *testing.T, shelf *Shelf, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		manifest := nodeproto.RenditionManifest{Prepare: nodeproto.Prepare{RenditionDir: dir}, SizeBytes: 100}
		if err := shelf.Shelve(manifest); err != nil {
			t.Fatal(err)
		}
	}
}

// A node holding no database knows what its disk holds from the manifests alone (D238 §2): a run
// directory without one, a making that never finished, is not listed.
func TestInventory_ListsTheShelvedRenditionsByTheirManifests(t *testing.T) {
	t.Parallel()
	disk := 50
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 95}, &disk, map[string]struct {
		bytes int
		ago   time.Duration
	}{
		"1/remux/aaaaaaaaaaaaaaaa": {100, time.Hour},
		"2/remux/bbbbbbbbbbbbbbbb": {100, time.Hour}, // never finished
	})
	shelve(t, shelf, "1/remux/aaaaaaaaaaaaaaaa")
	manifests, err := shelf.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 || manifests[0].Prepare.RenditionDir != "1/remux/aaaaaaaaaaaaaaaa" ||
		manifests[0].MadeAtMs != now.UnixMilli() || manifests[0].SizeBytes != 100 {
		t.Errorf("the inventory is %+v, want the one shelved rendition, made now", manifests)
	}
}

// The node evicts what the core does not keep, and a making that never finished, whatever their
// kept time; never one being written, nor one made since its hello listed the others (the owner,
// 2026-10-02).
func TestReconcile_EvictsWhatTheCoreDoesNotKeepAndUnfinishedMakingsButNeverOneBeingWrittenOrMadeSince(t *testing.T) {
	t.Parallel()
	disk := 50
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, KeptAfterLastPlay: time.Hour, DiskUsedLimitPercent: 95}, &disk,
		map[string]struct {
			bytes int
			ago   time.Duration
		}{
			"1/remux/aaaaaaaaaaaaaaaa": {100, time.Minute}, // kept
			"2/remux/bbbbbbbbbbbbbbbb": {100, time.Minute}, // listed, not kept
			"3/remux/cccccccccccccccc": {100, time.Minute}, // never finished
			"4/remux/dddddddddddddddd": {100, time.Minute}, // being written
			"5/h264/eeeeeeeeeeeeeeee":  {100, time.Minute}, // made since the listing
		})
	shelve(t, shelf, "1/remux/aaaaaaaaaaaaaaaa", "2/remux/bbbbbbbbbbbbbbbb", "5/h264/eeeeeeeeeeeeeeee")
	done := shelf.MarkWriting("4/remux/dddddddddddddddd")
	defer done()
	evicted, err := shelf.Reconcile(
		[]string{"1/remux/aaaaaaaaaaaaaaaa", "2/remux/bbbbbbbbbbbbbbbb"}, []string{"1/remux/aaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(evicted)
	if want := []string{"2/remux/bbbbbbbbbbbbbbbb", "3/remux/cccccccccccccccc"}; !slices.Equal(evicted, want) {
		t.Errorf("evicted %v, want %v", evicted, want)
	}
	want := []string{"1/remux/aaaaaaaaaaaaaaaa", "4/remux/dddddddddddddddd", "5/h264/eeeeeeeeeeeeeeee"}
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}
