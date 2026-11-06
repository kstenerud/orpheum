// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// jumpStage is film 1's picture played from chunk 1, its run from there, 1 through 30, on its way:
// given to node 1 now, as the core gives a run (given), or learned from its progress (begin), as a
// core started again learns it.
type jumpStage struct {
	service *Service
	queue   *fakeQueue
	clock   *clocktest.Fake
	wanted  *wanting
	picture Stream
	grid    string
	dir     string
}

func newJumpStage(t *testing.T, isGiven bool) jumpStage {
	t.Helper()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	fakeClock, isFake := service.clock.(*clocktest.Fake)
	if !isFake {
		t.Fatal("the fixture's clock is not a fake")
	}
	picture := Stream{Kind: nodeproto.StreamPicture}
	stage := jumpStage{
		service: service, queue: queue, clock: fakeClock, wanted: wantedBy(service), picture: picture,
		grid: mustGrid(t, service).ID(),
	}
	stage.dir = testStoreDir(t, service, picture, stage.grid)
	stage.wanted.say(playingAt(picture, 1))
	askedFor(t, service, picture, 1)
	stage.start(1, 21, isGiven)
	return stage
}

// start has the store's run from first, job jobID, on its way from now.
func (stage jumpStage) start(first int, jobID int64, isGiven bool) {
	if isGiven {
		stage.service.runs.given(stage.dir, first, first+29, jobID, 1, stage.clock.Now())
		return
	}
	stage.service.runs.begin(stage.dir, first, first+29, jobID, 1, stage.clock.Now())
}

// makes has the run from first make chunk n after the wall time given, its progress said as the
// node's is: each chunk is ten seconds of film.
func (stage jumpStage) makes(first, n int, after time.Duration) {
	stage.clock.Advance(after)
	stage.service.runs.madeThrough(stage.dir, first, n)
	stage.service.runs.sample(stage.dir, first, stage.clock.Now(), int64(n+1)*10_000)
}

// chunkOf is chunk n of film 1's stream on its keyframe grid gridID, as a player asks for it.
func chunkOf(t *testing.T, service *Service, stream Stream, gridID string, n int) StoreFile {
	t.Helper()
	return StoreFile{
		fileID: 1, kind: OnKeyframes, stream: stream, storeDir: testStoreDir(t, service, stream, gridID), runFrom: n,
		chunk: n,
	}
}

// jumpsTo has the player jump to chunk n and ask for it there.
func (stage jumpStage) jumpsTo(t *testing.T, n int) {
	t.Helper()
	stage.wanted.say(playingAt(stage.picture, n))
	sought := chunkOf(t, stage.service, stage.picture, stage.grid, n).AsSought()
	if err := stage.service.Want(t.Context(), sought); err != nil {
		t.Fatal(err)
	}
}

// A jump ahead of a measured run is left to wait for it while waiting costs less than a new run's
// start, and stops it once waiting costs more (D353): the run makes ten seconds of film a second and
// started in five, so a jump 40 s ahead, which Jellyfin's 24 s would restart, waits 4 s, and one
// 60 s ahead, which the minute before D353 kept, would wait 6 s and restarts.
func TestStopUnwanted_AJumpAheadOfAMeasuredRunWaitsWhileWaitingCostsLessThanRestarting(t *testing.T) {
	t.Parallel()
	stage := newJumpStage(t, true)
	stage.makes(1, 1, 5*time.Second)
	stage.makes(1, 2, time.Second)
	stage.jumpsTo(t, 7)
	if len(stage.queue.cancelledFirsts) != 0 {
		t.Fatalf("a jump 40 s past what the run makes cancelled %v; want it waiting the 4 s",
			stage.queue.cancelledFirsts)
	}
	stage.jumpsTo(t, 9)
	if !slices.Equal(stage.queue.cancelledFirsts, []int{1}) {
		t.Errorf("a jump 60 s past what the run makes cancelled %v; want the run from 1", stage.queue.cancelledFirsts)
	}
	if runs := stage.queue.runs(t, stage.service); len(runs) != 2 || runs[1].firstChunk != 9 {
		t.Errorf("the runs asked for = %+v; want the first, then one from the jump's chunk 9", runs)
	}
}

// A run's start is its store's last run's: a run stopped after starting in 5 s, and one after it
// started in 1 s making ten seconds of film a second, leave a jump 40 s ahead of the second waiting
// 4 s against a 1 s start, and it restarts (D353).
func TestStopUnwanted_AJumpIsJudgedByTheStartOfItsStoresLastRun(t *testing.T) {
	t.Parallel()
	stage := newJumpStage(t, true)
	stage.makes(1, 1, 5*time.Second)
	stage.service.runs.forget(stage.dir, 1)
	stage.start(2, 22, true)
	stage.makes(2, 2, time.Second)
	stage.makes(2, 3, time.Second)
	stage.jumpsTo(t, 8)
	if run, isCovered := stage.service.runs.covering(stage.dir, 4); isCovered {
		t.Errorf("a jump 40 s past a run that started in 1 s left %+v; want it stopped", run)
	}
	if runs := stage.queue.runs(t, stage.service); runs[len(runs)-1].firstChunk != 8 {
		t.Errorf("the runs asked for = %+v; want the last from the jump's chunk 8", runs)
	}
}

// However fast a run would start again, a player no more than two chunks past what it makes leaves
// it going: a chunk served is known made before the run's progress says it (D353 §4, D355 §4). The
// run makes two seconds of film a second and started in one, so restarting costs less than any wait.
func TestStopUnwanted_ARunIsNeverStoppedForAPlayerTwoChunksPastIt(t *testing.T) {
	t.Parallel()
	stage := newJumpStage(t, true)
	stage.makes(1, 1, time.Second)
	stage.makes(1, 2, 5*time.Second)
	movesTo := func(n int) {
		stage.wanted.say(playingAt(stage.picture, n))
		if err := stage.service.StopUnwanted(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
	}
	movesTo(5)
	if len(stage.queue.cancelledFirsts) != 0 {
		t.Fatalf("a player two chunks past what the run makes cancelled %v; want none", stage.queue.cancelledFirsts)
	}
	movesTo(6)
	if !slices.Equal(stage.queue.cancelledFirsts, []int{1}) {
		t.Errorf("a player three chunks past what the run makes cancelled %v; want the run from 1",
			stage.queue.cancelledFirsts)
	}
}

// A run the core learned from its progress, after a restart, has no start it saw, and is judged by
// Jellyfin's 24 s however fast it makes film: a jump 40 s ahead of it restarts (D353). Its first
// progress, 5 s after it was learned, is not its start.
func TestStopUnwanted_ARunLearnedFromItsProgressIsJudgedByJellyfinsGap(t *testing.T) {
	t.Parallel()
	stage := newJumpStage(t, false)
	stage.makes(1, 1, 5*time.Second)
	stage.makes(1, 2, time.Second)
	stage.jumpsTo(t, 7)
	if !slices.Equal(stage.queue.cancelledFirsts, []int{1}) {
		t.Errorf("a jump 40 s past a run not measured cancelled %v; want the run from 1", stage.queue.cancelledFirsts)
	}
}

// A run its own stream's player waits on is not stopped by where the other stream is: a picture asked
// for 40 s past the sound its run has not made yet, as hls.js loads a picture while its sound is late,
// left the sound's run cut and its asks refused, the sound waiting for want of a run (D353). The
// picture's ask moves the picture's position alone (D355 §1), and the sound's player is still where
// it asked.
func TestStopUnwanted_ARunItsOwnStreamWaitsOnGoesOnWhereverTheOtherStreamIs(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	sound := Stream{Kind: nodeproto.StreamSound, SoundKbps: 96}
	grid := mustGrid(t, service).ID()
	ask := func(stream Stream, n int, interests ...Interest) {
		wanted.say(interests...)
		if err := service.Want(t.Context(), chunkOf(t, service, stream, grid, n)); err != nil {
			t.Fatal(err)
		}
	}
	ask(sound, 1, playingAt(picture, 1), playingAt(sound, 1))
	ask(picture, 5, playingAt(picture, 5), playingAt(sound, 1))
	if len(queue.cancelledFirsts) != 0 {
		t.Errorf("a picture asked for 40 s past its sound cancelled %v; want the sound's run going on",
			queue.cancelledFirsts)
	}
}

// A run stopped for a jump says, at debug, the numbers it was judged by, so a report is explained from
// the log alone (D353).
func TestStopUnwanted_SaysTheNumbersAJumpWasJudgedBy(t *testing.T) {
	t.Parallel()
	stage := newJumpStage(t, true)
	var logs bytes.Buffer
	stage.service.logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	stage.makes(1, 1, 5*time.Second)
	stage.makes(1, 2, time.Second)
	stage.jumpsTo(t, 9)
	for _, line := range []string{
		testutil.EventInJSON("chunkRunJumpedAway"), `"makingChunk":3`,
		`"neededChunk":9`, `"gapMs":60000`, `"encoderSpeed":10`, `"startupMs":5000`, `"thresholdMs":50000`,
		`"isMeasured":true`,
	} {
		if !strings.Contains(logs.String(), line) {
			t.Errorf("the log lacks %s:\n%s", line, logs.String())
		}
	}
}

// A sound made alongside a sought picture is where its player waits for it, its position moved there
// with the picture's (D355 §2): the picture's ask then stops the sound's run where the player was,
// as the minute before D353 did.
func TestWantAlongside_ASoughtPicturesSoundStopsTheSoundsRunWhereThePlayerWas(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	wanted := wantedBy(service)
	picture := Stream{Kind: nodeproto.StreamPicture}
	sound := Stream{Kind: nodeproto.StreamSound, SoundKbps: 96}
	grid := mustGrid(t, service).ID()
	wanted.say(playingAt(picture, 1), playingAt(sound, 1))
	if err := service.Want(t.Context(), chunkOf(t, service, sound, grid, 1)); err != nil {
		t.Fatal(err)
	}
	wanted.say(playingAt(picture, 9), playingAt(sound, 9))
	sought := chunkOf(t, service, picture, grid, 9).AsSought()
	if err := service.WantAlongside(t.Context(), sought, chunkOf(t, service, sound, grid, 1)); err != nil {
		t.Fatal(err)
	}
	if err := service.Want(t.Context(), sought); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(queue.cancelledFirsts, []int{1}) {
		t.Errorf("a picture sought 80 s on cancelled %v; want the sound's run from 1", queue.cancelledFirsts)
	}
}

// A player's word that it sought away stops the run it left at once, though the store's last ask was
// the player's own, near what the run makes, before it sought (D355 §4): each stream's position is
// its own player's, so a store's last ask, which outlived the seek, no longer keeps the run until the
// player's first ask where it landed. The run makes ten seconds of film a second and started in five,
// so a jump 60 s past it restarts.
func TestStopUnwanted_AWordOfASeekStopsTheRunItLeftThoughTheStoresLastAskWasNearIt(t *testing.T) {
	t.Parallel()
	stage := newJumpStage(t, true)
	stage.makes(1, 1, 5*time.Second)
	stage.makes(1, 2, time.Second)
	stage.wanted.say(playingAt(stage.picture, 3))
	if err := stage.service.Want(t.Context(), chunkOf(t, stage.service, stage.picture, stage.grid, 3)); err != nil {
		t.Fatal(err)
	}
	stage.wanted.say(playingAt(stage.picture, 9))
	if err := stage.service.StopUnwanted(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stage.queue.cancelledFirsts, []int{1}) {
		t.Errorf("a word of a seek 60 s past the run cancelled %v; want the run from 1", stage.queue.cancelledFirsts)
	}
}
