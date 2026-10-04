// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancel_AQueuedJobNeverRunsAndARunningOnesHandlerIsToldAndItStaysCancelled(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	jobs.enqueue(t, 2, 0)
	first := jobs.job(t, 1)
	if err := jobs.queue.Cancel(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	var ran []int64
	var cause error
	second := jobs.job(t, 2)
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(ctx context.Context, job Job) error {
		ran = append(ran, job.ItemID)
		if err := jobs.queue.SetProgress(ctx, job.ID, 0.25); err != nil {
			t.Error(err)
		}
		if err := jobs.queue.Cancel(ctx, job.ID); err != nil {
			t.Error(err)
		}
		<-ctx.Done()
		cause = context.Cause(ctx)
		return ctx.Err() // the handler's answer, which must not undo the cancel
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0] != 2 {
		t.Errorf("ran %v, want only subject 2: a cancelled queued job never runs", ran)
	}
	if !errors.Is(cause, ErrCancelled) {
		t.Errorf("the running handler's cause = %v, want ErrCancelled", cause)
	}
	if view := jobs.view(t, second.ID); view.ProgressFraction == nil || *view.ProgressFraction != 0.25 {
		t.Errorf("the running job's progress = %v, want 0.25 kept", view.ProgressFraction)
	}
	for _, id := range []int64{first.ID, second.ID} {
		if view := jobs.view(t, id); view.State != "cancelled" || view.FinishedAt == nil {
			t.Errorf("job %d = %+v, want cancelled with a finish time", id, view)
		}
	}
	if err := jobs.queue.Cancel(t.Context(), second.ID); !errors.Is(err, ErrNotOpen) {
		t.Errorf("cancelling a cancelled job: %v, want ErrNotOpen", err)
	}
	if err := jobs.queue.Cancel(t.Context(), 999); !errors.Is(err, ErrNotOpen) {
		t.Errorf("cancelling no job: %v, want ErrNotOpen", err)
	}
}

// view is one job as the job view lists it.
func (fixed fixture) view(t *testing.T, id int64) View {
	t.Helper()
	views, err := fixed.queue.List(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.ID == id {
			return view
		}
	}
	t.Fatalf("no job %d in the view", id)
	return View{}
}

// A job given to a node shows as running, with where its node last said it is: waiting for a slot,
// running, or paused for play-driven work (D240 §6).
func TestList_AJobGivenToANodeRunsThereAndSaysWhereItsNodeSaysItIs(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	jobs.enqueue(t, 2, 0)
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error {
		return Assign(jobs.clock.Now().Add(time.Hour))
	})
	jobs.runDue(t, runner)
	jobs.runDue(t, runner)
	paused := jobs.job(t, 1).ID
	jobs.queue.SetNodeWords(func(jobID int64) (string, bool) { return "paused", jobID == paused })
	if view := jobs.view(t, paused); view.State != "running" || view.OnNode != "paused" {
		t.Errorf("the job its node paused shows %q, on its node %q; want running, paused", view.State, view.OnNode)
	}
	if view := jobs.view(t, jobs.job(t, 2).ID); view.State != "running" || view.OnNode != "" {
		t.Errorf("the job its node has not spoken of shows %q, on its node %q; want running, nothing said",
			view.State, view.OnNode)
	}
}

// A job a person asks for to play is wanted for play_priority_kept_seconds; one asked for in the
// background never is. Past its time it falls to background priority, and one given to a node says
// so, for its node to be told (D234 §3, S87).
func TestDemoteUnwanted_APlaysJobFallsToBackgroundOnceNoOneWaits(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, PriorityPlayNow)
	jobs.enqueue(t, 2, PriorityPlayNow)
	jobs.enqueue(t, 3, 0)
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error {
		return Assign(jobs.clock.Now().Add(time.Hour))
	})
	jobs.runDue(t, runner) // the first play's job is given to a node
	jobs.clock.Advance(9 * time.Minute)
	if demoted, err := jobs.queue.DemoteUnwanted(t.Context()); err != nil || len(demoted) != 0 {
		t.Fatalf("within the ten minutes demoted %+v (%v), want none", demoted, err)
	}
	if err := jobs.queue.KeepWanted(t.Context(), KindFetchMetadata, Target{ItemID: 2}); err != nil {
		t.Fatal(err)
	}
	jobs.clock.Advance(2 * time.Minute)
	demoted, err := jobs.queue.DemoteUnwanted(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(demoted) != 1 || demoted[0].ID != jobs.job(t, 1).ID || !demoted[0].IsAssigned {
		t.Errorf("eleven minutes on demoted %+v, want the first play's job alone, given to its node", demoted)
	}
	if first := jobs.job(t, 1); first.Priority != PriorityBackground {
		t.Errorf("the first play's job is at priority %d, want background", first.Priority)
	}
}
