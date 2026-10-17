// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// quality is the stream of a quality of kbps at height.
func quality(kbps, height int) Stream {
	return Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: kbps, Height: height}}
}

// capacityStage is a chunks service whose node 1 is making two runs, an older of q1200k480p and a
// newer of q700k432p, both asked for by their players.
type capacityStage struct {
	service      *Service
	queue        *fakeQueue
	clock        *clocktest.Fake
	older, newer string
}

func newCapacityStage(t *testing.T) capacityStage {
	t.Helper()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	fakeClock, isFake := service.clock.(*clocktest.Fake)
	if !isFake {
		t.Fatal("the fixture's clock is not a fake")
	}
	gridID := mustGrid(t, service).ID()
	stage := capacityStage{
		service: service, queue: queue, clock: fakeClock,
		older: StoreDir(1, quality(1200, 480), gridID),
		newer: StoreDir(1, quality(700, 432), gridID),
	}
	service.runs.begin(stage.older, 0, 29, 11, 1, fakeClock.Now())
	fakeClock.Advance(time.Second)
	service.runs.begin(stage.newer, 0, 29, 12, 1, fakeClock.Now())
	return stage
}

// progress samples both runs over 15 s, the older making film at olderSpeed and the newer at
// newerSpeed times playback, each asked for by its players while isAsked says so; then the node's
// runs are kept fast.
func (stage capacityStage) progress(t *testing.T, olderSpeed, newerSpeed float64, isAsked func(dir string) bool) {
	t.Helper()
	start := stage.clock.Now()
	for step := range 4 {
		now := stage.clock.Now()
		elapsedMs := float64(now.Sub(start).Milliseconds())
		stage.service.runs.sample(stage.older, 0, now, int64(olderSpeed*elapsedMs))
		stage.service.runs.sample(stage.newer, 0, now, int64(newerSpeed*elapsedMs))
		for _, dir := range []string{stage.older, stage.newer} {
			if isAsked(dir) {
				stage.service.runs.asked(dir, 0, 0, now)
			}
		}
		if step < 3 {
			stage.clock.Advance(5 * time.Second)
		}
	}
	if err := stage.service.keepRunsFast(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
}

// When one of a node's runs falls below 1.1x playback and every run is in use, the newest is evicted,
// and its store refuses its players with the reason until a stall asks for it again (D250 §6).
func TestKeepRunsFast_EvictsTheNewestWhenEveryRunIsInUse(t *testing.T) {
	t.Parallel()
	stage := newCapacityStage(t)
	stage.progress(t, 3, 1.05, func(string) bool { return true })
	if !slices.Equal(stage.queue.cancelledJobs, []int64{12}) {
		t.Fatalf("the jobs cancelled = %v; want the newer run's, 12", stage.queue.cancelledJobs)
	}
	newer := quality(700, 432)
	if err := stage.service.Ensure(t.Context(), 1, OnKeyframes, newer, 5); !errors.Is(err, ErrNoRoomToMake) {
		t.Errorf("a player asking for the evicted store's chunks: %v; want ErrNoRoomToMake", err)
	}
	stage.service.runs.unevict(stage.newer)
	if err := stage.service.Ensure(t.Context(), 1, OnKeyframes, newer, 5); err != nil {
		t.Errorf("a stall asking for it again: %v; want a run queued", err)
	}
}

// An evicted store still gives its players what is made and what another of its runs is making, and
// starts no run for them: only a player its eviction left without its chunks is refused (D250 §6). A
// chunk made was refused, answered 500 as it was served, while a run of its store made the rest at
// 2.5x playback (the owner's Wings on nas.example, 2026-10-17).
func TestEnsure_AnEvictedStoreGivesWhatIsMadeOrOnItsWayAndStartsNoRun(t *testing.T) {
	t.Parallel()
	stage := newCapacityStage(t)
	// A second run of the newer store, older than its first, which its players are asking for too.
	stage.service.runs.begin(stage.newer, 30, 59, 13, 1, stage.clock.Now())
	stage.service.runs.storeByDir[stage.newer].runByFirst[30].startedAt = stage.clock.Now().Add(-time.Minute)
	stage.service.runs.asked(stage.newer, 30, 30, stage.clock.Now())
	stage.service.runs.madeThrough(stage.newer, 0, 27)
	stage.progress(t, 3, 1.05, func(string) bool { return true })
	if !slices.Contains(stage.queue.cancelledJobs, 12) {
		t.Fatalf("the jobs cancelled = %v; want the newer run from 0's, 12", stage.queue.cancelledJobs)
	}
	newer := quality(700, 432)
	queued := len(stage.queue.requests)
	for _, chunk := range []int{27, 31} {
		if err := stage.service.Ensure(t.Context(), 1, OnKeyframes, newer, chunk); err != nil {
			t.Errorf("chunk %d, made or on its way: %v; want it given", chunk, err)
		}
	}
	if len(stage.queue.requests) != queued {
		t.Errorf("runs queued for an evicted store: %v; want none", stage.queue.requests[queued:])
	}
	if err := stage.service.Ensure(t.Context(), 1, OnKeyframes, newer, 28); !errors.Is(err, ErrNoRoomToMake) {
		t.Errorf("chunk 28, which the evicted run was to make: %v; want ErrNoRoomToMake", err)
	}
}

// A run in use whose players can all move onto a quality already made for another playback of the
// film is cancelled, its players moved, and its store not evicted: they are not stopped (D286 §4).
// One whose players cannot move is evicted as before.
func TestKeepRunsFast_MovesARunsPlayersOntoAQualityAlreadyMadeRatherThanEvictIt(t *testing.T) {
	t.Parallel()
	stage := newCapacityStage(t)
	var asked []string
	stage.service.MovePlayersBy(func(fileID int64, streamName string) bool {
		asked = append(asked, fmt.Sprintf("%d %s", fileID, streamName))
		return true
	})
	stage.progress(t, 3, 1.05, func(string) bool { return true })
	if !slices.Equal(stage.queue.cancelledJobs, []int64{12}) || !slices.Equal(asked, []string{"1 q700k432p"}) {
		t.Fatalf("the jobs cancelled = %v, players asked to move %v; want the newer run's, its players moved",
			stage.queue.cancelledJobs, asked)
	}
	if err := stage.service.Ensure(t.Context(), 1, OnKeyframes, quality(700, 432), 5); err != nil {
		t.Errorf("a player asking for the quality again: %v; want a run queued, not evicted", err)
	}

	unmoved := newCapacityStage(t)
	unmoved.service.MovePlayersBy(func(int64, string) bool { return false })
	unmoved.progress(t, 3, 1.05, func(string) bool { return true })
	err := unmoved.service.Ensure(t.Context(), 1, OnKeyframes, quality(700, 432), 5)
	if !errors.Is(err, ErrNoRoomToMake) {
		t.Errorf("a run whose players could not move: %v; want it evicted, ErrNoRoomToMake", err)
	}
}

// The oldest run no player has asked for lately is cancelled first, however fast it is, and its store
// refuses no one, since no one was left without its chunks.
func TestKeepRunsFast_CancelsTheOldestRunNoOneIsPlayingFromRefusingNoOne(t *testing.T) {
	t.Parallel()
	stage := newCapacityStage(t)
	stage.progress(t, 3, 1.05, func(dir string) bool { return dir == stage.newer })
	if !slices.Equal(stage.queue.cancelledJobs, []int64{11}) {
		t.Errorf("the jobs cancelled = %v; want the older run's, 11, which no one plays from",
			stage.queue.cancelledJobs)
	}
	// No one was left without chunks, so a player coming to the quality later is refused nothing: the
	// owner's friend's player was refused its quality for good (2026-10-12).
	if err := stage.service.Ensure(t.Context(), 1, OnKeyframes, quality(1200, 480), 5); err != nil {
		t.Errorf("a player asking later for the cancelled run's store: %v; want a new run", err)
	}
}

// Runs all faster than 1.1x, or one run alone however slow, evict nothing; after an eviction the
// node's runs are left to settle before another.
func TestKeepRunsFast_EvictsNothingWhileEveryRunKeepsUpOrOneRunsAlone(t *testing.T) {
	t.Parallel()
	stage := newCapacityStage(t)
	stage.progress(t, 3, 1.2, func(string) bool { return true })
	if len(stage.queue.cancelledJobs) != 0 {
		t.Fatalf("runs at 3x and 1.2x: cancelled %v; want none", stage.queue.cancelledJobs)
	}
	stage.progress(t, 3, 1.05, func(string) bool { return true })
	stage.progress(t, 0.5, 0.5, func(string) bool { return true }) // within the settling: nothing
	stage.clock.Advance(speedWindow)
	stage.progress(t, 0.5, 0.5, func(string) bool { return true }) // the newer is gone; the older runs alone
	if !slices.Equal(stage.queue.cancelledJobs, []int64{12}) {
		t.Errorf("the jobs cancelled = %v; want the newer run's alone", stage.queue.cancelledJobs)
	}
}

// A run of a quality a player still plays, but from far before it after a seek back, is no run anyone
// plays from, and is evicted before any that is: the run a seek back left behind kept the store in
// use, and the sound everyone needed was evicted in its place (the owner's test, 2026-10-12).
func TestKeepRunsFast_EvictsARunASeekLeftThoughItsQualityIsStillPlayed(t *testing.T) {
	t.Parallel()
	stage := newCapacityStage(t)
	left, playedFrom := stage.older, stage.newer
	// The older store's run is from chunk 0; its player sought back to before it, to a run of its own
	// from 60, which the node runs at the same time; the newer store is the sound, played from 0.
	stage.service.runs.begin(left, 60, 89, 13, 1, stage.clock.Now())
	start := stage.clock.Now()
	for step := range 4 {
		now := stage.clock.Now()
		elapsedMs := float64(now.Sub(start).Milliseconds())
		stage.service.runs.sample(left, 0, now, int64(elapsedMs))
		stage.service.runs.sample(left, 60, now, int64(1.05*elapsedMs))
		stage.service.runs.sample(playedFrom, 0, now, int64(1.05*elapsedMs))
		stage.service.runs.asked(left, 60, 66, now)
		stage.service.runs.asked(playedFrom, 0, 6, now)
		if step < 3 {
			stage.clock.Advance(5 * time.Second)
		}
	}
	if err := stage.service.keepRunsFast(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stage.queue.cancelledJobs, []int64{11}) {
		t.Errorf("the jobs cancelled = %v; want the run the seek left, 11", stage.queue.cancelledJobs)
	}
}
