// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
)

// assignFor is how long the tests' handlers give a node to say how a job ended.
const assignFor = time.Hour

// assigningRunner is a fetch runner whose handler gives every job to a node (Assign), and records
// which it ran.
func assigningRunner(jobs fixture, ran *[]target) *jobqueue.Runner[target] {
	return jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		*ran = append(*ran, job.Target)
		return jobqueue.Assign(jobs.clock.Now().Add(assignFor))
	})
}

func (fixed fixture) resolve(t *testing.T, runner *jobqueue.Runner[target], id int64, answer error) bool {
	t.Helper()
	isAssigned, err := runner.Resolve(t.Context(), id, answer)
	if err != nil {
		t.Fatal(err)
	}
	return isAssigned
}

// A job given to a node is assigned, and its runner goes on to the next: nothing waits for it.
// The node's outcome is taken later, once, as a handler's answer would be (D234 §7).
func TestAssign_AJobGivenToANodeFreesItsRunnerAndItsOutcomeIsTakenOnce(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "b"})
	var ran []target
	runner := assigningRunner(jobs, &ran)
	jobs.runDue(t, runner)
	if !slices.Equal(ran, []target{"a", "b"}) {
		t.Fatalf("ran %q, want both: a runner does not wait for an assigned job", ran)
	}
	assignedA := jobs.row(t, "a")
	wantUntil := jobs.clock.Now().Add(assignFor)
	if assignedA.State != jobqueue.StateAssigned || assignedA.AssignedUntil == nil ||
		!assignedA.AssignedUntil.Equal(wantUntil) || assignedA.AttemptCount != 1 {
		t.Fatalf("a after its handler gave it to a node: %+v, want assigned until %v, its attempt counted",
			assignedA, wantUntil)
	}
	if !jobs.resolve(t, runner, assignedA.ID, nil) {
		t.Fatal("Resolve did not find the assigned job")
	}
	if done := jobs.row(t, "a"); done.State != jobqueue.StateDone || done.AssignedUntil != nil {
		t.Errorf("a after its node said it was done: %+v, want done", done)
	}
	if jobs.resolve(t, runner, assignedA.ID, errors.New("a late word")) {
		t.Error("a second outcome for a job already resolved was taken")
	}
	if done := jobs.row(t, "a"); done.State != jobqueue.StateDone {
		t.Errorf("a second outcome changed a resolved job: %+v", done)
	}
}

// The node's outcome is a handler's answer: its own failure retries after the back-off with the
// attempt spent, and a destination down parks it at no cost.
func TestResolve_TakesTheNodesOutcomeAsAHandlersAnswer(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "fails"})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "parks"})
	var ran []target
	runner := assigningRunner(jobs, &ran)
	jobs.runDue(t, runner)
	now := jobs.clock.Now()
	jobs.resolve(t, runner, jobs.row(t, "fails").ID, errors.New("FFmpeg failed"))
	if failed := jobs.row(t, "fails"); failed.State != jobqueue.StateQueued || failed.AttemptCount != 1 ||
		!failed.RunnableAt.Equal(now.Add(firstRetryDelay)) || failed.LastFailure != "FFmpeg failed" {
		t.Errorf("after its node's failure: %+v, want queued again after %s with its attempt spent",
			failed, firstRetryDelay)
	}
	parking := jobqueue.Parking{Destination: "node:1", Reason: "not connected", AwaitsSignal: true}
	jobs.resolve(t, runner, jobs.row(t, "parks").ID, jobqueue.Park(parking, errors.New("gone")))
	if parked := jobs.row(t, "parks"); parked.State != jobqueue.StateQueued || parked.AttemptCount != 0 ||
		parked.Destination != "node:1" {
		t.Errorf("after its node went away: %+v, want queued on node:1 with its attempt given back", parked)
	}
}

// An assigned job whose time runs out is its own failure: retried with its attempt spent, and
// failed at its limit; an outcome after that finds nothing to take.
func TestAssign_AJobWhoseTimeRunsOutFailsAndALateOutcomeIsDropped(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "slow"})
	var ran []target
	var toldOverdue []int64
	jobs.queue.OnOverdue(func(_ context.Context, id int64) { toldOverdue = append(toldOverdue, id) })
	runner := assigningRunner(jobs, &ran)
	jobs.runDue(t, runner)
	id := jobs.row(t, "slow").ID
	jobs.clock.Advance(assignFor - time.Second)
	jobs.runDue(t, runner)
	if still := jobs.row(t, "slow"); still.State != jobqueue.StateAssigned {
		t.Fatalf("a second before its time ran out: %+v, want still assigned", still)
	}
	for attempt := int64(1); attempt < attemptLimit; attempt++ {
		jobs.clock.Advance(assignFor) // past its time
		jobs.runDue(t, runner)
		if overdue := jobs.row(t, "slow"); overdue.State != jobqueue.StateQueued || overdue.AttemptCount != attempt ||
			!strings.Contains(overdue.LastFailure, "did not say how it ended") {
			t.Fatalf("after attempt %d's time ran out: %+v, want it queued for its back-off, the attempt spent",
				attempt, overdue)
		}
		jobs.clock.Advance(time.Hour) // past its back-off
		jobs.runDue(t, runner)
		if again := jobs.row(t, "slow"); again.State != jobqueue.StateAssigned || again.AttemptCount != attempt+1 {
			t.Fatalf("after attempt %d's back-off: %+v, want it tried again and assigned anew", attempt, again)
		}
	}
	jobs.clock.Advance(assignFor) // the last attempt's time runs out
	jobs.runDue(t, runner)
	failed := jobs.row(t, "slow")
	if failed.State != jobqueue.StateFailed || !strings.Contains(failed.LastFailure, "did not say how it ended") {
		t.Errorf("after its last attempt's time ran out: %+v, want failed, saying the node never answered", failed)
	}
	if len(ran) != attemptLimit {
		t.Errorf("the handler ran %d times, want once for each of its %d attempts", len(ran), attemptLimit)
	}
	// Each time it ran out, the queue said so, so its machine can be told to stop the work (G4).
	if len(toldOverdue) != attemptLimit || toldOverdue[0] != id {
		t.Errorf("the overdue hook was told %v; want job %d once for each of its %d attempts", toldOverdue, id,
			attemptLimit)
	}
	if jobs.resolve(t, runner, id, nil) {
		t.Error("an outcome after the job's time ran out was taken")
	}
}

// An assigned job counts as running: a request it covers is dropped and another queued once to
// follow; a cancel ends it and drops its outcome; a restart leaves it with its node.
func TestAssign_AnAssignedJobCountsAsRunningForRequestsCancelsAndRestarts(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	var ran []target
	runner := assigningRunner(jobs, &ran)
	jobs.runDue(t, runner)
	covered := jobqueue.Request[target]{Kind: kindFetch, Target: "a"}
	if action := jobs.enqueue(t, covered); action != jobqueue.ActionDrop {
		t.Errorf("a request the assigned job covers = %s, want dropped", action)
	}
	if action := jobs.enqueue(t, jobqueue.Request[target]{
		Kind: kindFetch, Target: "a", ArgsJSON: `{"again":true}`,
	}); action != jobqueue.ActionInsert {
		t.Errorf("a request it does not cover = %s, want queued to follow", action)
	}
	requeued, err := jobs.storage.RequeueRunning(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assigned := slices.IndexFunc(jobs.storage.Rows(), func(row jobqueue.Row[target]) bool {
		return row.State == jobqueue.StateAssigned
	})
	if requeued != 0 || assigned < 0 {
		t.Fatalf("a restart requeued %d jobs, assigned at %d; want the assigned job left with its node",
			requeued, assigned)
	}
	id := jobs.storage.Rows()[assigned].ID
	open, err := jobs.storage.ListOpen(t.Context())
	if err != nil || !slices.ContainsFunc(open, func(row jobqueue.Row[target]) bool { return row.ID == id }) {
		t.Errorf("the open jobs %+v (%v) leave out the assigned one, which a retire must cancel", open, err)
	}
	if err := jobs.queue.Cancel(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if jobs.resolve(t, runner, id, nil) {
		t.Error("an outcome for a cancelled assigned job was taken")
	}
	for _, row := range jobs.storage.Rows() {
		if row.ID == id && row.State != jobqueue.StateCancelled {
			t.Errorf("a cancelled assigned job is %s after its node's outcome, want it to stay cancelled", row.State)
		}
	}
}

// ResolveBy decides an assigned job's answer from the job as it was assigned, its attempt among it;
// a cancel tells the queue's cancel hook which job it ended; and IsAssigned tells a handler's
// Assign from any other answer.
func TestResolveBy_DecidesFromTheJobAndACancelIsHeardByTheHook(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "b"})
	var ran []target
	runner := assigningRunner(jobs, &ran)
	jobs.runDue(t, runner)
	assignedA := jobs.row(t, "a")
	var decidedFor jobqueue.Job[target]
	resolved, err := runner.ResolveBy(t.Context(), assignedA.ID, func(job jobqueue.Job[target]) error {
		decidedFor = job
		return nil
	})
	if err != nil || !resolved || decidedFor.ID != assignedA.ID || decidedFor.Target != "a" ||
		decidedFor.AttemptNumber != 1 {
		t.Errorf("ResolveBy = %v, %v, deciding for %+v; want a's job, on its first attempt", resolved, err, decidedFor)
	}
	var heard []int64
	jobs.queue.OnCancel(func(_ context.Context, id int64) { heard = append(heard, id) })
	assignedB := jobs.row(t, "b")
	if err := jobs.queue.Cancel(t.Context(), assignedB.ID); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(heard, []int64{assignedB.ID}) {
		t.Errorf("the cancel hook heard %v, want b's id", heard)
	}
	if !jobqueue.IsAssigned(jobqueue.Assign(time.Time{})) || jobqueue.IsAssigned(errors.New("failed")) ||
		jobqueue.IsAssigned(nil) {
		t.Error("IsAssigned does not tell Assign from another answer")
	}
}

// A job given with AssignThen is handed over only once it is recorded assigned: what is handed, the
// work sent to its node, finds the job assigned, however soon the node's word comes back.
func TestAssignThen_HandsTheWorkOverOnceTheJobIsRecordedAssigned(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	var stateWhenHanded jobqueue.State
	handedCount := 0
	runner := jobs.queue.NewRunner(kindFetch, func(context.Context, jobqueue.Job[target]) error {
		return jobqueue.AssignThen(jobs.clock.Now().Add(assignFor), func(context.Context) {
			handedCount++
			stateWhenHanded = jobs.row(t, "a").State
		})
	})
	jobs.runDue(t, runner)
	if handedCount != 1 || stateWhenHanded != jobqueue.StateAssigned {
		t.Errorf("the work was handed over %d times, the job then %q; want once, the job assigned",
			handedCount, stateWhenHanded)
	}
}
