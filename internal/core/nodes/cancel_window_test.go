// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/nodes/nodestest"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// queueRooms forwards the exchanges' unparks to a real job queue's destinations, as the server's
// Service does.
type queueRooms struct {
	*recordingSender
	queue *jobqueue.Queue[string]
}

func (rooms queueRooms) Unpark(ctx context.Context, destination jobs.Destination) {
	rooms.queue.Unpark(ctx, destination)
}

// Work parked for room (AwaitsSignal) whose room is made between Assign's look and the runner's
// queue.Park is brought back: the unpark, landing on a destination not parked yet, is a signal the
// parking after it answers (D540).
func TestPark_RoomMadeBetweenTheLookAndTheParkIsNotLost(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	queue := jobqueue.New[string](&jobqueuetest.Memory[string]{}, bench.now,
		jobqueue.Config{Logger: slog.New(slog.DiscardHandler)})
	rooms := queueRooms{recordingSender: bench.sender, queue: queue}
	bench.exchanges = nodesWithRooms(t, bench, rooms)
	resolved := make(answers, 4)
	bench.exchanges.Handle(jobs.KindMakeChunks, assignedRun(resolved, nil))
	bench.sender.resources = &nodes.Resources{
		Capacity:          nodeproto.Capacity{EncodeSlotCount: 4, RemuxSlotCount: 4},
		Devices:           nodeproto.Devices{{Key: "zfs:tank", MountPaths: []string{"/media"}}},
		ReaderLimits:      map[string]int{"zfs:tank": 8},
		EncodesInSoftware: true,
	}
	encode := runOf(nodeproto.StreamQuality)
	// A played encode, in software.
	played := bench.exchanges.Assign(t.Context(), 1,
		jobs.Job{ID: 1, Kind: jobs.KindMakeChunks, Priority: jobs.PriorityPlayNow}, encode)
	jobqueue.HandOver(t.Context(), played)
	sent := testutil.Receive(t, bench.sender.sent, "the played encode")
	// A background encode's handler is answered: wait on the encode slot (parkBesidePlaying).
	answer := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: 2, Kind: jobs.KindMakeChunks}, encode)
	parking, isParked := jobqueue.ParkingOf(answer)
	if !isParked || !parking.AwaitsSignal {
		t.Fatalf("the background encode was answered %v, want it parked for room", answer)
	}
	// Before the runner records the parking (runner.finish -> runner.park -> queue.Park), the played
	// encode ends, which brings back its slot.
	deliver(t, bench.exchanges, 1, nodeproto.Result{
		JobID: 1, AttemptToken: sent.AttemptToken,
		MakeChunks: &nodeproto.MakeChunksReport{State: nodeproto.RunDone}, Work: &encode.Work,
	})
	<-resolved
	// The runner now records the parking its handler answered.
	queue.Park(t.Context(), parking)
	if state, isStillParked := queue.DestinationState(parking.Destination); isStillParked {
		t.Errorf("the node holds no encode, yet %s stays parked (%+v), awaiting a signal already given: "+
			"nothing held on the node will end to bring it back", parking.Destination, state)
	}
}

// cancellingSender is a recording node whose job is cancelled as its attempt is about to be kept:
// after Exchanges.Cancelled could look for it, before Assign keeps it.
type cancellingSender struct {
	*recordingSender
	cancel func()
}

func (sender *cancellingSender) WillSend(nodeID int64, token string) {
	sender.recordingSender.WillSend(nodeID, token)
	sender.cancel()
}

// A job cancelled while its handler gives it to a node, after the queue's cancel hook
// (Exchanges.Cancelled) looked for its attempt and before Assign kept it, leaves no attempt: Assign
// forgets the one it kept and answers the cancel, as HandleMakeChunks does (D540). Kept, it was never
// sent, filled the node's window until a status judged it lost, and counted as a broken invariant.
func TestAssign_AnAttemptKeptAsItsJobIsCancelledIsForgotten(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	jobCtx, stop := context.WithCancelCause(t.Context())
	defer stop(nil)
	sender := &cancellingSender{recordingSender: bench.sender}
	exchanges := nodestest.NewExchanges(t, nodes.ExchangeParts{
		Sender: sender, Rooms: bench.sender, Clock: bench.now, Logger: slog.New(slog.NewJSONHandler(bench.logged, nil)),
		Cache: bench.cache, Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
	})
	// The queue's cancel: the job's context stops, then its cancel hook runs (Queue.Cancel).
	sender.cancel = func() {
		stop(jobs.ErrCancelled)
		exchanges.Cancelled(t.Context(), 7)
	}
	resolved := make(answers, 4)
	exchanges.Handle(jobs.KindMakeChunks, assignedRun(resolved, nil))
	answer := exchanges.Assign(jobCtx, 1, jobs.Job{ID: 7, Kind: jobs.KindMakeChunks}, runCommand)
	if jobqueue.IsAssigned(answer) {
		t.Errorf("Assign answered %v for a job cancelled as it was given, want the cancel", answer)
	}
	// The runner drops the answer of a cancelled job: no HandOver. Nothing is sent, and the node's
	// status lists nothing held; past unsentGrace the core reconciles what is kept as lost work.
	bench.now.Advance(2 * time.Minute)
	exchanges.Reconcile(t.Context(), 1, nil, nil)
	if broken := exchanges.BrokenInvariants(1); broken != 0 {
		t.Errorf("a cancelled job the node was never sent left an attempt kept, reconciled as %d broken "+
			"invariant(s): it filled the node's window until then", broken)
	}
}

// nodesWithRooms is the bench's exchanges, built again over rooms.
func nodesWithRooms(t *testing.T, bench exchangeBench, rooms nodes.Rooms) *nodes.Exchanges {
	t.Helper()
	return nodestest.NewExchanges(t, nodes.ExchangeParts{
		Sender: bench.sender, Rooms: rooms, Clock: bench.now, Logger: slog.New(slog.NewJSONHandler(bench.logged, nil)),
		Cache: bench.cache, Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
	})
}
