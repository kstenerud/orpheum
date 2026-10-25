// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// holdEveryReader opens as many read transactions at once as the pool allows, then ends them all:
// the burst after which a pool with too few idle places closes the readers past them.
func holdEveryReader(t *testing.T, database *Database) {
	t.Helper()
	count := database.readers.Stats().MaxOpenConnections
	var entered, held sync.WaitGroup
	entered.Add(count)
	held.Add(1)
	errs := make(chan error, count)
	for range count {
		go func() {
			errs <- readTransaction(t.Context(), database.readers, func(transaction *sql.Tx) error {
				var one int
				err := transaction.QueryRowContext(t.Context(), "SELECT 1").Scan(&one)
				entered.Done()
				held.Wait()
				return err //nolint:wrapcheck // the test reports it
			})
		}()
	}
	entered.Wait()
	held.Done()
	for range count {
		if err := testutil.Receive(t, errs, "a read's end"); err != nil {
			t.Fatalf("a read: %v", err)
		}
	}
}

// Every reader a pool opens stays open once its read ends, whether or not the planner's statistics
// were just loaded (which closes the idle ones to reload them): a connection closed takes its parsed
// schema and its 16 MiB page cache with it, and the next one parses and reads them again (the audit
// of 2026-10-25, S1). Unless told otherwise, database/sql keeps only 2 idle.
func TestOpen_KeepsEveryReaderItOpens(t *testing.T) {
	t.Parallel()
	migrated := openMigrated(t) // its statistics loaded just now
	reopened, err := Open(t.Context(), filepath.Join(t.TempDir(), "reopened.db"), testOpenOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() }) //nolint:errcheck // a test's scratch database
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "cache.db"))
	for name, database := range map[string]*Database{
		"persistent.db after its statistics loaded": migrated, "a database with no statistics loaded": reopened,
		"cache.db": cache.database,
	} {
		closedBefore := database.readers.Stats().MaxIdleClosed
		holdEveryReader(t, database)
		stats := database.readers.Stats()
		if closed := stats.MaxIdleClosed - closedBefore; closed != 0 || stats.Idle != stats.MaxOpenConnections {
			t.Errorf("%s: after %d reads at once, %d readers closed and %d stayed; want none closed",
				name, stats.MaxOpenConnections, closed, stats.Idle)
		}
	}
}
