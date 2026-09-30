// SPDX-License-Identifier: GPL-3.0-or-later

package jobs

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
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
