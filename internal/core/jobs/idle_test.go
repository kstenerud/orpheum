// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// A runner with nothing due takes cache.db's writer no time: eleven of them tick every second, and
// each tick took it twice, to fail an overdue assignment and to claim, about 22 empty write
// transactions a second on an idle core (the audit of 2026-10-25, C6, G4). Once a job is due, the
// tick claims and runs it.
func TestRunDue_WithNothingDueTakesNoWriter(t *testing.T) {
	t.Parallel()
	fixed := newFixture(t)
	ran := 0
	runner := fixed.queue.NewRunner(KindFetchMetadata, func(context.Context, Job) error { ran++; return nil })
	request := Request{
		Kind: KindFetchMetadata, Target: Target{ItemID: 1}, Reason: "test",
		RunnableAt: fixed.clock.Now().Add(time.Hour),
	}
	if _, err := fixed.queue.Enqueue(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	ctx, tally := querytally.New(t.Context())
	if err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if tally.Writes() != 0 || ran != 0 {
		t.Errorf("a tick with only a job not due ran %d and took the writer %d times (%s); want neither",
			ran, tally.Writes(), tally)
	}
	fixed.clock.Advance(time.Hour)
	fixed.runDue(t, runner)
	if ran != 1 {
		t.Errorf("a tick once the job was due ran %d jobs; want it run", ran)
	}
}
