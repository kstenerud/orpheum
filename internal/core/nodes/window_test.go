// SPDX-License-Identifier: GPL-3.0-or-later

package nodes_test

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// prepareOf is a prepare of profile reading a film under the root on zfs:tank.
func prepareOf(profile nodeproto.PrepareProfile) nodeproto.Command {
	return nodeproto.Command{Work: nodeproto.Work{Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
		Profile: profile, Video: nodeproto.BytesFile{RootPath: "/media", Path: []byte("Film.mkv")},
	}}}
}

// assignAt gives job id its command, and says how its job was answered: the command sent, or the
// destination it parked on.
func (bench exchangeBench) assignAt(
	t *testing.T, id int64, command nodeproto.Command,
) (nodeproto.Command, jobs.Destination) {
	t.Helper()
	answer := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: id, Kind: jobs.KindPrepare}, command)
	if jobqueue.IsAssigned(answer) {
		jobqueue.Hand(t.Context(), answer)
		return testutil.Receive(t, bench.sender.sent, "the command"), ""
	}
	parking, isParked := jobqueue.ParkingOf(answer)
	if !isParked || !parking.AwaitsSignal {
		t.Fatalf("job %d was answered %v, want it assigned or waiting for room", id, answer)
	}
	return nodeproto.Command{}, parking.Destination
}

// The core gives a node only as many jobs of a slot as it runs at once, one more for the next
// (node_work_window_extra), and as many reading a device as may read it, one more: work beyond waits
// on the slot or device that is full, at no cost, and comes back when one of the node's jobs there
// ends (D240 §5).
func TestAssign_WorkBeyondANodesWindowWaitsForTheSlotOrDeviceThatIsFull(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(make(answers, 4), nil))
	bench.sender.resources = &nodes.Resources{
		Capacity:     nodeproto.Capacity{EncodeSlotCount: 4, RemuxSlotCount: 1},
		Devices:      nodeproto.Devices{{Key: "zfs:tank", MountPaths: []string{"/media"}}},
		ReaderLimits: map[string]int{"zfs:tank": 2},
	}
	remux := prepareOf(nodeproto.ProfileRemux)
	first, _ := bench.assignAt(t, 1, remux)
	if second, full := bench.assignAt(t, 2, remux); full != "" || first.JobID != 1 || second.JobID != 2 {
		t.Fatalf("remuxes 1 and 2, within the slot's one and the one more, were not both given: %q", full)
	}
	if _, full := bench.assignAt(t, 3, remux); full != jobs.SlotDestination(1, "remux") {
		t.Errorf("a third remux waits on %q, want the node's remux slot", full)
	}
	encode := prepareOf(nodeproto.ProfileH264At8M)
	if _, full := bench.assignAt(t, 4, encode); full != "" {
		t.Fatalf("an encode, with tank's two readers and one more, waits on %q", full)
	}
	if _, full := bench.assignAt(t, 5, encode); full != jobs.ReaderDestination(1, "zfs:tank") {
		t.Errorf("a fourth job reading tank waits on %q, want tank's readers", full)
	}
	deliver(t, bench.exchanges, 1, nodeproto.Result{
		JobID: 1, AttemptToken: first.AttemptToken,
		Prepare: &nodeproto.PrepareReport{State: nodeproto.PrepareDone}, Work: &remux.Work,
	})
	unparked := bench.unparked.list()
	if !slices.Contains(unparked, jobs.SlotDestination(1, "remux")) ||
		!slices.Contains(unparked, jobs.ReaderDestination(1, "zfs:tank")) {
		t.Errorf("once remux 1 ended, the exchanges brought back %v; want its slot and its device", unparked)
	}
}

// A job no one waits for any more is told to its node at its new priority, no longer play-driven,
// by its attempt's token (D234 §3, D240 §6).
func TestReprioritize_TellsTheNodeHoldingAJobsAttemptItsNewPriority(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindPrepare, assignedPrepare(make(answers, 2), nil))
	sent := bench.assign(t)
	bench.exchanges.Reprioritize(7, jobs.PriorityBackground)
	told := testutil.Receive(t, bench.sender.sent, "the reprioritize command")
	if told.Type != nodeproto.CommandReprioritize || told.JobID != 7 || told.AttemptToken != sent.AttemptToken ||
		told.Priority != jobs.PriorityBackground || told.IsPlayDriven {
		t.Errorf("the node was told %+v; want job 7's attempt at background priority, not play-driven", told)
	}
}
