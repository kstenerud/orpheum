// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// D543: while a node's prepared directory is parked for want of room, a player's ask for a chunk no
// run on the node is making is refused at once, ErrDiskFull, and queues nothing; a run the node took
// before goes on, its chunks still waited for. Once the node's status unparks it, the next ask queues
// the run again: no ask is refused for good.
func TestEnsure_ANodeWithNoRoomRefusesAtOnceAndAsksAgainOnceItHasRoom(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	picture := Stream{Kind: nodeproto.StreamPicture}
	sound := Stream{Kind: nodeproto.StreamSound, SoundKbps: 96}
	ensured(t, service, picture, 3)
	dir := testStoreDir(t, service, picture, mustGrid(t, service).ID())
	service.runs.given(dir, 3, 32, 7, 1, time.UnixMilli(0))
	queue.park(jobs.PreparedDirectoryDestination(1), true)
	ensured(t, service, picture, 10)
	if err := service.Ensure(t.Context(), 1, OnKeyframes, sound, 3); !errors.Is(err, ErrDiskFull) {
		t.Errorf("a chunk no run is making on a node with no room: %v; want ErrDiskFull", err)
	}
	// The node refused the picture's run for its disk: it waits parked, and its chunks are refused too.
	service.runs.requeued(dir, 3)
	if err := service.Ensure(t.Context(), 1, OnKeyframes, picture, 10); !errors.Is(err, ErrDiskFull) {
		t.Errorf("a chunk of the run the node refused: %v; want ErrDiskFull", err)
	}
	if runs := queue.runs(t, service); len(runs) != 1 {
		t.Fatalf("the runs asked for with no room = %+v; want only the first, from before", runs)
	}
	queue.park(jobs.PreparedDirectoryDestination(1), false)
	ensured(t, service, sound, 3)
	if runs := queue.runs(t, service); len(runs) != 2 || runs[1].stream != "sound0-96k" || runs[1].firstChunk != 3 {
		t.Errorf("the runs asked for once the node had room = %+v; want the sound's, from chunk 3", runs)
	}
}
