// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// derivedState is the state a test's sweep derives its work from: the targets owed a fetch.
type derivedState struct {
	mutex sync.Mutex
	owed  []target
	runs  []string
}

func (state *derivedState) sweep(queue *jobqueue.Queue[target], name string, every time.Duration) jobqueue.Sweep {
	return jobqueue.Sweep{
		Name: name,
		Derive: func(ctx context.Context) (int, error) {
			state.mutex.Lock()
			owed := slices.Clone(state.owed)
			state.runs = append(state.runs, name)
			state.mutex.Unlock()
			added := 0
			for _, owedTarget := range owed {
				action, err := queue.Enqueue(ctx, jobqueue.Request[target]{Kind: kindFetch, Target: owedTarget})
				if err != nil {
					return added, err
				}
				if action == jobqueue.ActionInsert {
					added++
				}
			}
			return added, nil
		},
		Every: func(context.Context) time.Duration { return every },
	}
}

func (state *derivedState) takeRuns() []string {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	runs := state.runs
	state.runs = nil
	return runs
}

func TestSweep_WorkLostFromTheQueueIsDerivedAgainAndARepeatMerges(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	state := &derivedState{owed: []target{"a", "b"}}
	jobs.queue.AddSweep(state.sweep(jobs.queue, "fetches", time.Hour))
	if added := jobs.queue.RunSweeps(t.Context()); added != 2 {
		t.Fatalf("the first sweep added %d jobs, want 2", added)
	}
	if added := jobs.queue.RunSweeps(t.Context()); added != 0 {
		t.Errorf("a second sweep added %d, want none: what it finds queued is merged", added)
	}
	// A cancel stops a run, not the work (D231 section 9): the state still owes it.
	if err := jobs.queue.Cancel(t.Context(), jobs.row(t, "a").ID); err != nil {
		t.Fatal(err)
	}
	if added := jobs.queue.RunSweeps(t.Context()); added != 1 || jobs.row(t, "a").State != jobqueue.StateQueued {
		t.Errorf("after a cancel the sweep added %d, and a is %s; want it queued again", added, jobs.row(t, "a").State)
	}
}

func TestSweeping_RunsEverySweepAtOnceThenEachOnItsTimerAndWhenAskedOrADestinationReturns(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	state := &derivedState{}
	jobs.queue.AddSweep(state.sweep(jobs.queue, "hourly", time.Hour))
	jobs.queue.AddSweep(state.sweep(jobs.queue, "daily", 24*time.Hour))
	ctx, cancel := context.WithCancel(t.Context())
	tick := make(chan time.Time)
	stopped := make(chan struct{})
	go func() {
		jobs.queue.Sweeping(ctx, tick)
		close(stopped)
	}()
	defer func() { cancel(); testutil.Receive(t, stopped, "the sweeping to stop") }()
	// expect sends ticks, which run nothing that is not due, until the sweeps have run as want
	// says: a tick is taken only once the loop has done what came before it, and a request's
	// nudge may be taken after a tick sent with it.
	expect := func(when string, want ...string) {
		t.Helper()
		var runs []string
		for range 1000 {
			tick <- jobs.clock.Now()
			if runs = append(runs, state.takeRuns()...); len(runs) >= len(want) {
				break
			}
		}
		if !slices.Equal(runs, want) {
			t.Errorf("%s the sweeps ran %v, want %v", when, runs, want)
		}
	}
	expect("at start", "hourly", "daily")
	jobs.clock.Advance(time.Hour)
	expect("an hour on", "hourly")
	jobs.queue.RequestSweeps("daily")
	expect("asked for the daily one,", "daily")
	jobs.queue.Park(t.Context(), jobqueue.Parking{Destination: "host", Reason: "down"})
	jobs.queue.Unpark(t.Context(), "host")
	expect("when a destination came back", "hourly", "daily")
}

func TestRunDue_AJobThatFailsRecordsItsItemsFailureAndOneThatParksOrIsCancelledDoesNot(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	for _, name := range []target{"exhausted", "permanent", "parked", "cancelled"} {
		jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: name})
	}
	recorded := map[target]string{}
	runner := jobs.queue.NewRunner(kindFetch, func(ctx context.Context, job jobqueue.Job[target]) error {
		switch job.Target {
		case "permanent":
			return jobqueue.Permanent(errors.New("no such work"))
		case "parked":
			return jobqueue.Park(jobqueue.Parking{Destination: "host", Reason: "down"}, errors.New("unreachable"))
		case "cancelled":
			if err := jobs.queue.Cancel(ctx, job.ID); err != nil {
				t.Error(err)
			}
			return jobqueue.Permanent(context.Cause(ctx))
		}
		return errors.New("a 500")
	}).WithFailureRecorder(func(_ context.Context, job jobqueue.Job[target], failure string) error {
		recorded[job.Target] = failure
		return nil
	})
	for range attemptLimit {
		jobs.runDue(t, runner)
		jobs.clock.Advance(time.Hour)
	}
	want := map[target]string{"exhausted": "a 500", "permanent": "no such work"}
	if len(recorded) != len(want) || recorded["exhausted"] != want["exhausted"] ||
		recorded["permanent"] != want["permanent"] {
		t.Errorf("recorded %v, want %v: an item's own failures, not a parking or a cancel", recorded, want)
	}
}

func TestCancelWhere_CancelsTheOpenWorkOfWhatIsRetiredAndForgetDropsItsDestination(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	for _, name := range []target{"retired-1", "retired-2", "kept"} {
		jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: name, Destination: "node:2"})
	}
	jobs.queue.Park(t.Context(), jobqueue.Parking{Destination: "node:2", Reason: "not connected", AwaitsSignal: true})
	cancelled, err := jobs.queue.CancelWhere(t.Context(), func(row jobqueue.Row[target]) bool {
		return row.Target != "kept"
	})
	if err != nil || cancelled != 2 {
		t.Errorf("CancelWhere = %d, %v; want the two retired jobs", cancelled, err)
	}
	for name, want := range map[target]jobqueue.State{
		"retired-1": jobqueue.StateCancelled, "retired-2": jobqueue.StateCancelled, "kept": jobqueue.StateQueued,
	} {
		if got := jobs.row(t, name).State; got != want {
			t.Errorf("%s is %s, want %s", name, got, want)
		}
	}
	jobs.queue.Forget("node:2")
	if _, isParked := jobs.queue.DestinationState("node:2"); isParked {
		t.Error("a forgotten destination is still parked")
	}
}
