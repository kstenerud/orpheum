// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// qualityRunOf is a run of chunks of a quality, a picture encoded for players, of a film under /media.
func qualityRunOf() nodeproto.Command {
	return nodeproto.Command{Work: nodeproto.Work{Type: nodeproto.CommandMakeChunks, MakeChunks: &nodeproto.MakeChunks{
		Video:  nodeproto.BytesFile{RootPath: "/media", Path: []byte("Film.mkv")},
		Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480},
	}}}
}

// newPlayingBench is the bump bench whose tank has room for every job a test gives, its node
// encoding in software or on a GPU, with runs of chunks handled.
func newPlayingBench(t *testing.T, encodesInSoftware bool) *bumpBench {
	t.Helper()
	bench := newBumpBench(t)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(bench.resolved, nil))
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(bench.resolved, nil))
	bench.sender.resources.ReaderLimits = map[string]int{"zfs:tank": 8}
	bench.sender.resources.EncodesInSoftware = encodesInSoftware
	return bench
}

// mustAssign gives job id its command at priority, which must be given to the node.
func (bench *bumpBench) mustAssign(id int64, kind jobs.Kind, priority int64, command nodeproto.Command) {
	bench.t.Helper()
	if answer := bench.assign(id, kind, priority, command); !jobqueue.IsAssigned(answer) {
		bench.t.Fatalf("job %d was answered %v; want it given to the node", id, answer)
	}
}

// A node that encodes in software and encodes a film for a player is given no other encode, which
// waits on its encode slot; lighter work runs beside it, and on a node that encodes on a GPU an encode
// does too (D324 §5).
func TestAssign_NoOtherEncodeIsGivenBesideAPlayedEncodeInSoftware(t *testing.T) {
	t.Parallel()
	software := newPlayingBench(t, true)
	if answer := software.assign(1, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf()); !jobqueue.IsAssigned(
		answer) {
		t.Fatalf("a played quality run was answered %v; want it given", answer)
	}
	parking, isParked := jobqueue.ParkingOf(software.assign(2, jobs.KindMakeChunks, jobs.PriorityLookedAt,
		runOf(nodeproto.StreamQuality)))
	if !isParked || parking.Destination != jobs.SlotDestination(1, string(nodeproto.SlotEncode)) ||
		!parking.AwaitsSignal || !parking.IsBackPressure {
		t.Errorf("a looked-at film's run beside it was parked %v on %q; want it waiting on the encode slot",
			isParked, parking.Destination)
	}
	if answer := software.assign(3, jobs.KindReadThrough, jobs.PriorityBackground, readThroughOf("/media")); !jobqueue.
		IsAssigned(answer) {
		t.Errorf("a read-through beside it was answered %v; want it given", answer)
	}
	gpu := newPlayingBench(t, false)
	gpu.mustAssign(1, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf())
	if answer := gpu.assign(2, jobs.KindMakeChunks, jobs.PriorityLookedAt, runOf(nodeproto.StreamQuality)); !jobqueue.
		IsAssigned(answer) {
		t.Errorf("a run beside a played encode on a GPU was answered %v; want it given", answer)
	}
}

// A node held back, its played encode behind playback, stops the work no one plays and is given
// none until the hold ends, when it is again; the played work runs on, and more is given (D324 §5).
func TestHoldBackFor_StopsWhatIsNotPlayedAndGivesNoneUntilItEnds(t *testing.T) {
	t.Parallel()
	bench := newPlayingBench(t, true)
	bench.mustAssign(1, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf())
	bench.mustAssign(2, jobs.KindReadThrough, jobs.PriorityShownInterest, readThroughOf("/media"))
	if stopped := bench.exchanges.HoldBackFor(t.Context(), 1, bench.now.Now().Add(20*time.Second)); stopped != 1 {
		t.Fatalf("held back, the node's work stopped was %d; want the read-through alone", stopped)
	}
	stop := testutil.Receive(t, bench.sender.sent, "the stop")
	if stop.Type != nodeproto.CommandCancel || stop.JobID != 2 {
		t.Errorf("held back, the node was sent %+v; want the read-through told to stop", stop)
	}
	if answer := testutil.Receive(t, bench.resolved, "the stopped job's answer"); answer == nil ||
		!strings.Contains(answer.Error(), "behind playback") {
		t.Errorf("the stopped read-through was answered %v; want it bumped, the film playing behind", answer)
	}
	parking, isParked := jobqueue.ParkingOf(bench.assign(3, jobs.KindReadThrough, jobs.PriorityLookedAt,
		readThroughOf("/media")))
	if !isParked || parking.Destination != jobs.PlayingDestination(1) ||
		!parking.ProbeAt.Equal(bench.now.Now().Add(20*time.Second)) || !parking.IsBackPressure {
		t.Errorf("a read-through while held back was parked %v (%+v); want it waiting on the film playing there "+
			"until the hold ends", isParked, parking)
	}
	if answer := bench.assign(4, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf()); !jobqueue.IsAssigned(
		answer) {
		t.Errorf("played work while held back was answered %v; want it given", answer)
	}
	bench.now.Advance(20 * time.Second)
	if answer := bench.assign(5, jobs.KindReadThrough, jobs.PriorityLookedAt, readThroughOf("/media")); !jobqueue.
		IsAssigned(answer) {
		t.Errorf("a read-through once the hold ended was answered %v; want it given", answer)
	}
}

// trickplayOf is a run of a video's scrub-bar pictures, of a film under /media.
func trickplayOf() nodeproto.Command {
	return nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandMakeTrickplay, MakeTrickplay: &nodeproto.MakeTrickplay{
			Video:      nodeproto.BytesFile{RootPath: "/media", Path: []byte("Film.mkv")},
			IntervalMs: 10_000, PictureWidth: 320, SheetColumns: 10, SheetRows: 10, WebPQuality: 80,
		},
	}}
}

// A hold to measure a played run alone (D572), on a node that encodes on a GPU as on one in
// software, stops a trickplay run going there and gives none until the hold ends, when it is given
// again and resumes at its next sheet (TestTrickplay_ARunStoppedPartWayResumesAfterTheSheetsKept,
// internal/core/api); another player's run of a quality goes on, never stopped.
func TestHoldBackFor_StopsATrickplayRunAndGivesNoneUntilItEndsOnAGPUToo(t *testing.T) {
	t.Parallel()
	bench := newPlayingBench(t, false)
	bench.exchanges.Handle(jobs.KindTrickplay, assignedRun(bench.resolved, nil))
	bench.mustAssign(1, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf())
	bench.mustAssign(2, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf())
	until := bench.now.Now().Add(35 * time.Second)
	if stopped := bench.exchanges.HoldBackFor(t.Context(), 1, until); stopped != 1 {
		t.Fatalf("held back, the node's work stopped was %d; want the trickplay run alone", stopped)
	}
	if stop := testutil.Receive(t, bench.sender.sent, "the stop"); stop.Type != nodeproto.CommandCancel ||
		stop.JobID != 2 {
		t.Errorf("held back, the node was sent %+v; want the trickplay run told to stop", stop)
	}
	if answer := testutil.Receive(t, bench.resolved, "the stopped trickplay's answer"); answer == nil {
		t.Error("the stopped trickplay run was answered nothing; want it bumped")
	}
	parking, isParked := jobqueue.ParkingOf(bench.assign(3, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf()))
	if !isParked || parking.Destination != jobs.PlayingDestination(1) || !parking.ProbeAt.Equal(until) {
		t.Errorf("a trickplay run while held back was parked %v (%+v); want it waiting until the hold ends",
			isParked, parking)
	}
	bench.now.Advance(35 * time.Second)
	if answer := bench.assign(4, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf()); !jobqueue.IsAssigned(
		answer) {
		t.Errorf("a trickplay run once the hold ended was answered %v; want it given", answer)
	}
}

// A played film's trickplay run displaces the trickplay runs on its node that no one plays the film
// of, the one running and the one waiting alike, so the played film's scrub-bar pictures come first;
// another background run is given again behind it, and a run raised only for a film looked at waits
// on the slot as before (D573).
func TestAssign_APlayedFilmsTrickplayDisplacesTheBackgroundTrickplayOnItsNode(t *testing.T) {
	t.Parallel()
	bench := newPlayingBench(t, true)
	bench.exchanges.Handle(jobs.KindTrickplay, assignedRun(bench.resolved, nil))
	// The trickplay slot's one run and one more.
	bench.mustAssign(1, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf())
	bench.mustAssign(2, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf())
	bench.mustAssign(3, jobs.KindTrickplay, jobs.PriorityPlayNow, trickplayOf())
	var stopped []int64
	for _, stop := range bench.stops {
		if stop.Type == nodeproto.CommandCancel {
			stopped = append(stopped, stop.JobID)
		}
	}
	if len(stopped) != 2 || !slices.Contains(stopped, 1) || !slices.Contains(stopped, 2) {
		t.Errorf("for the played film's trickplay the node was told to stop jobs %v; want both background runs",
			stopped)
	}
	for range 2 {
		if answer := testutil.Receive(t, bench.resolved, "a displaced run's answer"); answer == nil ||
			!strings.Contains(answer.Error(), "a higher priority took its place") {
			t.Errorf("a displaced background trickplay run was answered %v; want it bumped", answer)
		}
	}
	if told := bench.sent[3]; told.Priority >= jobs.PriorityPersonsRequest || told.IsPlayDriven {
		t.Errorf("the played film's trickplay was sent at %d, play-driven %v; want background work on the node",
			told.Priority, told.IsPlayDriven)
	}
	bench.mustAssign(4, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf())
	parking, isParked := jobqueue.ParkingOf(bench.assign(5, jobs.KindTrickplay, jobs.PriorityLookedAt, trickplayOf()))
	if !isParked || parking.Destination != jobs.SlotDestination(1, string(nodeproto.SlotTrickplay)) {
		t.Errorf("a looked-at film's trickplay on a full slot was parked %v on %q; want it waiting on the slot",
			isParked, parking.Destination)
	}
}

// A trickplay run its node already holds, raised as its film is played, displaces the background
// trickplay there as one given already raised does: the film's pictures do not wait for another
// film's run to end. A raise below a play's, a film looked at, displaces nothing (D573).
func TestReprioritize_ATrickplayRaisedToAPlayedFilmsDisplacesTheBackgroundTrickplayOnItsNode(t *testing.T) {
	t.Parallel()
	bench := newPlayingBench(t, true)
	bench.exchanges.Handle(jobs.KindTrickplay, assignedRun(bench.resolved, nil))
	bench.mustAssign(1, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf())
	bench.mustAssign(2, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf())
	bench.exchanges.Reprioritize(t.Context(), 2, jobs.PriorityLookedAt)
	told := testutil.Receive(t, bench.sender.sent, "the looked-at priority")
	if told.Type != nodeproto.CommandReprioritize {
		t.Fatalf("for a looked-at film's trickplay the node was told %+v; want its new priority alone", told)
	}
	bench.exchanges.Reprioritize(t.Context(), 2, jobs.PriorityPlayNow)
	told = testutil.Receive(t, bench.sender.sent, "the played priority")
	if told.Type != nodeproto.CommandReprioritize {
		t.Fatalf("for the played film's trickplay the node was first told %+v; want its new priority", told)
	}
	stop := testutil.Receive(t, bench.sender.sent, "the background run's stop")
	if stop.Type != nodeproto.CommandCancel || stop.JobID != 1 {
		t.Errorf("for the played film's raised trickplay the node was told %+v; want job 1 stopped", stop)
	}
	if answer := testutil.Receive(t, bench.resolved, "the displaced run's answer"); answer == nil ||
		!strings.Contains(answer.Error(), "a higher priority took its place") {
		t.Errorf("the displaced background trickplay run was answered %v; want it bumped", answer)
	}
}

// A hold to measure a played run is lifted as soon as its window is made, not at the time it was
// given (D573): the work it held is brought back and given again at once. It never shortens a hold for
// an encode behind playback, which stands to its own end.
func TestEndHoldToMeasure_LiftsItsHoldAtOnceButNeverAHoldForAnEncodeBehindPlayback(t *testing.T) {
	t.Parallel()
	bench := newPlayingBench(t, false)
	bench.exchanges.Handle(jobs.KindTrickplay, assignedRun(bench.resolved, nil))
	bench.mustAssign(1, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf())
	bench.exchanges.HoldBackToMeasure(t.Context(), 1, bench.now.Now().Add(90*time.Second))
	if _, isParked := jobqueue.ParkingOf(bench.assign(2, jobs.KindTrickplay, jobs.PriorityBackground,
		trickplayOf())); !isParked {
		t.Fatal("a trickplay run while held to measure was given; want it held back")
	}
	bench.now.Advance(6 * time.Second)
	bench.exchanges.EndHoldToMeasure(t.Context(), 1)
	if !slices.Contains(bench.sender.unparked.list(), jobs.PlayingDestination(1)) {
		t.Errorf("the hold lifted, the work brought back was on %v; want the film playing there's",
			bench.sender.unparked.list())
	}
	bench.mustAssign(3, jobs.KindTrickplay, jobs.PriorityBackground, trickplayOf())

	behind := newPlayingBench(t, true)
	behind.mustAssign(1, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf())
	behindUntil := behind.now.Now().Add(20 * time.Second)
	behind.exchanges.HoldBackFor(t.Context(), 1, behindUntil)
	behind.exchanges.HoldBackToMeasure(t.Context(), 1, behind.now.Now().Add(90*time.Second))
	behind.exchanges.EndHoldToMeasure(t.Context(), 1)
	parking, isParked := jobqueue.ParkingOf(behind.assign(2, jobs.KindReadThrough, jobs.PriorityBackground,
		readThroughOf("/media")))
	if !isParked || !parking.ProbeAt.Equal(behindUntil) {
		t.Errorf("with an encode behind playback, the measuring hold lifted, a read-through was parked %v (%+v); "+
			"want it held back to the encode's hold's end", isParked, parking)
	}
	if slices.Contains(behind.sender.unparked.list(), jobs.PlayingDestination(1)) {
		t.Error("the measuring hold lifted under an encode's hold brought its work back; want it left held")
	}
}
