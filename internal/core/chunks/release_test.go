// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"slices"
	"testing"
)

// A player that leaves a film lets go of every one of the film's chunk runs and its read-through, so
// each falls to background at the next demotion (D258 §2); another film's are not touched.
func TestReleaseWanted_LetsGoOfEveryStoreOfTheFilmAndItsReadThrough(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, reader := chunksFixture(t, queue, fakeNode{})
	stalledAt(t, service, 2000) // a picture's store and a sound's for film 1
	if err := service.ReleaseWanted(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	stores, err := service.cache.Read().ListChunkStoresOfFile(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	var released []int64
	for _, target := range queue.released {
		released = append(released, target.ChunkStoreID)
	}
	if len(stores) != 2 || !slices.Equal(released, stores) {
		t.Errorf("the stores let go of = %v; want film 1's two, %v", released, stores)
	}
	if !slices.Equal(reader.released, []int64{1}) {
		t.Errorf("the read-throughs let go of = %v; want film 1's", reader.released)
	}
	if err := service.ReleaseWanted(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	if len(queue.released) != 2 {
		t.Errorf("leaving film 2, with no stores, let go of %v", queue.released[2:])
	}
}
