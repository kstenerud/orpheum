// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"testing"
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
	runner := jobs.queue.NewRunner(KindFetch, func(ctx context.Context, job Job) error {
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
	if view := jobs.view(t, second.ID); view.Progress == nil || *view.Progress != 0.25 {
		t.Errorf("the running job's progress = %v, want 0.25 kept", view.Progress)
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
