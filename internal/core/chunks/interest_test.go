// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// wanting is what film 1's players want, as the playbacks would say it (InterestsBy), changed as a
// test moves them.
type wanting struct {
	mutex     sync.Mutex
	interests []Interest
}

// wantedBy has service's runs read their interests from a wanting, empty to start.
func wantedBy(service *Service) *wanting {
	wanted := &wanting{}
	service.InterestsBy(func(fileID int64) []Interest {
		wanted.mutex.Lock()
		defer wanted.mutex.Unlock()
		if fileID != 1 {
			return nil
		}
		return slices.Clone(wanted.interests)
	})
	return wanted
}

// say replaces what film 1's players want.
func (wanted *wanting) say(interests ...Interest) {
	wanted.mutex.Lock()
	defer wanted.mutex.Unlock()
	wanted.interests = interests
}

// askedFor is a player's ask for chunk wanted of film 1's stream, as Want makes it: the film's runs
// no player wants stopped, then the chunk ensured.
func askedFor(t *testing.T, service *Service, stream Stream, wanted int) {
	t.Helper()
	if err := service.StopUnwanted(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	ensured(t, service, stream, wanted)
}

// playingAt is a player wanting stream from chunk n of the fixture's film, ten seconds a chunk.
func playingAt(stream Stream, n int) Interest {
	return Interest{StreamName: stream.Name(), PositionMs: int64(n) * 10_000}
}

// A player that seeks away from a run stops it, queued or running, and its new place's run is queued;
// a seek back does the same: no one wants the run it left (D309 §2, after D250 §5's distances).
func TestStopUnwanted_APlayerThatSeeksAwayStopsTheRunItLeft(t *testing.T) {
	t.Parallel()
	queue, cache := jobsQueue(t)
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	wanted.say(playingAt(picture, 3))
	askedFor(t, service, picture, 3)
	for _, soughtTo := range []int{63, 3} {
		wanted.say(playingAt(picture, soughtTo))
		askedFor(t, service, picture, soughtTo)
		runs := openRuns(t, cache)
		if len(runs) != 1 {
			t.Fatalf("after a seek to chunk %d, the runs queued = %+v; want the seek's alone", soughtTo, runs)
		}
		for _, args := range runs {
			if args.FirstChunk != soughtTo {
				t.Errorf("after a seek to chunk %d, the queued run is %+v; want the seek's", soughtTo, args)
			}
		}
	}
}

// A run its player still plays into, or will within a minute of film, goes on however the player
// moved: forward inside it, or back to a little before it, where a short run fills the gap.
func TestStopUnwanted_ARunItsPlayerWillReachWithinAMinuteGoesOn(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	wanted.say(playingAt(picture, 10))
	askedFor(t, service, picture, 10)
	service.runs.begin(StoreDir(1, picture, mustGrid(t, service).ID()), 10, 39, 21, 1, service.clock.Now())
	for _, to := range []int{14, 8} {
		wanted.say(playingAt(picture, to))
		askedFor(t, service, picture, to)
	}
	if len(queue.cancelledFirsts) != 0 {
		t.Errorf("the runs cancelled start at %v; want none", queue.cancelledFirsts)
	}
	want := []askedRun{
		{stream: "picture", firstChunk: 10, lastChunk: 39}, {stream: "picture", firstChunk: 8, lastChunk: 9},
	}
	if runs := queue.runs(t, service); !slices.Equal(runs, want) {
		t.Errorf("the runs asked for = %+v; want the first, and the gap before it", runs)
	}
}

// A jump that lands just before a run still waiting in the queue starts one run from where it landed
// in its place: left, the queue, which keeps one job of a store queued, folded the jump's run into it,
// and the jump waited for chunks after its own (D286 §1).
func TestEnsure_AJumpJustBeforeAQueuedRunStartsOneRunWhereItLanded(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	wanted.say(playingAt(picture, 10))
	askedFor(t, service, picture, 10)
	wanted.say(playingAt(picture, 8))
	askedFor(t, service, picture, 8)
	if !slices.Equal(queue.cancelledFirsts, []int{10}) {
		t.Errorf("the runs cancelled start at %v; want the queued run from 10", queue.cancelledFirsts)
	}
	want := []askedRun{
		{stream: "picture", firstChunk: 10, lastChunk: 39}, {stream: "picture", firstChunk: 8, lastChunk: 37},
	}
	if runs := queue.runs(t, service); !slices.Equal(runs, want) {
		t.Errorf("the runs asked for = %+v; want the jump's, from 8 through five minutes", runs)
	}
}

// A run past chunks already made is where a player on them goes next, however far ahead it starts:
// cut, the next ask queued it again and each ask cut it again, without end (the owner's test,
// 2026-10-12).
func TestStopUnwanted_APlayerOnMadeChunksWantsTheRunThatMakesWhatFollowsThem(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	sound := Stream{Kind: nodeproto.StreamSound, SoundKbps: 96}
	dir := StoreDir(1, sound, mustGrid(t, service).ID())
	service.runs.madeThrough(dir, 0, 19) // ten chunks before the run made, by a run since ended
	wanted.say(playingAt(sound, 20))
	askedFor(t, service, sound, 20)
	wanted.say(playingAt(sound, 8))
	askedFor(t, service, sound, 8)
	if len(queue.cancelledFirsts) != 0 {
		t.Errorf("the runs cancelled start at %v; want none, the run from 20 making what follows the made chunks",
			queue.cancelledFirsts)
	}
}

// A run of a quality its player left goes the moment the player says so, whatever its speed: the
// node had two 720p runs left from before the player moved to 360p, one measured slow, and the 360p
// run the player needed was evicted in their place (the owner's Wings on nas.example, 2026-10-17). One
// player wants one quality at a time (D309 §1, §2).
func TestStopUnwanted_ARunOfAQualityItsPlayerLeftStops(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	high, low := quality(1200, 480), quality(700, 432)
	wanted.say(playingAt(high, 3))
	askedFor(t, service, high, 3)
	wanted.say(playingAt(low, 5))
	askedFor(t, service, low, 5)
	if !slices.Equal(queue.cancelledFirsts, []int{3}) {
		t.Fatalf("the runs cancelled start at %v; want the left quality's, from 3", queue.cancelledFirsts)
	}
	if service.runs.hasRun(StoreDir(1, high, mustGrid(t, service).ID())) {
		t.Error("the left quality still has a run; want it forgotten")
	}
	if !service.runs.hasRun(StoreDir(1, low, mustGrid(t, service).ID())) {
		t.Error("the quality the player plays has no run; want it kept")
	}
}

// A paused player wants only chunk_kept_ahead_seconds after it, 30 s: its run makes that and stops,
// so a tab paused for an hour holds no encoder, and playing again starts at once (D309 §7). Once no
// one wants a film at all, as when every interest lapsed, each step of a run stops it (§3).
func TestStopUnwanted_APausedPlayersRunStopsOnceWhatItKeepsAheadIsMade(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	dir := StoreDir(1, picture, mustGrid(t, service).ID())
	paused := playingAt(picture, 3)
	paused.IsStopped = true
	wanted.say(paused)
	askedFor(t, service, picture, 3)
	service.runs.madeThrough(dir, 3, 4) // through 50 s: the 30 s from 30 s end in chunk 5
	if err := service.StopUnwanted(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if len(queue.cancelledFirsts) != 0 {
		t.Fatalf("with chunk 4 made: cancelled %v; want the run going on to make chunk 5", queue.cancelledFirsts)
	}
	service.runs.madeThrough(dir, 3, 5)
	if err := service.StopUnwanted(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(queue.cancelledFirsts, []int{3}) {
		t.Fatalf("with the 30 s made: cancelled %v; want the run from 3", queue.cancelledFirsts)
	}

	wanted.say(playingAt(picture, 40))
	askedFor(t, service, picture, 40)
	wanted.say()
	if err := service.StopUnwanted(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(queue.cancelledFirsts, []int{3, 40}) {
		t.Errorf("with no one wanting the film: cancelled %v; want the run from 40 too", queue.cancelledFirsts)
	}
}

// A paused player filling its buffer starts no run past what it keeps ahead: one was queued at each
// ask and stopped within a second (staging, 2026-10-18). The server's own start of a player's run,
// as it starts or stalls, before its interest is recorded, still starts one.
func TestAskedFor_APausedPlayerStartsNoRunPastWhatItKeepsAhead(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	service.runs.madeThrough(StoreDir(1, picture, mustGrid(t, service).ID()), 3, 8)
	paused := playingAt(picture, 3)
	paused.IsStopped = true
	wanted.say(paused)
	if err := service.askedFor(t.Context(), 1, OnKeyframes, picture, 6); err != nil {
		t.Fatal(err)
	}
	if runs := queue.runs(t, service); len(runs) != 0 {
		t.Fatalf("a paused player's ask queued %+v; want nothing past the 30 s it keeps ahead", runs)
	}
	ensured(t, service, picture, 6)
	if runs := queue.runs(t, service); len(runs) != 1 || runs[0].firstChunk != 9 {
		t.Errorf("the server's own start queued %+v; want one from chunk 9", runs)
	}
}

// A run stopped as its player's request ends is still cancelled: the tracker forgets it before its
// job is cancelled, and a cancel on the request's context, cut short, left its job making chunks no
// one wanted, the server no longer knowing of it (a paused Firefox on staging, 2026-10-18).
func TestStopUnwanted_ARunStoppedAsItsRequestEndsIsStillCancelled(t *testing.T) {
	t.Parallel()
	queue, cache := jobsQueue(t)
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	wanted.say(playingAt(picture, 3))
	askedFor(t, service, picture, 3)
	wanted.say()
	ended, end := context.WithCancel(t.Context())
	end()
	if err := service.StopUnwanted(ended, 1); err != nil {
		t.Fatalf("stopping as the request ended: %v; want the run cancelled all the same", err)
	}
	if runs := openRuns(t, cache); len(runs) != 0 {
		t.Errorf("the runs open after the stop = %+v; want none", runs)
	}
}

// A paused player's ask for a file of its film, as the API makes it (Want), starts no run past what it
// keeps ahead: Want planned as the server's own start, which wants every run, and a paused player
// filling its buffer queued one at each ask (staging, 2026-10-18).
func TestWant_APausedPlayersAskStartsNoRunPastWhatItKeepsAhead(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	dir := StoreDir(1, picture, mustGrid(t, service).ID())
	service.runs.madeThrough(dir, 3, 8)
	paused := playingAt(picture, 3)
	paused.IsStopped = true
	wanted.say(paused)
	file := StoreFile{fileID: 1, kind: OnKeyframes, stream: picture, storeDir: dir, runFrom: 9, chunk: 9}
	if err := service.Want(t.Context(), file); err != nil {
		t.Fatal(err)
	}
	if runs := queue.runs(t, service); len(runs) != 0 {
		t.Errorf("a paused player's ask queued %+v; want nothing past the 30 s it keeps ahead", runs)
	}
}
