// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// ensured is Ensure of film 1's stream at chunk wanted, failing the test on an error.
func ensured(t *testing.T, service *Service, stream Stream, wanted int) {
	t.Helper()
	if err := service.Ensure(t.Context(), 1, stream, wanted); err != nil {
		t.Fatalf("Ensure(%s, %d) = %v", stream.Name(), wanted, err)
	}
}

func TestEnsure_QueuesOneRunFromTheChunkThroughFiveMinutesOfFilmAtPlayPriority(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	picture := Stream{Kind: nodeproto.StreamPicture}
	ensured(t, service, picture, 3)
	gridID := mustGrid(t, service).ID()
	// Chunk 3 starts at 30 s; five minutes on is 330 s, so the run ends with chunk 32, at 320 s.
	if runs := queue.runs(t); !slices.Equal(runs, []runArgs{{
		Stream: "picture", GridID: gridID, FirstChunk: 3, LastChunk: 32,
	}}) {
		t.Fatalf("the runs asked for = %+v; want chunks 3 to 32 of the picture", runs)
	}
	request := queue.requests[0]
	if request.Kind != jobs.KindMakeChunks || request.Target != (jobs.Target{FileID: 1}) ||
		request.Priority != jobs.PriorityPlayNow || request.Destination != jobs.NodeDestination(1) {
		t.Errorf("the run was asked for as %+v; want make_chunks of file 1, at play priority, on node 1", request)
	}
	// A run near the end ends with the film's last chunk, 119.
	ensured(t, service, picture, 100)
	if runs := queue.runs(t); len(runs) != 2 || runs[1].FirstChunk != 100 || runs[1].LastChunk != 119 {
		t.Errorf("the runs asked for = %+v; want a second, chunks 100 to the last, 119", runs)
	}
}

func TestEnsure_NeverQueuesARunOverChunksARunWillMakeOrThatAreMade(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	picture := Stream{Kind: nodeproto.StreamPicture}
	ensured(t, service, picture, 3)
	// Every chunk of the minute from chunk 3 or chunk 20 is on its way: nothing more is queued.
	ensured(t, service, picture, 3)
	ensured(t, service, picture, 20)
	if runs := queue.runs(t); len(runs) != 1 {
		t.Fatalf("the runs asked for = %+v; want only the first", runs)
	}
	// From chunk 30, the minute reaches past the run's last, 32: a run starts after it.
	ensured(t, service, picture, 30)
	if runs := queue.runs(t); len(runs) != 2 || runs[1].FirstChunk != 33 || runs[1].LastChunk != 62 {
		t.Fatalf("the runs asked for = %+v; want a second from chunk 33, after the first's", runs)
	}
	// A chunk the node served is made: a run before it stops at the chunk before it.
	dir := StoreDir(1, picture, mustGrid(t, service).ID())
	service.runs.served(dir, 0)
	service.runs.served(dir, 2)
	ensured(t, service, picture, 0)
	if runs := queue.runs(t); len(runs) != 3 || runs[2].FirstChunk != 1 || runs[2].LastChunk != 1 {
		t.Errorf("the runs asked for = %+v; want a third, chunk 1 alone, between the made ones", runs)
	}
	// Another stream is another store.
	ensured(t, service, Stream{Kind: nodeproto.StreamSound, IsBoosted: true}, 3)
	if runs := queue.runs(t); len(runs) != 4 || runs[3].Stream != "sound0-boost" || runs[3].FirstChunk != 3 {
		t.Errorf("the runs asked for = %+v; want a fourth, of the boosted sound from chunk 3", runs)
	}
	if queue.keptWanted == 0 {
		t.Errorf("a player asking for chunks a run is making did not keep it wanted (D234 §3)")
	}
}

func TestEnsure_ARunTheQueueDidNotTakeIsAskedForAgainAtTheNextAsk(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	picture := Stream{Kind: nodeproto.StreamPicture}
	// The film's sound has a run queued: the picture's is folded into it, and not queued.
	queue.isTaken = func(jobs.Request) bool { return false }
	ensured(t, service, picture, 3)
	queue.isTaken = nil
	ensured(t, service, picture, 3)
	if runs := queue.runs(t); len(runs) != 2 || runs[1] != runs[0] {
		t.Errorf("the runs asked for = %+v; want the same run asked for again once it was not queued", runs)
	}
}

func TestEnsure_AFilmWithNoGridYetIsSaidSo(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	err := service.Ensure(t.Context(), 2, Stream{Kind: nodeproto.StreamPicture}, 0)
	if !errors.Is(err, ErrNoGrid) || len(queue.requests) != 0 {
		t.Errorf("Ensure of a film with no timeline = %v, %d runs; want ErrNoGrid and none", err, len(queue.requests))
	}
}

// mustGrid is film 1's grid, failing the test without one.
func mustGrid(t *testing.T, service *Service) Grid {
	t.Helper()
	grid, isCut, err := service.Grid(t.Context(), 1)
	if err != nil || !isCut {
		t.Fatalf("film 1's grid = %+v, %t, %v", grid, isCut, err)
	}
	return grid
}
