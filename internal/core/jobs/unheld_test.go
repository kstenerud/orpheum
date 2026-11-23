// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A job cache.db marks running that no runner holds, as a write that failed leaves one, is given
// back to the queue by the queue's own sweep, two looks a grace apart: left so, every request it
// covered was dropped until a restart (D540).
func TestSweepUnheldRunning_GivesBackAJobNoRunnerHolds(t *testing.T) {
	t.Parallel()
	storage := storetest.OpenCache(t).Jobs()
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	queue := newQueue(storage, QueueParts{
		Clock: fakeClock, Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
		Random: jobqueuetest.Unjittered(), Logger: slog.New(slog.DiscardHandler),
	})
	request := Request{Kind: KindFetchMetadata, Target: Target{ItemID: 1}, Reason: "test"}
	if _, err := queue.Enqueue(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	// Claimed, as by a runner whose outcome was then lost: nothing here holds it.
	row, isClaimed, err := storage.Claim(t.Context(), jobqueue.Claim{Kind: KindFetchMetadata, Now: fakeClock.Now()})
	if err != nil || !isClaimed {
		t.Fatalf("claim: %v, %t", err, isClaimed)
	}
	stateOf := func() jobqueue.State {
		t.Helper()
		rows, err := storage.List(t.Context(), 0, 10)
		if err != nil || len(rows) != 1 {
			t.Fatalf("list: %v, %d rows", err, len(rows))
		}
		return rows[0].State
	}
	queue.RunSweeps(t.Context(), SweepUnheldRunning)
	if state := stateOf(); state != jobqueue.StateRunning {
		t.Fatalf("at the first look, job %d is %s; want it left running, as one just claimed is", row.ID, state)
	}
	fakeClock.Advance(jobqueue.UnheldRunningGrace)
	queue.RunSweeps(t.Context(), SweepUnheldRunning)
	if state := stateOf(); state != jobqueue.StateQueued {
		t.Errorf("a grace after a look found job %d running with no runner holding it, it is %s; want queued",
			row.ID, state)
	}
}
