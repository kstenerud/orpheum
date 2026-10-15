// SPDX-License-Identifier: AGPL-3.0-or-later

package store_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
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
