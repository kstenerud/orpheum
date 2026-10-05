// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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
	// Chunk 3 starts at 30 s; five minutes on is 330 s, so the run ends with chunk 32, at 320 s.
	want := []askedRun{{stream: "picture", firstChunk: 3, lastChunk: 32}}
	if runs := queue.runs(t, service); !slices.Equal(runs, want) {
		t.Fatalf("the runs asked for = %+v; want chunks 3 to 32 of the picture", runs)
	}
	request := queue.requests[0]
	if request.Kind != jobs.KindMakeChunks || request.ChunkStoreID == 0 || request.FileID != 0 ||
		request.Priority != jobs.PriorityPlayNow || request.Destination != jobs.NodeDestination(1) {
		t.Errorf("the run was asked for as %+v; want make_chunks of its store, at play priority, on node 1", request)
	}
	// A run near the end ends with the film's last chunk, 119.
	ensured(t, service, picture, 100)
	if runs := queue.runs(t, service); len(runs) != 2 || runs[1].firstChunk != 100 || runs[1].lastChunk != 119 {
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
	if runs := queue.runs(t, service); len(runs) != 1 {
		t.Fatalf("the runs asked for = %+v; want only the first", runs)
	}
	// From chunk 30, the minute reaches past the run's last, 32: a run starts after it.
	ensured(t, service, picture, 30)
	if runs := queue.runs(t, service); len(runs) != 2 || runs[1].firstChunk != 33 || runs[1].lastChunk != 62 {
		t.Fatalf("the runs asked for = %+v; want a second from chunk 33, after the first's", runs)
	}
	// A chunk the node served is made: a run before it stops at the chunk before it.
	dir := StoreDir(1, picture, mustGrid(t, service).ID())
	service.runs.served(dir, 0)
	service.runs.served(dir, 2)
	ensured(t, service, picture, 0)
	if runs := queue.runs(t, service); len(runs) != 3 || runs[2].firstChunk != 1 || runs[2].lastChunk != 1 {
		t.Errorf("the runs asked for = %+v; want a third, chunk 1 alone, between the made ones", runs)
	}
	// Another stream is another store.
	ensured(t, service, Stream{Kind: nodeproto.StreamSound, IsBoosted: true}, 3)
	if runs := queue.runs(t, service); len(runs) != 4 || runs[3].stream != "sound0-boost" || runs[3].firstChunk != 3 {
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
	// A run of the picture's store is queued: this one is folded into it, and not queued.
	queue.isTaken = func(jobs.Request) bool { return false }
	ensured(t, service, picture, 3)
	queue.isTaken = nil
	ensured(t, service, picture, 3)
	if runs := queue.runs(t, service); len(runs) != 2 || runs[1] != runs[0] {
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

// jobsQueue is the job queue as the server has it, over its own cache.db.
func jobsQueue(t *testing.T) (*jobs.Queue, *store.Cache) {
	t.Helper()
	cache := storetest.OpenCache(t)
	return jobs.NewQueue(jobs.QueueParts{
		Cache: cache, Clock: clocktest.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)),
		Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)), Random: jobqueuetest.Unjittered(),
		Logger: slog.New(slog.DiscardHandler),
	}), cache
}

// openRuns are the make_chunks jobs open in cache, each its target's store and its arguments.
func openRuns(t *testing.T, cache *store.Cache) map[int64]runArgs {
	t.Helper()
	all, err := cache.Read().ListEveryOpenJob(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	runByStore := map[int64]runArgs{}
	for _, job := range all {
		var args runArgs
		if job.Kind != string(jobs.KindMakeChunks) || job.ChunkStoreID == nil ||
			json.Unmarshal([]byte(job.ArgsJSON), &args) != nil {
			t.Fatalf("an open job %+v is not a run of a store", job)
		}
		if _, isTwice := runByStore[*job.ChunkStoreID]; isTwice {
			t.Errorf("store %d has two runs open", *job.ChunkStoreID)
		}
		runByStore[*job.ChunkStoreID] = args
	}
	return runByStore
}

func TestEnsure_TwoStreamsOfOneFilmWarmedTogetherEachQueueARun(t *testing.T) {
	t.Parallel()
	queue, cache := jobsQueue(t)
	service, _ := chunksFixture(t, queue, fakeNode{})
	// A first stall warms a quality and the sound before any chunk is asked for (chunks.md §4).
	ensured(t, service, Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 1200, Height: 480}}, 3)
	ensured(t, service, Stream{Kind: nodeproto.StreamSound}, 3)
	if runs := openRuns(t, cache); len(runs) != 2 {
		t.Errorf("the runs queued = %+v; want one for each stream's store", runs)
	}
}

func TestEnsure_ASecondRunOfAStoreWhileItsRunIsQueuedAddsNone(t *testing.T) {
	t.Parallel()
	queue, cache := jobsQueue(t)
	service, _ := chunksFixture(t, queue, fakeNode{})
	picture := Stream{Kind: nodeproto.StreamPicture}
	ensured(t, service, picture, 3)
	// The player seeks ten minutes on while the first run waits: the queue folds the second into it.
	ensured(t, service, picture, 63)
	runs := openRuns(t, cache)
	if len(runs) != 1 {
		t.Fatalf("the runs queued = %+v; want the one", runs)
	}
	for _, args := range runs {
		if args != (runArgs{FirstChunk: 3, LastChunk: 32}) {
			t.Errorf("the queued run is %+v; want the first's chunks, 3 to 32, kept", args)
		}
	}
	// The second was not kept as on its way: the player's next ask for it plans it again.
	first, last, isPlanned := service.runs.plan(StoreDir(1, picture, mustGrid(t, service).ID()),
		mustGrid(t, service), 63)
	if !isPlanned || first != 63 || last != 92 {
		t.Errorf("the next ask from chunk 63 plans %d to %d (%t); want 63 to 92 again", first, last, isPlanned)
	}
}
