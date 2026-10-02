// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// blockingWork is artwork work that says when it starts, then runs until release is closed or its
// context ends, and says which.
type blockingWork struct {
	started chan int64
	ended   chan error
	release chan struct{}
}

func newBlockingWork() blockingWork {
	return blockingWork{started: make(chan int64, 8), ended: make(chan error, 8), release: make(chan struct{})}
}

func (blocking blockingWork) held(jobID, priority int64) heldWork {
	held := attempt{jobID: jobID, token: tokenOf(jobID)}
	return heldWork{attempt: held, priority: priority, slot: slotArtwork, run: func(ctx context.Context) {
		blocking.started <- jobID
		select {
		case <-blocking.release:
			blocking.ended <- nil
		case <-ctx.Done():
			blocking.ended <- ctx.Err()
		}
	}}
}

// tokenOf is the attempt token the tests give job jobID's one attempt.
func tokenOf(jobID int64) string { return "attempt-" + strconv.FormatInt(jobID, 10) }

func TestWorkList_ACancelledWaitingJobNeverStartsAndARunningOneStops(t *testing.T) {
	t.Parallel()
	list := newWorkList(nodeproto.Capacity{})
	blocking := newBlockingWork()
	if list.cancel(tokenOf(5)) {
		t.Error("cancelling job 5, which the node does not hold, said it was held")
	}
	if isHeld, isWaiting := list.hold(t.Context(), blocking.held(6, 0)); !isHeld || isWaiting {
		t.Fatalf("job 6, alone in the artwork slot, was held %v and waiting %v; want held and running",
			isHeld, isWaiting)
	}
	if started := testutil.Receive(t, blocking.started, "job 6's start"); started != 6 {
		t.Fatalf("job %d started, want 6", started)
	}
	if _, isWaiting := list.hold(t.Context(), blocking.held(5, 0)); !isWaiting {
		t.Fatal("job 5, behind job 6 in the one artwork slot, did not wait")
	}
	if !list.cancel(tokenOf(5)) {
		t.Error("cancelling waiting job 5 said the node did not hold it")
	}
	if !list.cancel(tokenOf(6)) {
		t.Error("cancelling running job 6 said the node did not hold it")
	}
	if err := testutil.Receive(t, blocking.ended, "job 6's end"); err == nil {
		t.Error("cancelling running job 6 did not stop it")
	}
	// Job 5 was dropped before it started: the slot job 6 freed is taken by the next job held.
	list.hold(t.Context(), blocking.held(7, 0))
	if started := testutil.Receive(t, blocking.started, "the next start"); started != 7 {
		t.Errorf("job %d started once job 6 stopped, want 7: cancelled job 5 never starts", started)
	}
	close(blocking.release)
}

// What waits for a slot starts the highest priority first and, at one priority, the first to come
// (D234 §3); a job held already, sent again, is not held twice.
func TestWorkList_StartsWhatWaitsByPriorityThenArrivalAndHoldsAJobOnce(t *testing.T) {
	t.Parallel()
	list := newWorkList(nodeproto.Capacity{})
	blocking := newBlockingWork()
	list.hold(t.Context(), blocking.held(1, 0))
	testutil.Receive(t, blocking.started, "job 1's start")
	for _, waiting := range []struct{ jobID, priority int64 }{{2, 0}, {3, 1000}, {4, 20}, {5, 1000}} {
		if _, isWaiting := list.hold(t.Context(), blocking.held(waiting.jobID, waiting.priority)); !isWaiting {
			t.Fatalf("job %d did not wait behind job 1", waiting.jobID)
		}
	}
	if isHeld, _ := list.hold(t.Context(), blocking.held(3, 1000)); isHeld {
		t.Error("job 3, sent again, was held a second time")
	}
	close(blocking.release)
	var order []int64
	for range 5 {
		if err := testutil.Receive(t, blocking.ended, "a job's end"); err != nil {
			t.Errorf("a job ended %v, want released", err)
		}
		if len(order) < 4 {
			order = append(order, testutil.Receive(t, blocking.started, "the next start"))
		}
	}
	want := []int64{3, 5, 4, 2}
	if len(order) != len(want) || order[0] != 3 || order[1] != 5 || order[2] != 4 || order[3] != 2 {
		t.Errorf("the waiting jobs started in the order %v, want %v", order, want)
	}
}
