// SPDX-License-Identifier: AGPL-3.0-or-later

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
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The states the tests read from the job table.
const (
	stateDone   = string(jobqueue.StateDone)
	stateFailed = string(jobqueue.StateFailed)
)

type fixture struct {
	queue  *Queue
	cache  *store.Cache
	clock  *clocktest.Fake
	config *settings.Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, config, settings.JobAttemptLimit, "3")
	settingstest.Set(t, config, settings.JobFirstRetryDelaySeconds, "10")
	cache := storetest.OpenCache(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	logger := slog.New(slog.NewTextHandler(t.Output(), nil))
	queue := NewQueue(QueueParts{
		Cache: cache, Clock: fakeClock, Settings: config, Random: jobqueuetest.Unjittered(), Logger: logger,
	})
	return fixture{queue: queue, cache: cache, clock: fakeClock, config: config}
}

func (fixed fixture) enqueue(t *testing.T, itemID, priority int64) {
	t.Helper()
	job := Request{Kind: KindFetchMetadata, Target: Target{ItemID: itemID}, Priority: priority, Reason: "test"}
	if _, err := fixed.queue.Enqueue(t.Context(), job); err != nil {
		t.Fatal(err)
	}
}

func (fixed fixture) runDue(t *testing.T, runner *Runner) {
	t.Helper()
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func (fixed fixture) job(t *testing.T, itemID int64) cachegen.Job {
	t.Helper()
	jobs, err := fixed.cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.ItemID != nil && *job.ItemID == itemID {
			return job
		}
	}
	t.Fatalf("no job for subject %d", itemID)
	return cachegen.Job{}
}

func TestEnqueue_QueuesATargetOnceWhileItIsOpen(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	for range 2 {
		jobs.enqueue(t, 7, 0)
	}
	if listed, err := jobs.cache.Read().ListJobs(t.Context(), 10); err != nil || len(listed) != 1 {
		t.Fatalf("%d jobs (%v) after queuing one subject twice, want 1", len(listed), err)
	}
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error { return nil })
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if added, err := jobs.queue.Enqueue(t.Context(), Request{
		Kind:   KindFetchMetadata,
		Target: Target{ItemID: 7},
	}); err != nil || !added {
		t.Errorf("a finished job's subject could not be queued again: %v", err)
	}
}

func TestRunDue_RunsTheHighestPriorityFirst(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	jobs.enqueue(t, 2, 10)
	jobs.enqueue(t, 3, 0)
	var ran []int64
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(_ context.Context, job Job) error {
		ran = append(ran, job.ItemID)
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
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error {
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
	if runs != 3 || job.State != stateFailed || job.LastFailure == nil {
		t.Errorf("after %v: %d runs, job %+v; want 3 runs, then failed with the reply",
			jobs.clock.Now().Sub(start), runs, job)
	}
}

func TestRunDue_APermanentFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error {
		return MarkPermanent(errors.New("the key was refused"))
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if job := jobs.job(t, 1); job.State != stateFailed || job.AttemptCount != 1 {
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

func TestRunDue_AJobParkedOnItsDestinationWaitsOnCacheDBAtNoCostAndTheViewSaysWhy(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 10) // finds its destination down when it runs
	named := Request{
		Kind: KindFetchMetadata, Target: Target{ItemID: 2}, Reason: "test", Destination: DestinationTMDBAPI,
	}
	if _, err := jobs.queue.Enqueue(t.Context(), named); err != nil {
		t.Fatal(err)
	}
	isKeySet := false
	var ran []int64
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(_ context.Context, job Job) error {
		ran = append(ran, job.ItemID)
		if !isKeySet {
			return Park(Parking{Destination: DestinationTMDBAPI, Reason: "the key is not set", AwaitsSignal: true},
				errors.New("no key"))
		}
		return nil
	})
	parkedAt := jobs.clock.Now()
	for range 3 {
		jobs.runDue(t, runner)
		jobs.clock.Advance(time.Hour)
	}
	if job := jobs.job(t, 1); job.State != "queued" || job.AttemptCount != 0 || !slices.Equal(ran, []int64{1}) {
		t.Errorf("while the key is not set: job %+v, ran %v; want the first run once, queued, no attempt spent,"+
			" and the one queued on TMDB never", job, ran)
	}
	views, err := jobs.queue.List(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	want := DestinationState{
		Destination: DestinationTMDBAPI, Reason: "the key is not set", Since: parkedAt, AwaitsSignal: true,
	}
	for _, view := range views {
		if view.Destination != DestinationTMDBAPI || view.WaitingFor == nil || *view.WaitingFor != want {
			t.Errorf("job %d's view: on %q, waiting for %+v; want %+v", view.ItemID, view.Destination,
				view.WaitingFor, want)
		}
	}
	isKeySet = true
	jobs.queue.Unpark(t.Context(), DestinationTMDBAPI)
	jobs.runDue(t, runner)
	if jobs.job(t, 1).State != stateDone || jobs.job(t, 2).State != stateDone || !slices.Equal(ran, []int64{1, 1, 2}) {
		t.Errorf("once the key changed: ran %v; want both jobs done, the higher first", ran)
	}
}

func TestRunDue_AKindWithItsOwnScheduleRetriesOnItThenFails(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	runs := 0
	schedule := []time.Duration{time.Minute, 5 * time.Minute}
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error {
		runs++
		return errors.New("the image server answered 503")
	}).WithRetrySchedule(schedule)
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

func TestEnqueue_AJobWaitsUntilItsRunnableAt(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	later := jobs.clock.Now().Add(12 * time.Hour)
	if _, err := jobs.queue.Enqueue(t.Context(), Request{
		Kind: KindFetchMetadata, Target: Target{ItemID: 4},
		RunnableAt: later,
	}); err != nil {
		t.Fatal(err)
	}
	ran := 0
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error { ran++; return nil })
	if err := runner.RunDue(t.Context()); err != nil || ran != 0 {
		t.Fatalf("ran %d before its time (%v)", ran, err)
	}
	jobs.clock.Set(later)
	if err := runner.RunDue(t.Context()); err != nil || ran != 1 {
		t.Errorf("ran %d at its time (%v)", ran, err)
	}
}

func TestRun_AQueuedJobWakesARunnerOfItsOwnKindAtOnce(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	ran := make(chan Kind, 4)
	never := make(chan time.Time) // no tick: only a wake-up can start a run
	kinds := []Kind{KindFetchMetadata, KindMakeArtworkSizes, KindReadThrough, KindExtractSubtitle}
	for _, kind := range kinds {
		runner := jobs.queue.NewRunner(kind, func(_ context.Context, job Job) error {
			ran <- job.Kind
			return nil
		})
		go func() { _ = runner.Run(ctx, never) }() //nolint:errcheck // Run returns nil when ctx ends
	}
	for index, kind := range kinds {
		if _, err := jobs.queue.Enqueue(t.Context(), Request{
			Kind:   kind,
			Target: Target{ItemID: int64(index + 1)},
		}); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-ran:
			if got != kind {
				t.Errorf("queued a %s job, and a %s job ran", kind, got)
			}
		case <-ctx.Done():
			t.Fatalf("a queued %s job did not run: its runner was not woken", kind)
		}
	}
}

func TestRun_SeveralRunnersOfOneKindRunItsJobsAtOnceEachOnce(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	const runners = 3
	started := make(chan int64, runners*2)
	release := make(chan struct{})
	never := make(chan time.Time)
	for range runners {
		runner := jobs.queue.NewRunner(KindReadThrough, func(ctx context.Context, job Job) error {
			started <- job.FileID
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		go func() { _ = runner.Run(ctx, never) }() //nolint:errcheck // Run returns nil when ctx ends
	}
	for subject := range int64(runners) {
		if _, err := jobs.queue.Enqueue(t.Context(), Request{
			Kind:   KindReadThrough,
			Target: Target{FileID: subject + 1},
		}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[int64]bool{}
	for range runners {
		select {
		case subject := <-started:
			if seen[subject] {
				t.Errorf("job %d ran twice", subject)
			}
			seen[subject] = true
		case <-ctx.Done():
			t.Fatalf("only %d of %d jobs started while the others were running: not at once", len(seen), runners)
		}
	}
	close(release)
}

func TestRunDue_TheBackoffNeverShrinksHoweverManyAttemptsTheLimitAllows(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	settingstest.Set(t, jobs.config, settings.JobAttemptLimit, "100")
	settingstest.Set(t, jobs.config, settings.JobFirstRetryDelaySeconds, "86400")
	jobs.enqueue(t, 1, 0)
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error {
		return errors.New("the provider answered 500")
	})
	var lastWait time.Duration
	for attempt := 1; attempt <= 30; attempt++ {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		job := jobs.job(t, 1)
		wait := time.UnixMilli(job.RunnableAt).Sub(jobs.clock.Now())
		if job.State != "queued" || wait < lastWait {
			t.Fatalf("after attempt %d: state %s, waits %v after waiting %v; want queued, waiting no less",
				attempt, job.State, wait, lastWait)
		}
		lastWait = wait
		jobs.clock.Set(time.UnixMilli(job.RunnableAt))
	}
}

func TestRunDue_TheBackoffStopsAtADayThenRetriesDailyUntilTheLimit(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	const attemptLimit = 20
	settingstest.Set(t, jobs.config, settings.JobAttemptLimit, "20")
	jobs.enqueue(t, 1, 0)
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error {
		return errors.New("the provider answered 500")
	})
	want := 10 * time.Second
	for attempt := 1; attempt < attemptLimit; attempt++ {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		job := jobs.job(t, 1)
		wait := time.UnixMilli(job.RunnableAt).Sub(jobs.clock.Now())
		if job.State != "queued" || wait != want {
			t.Fatalf("after attempt %d: state %s, waits %v; want queued, waiting %v", attempt, job.State, wait, want)
		}
		want = min(2*want, 24*time.Hour)
		jobs.clock.Set(time.UnixMilli(job.RunnableAt))
	}
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if job := jobs.job(t, 1); job.State != stateFailed {
		t.Errorf("after attempt %d: state %s; want failed at the attempt limit", attemptLimit, job.State)
	}
}

func TestScenario_S86_AJobThatBringsTheServerDownEveryTimeFailsAtItsLimitAndAPanicFailsOnlyItsJob(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, 0)
	jobs.enqueue(t, 2, 0)
	// Item 1's job stops the server in the middle of every attempt: claimed, never finished, and
	// returned to the queue at the next start. The fixture's attempt limit is 3.
	for range 3 {
		err := jobs.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
			_, startErr := queries.StartJob(t.Context(), jobs.job(t, 1).ID)
			return startErr
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := jobs.queue.RequeueRunning(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	var ran []int64
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(_ context.Context, job Job) error {
		ran = append(ran, job.ItemID)
		panic("a parser met bytes it did not expect") // item 2's job panics every time
	})
	for range 2 {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
		jobs.clock.Advance(10 * time.Second)
	}
	if job := jobs.job(t, 1); job.State != stateFailed || job.AttemptCount != 3 {
		t.Errorf("the job that stops the server: %s after %d attempts, want failed unrun at its limit of 3",
			job.State, job.AttemptCount)
	}
	if job := jobs.job(t, 2); job.State != stateFailed || job.AttemptCount != 1 {
		t.Errorf("the job that panics: %s after %d attempts, want failed the second time, the first free",
			job.State, job.AttemptCount)
	}
	if !slices.Equal(ran, []int64{2, 2}) {
		t.Errorf("ran %v, want the panicking job twice and the other never", ran)
	}
}

func TestNewRunner_AHandlerIsToldItsJobsPriority(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	jobs.enqueue(t, 1, PriorityPersonsRequest)
	var told int64
	runner := jobs.queue.NewRunner(KindFetchMetadata, func(_ context.Context, job Job) error {
		told = job.Priority
		return nil
	})
	jobs.runDue(t, runner)
	if told != PriorityPersonsRequest {
		t.Errorf("the handler was told priority %d, want %d: it asks a provider by it (providers.JobImportance)",
			told, PriorityPersonsRequest)
	}
}

// A job that ended is forgotten once finished_jobs_kept_days have passed since; an open one, and one
// that ended lately, are kept (D233 §8).
func TestPruneFinished_ForgetsWhatEndedLongerAgoThanTheSettingKeeps(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t)
	settingstest.Set(t, fixed.config, settings.FinishedJobsKeptDays, "30")
	runner := fixed.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error { return nil })
	fixed.enqueue(t, 1, PriorityBackground)
	fixed.runDue(t, runner)
	fixed.clock.Advance(20 * 24 * time.Hour)
	fixed.enqueue(t, 2, PriorityBackground)
	fixed.runDue(t, runner)
	fixed.enqueue(t, 3, PriorityBackground) // still queued: the runner is not run again
	fixed.clock.Advance(11 * 24 * time.Hour)
	pruned, err := fixed.queue.PruneFinished(t.Context())
	if err != nil || pruned != 1 {
		t.Fatalf("PruneFinished = %d, %v; want the one job that ended 31 days ago", pruned, err)
	}
	listed, err := fixed.cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	var left []int64
	for _, job := range listed {
		left = append(left, *job.ItemID)
	}
	slices.Sort(left)
	if !slices.Equal(left, []int64{2, 3}) {
		t.Errorf("the jobs left are for items %v; want 2, which ended 11 days ago, and 3, still queued", left)
	}
}

// A backup and the certificate's renewal work on the whole server and name no target: each is
// queued once at a time, and one that names a target is refused (D243 §6, §11).
func TestEnqueue_AWholeServerJobNamesNoTargetAndIsQueuedOnce(t *testing.T) {
	t.Parallel()
	jobs := newFixture(t)
	for _, kind := range []Kind{KindBackup, KindRenewCertificate} {
		for range 2 {
			request := Request{Kind: kind, ArgsJSON: "{}", Reason: "test"}
			if _, err := jobs.queue.Enqueue(t.Context(), request); err != nil {
				t.Fatalf("queue a %s: %v", kind, err)
			}
		}
		if _, err := jobs.queue.Enqueue(t.Context(), Request{
			Kind: kind, Target: Target{ItemID: 7}, ArgsJSON: "{}", Reason: "test",
		}); !errors.Is(err, ErrNotOneTarget) {
			t.Errorf("a %s naming an item: %v, want ErrNotOneTarget", kind, err)
		}
	}
	if listed, err := jobs.cache.Read().ListJobs(t.Context(), 10); err != nil || len(listed) != 2 {
		t.Errorf("%d jobs (%v) after asking for each whole-server kind twice; want one of each", len(listed), err)
	}
}
