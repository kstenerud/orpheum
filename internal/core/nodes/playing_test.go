// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
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
// encoding in software or on a GPU, with prepares and runs of chunks handled.
func newPlayingBench(t *testing.T, encodesInSoftware bool) *bumpBench {
	t.Helper()
	bench := newBumpBench(t)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(bench.resolved, nil))
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedPrepare(bench.resolved, nil))
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
	parking, isParked := jobqueue.ParkingOf(software.assign(2, jobs.KindPrepare, jobs.PriorityLookedAt,
		prepareOf(nodeproto.ProfileH264)))
	if !isParked || parking.Destination != jobs.SlotDestination(1, string(nodeproto.SlotEncode)) ||
		!parking.AwaitsSignal || !parking.IsBackPressure {
		t.Errorf("a looked-at film's prepare beside it was parked %v on %q; want it waiting on the encode slot",
			isParked, parking.Destination)
	}
	if answer := software.assign(3, jobs.KindReadThrough, jobs.PriorityBackground, readThroughOf("/media")); !jobqueue.
		IsAssigned(answer) {
		t.Errorf("a read-through beside it was answered %v; want it given", answer)
	}
	gpu := newPlayingBench(t, false)
	gpu.mustAssign(1, jobs.KindMakeChunks, jobs.PriorityPlayNow, qualityRunOf())
	if answer := gpu.assign(2, jobs.KindPrepare, jobs.PriorityLookedAt, prepareOf(nodeproto.ProfileH264)); !jobqueue.
		IsAssigned(answer) {
		t.Errorf("a prepare beside a played encode on a GPU was answered %v; want it given", answer)
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
