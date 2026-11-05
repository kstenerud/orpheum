// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
)

// jobStart is when the job storage tests' jobs are queued.
var jobStart = time.UnixMilli(1_790_000_000_000)

type jobStorageFixture struct {
	t    *testing.T
	jobs *JobStorage
}

func newJobStorageFixture(t *testing.T) jobStorageFixture {
	t.Helper()
	return jobStorageFixture{t: t, jobs: openCacheAt(t, filepath.Join(t.TempDir(), "cache.db")).Jobs()}
}

// insert queues a job, and returns the open jobs its plan was shown.
func (fixed jobStorageFixture) insert(
	kind jobqueue.Kind, target JobTarget, priority int64, runnableAt time.Time,
) jobqueue.Open[JobTarget] {
	fixed.t.Helper()
	var open jobqueue.Open[JobTarget]
	err := fixed.jobs.Enqueue(fixed.t.Context(), kind, target,
		func(found jobqueue.Open[JobTarget]) (jobqueue.Change, error) {
			open = found
			return jobqueue.Change{
				Action: jobqueue.ActionInsert, ArgsJSON: "{}", Priority: priority, Reason: "test",
				RunnableAt: runnableAt, CreatedAt: jobStart,
			}, nil
		})
	if err != nil {
		fixed.t.Fatal(err)
	}
	return open
}

func (fixed jobStorageFixture) claim(kind jobqueue.Kind, now time.Time) (jobqueue.Row[JobTarget], bool) {
	fixed.t.Helper()
	row, found, err := fixed.jobs.Claim(fixed.t.Context(), jobqueue.Claim{Kind: kind, Now: now})
	if err != nil {
		fixed.t.Fatal(err)
	}
	return row, found
}

// byID is every job, by its id.
func (fixed jobStorageFixture) byID() map[int64]jobqueue.Row[JobTarget] {
	fixed.t.Helper()
	rows, err := fixed.jobs.List(fixed.t.Context(), 0, 100)
	if err != nil {
		fixed.t.Fatal(err)
	}
	found := map[int64]jobqueue.Row[JobTarget]{}
	for _, row := range rows {
		found[row.ID] = row
	}
	return found
}

func TestJobStorage_ClaimsInOrderAndCountsTheAttempt(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	item := JobTarget{ItemID: 7}
	storage.insert("fetch_metadata", item, 0, jobStart)
	storage.insert("fetch_metadata", JobTarget{ItemID: 8}, 5, jobStart)
	storage.insert("make_chunks", JobTarget{ChunkStoreID: 7}, 9, jobStart.Add(time.Minute))
	if first, _ := storage.claim("fetch_metadata", jobStart); first.Target.ItemID != 8 {
		t.Errorf("claimed item %d first, want 8, the higher priority", first.Target.ItemID)
	}
	if _, found := storage.claim("make_chunks", jobStart); found {
		t.Error("claimed a job before its time")
	}
	running, found := storage.claim("fetch_metadata", jobStart)
	if !found || running.Target != item || running.State != jobqueue.StateRunning || running.AttemptCount != 1 {
		t.Fatalf("claimed %+v (%t); want the item's job running with its attempt counted", running, found)
	}
}

func TestJobStorage_ShowsThePlanTheOpenJobsAndMergesOnlyIntoAQueuedOne(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	item := JobTarget{ItemID: 7}
	storage.insert("fetch_metadata", item, 0, jobStart)
	running, _ := storage.claim("fetch_metadata", jobStart)
	if open := storage.insert("fetch_metadata", item, 0, jobStart); len(open.Running) != 1 ||
		open.Running[0].ID != running.ID || open.Queued != nil {
		t.Errorf("the open jobs for the item = %+v, want only the running one", open)
	}
	open := storage.insert("fetch_metadata", item, 0, jobStart)
	if open.Queued == nil || len(open.Running) != 1 {
		t.Fatalf("the open jobs for the item = %+v, want the running one and its follow-up", open)
	}
	err := storage.jobs.Enqueue(t.Context(), "fetch_metadata", item,
		func(jobqueue.Open[JobTarget]) (jobqueue.Change, error) {
			return jobqueue.Change{
				Action: jobqueue.ActionMerge, MergeIntoID: open.Queued.ID, ArgsJSON: `{"isRefresh":true}`, Priority: 20,
				Reason: "a person asked", RunnableAt: jobStart,
			}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if merged := storage.byID()[open.Queued.ID]; merged.Priority != 20 || merged.Reason != "a person asked" ||
		merged.ArgsJSON != `{"isRefresh":true}` || merged.State != jobqueue.StateQueued {
		t.Errorf("merged job = %+v, want its priority, reason and arguments changed and still queued", merged)
	}
	err = storage.jobs.Enqueue(t.Context(), "fetch_metadata", item,
		func(jobqueue.Open[JobTarget]) (jobqueue.Change, error) {
			return jobqueue.Change{Action: jobqueue.ActionMerge, MergeIntoID: running.ID, ArgsJSON: "{}"}, nil
		})
	if err == nil {
		t.Error("merged a request into a running job; want it refused, as only a queued job merges")
	}
}

func TestJobStorage_RecordsAWaitWithItsFlagsAndNeverUndoesACancel(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	storage.insert("fetch_metadata", JobTarget{ItemID: 7}, 0, jobStart)
	storage.insert("read_through", JobTarget{FileID: 3}, 0, jobStart)
	running, _ := storage.claim("fetch_metadata", jobStart)
	snoozeUntil := jobStart.Add(time.Hour)
	if err := storage.jobs.Record(t.Context(), jobqueue.Outcome{
		ID: running.ID, State: jobqueue.StateQueued, At: snoozeUntil, AttemptCount: 0, IsSnoozed: true,
		ToolFailed: true, LastFailure: "stalled",
	}); err != nil {
		t.Fatal(err)
	}
	if snoozed := storage.byID()[running.ID]; !snoozed.IsSnoozed || !snoozed.LastAttemptToolFailed ||
		snoozed.AttemptCount != 0 || !snoozed.RunnableAt.Equal(snoozeUntil) || snoozed.LastFailure != "stalled" {
		t.Errorf("waiting job = %+v, want queued until its time, snoozed, its tool failed, its attempt given back",
			snoozed)
	}
	next, _ := storage.claim("read_through", jobStart)
	if wasOpen, err := storage.jobs.Cancel(t.Context(), next.ID, jobStart); err != nil || !wasOpen {
		t.Fatalf("Cancel = %t, %v; want it open", wasOpen, err)
	}
	outcome := jobqueue.Outcome{ID: next.ID, State: jobqueue.StateDone, At: jobStart, AttemptCount: 1}
	if err := storage.jobs.Record(t.Context(), outcome); err != nil {
		t.Fatal(err)
	}
	if cancelled := storage.byID()[next.ID]; cancelled.State != jobqueue.StateCancelled {
		t.Errorf("a cancelled job's outcome made it %s, want it cancelled still", cancelled.State)
	}
}

func TestJobStorage_RequeuesWithTheAttemptCountedAndKnowsATargetByItsColumn(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	store := JobTarget{ChunkStoreID: 7}
	storage.insert("make_chunks", store, 0, jobStart)
	run, _ := storage.claim("make_chunks", jobStart)
	if count, err := storage.jobs.RequeueRunning(t.Context()); err != nil || count != 1 {
		t.Fatalf("RequeueRunning = %d, %v; want the one running", count, err)
	}
	if requeued := storage.byID()[run.ID]; requeued.State != jobqueue.StateQueued || requeued.AttemptCount != 1 {
		t.Errorf("requeued job = %+v, want queued with its attempt counted", requeued)
	}
	storage.insert("read_through", JobTarget{FileID: 8}, 0, jobStart)
	measure, _ := storage.claim("read_through", jobStart)
	done := jobqueue.Outcome{ID: measure.ID, State: jobqueue.StateDone, At: jobStart, AttemptCount: 1}
	if err := storage.jobs.Record(t.Context(), done); err != nil {
		t.Fatal(err)
	}
	open, err := storage.jobs.ListOpen(t.Context())
	if err != nil || len(open) != 1 || open[0].ID != run.ID || open[0].Target != store {
		t.Errorf("ListOpen = %+v, %v; want the run alone, its target store 7 by its column", open, err)
	}
}

func TestJobStorage_AClaimPassesOverAParkedDestinationAndAWaitKeepsWhereItWasParked(t *testing.T) {
	t.Parallel()
	fixed := newJobStorageFixture(t)
	ctx := t.Context()
	insert := func(itemID, priority int64, destination jobqueue.Destination) {
		t.Helper()
		err := fixed.jobs.Enqueue(ctx, "fetch_metadata", JobTarget{ItemID: itemID},
			func(jobqueue.Open[JobTarget]) (jobqueue.Change, error) {
				return jobqueue.Change{
					Action: jobqueue.ActionInsert, ArgsJSON: "{}", Priority: priority, RunnableAt: jobStart,
					Destination: destination, CreatedAt: jobStart,
				}, nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(1, 10, "tmdb-api")
	insert(2, 5, "")
	insert(3, 1, "node:2")
	claim := func(unclaimable ...jobqueue.Destination) (jobqueue.Row[JobTarget], bool) {
		t.Helper()
		row, found, err := fixed.jobs.Claim(ctx, jobqueue.Claim{
			Kind: "fetch_metadata", Now: jobStart, Unclaimable: unclaimable,
		})
		if err != nil {
			t.Fatal(err)
		}
		return row, found
	}
	first, found := claim("tmdb-api", "node:9")
	if !found || first.Target.ItemID != 2 {
		t.Fatalf("with tmdb-api parked, the claim took %+v (found %v); want item 2's job, which names none",
			first, found)
	}
	err := fixed.jobs.Record(ctx, jobqueue.Outcome{
		ID: first.ID, State: jobqueue.StateQueued, At: jobStart, AttemptCount: 0, Destination: "node:9",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, found := claim("tmdb-api", "node:9")
	if !found || second.Target.ItemID != 3 || second.Destination != "node:2" {
		t.Fatalf("the next claim took %+v (found %v); want item 3's, on node:2, since item 2's now waits on node:9",
			second, found)
	}
	if _, found := claim("tmdb-api", "node:9"); found {
		t.Error("a claim took a job whose destination is parked")
	}
	if third, found := claim(); !found || third.Target.ItemID != 1 || third.Destination != "tmdb-api" {
		t.Errorf("with nothing parked, the claim took %+v (found %v); want item 1's, the highest", third, found)
	}
}

// assign queues a job of kind for target, claims it and records it given to a node until until,
// and returns its id.
func (fixed jobStorageFixture) assign(kind jobqueue.Kind, target JobTarget, until time.Time) int64 {
	fixed.t.Helper()
	fixed.insert(kind, target, 0, jobStart)
	row, _ := fixed.claim(kind, jobStart)
	assigned := jobqueue.Outcome{ID: row.ID, State: jobqueue.StateAssigned, At: until, AttemptCount: 1}
	if err := fixed.jobs.Record(fixed.t.Context(), assigned); err != nil {
		fixed.t.Fatal(err)
	}
	return row.ID
}

// An assigned job is kept on cache.db with when its time runs out: open, running for a request,
// and left assigned by a restart, since its node still has it (D234 §7).
func TestJobStorage_KeepsAnAssignedJobOpenAndRunningThroughARestart(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	until := jobStart.Add(time.Hour)
	run := storage.assign("make_chunks", JobTarget{ChunkStoreID: 7}, until)
	if row := storage.byID()[run]; row.State != jobqueue.StateAssigned || row.AssignedUntil == nil ||
		!row.AssignedUntil.Equal(until) || row.AttemptCount != 1 {
		t.Fatalf("an assigned run = %+v, want assigned until %v with its attempt", row, until)
	}
	if open := storage.insert("make_chunks", JobTarget{ChunkStoreID: 7}, 0, jobStart); len(open.Running) != 1 ||
		open.Running[0].ID != run {
		t.Errorf("a request's plan was shown %+v, want the assigned run as running", open)
	}
	if count, err := storage.jobs.RequeueRunning(t.Context()); err != nil || count != 0 {
		t.Errorf("RequeueRunning = %d, %v; want the assigned job left with its node", count, err)
	}
	if open, err := storage.jobs.ListOpen(t.Context()); err != nil || len(open) != 2 {
		t.Errorf("ListOpen = %d jobs, %v; want the assigned run and its follow-up", len(open), err)
	}
}

// A cancel ends an assigned job, and no outcome takes it after.
func TestJobStorage_ACancelEndsAnAssignedJobAndNoOutcomeTakesIt(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	run := storage.assign("make_chunks", JobTarget{ChunkStoreID: 7}, jobStart.Add(time.Hour))
	if cancelled, err := storage.jobs.Cancel(t.Context(), run, jobStart); err != nil || !cancelled {
		t.Fatalf("cancelling the assigned run: %v, %v", cancelled, err)
	}
	_, found, takeErr := storage.jobs.TakeAssigned(t.Context(), run)
	if row := storage.byID()[run]; takeErr != nil || found || row.State != jobqueue.StateCancelled ||
		row.AssignedUntil != nil {
		t.Errorf("an outcome for the cancelled run: taken %v (%v), and it is %+v; want it left cancelled",
			found, takeErr, row)
	}
}

// An assigned job is taken back once: by its kind once its time ran out, or by its outcome, and
// whichever comes second finds nothing.
func TestJobStorage_TakesAnAssignedJobBackOnceByItsKindWhenItsTimeRanOut(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	until := jobStart.Add(time.Hour)
	run := storage.assign("make_chunks", JobTarget{ChunkStoreID: 7}, until)
	storage.assign("read_through", JobTarget{FileID: 8}, jobStart)
	takeOverdue := func(kind jobqueue.Kind, now time.Time) (jobqueue.Row[JobTarget], bool) {
		t.Helper()
		row, found, err := storage.jobs.TakeOverdueAssigned(t.Context(), kind, now)
		if err != nil {
			t.Fatal(err)
		}
		return row, found
	}
	if _, found := takeOverdue("make_chunks", until.Add(-time.Millisecond)); found {
		t.Error("a run was taken as overdue before its time ran out")
	}
	overdue, found := takeOverdue("make_chunks", until)
	if !found || overdue.ID != run || overdue.State != jobqueue.StateRunning || overdue.AssignedUntil == nil ||
		!overdue.AssignedUntil.Equal(until) {
		t.Fatalf("the overdue run = %+v (%v); want it back to running, its time kept", overdue, found)
	}
	if _, found := takeOverdue("make_chunks", until.Add(24*time.Hour)); found {
		t.Error("an overdue run was taken twice, or the measurement, of another kind, taken as a run")
	}
	if _, found, err := storage.jobs.TakeAssigned(t.Context(), run); err != nil || found {
		t.Errorf("an outcome after its time ran out took the run again: %v, %v", found, err)
	}
}

// A job kept wanted keeps the later of the times it was given, by its kind and target; past it, it
// falls to the priority given and forgets its time, and is returned by its id and state; a job never
// said wanted is left (D234 §3).
func TestJobStorage_KeepsAJobWantedAndDemotesItOncePast(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	film, show := JobTarget{ChunkStoreID: 7}, JobTarget{ChunkStoreID: 8}
	storage.insert("make_chunks", film, 1000, jobStart)
	storage.insert("make_chunks", show, 1000, jobStart)
	storage.insert("make_chunks", JobTarget{ChunkStoreID: 9}, 1000, jobStart)
	for _, keep := range []struct {
		target JobTarget
		until  time.Time
	}{{film, jobStart.Add(10 * time.Minute)}, {show, jobStart.Add(time.Hour)}, {show, jobStart.Add(time.Minute)}} {
		if err := storage.jobs.KeepWanted(t.Context(), "make_chunks", keep.target, keep.until); err != nil {
			t.Fatal(err)
		}
	}
	demoted, err := storage.jobs.DemoteUnwanted(t.Context(), jobStart.Add(20*time.Minute), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(demoted) != 1 || demoted[0].State != jobqueue.StateQueued {
		t.Fatalf("demoted %+v, want the film's job alone, queued", demoted)
	}
	jobs := storage.byID()
	filmJob, showJob := jobs[demoted[0].ID], jobs[demoted[0].ID+1]
	if filmJob.Target != film || filmJob.Priority != 0 || filmJob.WantedUntil != nil ||
		showJob.Target != show || showJob.Priority != 1000 || showJob.WantedUntil == nil ||
		!showJob.WantedUntil.Equal(jobStart.Add(time.Hour)) {
		t.Errorf("the film is %+v and the show %+v; want the film fallen to 0, the show wanted for its hour",
			filmJob, showJob)
	}
}

// A job released, no person waiting for it any more, is demoted at once, though the time it was kept
// wanted had not passed; another target's is left (D258 §2).
func TestJobStorage_AReleasedJobIsDemotedAtOnce(t *testing.T) {
	t.Parallel()
	storage := newJobStorageFixture(t)
	film, show := JobTarget{ChunkStoreID: 7}, JobTarget{ChunkStoreID: 8}
	storage.insert("make_chunks", film, 1000, jobStart)
	storage.insert("make_chunks", show, 1000, jobStart)
	for _, kept := range []JobTarget{film, show} {
		if err := storage.jobs.KeepWanted(t.Context(), "make_chunks", kept, jobStart.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.jobs.ReleaseWanted(t.Context(), "make_chunks", film, jobStart.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	demoted, err := storage.jobs.DemoteUnwanted(t.Context(), jobStart.Add(time.Minute), 0)
	if err != nil || len(demoted) != 1 || storage.byID()[demoted[0].ID].Target != film {
		t.Errorf("demoted %+v (%v); want the film's job alone", demoted, err)
	}
}
