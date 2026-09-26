// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

type fixture struct {
	queue *Queue
	cache *store.Cache
	clock *clocktest.Fake
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	config := settingstest.Seeded(t, storetest.Open(t))
	settingstest.Set(t, config, settings.JobAttemptLimit, "3")
	settingstest.Set(t, config, settings.JobRetryBackoffS, "10")
	cache := storetest.OpenCache(t)
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	logger := slog.New(slog.NewTextHandler(t.Output(), nil))
	return fixture{queue: NewQueue(cache, now, config, logger), cache: cache, clock: now}
}

func (fixed fixture) enqueue(t *testing.T, subject, priority int64) {
	t.Helper()
	job := New{Kind: KindFetch, Subject: subject, Priority: priority, Reason: "test"}
	if _, err := fixed.queue.Enqueue(t.Context(), job); err != nil {
		t.Fatal(err)
	}
}

func (fixed fixture) job(t *testing.T, subject int64) cachegen.Job {
	t.Helper()
	jobs, err := fixed.cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.Subject != nil && *job.Subject == subject {
			return job
		}
	}
	t.Fatalf("no job for subject %d", subject)
	return cachegen.Job{}
}

func TestEnqueue_QueuesASubjectOnceWhileItIsOpen(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	for range 2 {
		jobs.enqueue(t, 7, 0)
	}
	if listed, err := jobs.cache.Read().ListJobs(t.Context(), 10); err != nil || len(listed) != 1 {
		t.Fatalf("%d jobs (%v) after queuing one subject twice, want 1", len(listed), err)
	}
	runner := jobs.queue.NewRunner(KindFetch, func(context.Context, Job) error { return nil })
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if added, err := jobs.queue.Enqueue(t.Context(), New{Kind: KindFetch, Subject: 7}); err != nil || !added {
		t.Errorf("a finished job's subject could not be queued again: %v", err)
	}
	if ever, err := jobs.queue.EverQueued(t.Context(), KindFetch, 7); err != nil || !ever {
		t.Errorf("EverQueued forgot subject 7: %v", err)
	}
	if ever, err := jobs.queue.EverQueued(t.Context(), KindFetch, 8); err != nil || ever {
		t.Errorf("EverQueued claims subject 8, never queued: %v", err)
	}
}

func TestRunDue_RunsTheHighestPriorityFirst(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	jobs.enqueue(t, 2, 10)
	jobs.enqueue(t, 3, 0)
	var ran []int64
	runner := jobs.queue.NewRunner(KindFetch, func(_ context.Context, job Job) error {
		ran = append(ran, job.Subject)
		return nil
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := []int64{2, 1, 3}; !slices.Equal(ran, want) {
		t.Errorf("ran %v, want %v", ran, want)
	}
	if job := jobs.job(t, 1); job.State != stateDone || job.FinishedAt == nil {
		t.Errorf("job 1 = %+v, want done", job)
	}
}

func TestRunDue_RetriesAFailureWithDoublingBackoffThenFails(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	runs := 0
	runner := jobs.queue.NewRunner(KindFetch, func(context.Context, Job) error {
		runs++
		return errors.New("the provider answered 500")
	})
	start := jobs.clock.Now()
	for _, wait := range []time.Duration{10 * time.Second, 20 * time.Second} {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		job := jobs.job(t, 1)
		if job.State != "queued" || job.RunnableAt != jobs.clock.Now().Add(wait).UnixMilli() {
			t.Fatalf("after %d runs: state %s, not before +%v; want queued, +%v", runs, job.State,
				time.UnixMilli(job.RunnableAt).Sub(jobs.clock.Now()), wait)
		}
		jobs.clock.Advance(wait - time.Second)
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		jobs.clock.Advance(time.Second)
	}
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	job := jobs.job(t, 1)
	if runs != 3 || job.State != stateFailed || job.LastError == nil {
		t.Errorf("after %v: %d runs, job %+v; want 3 runs, then failed with the reply",
			jobs.clock.Now().Sub(start), runs, job)
	}
}

func TestRunDue_ARetryAfterWaitsWithoutCostingAnAttemptAndHoldsTheKind(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 10)
	jobs.enqueue(t, 2, 0)
	until := jobs.clock.Now().Add(time.Minute)
	var ran []int64
	runner := jobs.queue.NewRunner(KindFetch, func(_ context.Context, job Job) error {
		ran = append(ran, job.Subject)
		if jobs.clock.Now().Before(until) {
			return &RetryAtError{At: until, Err: errors.New("429")}
		}
		return nil
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if job := jobs.job(t, 1); job.Attempts != 0 || job.State != "queued" || job.RunnableAt != until.UnixMilli() {
		t.Errorf("rate-limited job = %+v; want queued until the provider's time with no attempt spent", job)
	}
	if !slices.Equal(ran, []int64{1}) {
		t.Errorf("ran %v while the provider asked to wait; want only the first", ran)
	}
	jobs.clock.Set(until)
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []int64{1, 1, 2}) {
		t.Errorf("ran %v after the wait, want [1 1 2]", ran)
	}
}

func TestRunDue_APermanentFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	runner := jobs.queue.NewRunner(KindFetch, func(context.Context, Job) error {
		return Permanent(errors.New("the key was refused"))
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if job := jobs.job(t, 1); job.State != stateFailed || job.Attempts != 1 {
		t.Errorf("job = %+v, want failed after one attempt", job)
	}
}

func TestRequeueRunning_ReturnsWhatACrashLeftRunning(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	err := jobs.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		_, startErr := queries.StartJob(t.Context(), jobs.job(t, 1).ID)
		return startErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if count, err := jobs.queue.RequeueRunning(t.Context()); err != nil || count != 1 {
		t.Fatalf("RequeueRunning = %d, %v; want 1", count, err)
	}
	if job := jobs.job(t, 1); job.State != "queued" {
		t.Errorf("job = %+v, want queued", job)
	}
}

func TestRunDue_ABlockedKindWaitsTheBackoffWithoutCostingAttempts(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	jobs.enqueue(t, 2, 0)
	blocked := true
	runs := 0
	runner := jobs.queue.NewRunner(KindFetch, func(context.Context, Job) error {
		runs++
		if blocked {
			return Blocked(errors.New("no key"))
		}
		return nil
	})
	for range 5 {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		jobs.clock.Advance(10 * time.Second)
	}
	if job := jobs.job(t, 1); job.State != "queued" || job.Attempts != 0 || runs != 5 {
		t.Errorf("after five blocked tries: job %+v, %d runs; want queued, no attempt spent, one run per wait",
			job, runs)
	}
	blocked = false
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if jobs.job(t, 1).State != stateDone || jobs.job(t, 2).State != stateDone {
		t.Error("once unblocked, the queue did not run both jobs")
	}
}

func TestRunDue_AKindWithItsOwnScheduleRetriesOnItThenFails(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	runs := 0
	schedule := []time.Duration{time.Minute, 5 * time.Minute}
	runner := jobs.queue.NewRunner(KindFetch, func(context.Context, Job) error {
		runs++
		return errors.New("the image server answered 503")
	}).RetryOn(schedule)
	for _, wait := range schedule {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		if job := jobs.job(t, 1); job.State != "queued" || job.RunnableAt != jobs.clock.Now().Add(wait).UnixMilli() {
			t.Fatalf("after %d runs: %s, not before +%v; want queued, +%v", runs, job.State,
				time.UnixMilli(job.RunnableAt).Sub(jobs.clock.Now()), wait)
		}
		jobs.clock.Advance(wait)
	}
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if job := jobs.job(t, 1); runs != 3 || job.State != stateFailed {
		t.Errorf("%d runs, job %s; want 3 runs, then failed (the settings' limit of 3 is not what decides)",
			runs, job.State)
	}
}

func TestEnqueue_AJobWaitsUntilItsNotBefore(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	later := jobs.clock.Now().Add(12 * time.Hour)
	if _, err := jobs.queue.Enqueue(t.Context(), New{Kind: KindFetch, Subject: 4, RunnableAt: later}); err != nil {
		t.Fatal(err)
	}
	ran := 0
	runner := jobs.queue.NewRunner(KindFetch, func(context.Context, Job) error { ran++; return nil })
	if err := runner.RunDue(t.Context()); err != nil || ran != 0 {
		t.Fatalf("ran %d before its time (%v)", ran, err)
	}
	jobs.clock.Set(later)
	if err := runner.RunDue(t.Context()); err != nil || ran != 1 {
		t.Errorf("ran %d at its time (%v)", ran, err)
	}
}
