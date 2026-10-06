// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"errors"
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
				stage.service.runs.asked(dir, now)
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
	if err := stage.service.Ensure(t.Context(), 1, newer, 5); !errors.Is(err, ErrNoRoomToMake) {
		t.Errorf("a player asking for the evicted store's chunks: %v; want ErrNoRoomToMake", err)
	}
	stage.service.runs.unevict(stage.newer)
	if err := stage.service.Ensure(t.Context(), 1, newer, 5); err != nil {
		t.Errorf("a stall asking for it again: %v; want a run queued", err)
	}
}

// The oldest run no player has asked for lately is evicted first, however fast it is.
func TestKeepRunsFast_EvictsTheOldestRunNoOneIsPlayingFrom(t *testing.T) {
	t.Parallel()
	stage := newCapacityStage(t)
	stage.progress(t, 3, 1.05, func(dir string) bool { return dir == stage.newer })
	if !slices.Equal(stage.queue.cancelledJobs, []int64{11}) {
		t.Errorf("the jobs cancelled = %v; want the older run's, 11, which no one plays from",
			stage.queue.cancelledJobs)
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
