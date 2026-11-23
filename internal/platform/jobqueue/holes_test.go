// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue_test

// The holes the queue audit of 2026-11-23 found between a claim and its record (D540): a cancel
// that lands meanwhile, an outcome not written, a policy not read.

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
)

// hookedStorage is the in-memory storage with a hook before each Record: a cancel landing then,
// or a write the database refuses.
type hookedStorage struct {
	*jobqueuetest.Memory[target]
	beforeRecord func(outcome jobqueue.Outcome) error
}

func (storage *hookedStorage) Record(ctx context.Context, outcome jobqueue.Outcome) (bool, error) {
	if storage.beforeRecord != nil {
		if err := storage.beforeRecord(outcome); err != nil {
			return false, err
		}
	}
	return storage.Memory.Record(ctx, outcome)
}

// flakyPolicy is fixedPolicy whose attempt limit cannot be read failCount more times.
type flakyPolicy struct {
	fixedPolicy
	failCount int
}

func (policy *flakyPolicy) AttemptLimit(ctx context.Context) (int64, error) {
	if policy.failCount > 0 {
		policy.failCount--
		return 0, errors.New("read jobAttemptLimit: disk I/O error")
	}
	return policy.fixedPolicy.AttemptLimit(ctx)
}

// hookedFixture is a queue over hookedStorage, on a fake clock, unjittered.
type hookedFixture struct {
	queue   *jobqueue.Queue[target]
	storage *hookedStorage
	clock   *clocktest.Fake
}

func newHookedFixture(t *testing.T, config jobqueue.Config) hookedFixture {
	t.Helper()
	storage := &hookedStorage{Memory: &jobqueuetest.Memory[target]{}}
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	if config.Policy == nil {
		config.Policy = fixedPolicy{attemptLimit: attemptLimit, firstRetryDelay: firstRetryDelay}
	}
	config.Random = jobqueuetest.Unjittered()
	config.Logger = slog.New(slog.NewTextHandler(t.Output(), nil))
	return hookedFixture{queue: jobqueue.New(storage, fakeClock, config), storage: storage, clock: fakeClock}
}

func (fixed hookedFixture) rowOf(t *testing.T, wanted target) jobqueue.Row[target] {
	t.Helper()
	for _, row := range fixed.storage.Rows() {
		if row.Target == wanted {
			return row
		}
	}
	t.Fatalf("no job for %q", wanted)
	return jobqueue.Row[target]{}
}

// A cancel that lands after the handler answered AssignThen, before the queue records the job
// assigned, hands no work over: Record says it wrote nothing, and the cancel hook, which ran before
// the work was given, is told again (D540).
func TestRunDue_ACancelLandingBeforeTheAssignIsRecordedHandsNoWorkOver(t *testing.T) {
	t.Parallel()
	jobs := newHookedFixture(t, jobqueue.Config{})
	ctx := t.Context()
	var said []string
	jobs.queue.OnCancel(func(context.Context, int64) { said = append(said, "cancel hook: stop the node") })
	if _, err := jobs.queue.Enqueue(ctx, jobqueue.Request[target]{Kind: kindMake, Target: "film"}); err != nil {
		t.Fatal(err)
	}
	jobs.storage.beforeRecord = func(outcome jobqueue.Outcome) error {
		if outcome.State == jobqueue.StateAssigned {
			// A person's cancel, or a seek's CancelOpenOf, on another goroutine, lands now.
			if err := jobs.queue.Cancel(ctx, outcome.ID); err != nil {
				t.Errorf("cancel: %v", err)
			}
		}
		return nil
	}
	runner := jobs.queue.NewRunner(kindMake, func(context.Context, jobqueue.Job[target]) error {
		return jobqueue.AssignThen(jobs.clock.Now().Add(time.Hour), func(context.Context) {
			said = append(said, "handed over to the node")
		})
	})
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if row := jobs.rowOf(t, "film"); row.State != jobqueue.StateCancelled {
		t.Fatalf("the job is %s, want cancelled", row.State)
	}
	if slices.Contains(said, "handed over to the node") {
		t.Errorf("a job cancelled before its assignment was recorded had its work handed over anyway: %q", said)
	}
}

// An assigned job whose node's outcome is taken (TakeAssigned moves it to running) but whose Record
// then fails for a cause that is not the storage's (a cancelled context, a busy database) is given
// back to the queue: left running, nothing moved it but a restart, and every request for its target
// that it covered was dropped (D540).
func TestResolve_AnOutcomeWhoseRecordFailsLeavesNoJobRunning(t *testing.T) {
	t.Parallel()
	jobs := newHookedFixture(t, jobqueue.Config{})
	ctx := t.Context()
	request := jobqueue.Request[target]{Kind: kindMake, Target: "film"}
	if _, err := jobs.queue.Enqueue(ctx, request); err != nil {
		t.Fatal(err)
	}
	runner := jobs.queue.NewRunner(kindMake, func(context.Context, jobqueue.Job[target]) error {
		return jobqueue.Assign(jobs.clock.Now().Add(time.Hour))
	})
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	id := jobs.rowOf(t, "film").ID
	failOnce := true
	jobs.storage.beforeRecord = func(jobqueue.Outcome) error {
		if failOnce {
			failOnce = false
			return context.Canceled // the node's connection ended as its result was resolved
		}
		return nil
	}
	if _, err := runner.Resolve(ctx, id, nil); err == nil {
		t.Fatal("the resolve whose record failed answered no error")
	}
	// A day later, well past the job's assigned time, the runner has run many times.
	for range 24 {
		jobs.clock.Advance(time.Hour)
		if err := runner.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if row := jobs.rowOf(t, "film"); row.State == jobqueue.StateRunning {
		t.Errorf("a day after its outcome could not be recorded, job %d is still %s, with no handler", id, row.State)
	}
	action, err := jobs.queue.Enqueue(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if action == jobqueue.ActionDrop {
		t.Errorf("a new request for the target was dropped: the stuck running job covers it")
	}
}

// Every outcome the storage could not write is written once it can: kept in one field of the
// runner, the second, from Resolve on a node's goroutine, replaced the first, which stayed running
// until a restart, and the two writers raced (D540).
func TestRunDue_TwoOutcomesTheStorageCouldNotWriteAreBothWrittenOnceItCan(t *testing.T) {
	t.Parallel()
	jobs := newHookedFixture(t, jobqueue.Config{
		StorageDestination: "storage", IsStorageFailure: func(err error) bool { return errors.Is(err, errFull) },
	})
	ctx := t.Context()
	for _, name := range []target{"given", "done here"} {
		if _, err := jobs.queue.Enqueue(ctx, jobqueue.Request[target]{Kind: kindMake, Target: name}); err != nil {
			t.Fatal(err)
		}
	}
	isFull := false
	jobs.storage.beforeRecord = func(outcome jobqueue.Outcome) error {
		if isFull && outcome.State == jobqueue.StateDone {
			return errFull
		}
		return nil
	}
	runner := jobs.queue.NewRunner(kindMake, func(_ context.Context, job jobqueue.Job[target]) error {
		if job.Target == "given" {
			return jobqueue.Assign(jobs.clock.Now().Add(time.Hour))
		}
		isFull = true // the disk fills as this job ends
		return nil
	})
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	given := jobs.rowOf(t, "given").ID
	// The node's word on the assigned job comes while the disk is still full.
	if _, err := runner.Resolve(ctx, given, nil); err != nil {
		t.Fatal(err)
	}
	isFull = false
	for range 10 {
		jobs.clock.Advance(time.Hour)
		if err := runner.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []target{"given", "done here"} {
		if row := jobs.rowOf(t, name); row.State != jobqueue.StateDone {
			t.Errorf("once the storage had room for hours, job %q is %s; want done", name, row.State)
		}
	}
}

// An attempt limit that cannot be read is read before the claim, so it leaves no destination
// probing: read after a claim that began the destination's probe, the early return left it
// "probing" for good, no job of it claimed again until a restart, and the job running (D540).
func TestRunDue_AnAttemptLimitThatCannotBeReadLeavesNoProbeRunning(t *testing.T) {
	t.Parallel()
	policy := &flakyPolicy{fixedPolicy: fixedPolicy{attemptLimit: attemptLimit, firstRetryDelay: firstRetryDelay}}
	jobs := newHookedFixture(t, jobqueue.Config{Policy: policy})
	ctx := t.Context()
	jobs.queue.Park(ctx, jobqueue.Parking{Destination: "host", Reason: "unreachable"})
	if _, err := jobs.queue.Enqueue(ctx, jobqueue.Request[target]{
		Kind: kindFetch, Target: "first", Destination: "host",
	}); err != nil {
		t.Fatal(err)
	}
	var ran []target
	runner := jobs.queue.NewRunner(kindFetch, func(_ context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.Target)
		return nil
	})
	state, _ := jobs.queue.DestinationState("host")
	jobs.clock.Set(state.NextProbeAt)
	policy.failCount = 1
	if err := runner.RunDue(ctx); err == nil {
		t.Fatal("a claim that could not read the attempt limit answered no error")
	}
	if _, err := jobs.queue.Enqueue(ctx, jobqueue.Request[target]{
		Kind: kindFetch, Target: "second", Destination: "host",
	}); err != nil {
		t.Fatal(err)
	}
	for range 48 {
		jobs.clock.Advance(time.Hour)
		if err := runner.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	state, isParked := jobs.queue.DestinationState("host")
	if !slices.Contains(ran, "second") {
		t.Errorf("two days on, ran %q; the host is parked %v, probing %v: its probe never ended", ran, isParked,
			state.IsProbing)
	}
	if row := jobs.rowOf(t, "first"); row.State == jobqueue.StateRunning {
		t.Errorf("the job claimed before the read failed is still %s, with no handler", row.State)
	}
}

// The same, on the storage: while the storage is parked its probe is a claim, and the early return
// left the storage probing for good, and no runner of any kind claiming anything (D540).
func TestRunDue_AStorageProbeThatCannotReadTheAttemptLimitStopsNoQueue(t *testing.T) {
	t.Parallel()
	policy := &flakyPolicy{fixedPolicy: fixedPolicy{attemptLimit: attemptLimit, firstRetryDelay: firstRetryDelay}}
	jobs := newHookedFixture(t, jobqueue.Config{
		Policy: policy, StorageDestination: "storage",
		IsStorageFailure: func(err error) bool { return errors.Is(err, errFull) },
	})
	ctx := t.Context()
	for _, request := range []jobqueue.Request[target]{
		{Kind: kindFetch, Target: "probe"}, {Kind: kindMake, Target: "other kind"},
	} {
		if _, err := jobs.queue.Enqueue(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	var ran []target
	handler := func(_ context.Context, job jobqueue.Job[target]) error {
		ran = append(ran, job.Target)
		return nil
	}
	fetch, maker := jobs.queue.NewRunner(kindFetch, handler), jobs.queue.NewRunner(kindMake, handler)
	jobs.queue.Park(ctx, jobqueue.Parking{Destination: "storage", Reason: "cannot write: disk full"})
	state, _ := jobs.queue.DestinationState("storage")
	jobs.clock.Set(state.NextProbeAt)
	policy.failCount = 1
	if err := fetch.RunDue(ctx); err == nil {
		t.Fatal("the storage's probe that could not read the attempt limit answered no error")
	}
	for range 48 {
		jobs.clock.Advance(time.Hour)
		for _, runner := range []*jobqueue.Runner[target]{fetch, maker} {
			if err := runner.RunDue(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !slices.Contains(ran, "other kind") {
		state, _ = jobs.queue.DestinationState("storage")
		t.Errorf("two days on, with the storage writable, ran %q; the storage probing %v: every claim waits on it",
			ran, state.IsProbing)
	}
}

// An assignment the storage refuses for want of room hands no work over: its machine's attempt is
// forgotten (the overdue hook), and the job, kept as a return to the queue, is given again once
// there is room. Handed over, the machine's word could come before the assignment was written, find
// the job running, not assigned, and be dropped, and the job then waited out its assigned time
// (D540).
func TestRunDue_AnAssignmentTheStorageCannotWriteHandsNoWorkOver(t *testing.T) {
	t.Parallel()
	jobs := newHookedFixture(t, jobqueue.Config{
		StorageDestination: "storage", IsStorageFailure: func(err error) bool { return errors.Is(err, errFull) },
	})
	ctx := t.Context()
	var said []string
	jobs.queue.OnOverdue(func(context.Context, int64) { said = append(said, "overdue hook: forget the attempt") })
	if _, err := jobs.queue.Enqueue(ctx, jobqueue.Request[target]{Kind: kindMake, Target: "film"}); err != nil {
		t.Fatal(err)
	}
	isFull := true
	jobs.storage.beforeRecord = func(outcome jobqueue.Outcome) error {
		if isFull && outcome.State == jobqueue.StateAssigned {
			return errFull
		}
		return nil
	}
	runner := jobs.queue.NewRunner(kindMake, func(context.Context, jobqueue.Job[target]) error {
		return jobqueue.AssignThen(jobs.clock.Now().Add(time.Hour), func(context.Context) {
			said = append(said, "handed over to the node")
		})
	})
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"overdue hook: forget the attempt"}; !slices.Equal(said, want) {
		t.Errorf("an assignment the storage could not write: %q, want %q", said, want)
	}
	isFull = false
	jobs.clock.Advance(time.Hour)
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	row := jobs.rowOf(t, "film")
	if row.State != jobqueue.StateAssigned || !slices.Contains(said, "handed over to the node") {
		t.Errorf("once the storage had room the job is %s, having %q; want it given again, assigned and handed over",
			row.State, said)
	}
}

// GiveBackUnheld gives back to the queue a running job none holds, once two looks a grace apart
// found it so, and never one a runner holds, however long it runs (D540).
func TestGiveBackUnheld_GivesBackOnlyARunningJobNoneHolds(t *testing.T) {
	t.Parallel()
	jobs := newHookedFixture(t, jobqueue.Config{})
	ctx := t.Context()
	for _, name := range []target{"lost", "held"} {
		if _, err := jobs.queue.Enqueue(ctx, jobqueue.Request[target]{Kind: kindMake, Target: name}); err != nil {
			t.Fatal(err)
		}
	}
	// Claimed with no runner: as a job whose outcome a failed write lost.
	claim := jobqueue.Claim{Kind: kindMake, Now: jobs.clock.Now()}
	if _, isClaimed, err := jobs.storage.Claim(ctx, claim); err != nil || !isClaimed {
		t.Fatalf("claim: %v, %t", err, isClaimed)
	}
	var givenBack []int
	runner := jobs.queue.NewRunner(kindMake, func(_ context.Context, job jobqueue.Job[target]) error {
		if job.Target != "held" {
			return nil
		}
		for range 2 {
			count, err := jobs.queue.GiveBackUnheld(ctx)
			if err != nil {
				t.Error(err)
			}
			givenBack = append(givenBack, count)
			jobs.clock.Advance(jobqueue.UnheldRunningGrace)
		}
		return nil
	})
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(givenBack, []int{0, 1}) {
		t.Errorf("the looks gave back %v jobs; want none at the first and the one none held at the second", givenBack)
	}
	if row := jobs.rowOf(t, "held"); row.State != jobqueue.StateDone {
		t.Errorf("the job its runner held is %s; want done, never given back", row.State)
	}
}
