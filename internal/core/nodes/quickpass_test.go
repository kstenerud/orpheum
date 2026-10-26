// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// fakeQuickPasses are the roots of node 1 a scan is on its quick pass of, as a test sets them.
type fakeQuickPasses struct {
	mutex sync.Mutex
	roots []string // guarded by mutex
}

func (passes *fakeQuickPasses) RootsOnQuickPass(_ context.Context, nodeID int64) ([]string, error) {
	passes.mutex.Lock()
	defer passes.mutex.Unlock()
	if nodeID != 1 {
		return nil, nil
	}
	return passes.roots, nil
}

func (passes *fakeQuickPasses) set(roots ...string) {
	passes.mutex.Lock()
	defer passes.mutex.Unlock()
	passes.roots = roots
}

// readThroughOf is a read-through of a film under root.
func readThroughOf(root string) nodeproto.Command {
	video := nodeproto.BytesFile{RootPath: root, Path: []byte("Film.mkv")}
	return nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandReadThrough, ReadThrough: &nodeproto.ReadThrough{Video: video},
	}}
}

// While a scan is on its quick pass of a disk, a read-through no one waits for, which reads a whole
// film, waits on the disk's quick pass, not its readers, and asks again in half a minute; the scan's
// own examining, a read-through on another disk and one a person waits for go at once, and once the
// quick pass ends the read-through no longer waits for it (D255).
func TestAssign_ABackgroundReadThroughWaitsForAScansQuickPassOfItsDisk(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindReadThrough, assignedRun(make(answers, 8), nil))
	bench.exchanges.Handle(jobs.KindExamine, assignedRun(make(answers, 8), nil))
	bench.sender.resources = &nodes.Resources{
		Capacity: nodeproto.Capacity{EncodeSlotCount: 4, RemuxSlotCount: 4},
		Devices: nodeproto.Devices{
			{Key: "zfs:tank", MountPaths: []string{"/media"}}, {Key: "disk:sdb", MountPaths: []string{"/other"}},
		},
		ReaderLimits: map[string]int{"zfs:tank": 8, "disk:sdb": 8},
	}
	passes := &fakeQuickPasses{}
	passes.set("/media/films")
	bench.exchanges.SetQuickPasses(passes)
	assign := func(id int64, kind jobs.Kind, priority int64, command nodeproto.Command) error {
		t.Helper()
		answer := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: id, Kind: kind, Priority: priority}, command)
		if jobqueue.IsAssigned(answer) {
			jobqueue.HandOver(t.Context(), answer)
		}
		return answer
	}
	answer := assign(1, jobs.KindReadThrough, jobs.PriorityBackground, readThroughOf("/media"))
	parking, isParked := jobqueue.ParkingOf(answer)
	if !isParked || parking.Destination != jobs.QuickPassDestination(1, "zfs:tank") || parking.AwaitsSignal ||
		parking.ProbeAt.IsZero() || !parking.IsBackPressure {
		t.Fatalf("a background read-through on tank during its quick pass was answered %v (%+v); want it "+
			"waiting its turn on tank's quick pass, not a destination down, asking again", answer, parking)
	}
	examine := nodeproto.Command{Work: nodeproto.Work{Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{
		RootPath: "/media", Files: []nodeproto.ExamineFile{{Path: nodeproto.Path{Text: "films/Film.mkv"}}},
	}}}
	for _, goes := range []struct {
		what     string
		id       int64
		kind     jobs.Kind
		priority int64
		command  nodeproto.Command
	}{
		{"the scan's examining of tank", 2, jobs.KindExamine, jobs.PriorityBackground, examine},
		{
			"a background read-through on another disk", 3, jobs.KindReadThrough, jobs.PriorityBackground,
			readThroughOf("/other"),
		},
		{
			"a read-through a person waits for on tank", 4, jobs.KindReadThrough, jobs.PriorityPlayNow,
			readThroughOf("/media"),
		},
	} {
		if given := assign(goes.id, goes.kind, goes.priority, goes.command); !jobqueue.IsAssigned(given) {
			t.Errorf("%s was answered %v; want it given to the node", goes.what, given)
		}
	}
	// Once the quick pass ends it waits for nothing of the scan's: here, only the read-through slot
	// that 3 and 4 fill.
	passes.set()
	answer = assign(5, jobs.KindReadThrough, jobs.PriorityBackground, readThroughOf("/media"))
	if parking, isParked := jobqueue.ParkingOf(answer); isParked &&
		parking.Destination == jobs.QuickPassDestination(1, "zfs:tank") {
		t.Errorf("a background read-through on tank after its quick pass still waits on it: %v", answer)
	}
}

// A read-through of a film someone looks at, a person's work below a play's priority, goes ahead of
// a scan's quick pass of its disk as a play's does; one at the shown-interest rung waits (D324 §4).
func TestAssign_APersonsReadThroughGoesAheadOfAScansQuickPass(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	bench.exchanges.Handle(jobs.KindReadThrough, assignedRun(make(answers, 8), nil))
	bench.sender.resources = &nodes.Resources{
		Capacity:     nodeproto.Capacity{EncodeSlotCount: 4, RemuxSlotCount: 4},
		Devices:      nodeproto.Devices{{Key: "zfs:tank", MountPaths: []string{"/media"}}},
		ReaderLimits: map[string]int{"zfs:tank": 8},
	}
	passes := &fakeQuickPasses{}
	passes.set("/media/films")
	bench.exchanges.SetQuickPasses(passes)
	shown := bench.exchanges.Assign(t.Context(), 1,
		jobs.Job{ID: 1, Kind: jobs.KindReadThrough, Priority: jobs.PriorityShownInterest}, readThroughOf("/media"))
	if parking, isParked := jobqueue.ParkingOf(shown); !isParked ||
		parking.Destination != jobs.QuickPassDestination(1, "zfs:tank") {
		t.Errorf("a read-through at the shown-interest rung during tank's quick pass was answered %v; want it "+
			"waiting on the quick pass", shown)
	}
	looked := bench.exchanges.Assign(t.Context(), 1,
		jobs.Job{ID: 2, Kind: jobs.KindReadThrough, Priority: jobs.PriorityLookedAt}, readThroughOf("/media"))
	if !jobqueue.IsAssigned(looked) {
		t.Fatalf("a read-through of a film someone looks at during tank's quick pass was answered %v; want it "+
			"given to the node", looked)
	}
	jobqueue.HandOver(t.Context(), looked)
}
