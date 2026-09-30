// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fillingDisk is cache.db's job storage on a disk that fills and empties: while it is full, every
// write answers what SQLite answers a write to a full disk.
type fillingDisk struct {
	jobqueue.Storage[Target]
	isFull  atomic.Bool
	fullErr error
}

func (disk *fillingDisk) Claim(ctx context.Context, claim jobqueue.Claim) (jobqueue.Row[Target], bool, error) {
	if disk.isFull.Load() {
		return jobqueue.Row[Target]{}, false, fmt.Errorf("claim: %w", disk.fullErr)
	}
	return disk.Storage.Claim(ctx, claim)
}

func (disk *fillingDisk) Record(ctx context.Context, outcome jobqueue.Outcome) error {
	if disk.isFull.Load() {
		return fmt.Errorf("record: %w", disk.fullErr)
	}
	return disk.Storage.Record(ctx, outcome)
}

// S77: the core's own disk fills: cache.db's and library.db's writes fail. Nothing that cost
// something is lost, the server says why, once, rather than every runner every second, no job is
// left running for want of a write, and work resumes on its own once there is room (D231 §5, §6).
func TestScenario_S77_AFullCoreDiskParksTheQueueSaysWhyOnceAndResumesWhenThereIsRoom(t *testing.T) {
	t.Parallel()
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, config, settings.JobAttemptLimit, "3")
	disk := &fillingDisk{Storage: storetest.OpenCache(t).Jobs(), fullErr: storetest.DiskFullError(t)}
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	logs := &bytes.Buffer{}
	queue := newQueue(disk, QueueParts{
		Clock: fakeClock, Settings: config, Random: jobqueuetest.Unjittered(),
		Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})
	for itemID := range int64(3) {
		request := Request{Kind: KindFetchMetadata, Target: Target{ItemID: itemID + 1}, Reason: "test"}
		if _, err := queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	couldNotKeep := false
	runner := queue.NewRunner(KindFetchMetadata, func(_ context.Context, job Job) error {
		switch job.ItemID {
		case 1:
			disk.isFull.Store(true) // the disk fills as the job ends: its outcome cannot be written
		case 2:
			if !couldNotKeep {
				couldNotKeep = true // library.db cannot keep what it fetched
				return fmt.Errorf("keep item 2's record: %w", disk.fullErr)
			}
		}
		return nil
	})
	for range 10 {
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatalf("a runner on a full disk: %v; want it waiting, quietly", err)
		}
		fakeClock.Advance(time.Second)
	}
	checkParkedForAFullDiskAndSaidOnce(t, queue, logs.String())
	disk.isFull.Store(false)
	for range 3 {
		if state, isParked := queue.DestinationState(DestinationCoreStorage); isParked {
			fakeClock.Set(state.NextProbeAt)
		}
		if err := runner.RunDue(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	checkEveryJobDoneAtItsFirstAttempt(t, queue)
}

// checkParkedForAFullDiskAndSaidOnce checks that the core's storage is parked for a full disk, and
// that the log said so once, with no error from any runner.
func checkParkedForAFullDiskAndSaidOnce(t *testing.T, queue *Queue, logged string) {
	t.Helper()
	parked := queue.ParkedDestinations()
	if len(parked) != 1 || parked[0].Destination != DestinationCoreStorage ||
		!strings.Contains(parked[0].Reason, "database or disk is full") {
		t.Errorf("parked %+v; want the core's storage, saying it is full", parked)
	}
	said, isError := strings.Count(logged, "a destination is down"), strings.Contains(logged, "level=ERROR")
	if said != 1 || isError {
		t.Errorf("the log said the disk was full %d times, with errors %v; want once, and no error per runner per "+
			"second:\n%s", said, isError, logged)
	}
}

// checkEveryJobDoneAtItsFirstAttempt checks that no job is left running or failed, and that none
// spent an attempt on the full disk, and nothing is parked.
func checkEveryJobDoneAtItsFirstAttempt(t *testing.T, queue *Queue) {
	t.Helper()
	views, err := queue.List(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.State != "done" || view.AttemptCount != 1 {
			t.Errorf("item %d's job once there was room: %s after %d attempts; want done at its first attempt, "+
				"none left running", view.ItemID, view.State, view.AttemptCount)
		}
	}
	if len(queue.ParkedDestinations()) != 0 {
		t.Errorf("still parked once there was room: %+v", queue.ParkedDestinations())
	}
}

// The items of S71's four requests, one for each state its job is in when asked for again.
const (
	queuedItem  int64 = 1
	waitingItem int64 = 2
	parkedItem  int64 = 3
	runningItem int64 = 4
)

// repeatWorld is S71's queue and its handler: the waiting item fails its first attempt, the
// parked item's TMDB is down until it is not, and the running item's first run holds until
// released.
type repeatWorld struct {
	t          *testing.T
	fixed      fixture
	ran        []int64
	isTMDBDown bool
	running    chan struct{}
	release    chan struct{}
}

func (world *repeatWorld) request(itemID, priority int64, argsJSON string) {
	world.t.Helper()
	asked := Request{
		Kind: KindFetchMetadata, Target: Target{ItemID: itemID}, Priority: priority, ArgsJSON: argsJSON,
		Reason: "test",
	}
	if itemID == parkedItem {
		asked.Destination = DestinationTMDBAPI
	}
	if _, err := world.fixed.queue.Enqueue(world.t.Context(), asked); err != nil {
		world.t.Fatal(err)
	}
}

func (world *repeatWorld) handle(_ context.Context, job Job) error {
	world.ran = append(world.ran, job.ItemID)
	switch {
	case job.ItemID == waitingItem && job.AttemptNumber == 1:
		return errors.New("a 500: the item's own failure")
	case job.ItemID == parkedItem && world.isTMDBDown:
		return Park(Parking{Destination: DestinationTMDBAPI, Reason: "unreachable"}, errors.New("refused"))
	case job.ItemID == runningItem && job.ArgsJSON == "{}":
		close(world.running)
		<-world.release
	}
	return nil
}

// queuedCount is how many queued jobs an item has.
func (world *repeatWorld) queuedCount(itemID int64) int {
	world.t.Helper()
	rows, err := world.fixed.cache.Read().ListJobs(world.t.Context(), 100)
	if err != nil {
		world.t.Fatal(err)
	}
	count := 0
	for _, row := range rows {
		if row.ItemID != nil && *row.ItemID == itemID && row.State == "queued" {
			count++
		}
	}
	return count
}

// S71: the same target is asked for again while its job is queued, waiting to retry, parked and
// running. There is one job, which runs as soon as the most urgent request needs and does what
// every request asked (D231 §6, §8).
func TestScenario_S71_ARepeatRequestRaisesWakesAndMerges(t *testing.T) {
	t.Parallel()
	world := &repeatWorld{
		t: t, fixed: newFixture(t), isTMDBDown: true, running: make(chan struct{}), release: make(chan struct{}),
	}
	runner := world.fixed.queue.NewRunner(KindFetchMetadata, world.handle)

	// While queued: the higher priority wins.
	world.request(queuedItem, -10, "")
	world.request(queuedItem, PriorityPersonsRequest, "")
	if job := world.fixed.job(t, queuedItem); job.Priority != PriorityPersonsRequest {
		t.Errorf("queued: priority %d, want the person's %d", job.Priority, PriorityPersonsRequest)
	}
	world.fixed.runDue(t, runner)

	// While waiting on its back-off: woken, so it runs now, not after the first retry delay.
	world.request(waitingItem, 0, "")
	world.fixed.runDue(t, runner)
	if job := world.fixed.job(t, waitingItem); job.State != "queued" || job.AttemptCount != 1 {
		t.Fatalf("after its own failure: %+v, want it waiting to retry", job)
	}
	world.request(waitingItem, PriorityPersonsRequest, "")
	world.fixed.runDue(t, runner)
	if job := world.fixed.job(t, waitingItem); job.State != stateDone {
		t.Errorf("waiting to retry: %+v after a repeat request; want it woken and done at once", job)
	}

	// While parked: a person's request looks at the destination at once, at their priority.
	world.request(parkedItem, 0, "")
	world.fixed.runDue(t, runner)
	world.isTMDBDown = false
	world.request(parkedItem, PriorityPersonsRequest, "")
	world.fixed.runDue(t, runner)
	if job := world.fixed.job(t, parkedItem); job.State != stateDone || job.Priority != PriorityPersonsRequest ||
		job.AttemptCount != 1 {
		t.Errorf("parked: %+v after a person's request; want it tried at once, at their priority, with the "+
			"parking costing no attempt", job)
	}

	// While running: a request it covers is dropped; one it does not is queued once to follow.
	world.request(runningItem, 0, "")
	done := make(chan error)
	go func() { done <- runner.RunDue(t.Context()) }()
	<-world.running
	world.request(runningItem, 0, "") // covered: the same arguments
	world.request(runningItem, 0, `{"shouldRetryImages":true}`)
	world.request(runningItem, PriorityPersonsRequest, `{"shouldRetryImages":true}`)
	followUps := world.queuedCount(runningItem)
	close(world.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if followUps != 1 {
		t.Errorf("while running: %d follow-ups queued, want one for what the run does not cover", followUps)
	}
	wantRan := []int64{queuedItem, waitingItem, waitingItem, parkedItem, parkedItem, runningItem, runningItem}
	if !slices.Equal(world.ran, wantRan) {
		t.Errorf("ran %v, want %v: each job once per request it had to meet", world.ran, wantRan)
	}
}
