// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// examineOf is a scan's examining of a film under /media.
func examineOf() nodeproto.Command {
	return nodeproto.Command{Work: nodeproto.Work{Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{
		RootPath: "/media", Files: []nodeproto.ExamineFile{{Path: nodeproto.Path{Text: "films/Film.mkv"}}},
	}}}
}

// bumpBench is exchanges whose node's one disk, tank, has one reader, recording each job's command,
// the stops sent before it, and the answers bumped jobs are resolved with.
type bumpBench struct {
	exchangeBench
	t        *testing.T
	resolved answers
	sent     map[int64]nodeproto.Command
	stops    []nodeproto.Command
}

func newBumpBench(t *testing.T) *bumpBench {
	t.Helper()
	bench := &bumpBench{
		exchangeBench: newExchanges(t), t: t, resolved: make(answers, 8), sent: map[int64]nodeproto.Command{},
	}
	bench.exchanges.Handle(jobs.KindReadThrough, assignedPrepare(bench.resolved, nil))
	bench.exchanges.Handle(jobs.KindExamine, assignedPrepare(bench.resolved, nil))
	bench.sender.resources = &nodes.Resources{
		Capacity:     nodeproto.Capacity{EncodeSlotCount: 4, RemuxSlotCount: 4},
		Devices:      nodeproto.Devices{{Key: "zfs:tank", MountPaths: []string{"/media"}}},
		ReaderLimits: map[string]int{"zfs:tank": 1},
	}
	return bench
}

// assign gives job id its command at priority, and says how it was answered. A job bumped for it is
// told to stop first, so its reader is free before the work comes: that stop is kept.
func (bench *bumpBench) assign(id int64, kind jobs.Kind, priority int64, command nodeproto.Command) error {
	bench.t.Helper()
	answer := bench.exchanges.Assign(bench.t.Context(), 1, jobs.Job{ID: id, Kind: kind, Priority: priority}, command)
	if !jobqueue.IsAssigned(answer) {
		return answer
	}
	jobqueue.HandOver(bench.t.Context(), answer)
	for {
		told := testutil.Receive(bench.t, bench.sender.sent, "the command")
		if told.JobID == id && told.Type != nodeproto.CommandCancel {
			bench.sent[id] = told
			return answer
		}
		bench.stops = append(bench.stops, told)
	}
}

// wantBumped checks that job id, and only it, was told to stop and bumped for the work named.
func (bench *bumpBench) wantBumped(id int64, forWork string) {
	bench.t.Helper()
	if len(bench.stops) != 1 {
		bench.t.Fatalf("for %s, the node was told to stop %+v; want one job", forWork, bench.stops)
	}
	stop := bench.stops[0]
	bench.stops = nil
	if stop.Type != nodeproto.CommandCancel || stop.JobID != id || stop.AttemptToken != bench.sent[id].AttemptToken {
		bench.t.Errorf("for %s, the node was sent %+v; want job %d told to stop", forWork, stop, id)
	}
	answer := testutil.Receive(bench.t, bench.resolved, "the bumped job's answer")
	if answer == nil || !strings.Contains(answer.Error(), "a higher priority took its place") {
		bench.t.Errorf("for %s, job %d was answered %v; want it bumped", forWork, id, answer)
	}
}

// Where a disk's readers are full, work of a higher tier bumps the lowest held there, the newest of
// it: its node is told to stop, and its job goes back to its queue, bumped; work of the same tier or
// lower waits as it did (D255 §1, §2).
func TestAssign_WorkOfAHigherTierBumpsTheLowestWhereNothingIsFree(t *testing.T) {
	t.Parallel()
	bench := newBumpBench(t)
	// Tank's one reader and one more: two read-throughs, slow.
	for _, id := range []int64{1, 2} {
		answer := bench.assign(id, jobs.KindReadThrough, jobs.PriorityBackground, readThroughOf("/media"))
		if !jobqueue.IsAssigned(answer) {
			t.Fatalf("read-through %d within tank's readers was answered %v", id, answer)
		}
		bench.now.Advance(time.Second)
	}
	// A scan's examining is fast: it bumps the newer read-through.
	if answer := bench.assign(3, jobs.KindExamine, jobs.PriorityBackground, examineOf()); !jobqueue.IsAssigned(answer) {
		t.Fatalf("an examining on full tank was answered %v; want it given, in a read-through's place", answer)
	}
	bench.wantBumped(2, "the examining")
	// Another read-through is slow, as one held is: it waits for tank's readers.
	if parking, _ := jobqueue.ParkingOf(bench.assign(4, jobs.KindReadThrough, jobs.PriorityBackground,
		readThroughOf("/media"))); parking.Destination != jobs.ReaderDestination(1, "zfs:tank") {
		t.Errorf("a read-through on full tank waits on %q; want tank's readers", parking.Destination)
	}
	// A scan a person asked for bumps the lowest held, the read-through, not the examining.
	if answer := bench.assign(5, jobs.KindExamine, jobs.PriorityPersonsRequest, examineOf()); !jobqueue.IsAssigned(
		answer) {
		t.Fatalf("a person's examining on full tank was answered %v; want it given", answer)
	}
	bench.wantBumped(1, "the person's examining")
}

// Two works bumping at once bump two attempts: one chosen is not chosen again while it is told to
// stop and resolved, so the second does not count room the first already took (the M4 closing
// audit, §6).
func TestAssign_TwoBumpsAtOnceBumpTwoAttempts(t *testing.T) {
	t.Parallel()
	bench := newBumpBench(t)
	for _, id := range []int64{1, 2} { // tank's one reader and one more
		if answer := bench.assign(id, jobs.KindReadThrough, jobs.PriorityBackground,
			readThroughOf("/media")); !jobqueue.IsAssigned(answer) {
			t.Fatalf("read-through %d within tank's readers was answered %v", id, answer)
		}
		bench.now.Advance(time.Second)
	}
	// The second examining comes while the first's bump is telling read-through 2 to stop.
	var during error
	bench.sender.onWithdraw = func() {
		during = bench.exchanges.Assign(t.Context(), 1, jobs.Job{
			ID: 4, Kind: jobs.KindExamine,
			Priority: jobs.PriorityBackground,
		}, examineOf())
	}
	if answer := bench.assign(3, jobs.KindExamine, jobs.PriorityBackground, examineOf()); !jobqueue.IsAssigned(answer) {
		t.Fatalf("the first examining on full tank was answered %v; want it given", answer)
	}
	if !jobqueue.IsAssigned(during) {
		t.Fatalf("the examining during the first's bump was answered %v; want it given", during)
	}
	stopped := map[int64]bool{}
	for _, stop := range bench.stops {
		stopped[stop.JobID] = true
	}
	if !stopped[1] || !stopped[2] || len(bench.stops) != 2 {
		t.Errorf("the node was told to stop %+v; want read-throughs 1 and 2, once each", bench.stops)
	}
}
