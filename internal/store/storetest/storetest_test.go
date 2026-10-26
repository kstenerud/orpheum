// SPDX-License-Identifier: AGPL-3.0-or-later

package storetest_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// ExecRaw's connection waits for a lock as the store's own do, 15 s: with none, a lock held for an
// instant just after a server stopped failed a test about one run in fifty (2026-10-06). The
// statement fails, and so ExecRaw, unless the connection waits that long: abs of the smallest
// integer overflows.
func TestExecRaw_WaitsForALockAsTheStoresOwnConnectionsDo(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "raw.db")
	storetest.ExecRaw(t, path,
		"SELECT CASE WHEN (SELECT timeout FROM pragma_busy_timeout) >= 15000 THEN 1 "+
			"ELSE abs(-9223372036854775808) END")
}

// A test's reads of one file share one connection, so a read pays the schema's parse once rather
// than on every call (audit 2026-10-25, G1). The connection's cache size, set by one read, is the
// next read's only if both run on the same connection.
func TestQueryRaw_ReadsAFileOnOneConnectionForTheWholeTest(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "raw.db")
	storetest.ExecRaw(t, path, "CREATE TABLE row (id INTEGER)")
	storetest.QueryRaw(t, path, "PRAGMA cache_size = 1234")
	if got := storetest.QueryRaw(t, path, "PRAGMA cache_size"); !slices.Equal(got, []string{"1234"}) {
		t.Fatalf("a second read's connection has cache size %v, want the first read's 1234", got)
	}
}

// A test's ExecRaw calls on one file share one connection too (finding 5, 2026-10-26): a TEMP
// table lives only on the connection that created it, so a later call reaches it only if it runs
// on the same one. The file's first call always opens its own connection, as QueryRaw's does, so
// the table is made on the second call and read back on the third.
func TestExecRaw_RunsOnOneConnectionForTheWholeTest(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "raw.db")
	storetest.ExecRaw(t, path, "CREATE TABLE anchor (id INTEGER)")
	storetest.ExecRaw(t, path, "CREATE TEMP TABLE marker (id INTEGER)")
	// A third call that opened a connection of its own would fail here: a TEMP table lives only on
	// the connection that created it.
	storetest.ExecRaw(t, path, "INSERT INTO marker (id) VALUES (1)")
}

// A file at a path ExecRaw or QueryRaw already holds a connection to can be replaced between
// calls, as a cache's discard-and-rebuild replaces one: the next call must open a fresh
// connection rather than write to, or read, the file that is gone (finding 5, 2026-10-26). Only
// the sanctioned packages replace a database file (principles §2, D144), so this is exercised
// where the server does it for real: TestServe_DiscardsACacheFromAnotherVersionAndBuildsItAgain
// (cmd/orpheum/serve_test.go).

// BenchmarkQueryRaw_OnAMigratedDatabase is the cost of one read of a migrated persistent.db,
// whose schema a new connection parses before its first query.
func BenchmarkQueryRaw_OnAMigratedDatabase(b *testing.B) {
	path := storetest.OpenMigrated(b).Path()
	for b.Loop() {
		storetest.QueryRaw(b, path, "SELECT count(*) FROM item")
	}
}
