// SPDX-License-Identifier: GPL-3.0-or-later

package jobqueue_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
)

// target is what the tests' jobs work on: a name, which the queue only compares.
type target string

// kindFetch and kindMake are two kinds of job, as opaque to the queue as their targets.
const (
	kindFetch jobqueue.Kind = "fetch"
	kindMake  jobqueue.Kind = "make"
)

// fixedPolicy is a retry policy that does not change.
type fixedPolicy struct {
	attemptLimit    int64
	firstRetryDelay time.Duration
}

func (policy fixedPolicy) AttemptLimit(context.Context) (int64, error) {
	return policy.attemptLimit, nil
}

func (policy fixedPolicy) FirstRetryDelay(context.Context) (time.Duration, error) {
	return policy.firstRetryDelay, nil
}

type fixture struct {
	queue   *jobqueue.Queue[target]
	storage *jobqueuetest.Memory[target]
	clock   *clocktest.Fake
}

// firstRetryDelay and attemptLimit are the fixture's policy.
const (
	firstRetryDelay = 10 * time.Second
	attemptLimit    = 3
)

func newFixture(t *testing.T, random io.Reader, merges map[jobqueue.Kind]jobqueue.ArgsMerge) fixture {
	t.Helper()
	return newFixtureWith(t, jobqueue.Config{Random: random, ArgsMerges: merges})
}

// newFixtureWith is a fixture whose queue has config, with the fixture's policy and a logger.
func newFixtureWith(t *testing.T, config jobqueue.Config) fixture {
	t.Helper()
	storage := &jobqueuetest.Memory[target]{}
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	config.Policy = fixedPolicy{attemptLimit: attemptLimit, firstRetryDelay: firstRetryDelay}
	config.Logger = slog.New(slog.NewTextHandler(t.Output(), nil))
	queue := jobqueue.New(storage, fakeClock, config)
	return fixture{queue: queue, storage: storage, clock: fakeClock}
}

func (fixed fixture) enqueue(t *testing.T, request jobqueue.Request[target]) jobqueue.Action {
	t.Helper()
	action, err := fixed.queue.Enqueue(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

// row is the one job for target, of any kind, that is not finished, or the newest when all are.
func (fixed fixture) row(t *testing.T, wanted target) jobqueue.Row[target] {
	t.Helper()
	var found *jobqueue.Row[target]
	for _, row := range fixed.storage.Rows() {
		if row.Target == wanted && (found == nil || row.FinishedAt == nil) {
			found = &row
		}
	}
	if found == nil {
		t.Fatalf("no job for %q", wanted)
	}
	return *found
}

func (fixed fixture) runDue(t *testing.T, runner *jobqueue.Runner[target]) {
	t.Helper()
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestClaim_TakesTheHighestPriorityThenTheEarliestThenTheOldest(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	now := jobs.clock.Now()
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "old", RunnableAt: now.Add(-time.Minute)})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "urgent", Priority: 10})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "older", RunnableAt: now.Add(-time.Hour)})
	jobs.enqueue(t, jobqueue.Request[target]{
		Kind: kindFetch, Target: "later", Priority: 50, RunnableAt: now.Add(time.Hour),
	})
	jobs.enqueue(t, jobqueue.Request[target]{
		Kind: kindFetch, Target: "same time, newer", RunnableAt: now.Add(-time.Hour),
	})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindMake, Target: "another kind", Priority: 100})
	var ran []target
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.Target)
		return nil
	})
	jobs.runDue(t, runner)
	want := []target{"urgent", "older", "same time, newer", "old"}
	if !slices.Equal(ran, want) {
		t.Errorf("ran %q, want %q: the highest priority, then the earliest, then the oldest, and none not due or"+
			" of another kind", ran, want)
	}
}

func TestEnqueue_ARepeatRequestWhileQueuedTakesTheHigherPriorityAndMergesTheArguments(t *testing.T) {
	t.Parallel()
	joinArgs := jobqueue.ArgsMerge{Merge: func(queued, requested string) (string, error) {
		return queued + "+" + requested, nil
	}}
	jobs := newFixture(t, jobqueuetest.Unjittered(), map[jobqueue.Kind]jobqueue.ArgsMerge{kindMake: joinArgs})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindMake, Target: "a", ArgsJSON: "1", Priority: 5, Reason: "first"})
	if action := jobs.enqueue(t, jobqueue.Request[target]{
		Kind: kindMake, Target: "a", ArgsJSON: "2", Priority: 20, Reason: "a person asked",
	}); action != jobqueue.ActionMerge {
		t.Errorf("a repeat request = %s, want merged", action)
	}
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindMake, Target: "a", ArgsJSON: "3", Priority: 1, Reason: "later"})
	rows := jobs.storage.Rows()
	if len(rows) != 1 {
		t.Fatalf("%d jobs after three requests for one target, want 1", len(rows))
	}
	if row := rows[0]; row.Priority != 20 || row.Reason != "a person asked" || row.ArgsJSON != "1+2+3" {
		t.Errorf("merged job = priority %d, reason %q, args %q; want the highest priority with its reason, and"+
			" every request's arguments merged", row.Priority, row.Reason, row.ArgsJSON)
	}
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "b", ArgsJSON: `{"x":1}`})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "b", ArgsJSON: `{"x":2}`})
	if row := jobs.row(t, "b"); row.ArgsJSON != `{"x":1}` {
		t.Errorf("a kind with no merge of its own merged its arguments to %s; want the queued job's kept", row.ArgsJSON)
	}
	if count := len(jobs.storage.Rows()); count != 3 {
		t.Errorf("%d jobs, want 3: another kind on the same target, or another target, is a job of its own", count)
	}
}

func TestEnqueue_ARepeatRequestWakesAJobWaitingOnItsBackoffButNotASnoozedOne(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	start := jobs.clock.Now()
	for _, name := range []target{"backing off", "snoozed", "scheduled"} {
		jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: name, RunnableAt: start.Add(time.Hour)})
	}
	jobs.clock.Set(start.Add(time.Hour))
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		switch job.Target {
		case "backing off":
			return errors.New("the provider answered 500")
		case "snoozed":
			return jobqueue.Snooze(errors.New("come back later"), start.Add(3*time.Hour))
		}
		return nil
	})
	jobs.runDue(t, runner)
	now := jobs.clock.Now()
	for _, name := range []target{"backing off", "snoozed"} {
		jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: name})
	}
	if row := jobs.row(t, "backing off"); !row.RunnableAt.Equal(now) {
		t.Errorf("a job in back-off, asked for again, waits until %v; want woken, now (%v)", row.RunnableAt, now)
	}
	if row := jobs.row(t, "snoozed"); !row.RunnableAt.Equal(start.Add(3 * time.Hour)) {
		t.Errorf("a snoozed job, asked for again, waits until %v; want the time it was given kept", row.RunnableAt)
	}
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "later", RunnableAt: now.Add(2 * time.Hour)})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "later", RunnableAt: now.Add(time.Hour)})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "later", RunnableAt: now.Add(3 * time.Hour)})
	if row := jobs.row(t, "later"); !row.RunnableAt.Equal(now.Add(time.Hour)) {
		t.Errorf("a job asked for at three times waits until %v, want the soonest", row.RunnableAt.Sub(now))
	}
}

func TestEnqueue_WhileAJobRunsACoveredRequestIsDroppedAndAnotherQueuedOnceToFollow(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", ArgsJSON: "same"})
	var actions []jobqueue.Action
	var ran []string
	runner := jobs.queue.NewRunner(kindFetch, func(ctx context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.ArgsJSON)
		if len(ran) > 1 {
			return nil
		}
		for _, args := range []string{"same", "more", "more still"} {
			action, err := jobs.queue.Enqueue(ctx, jobqueue.Request[target]{
				Kind: kindFetch, Target: "a", ArgsJSON: args, Priority: 7,
			})
			if err != nil {
				t.Error(err)
			}
			actions = append(actions, action)
		}
		return nil
	})
	jobs.runDue(t, runner)
	want := []jobqueue.Action{jobqueue.ActionDrop, jobqueue.ActionInsert, jobqueue.ActionMerge}
	if !slices.Equal(actions, want) {
		t.Errorf("requests while the job ran = %v, want %v: the one it covers dropped, one follow-up queued, and"+
			" the next merged into it", actions, want)
	}
	if !slices.Equal(ran, []string{"same", "more"}) {
		t.Errorf("ran %q, want the job, then its one follow-up", ran)
	}
	if count := len(jobs.storage.Rows()); count != 2 {
		t.Errorf("%d jobs, want 2: at most one running and one queued per target", count)
	}
}

func TestRunDue_RetriesAFailureWithJitteredDoublingBackoffThenFails(t *testing.T) {
	t.Parallel()
	for _, jitter := range []struct {
		name   string
		random io.Reader
		factor float64
	}{
		{"unjittered", jobqueuetest.Unjittered(), 1},
		{"earliest", jobqueuetest.EarliestJitter(), 0.9},
		{"latest", jobqueuetest.LatestJitter(), 1.1},
	} {
		t.Run(jitter.name, func(t *testing.T) {
			t.Parallel()
			jobs := newFixture(t, jitter.random, nil)
			jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
			runner := jobs.queue.NewRunner(kindFetch, func(context.Context, jobqueue.Job[target]) error {
				return errors.New("the provider answered 500")
			})
			for attempt, doubled := range []time.Duration{firstRetryDelay, 2 * firstRetryDelay} {
				jobs.runDue(t, runner)
				row := jobs.row(t, "a")
				wait := row.RunnableAt.Sub(jobs.clock.Now())
				want := time.Duration(float64(doubled) * jitter.factor)
				if row.State != jobqueue.StateQueued || row.AttemptCount != int64(attempt+1) ||
					wait < want-time.Millisecond || wait > want {
					t.Fatalf("after attempt %d: %s with %d attempts spent, waiting %v; want queued, waiting %v",
						attempt+1, row.State, row.AttemptCount, wait, want)
				}
				jobs.clock.Set(row.RunnableAt)
			}
			jobs.runDue(t, runner)
			row := jobs.row(t, "a")
			if row.State != jobqueue.StateFailed || row.LastFailure != "the provider answered 500" {
				t.Errorf("at the attempt limit: %s (%q), want failed with what the last attempt said", row.State,
					row.LastFailure)
			}
		})
	}
}

func TestRunDue_NoJitterTakesAWaitPastADay(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.LatestJitter(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	runner := jobs.queue.NewRunner(kindFetch, func(context.Context, jobqueue.Job[target]) error {
		return errors.New("the image server answered 503")
	}).WithRetrySchedule([]time.Duration{23 * time.Hour, 48 * time.Hour})
	for _, want := range []time.Duration{jobqueue.LongestRetryDelay, jobqueue.LongestRetryDelay} {
		jobs.runDue(t, runner)
		row := jobs.row(t, "a")
		if wait := row.RunnableAt.Sub(jobs.clock.Now()); wait != want {
			t.Fatalf("a wait jittered to its latest came to %v, want %v: no retry waits past a day", wait, want)
		}
		jobs.clock.Set(row.RunnableAt)
	}
}

func TestRunDue_ASnoozeCostsNoAttemptAndTheRunnerGoesOn(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a", Priority: 1})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "b"})
	until := jobs.clock.Now().Add(time.Hour)
	var ran []target
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.Target)
		if job.Target == "a" && jobs.clock.Now().Before(until) {
			return jobqueue.Snooze(errors.New("waiting for the disk"), until)
		}
		return nil
	})
	for range 3 {
		jobs.runDue(t, runner)
	}
	row := jobs.row(t, "a")
	if row.State != jobqueue.StateQueued || row.AttemptCount != 0 || !row.RunnableAt.Equal(until) || !row.IsSnoozed {
		t.Errorf("snoozed job = %s, %d attempts, until %v; want queued until its time with no attempt spent",
			row.State, row.AttemptCount, row.RunnableAt)
	}
	if !slices.Equal(ran, []target{"a", "b"}) {
		t.Errorf("ran %q, want the snoozed job once and the next job at once after it", ran)
	}
	jobs.clock.Set(until)
	jobs.runDue(t, runner)
	if row := jobs.row(t, "a"); row.State != jobqueue.StateDone || row.AttemptCount != 1 {
		t.Errorf("after its time: %s after %d attempts, want done after 1", row.State, row.AttemptCount)
	}
}

func TestRunDue_APanicIsTheToolsFailureFreeOnceAndTheJobsFailureTheSecondTimeInARow(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	var toldToolFailed []bool
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		toldToolFailed = append(toldToolFailed, job.PreviousAttemptToolFailed)
		panic("a parser met bytes it did not expect")
	})
	jobs.runDue(t, runner)
	row := jobs.row(t, "a")
	if row.State != jobqueue.StateQueued || row.AttemptCount != 0 || !row.LastAttemptToolFailed ||
		!strings.Contains(row.LastFailure, "panicked: a parser met bytes") {
		t.Fatalf("after one panic: %s, %d attempts, tool failed %t, %q; want queued again at no cost, the panic"+
			" its last failure", row.State, row.AttemptCount, row.LastAttemptToolFailed, row.LastFailure)
	}
	if wait := row.RunnableAt.Sub(jobs.clock.Now()); wait != firstRetryDelay {
		t.Errorf("after a panic the job waits %v, want the first retry delay", wait)
	}
	jobs.clock.Set(row.RunnableAt)
	jobs.runDue(t, runner)
	if row := jobs.row(t, "a"); row.State != jobqueue.StateFailed ||
		!strings.Contains(row.LastFailure, "two attempts in a row") {
		t.Errorf("after a second panic in a row: %s (%q), want failed as the job's own failure", row.State,
			row.LastFailure)
	}
	if !slices.Equal(toldToolFailed, []bool{false, true}) {
		t.Errorf("each attempt was told its last one's tool failed: %v, want [false true]", toldToolFailed)
	}
}

func TestRunDue_AClaimRefusesAJobThatHasHadItsAttempts(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	ran := 0
	stopMidway := jobs.queue.NewRunner(kindFetch, func(context.Context, jobqueue.Job[target]) error {
		ran++
		return nil
	})
	// Each claim with no outcome recorded after is a stop in the middle of the attempt; the start
	// after it returns the job to the queue.
	for range attemptLimit {
		claim := jobqueue.Claim{Kind: kindFetch, Now: jobs.clock.Now()}
		if _, found, err := jobs.storage.Claim(t.Context(), claim); err != nil || !found {
			t.Fatalf("claim: %v, %t", err, found)
		}
		if count, err := jobs.queue.RequeueRunning(t.Context()); err != nil || count != 1 {
			t.Fatalf("RequeueRunning = %d, %v; want 1", count, err)
		}
	}
	jobs.runDue(t, stopMidway)
	row := jobs.row(t, "a")
	if ran != 0 || row.State != jobqueue.StateFailed || !strings.Contains(row.LastFailure, "has had its 3 attempts") {
		t.Errorf("after %d stops mid-attempt: ran %d times, %s (%q); want failed unrun at the limit (S86)",
			attemptLimit, ran, row.State, row.LastFailure)
	}
}

func TestRequeueRunning_ReturnsWhatAStopLeftRunningWithItsAttemptCounted(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "a"})
	claim := jobqueue.Claim{Kind: kindFetch, Now: jobs.clock.Now()}
	if _, _, err := jobs.storage.Claim(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
	if count, err := jobs.queue.RequeueRunning(t.Context()); err != nil || count != 1 {
		t.Fatalf("RequeueRunning = %d, %v; want 1", count, err)
	}
	if row := jobs.row(t, "a"); row.State != jobqueue.StateQueued || row.AttemptCount != 1 {
		t.Errorf("after a stop: %s with %d attempts, want queued again with its attempt counted", row.State,
			row.AttemptCount)
	}
	var attempts []int64
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		attempts = append(attempts, job.AttemptNumber)
		return nil
	})
	jobs.runDue(t, runner)
	if !slices.Equal(attempts, []int64{2}) {
		t.Errorf("the job restarted as attempts %v, want [2]", attempts)
	}
}

func TestCancel_AQueuedJobNeverRunsAndARunningOnesContextEndsWithErrCancelled(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "queued"})
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "running"})
	if err := jobs.queue.Cancel(t.Context(), jobs.row(t, "queued").ID); err != nil {
		t.Fatal(err)
	}
	var cause error
	var ran []target
	runner := jobs.queue.NewRunner(kindFetch, func(ctx context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.Target)
		if err := jobs.queue.Cancel(ctx, job.ID); err != nil {
			t.Error(err)
		}
		<-ctx.Done()
		cause = context.Cause(ctx)
		return errors.New("stopped") // an answer that must not undo the cancel
	})
	jobs.runDue(t, runner)
	if !slices.Equal(ran, []target{"running"}) || !errors.Is(cause, jobqueue.ErrCancelled) {
		t.Errorf("ran %q with cause %v; want only the job not cancelled first, told ErrCancelled", ran, cause)
	}
	for _, name := range []target{"queued", "running"} {
		if row := jobs.row(t, name); row.State != jobqueue.StateCancelled || row.FinishedAt == nil {
			t.Errorf("%s job = %s, want cancelled with a finish time", name, row.State)
		}
	}
	if err := jobs.queue.Cancel(t.Context(), jobs.row(t, "running").ID); !errors.Is(err, jobqueue.ErrNotOpen) {
		t.Errorf("cancelling a cancelled job: %v, want ErrNotOpen", err)
	}
}

// A job that ended longer ago than it is kept for is forgotten; an open one, and one that ended
// lately, are kept (D233 §8).
func TestPruneFinished_ForgetsWhatEndedLongerAgoThanItIsKeptFor(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t, jobqueuetest.Unjittered(), nil)
	runner := jobs.queue.NewRunner(kindFetch, func(context.Context, jobqueue.Job[target]) error { return nil })
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "old"})
	jobs.runDue(t, runner)
	jobs.clock.Advance(time.Hour)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "recent"})
	jobs.runDue(t, runner)
	later := jobs.clock.Now().Add(time.Hour)
	jobs.enqueue(t, jobqueue.Request[target]{Kind: kindFetch, Target: "open", RunnableAt: later})
	jobs.clock.Advance(30 * time.Minute)
	pruned, err := jobs.queue.PruneFinished(t.Context(), time.Hour)
	if err != nil || pruned != 1 {
		t.Fatalf("PruneFinished = %d, %v; want the one that ended 90 minutes ago", pruned, err)
	}
	var left []target
	for _, row := range jobs.storage.Rows() {
		left = append(left, row.Target)
	}
	slices.Sort(left)
	if !slices.Equal(left, []target{"open", "recent"}) {
		t.Errorf("the jobs left are %q; want the open one and the one that ended half an hour ago", left)
	}
}
