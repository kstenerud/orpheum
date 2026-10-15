// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A write that holds the one writer past SlowWriteAfter is reported with the file, the function
// outside the store that wrote, and the time; a quick one is not.
func TestWrite_ReportsAWriteThatHeldTheWriterTooLong(t *testing.T) {
	t.Parallel()
	now := clocktest.NewFake(time.Date(2026, 10, 15, 18, 40, 0, 0, time.UTC))
	var reported []store.SlowWrite
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), store.OpenOptions{
		MinimumReaders: 1, Clock: now, OnSlowWrite: func(write store.SlowWrite) { reported = append(reported, write) },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() }) //nolint:errcheck // the test is over
	for _, took := range []time.Duration{100 * time.Millisecond, 3 * time.Second} {
		err = database.Write(t.Context(), func(*sqlcgen.Queries) error {
			now.Advance(took)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(reported) != 1 || reported[0].Held != 3*time.Second || reported[0].File != "persistent.db" ||
		!strings.HasSuffix(reported[0].Caller, "TestWrite_ReportsAWriteThatHeldTheWriterTooLong") {
		t.Errorf("reported %+v; want the one write that held the writer 3 s, by this test", reported)
	}
}

// A write that waited long is reported with the write that held the writer when it asked.
func TestWrite_ASlowWaitNamesTheWriteItWaitedBehind(t *testing.T) {
	t.Parallel()
	now := clocktest.NewFake(time.Date(2026, 10, 15, 23, 0, 0, 0, time.UTC))
	var (
		mutex    sync.Mutex
		reported []store.SlowWrite
	)
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), store.OpenOptions{
		MinimumReaders: 1, Clock: now, OnSlowWrite: func(write store.SlowWrite) {
			mutex.Lock()
			reported = append(reported, write)
			mutex.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() }) //nolint:errcheck // the test is over
	holding, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- holdTheWriter(t, database, now, holding) }()
	testutil.Receive(t, holding, "the first write to hold the writer")
	if err := waitBehindTheHolder(t.Context(), database); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Receive(t, done, "the holding write to finish"); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	var waiter *store.SlowWrite
	for i := range reported {
		if strings.HasSuffix(reported[i].Caller, "waitBehindTheHolder") {
			waiter = &reported[i]
		}
	}
	if waiter == nil || waiter.Waited != 3*time.Second || !strings.HasSuffix(waiter.Behind, "holdTheWriter") {
		t.Errorf("reported %+v; want waitBehindTheHolder's write, 3 s, behind holdTheWriter's", reported)
	}
}

// holdTheWriter holds the writer until another write waits for it, then for 3 s more.
func holdTheWriter(t *testing.T, database *store.Database, now *clocktest.Fake, holding chan<- struct{}) error {
	t.Helper()
	err := database.Write(t.Context(), func(*sqlcgen.Queries) error {
		close(holding)
		for store.WritesWaiting(database) == 0 {
			time.Sleep(time.Millisecond)
		}
		now.Advance(3 * time.Second)
		return nil
	})
	if err != nil {
		return fmt.Errorf("hold the writer: %w", err)
	}
	return nil
}

// waitBehindTheHolder is a write that asks for the writer while another holds it.
func waitBehindTheHolder(ctx context.Context, database *store.Database) error {
	if err := database.Write(ctx, func(*sqlcgen.Queries) error { return nil }); err != nil {
		return fmt.Errorf("wait behind the holder: %w", err)
	}
	return nil
}
