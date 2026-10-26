// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// At each hello the node evicts every run directory that is not a store with its manifest: a store
// never opened, and what a whole-film prepare left, whatever their kept time; never one being
// written (D238 §2, D344 §1).
func TestEvictUnfinished_EvictsAllButTheStoresWithTheirManifestsAndOnesBeingWritten(t *testing.T) {
	t.Parallel()
	disk := 50
	shelf := shelfWith(t, Budget{MaxBytes: 1 << 30, KeptAfterLastPlay: time.Hour, DiskUsedLimitPercent: 95}, &disk,
		map[string]struct {
			bytes int
			ago   time.Duration
		}{
			"8/chunks-picture/fedcba9876543210":   {100, time.Minute}, // never opened as a store
			"2/remux/bbbbbbbbbbbbbbbb":            {100, time.Minute}, // a prepare's, from before D344
			"4/chunks-q400k360p/dddddddddddddddd": {100, time.Minute}, // being written
		})
	if _, err := shelf.OpenStore(nodeproto.StoreManifest{
		StoreDir: testStore, Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
	}); err != nil {
		t.Fatal(err)
	}
	done := shelf.MarkWriting("4/chunks-q400k360p/dddddddddddddddd")
	defer done()
	evicted, err := shelf.EvictUnfinished()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(evicted)
	if want := []string{"2/remux/bbbbbbbbbbbbbbbb", "8/chunks-picture/fedcba9876543210"}; !slices.Equal(evicted, want) {
		t.Errorf("evicted %v, want %v", evicted, want)
	}
	want := []string{"4/chunks-q400k360p/dddddddddddddddd", testStore}
	slices.Sort(want)
	if got := remainingRunDirs(t, shelf); !slices.Equal(got, want) {
		t.Errorf("left %v, want %v", got, want)
	}
}
