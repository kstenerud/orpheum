// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A demotion failing at every 5 s tick, cache.db gone, says so at its start and then once a minute
// with how often it failed: it logged every tick (the audit of 2026-10-25, C12).
func TestDemoteUnwanted_SaysALastingFailureOnceAMinute(t *testing.T) {
	t.Parallel()
	cache, err := store.OpenCache(t.Context(), filepath.Join(t.TempDir(), "cache.db"), storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.Close(); err != nil { // every demotion fails from here on
		t.Fatal(err)
	}
	now := clocktest.NewFake(time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC))
	logged := &testutil.Log{}
	logger := slog.New(slog.NewTextHandler(logged, nil))
	queue := jobs.NewQueue(jobs.QueueParts{
		Cache: cache, Clock: now, Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
		Random: jobqueuetest.Unjittered(), Logger: logger,
	})
	tick, done := make(chan time.Time), make(chan struct{})
	ctx, stop := context.WithCancel(t.Context())
	go func() {
		defer close(done)
		demoteUnwanted(ctx, queue, nil, tick, logger)
	}()
	for second := 0; second <= 150; second += 5 { // 31 ticks; each sent once the one before is done
		tick <- now.Now().Add(time.Duration(second) * time.Second)
	}
	stop()
	testutil.Receive(t, done, "the loop's end")
	if said := len(testutil.LinesWith(logged.String(), "demoteFailed")); said != 3 ||
		!strings.Contains(logged.String(), "failureCount=25") {
		t.Errorf("31 failed demotions 5 s apart logged %d times:\n%s\nwant 3, at 0, 60 and 120 s, the last "+
			"counting 25", said, logged.String())
	}
}
