// SPDX-License-Identifier: GPL-3.0-or-later

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
	rows, err := fixed.jobs.List(fixed.t.Context(), 100)
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
	storage.insert("prepare", JobTarget{RenditionID: 7}, 9, jobStart.Add(time.Minute))
	if first, _ := storage.claim("fetch_metadata", jobStart); first.Target.ItemID != 8 {
		t.Errorf("claimed item %d first, want 8, the higher priority", first.Target.ItemID)
	}
	if _, found := storage.claim("prepare", jobStart); found {
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
	if open := storage.insert("fetch_metadata", item, 0, jobStart); open.Running == nil ||
		open.Running.ID != running.ID || open.Queued != nil {
		t.Errorf("the open jobs for the item = %+v, want only the running one", open)
	}
	open := storage.insert("fetch_metadata", item, 0, jobStart)
	if open.Queued == nil || open.Running == nil {
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
	storage.insert("measure_loudness", JobTarget{FileID: 3}, 0, jobStart)
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
	next, _ := storage.claim("measure_loudness", jobStart)
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
	rendition := JobTarget{RenditionID: 7}
	storage.insert("prepare", rendition, 0, jobStart)
	prepare, _ := storage.claim("prepare", jobStart)
	if count, err := storage.jobs.RequeueRunning(t.Context()); err != nil || count != 1 {
		t.Fatalf("RequeueRunning = %d, %v; want the one running", count, err)
	}
	if requeued := storage.byID()[prepare.ID]; requeued.State != jobqueue.StateQueued || requeued.AttemptCount != 1 {
		t.Errorf("requeued job = %+v, want queued with its attempt counted", requeued)
	}
	if ever, err := storage.jobs.EverQueued(t.Context(), "prepare", rendition); err != nil || !ever {
		t.Errorf("EverQueued(prepare, rendition 7) = %t, %v; want true", ever, err)
	}
	if ever, err := storage.jobs.EverQueued(t.Context(), "prepare", JobTarget{ItemID: 7}); err != nil || ever {
		t.Errorf("EverQueued(prepare, item 7) = %t, %v; want false: a target is its column and its id", ever, err)
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
