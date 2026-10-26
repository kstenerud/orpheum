// SPDX-License-Identifier: AGPL-3.0-or-later

package jobqueue_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// brokenClaims is storage whose first brokenCount claims fail, with an error the queue does not take
// for the storage being down. Each claim moves the clock on a second, so a tick's run, which claims
// once, happens a second after the last.
type brokenClaims struct {
	*jobqueuetest.Memory[target]
	brokenCount, claimCount atomic.Int64
	clock                   *clocktest.Fake
}

func (storage *brokenClaims) Claim(
	ctx context.Context, claim jobqueue.Claim,
) (jobqueue.Row[target], bool, error) {
	storage.clock.Advance(time.Second)
	if storage.claimCount.Add(1) <= storage.brokenCount.Load() {
		return jobqueue.Row[target]{}, false, errors.New("the claim broke")
	}
	return storage.Memory.Claim(ctx, claim)
}

// A runner that cannot run, ticking each second, says so once, then once a minute with how often it
// failed, then once when it runs again: it logged every tick (the audit of 2026-10-25, C12).
func TestRun_SaysALastingFailureOnceAMinuteAndItsEnd(t *testing.T) {
	t.Parallel()
	logged := &testutil.Log{}
	now := clocktest.NewFake(time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC))
	storage := &brokenClaims{Memory: &jobqueuetest.Memory[target]{}, clock: now}
	storage.brokenCount.Store(61)
	queue := jobqueue.New[target](storage, now, jobqueue.Config{
		Random: jobqueuetest.Unjittered(), Policy: fixedPolicy{attemptLimit: attemptLimit},
		Logger: slog.New(slog.NewTextHandler(logged, nil)),
	})
	ctx, cancel := context.WithCancel(t.Context())
	tick, ran := make(chan time.Time), make(chan error, 1)
	go func() { ran <- queue.NewRunner(kindFetch, nil).Run(ctx, tick) }()
	for range 70 {
		tick <- time.Time{}
	}
	tick <- time.Time{} // the 70th tick's run ends before this one is taken
	cancel()
	if err := testutil.Receive(t, ran, "the runner to stop"); err != nil {
		t.Fatal(err)
	}
	text := logged.String()
	if strings.Count(text, "could not run") != 2 || strings.Count(text, "failureCount=61") != 2 ||
		strings.Count(text, "runs again") != 1 {
		t.Errorf("61 failed ticks, then good ones, logged:\n%s\nwant the failure twice, at its start and a minute on "+
			"with its count, then its end once", text)
	}
}
