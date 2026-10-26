// SPDX-License-Identifier: AGPL-3.0-or-later

package storetest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// OpenOptions are the store options tests use: in production they come from the config file.
func OpenOptions() store.OpenOptions {
	return store.OpenOptions{MinimumReaders: 4}
}

// MigrateOptions are the runner options tests use, snapshotting into directory.
func MigrateOptions(directory string) store.MigrateOptions {
	return store.MigrateOptions{SnapshotDirectory: directory, FreeSpaceDatabaseMultiple: 3}
}

// migratedTemplate is persistent.db migrated to the current schema once per test process, as bytes:
// every test starts from a copy, because replaying every migration step is most of what a test
// that opens a database costs. The first test to ask builds it, in its own temporary directory.
//
//nolint:gochecknoglobals // one migration per test process is the point: a test cannot share it otherwise
var migratedTemplate struct {
	once     sync.Once
	contents []byte
	err      error
}

// migratedContents is the template's bytes, built by the first caller.
func migratedContents(tb testing.TB) ([]byte, error) {
	tb.Helper()
	migratedTemplate.once.Do(func() {
		directory := tb.TempDir()
		path := filepath.Join(directory, "template.db")
		database, err := store.Open(tb.Context(), path, OpenOptions())
		if err != nil {
			migratedTemplate.err = fmt.Errorf("open the template: %w", err)
			return
		}
		migrated := database.Migrate(tb.Context(), MigrateOptions(directory))
		if closed := database.Close(); migrated != nil || closed != nil {
			migratedTemplate.err = errors.Join(migrated, closed)
			return
		}
		migratedTemplate.contents, migratedTemplate.err = os.ReadFile(path) //nolint:gosec // a path this test made
	})
	return migratedTemplate.contents, migratedTemplate.err
}

// OpenMigrated returns a new database in a temporary directory, migrated to the current schema and
// closed when the test ends. It starts from a copy of one database migrated once per process, and
// is migrated again, which finds nothing to do and so checks the copy is current.
func OpenMigrated(tb testing.TB) *store.Database {
	tb.Helper()
	contents, err := migratedContents(tb)
	if err != nil {
		tb.Fatalf("make the migrated template: %v", err)
	}
	directory := tb.TempDir()
	path := filepath.Join(directory, "persistent.db")
	if err = os.WriteFile(path, contents, 0o600); err != nil {
		tb.Fatalf("copy the migrated template: %v", err)
	}
	options, readerWrites := OpenOptions(), &readerWrites{}
	options.OnReaderWrite = readerWrites.note
	database, err := store.Open(tb.Context(), path, options)
	if err != nil {
		tb.Fatalf("open a test database: %v", err)
	}
	tb.Cleanup(func() {
		checkInvariants(tb, database)
		if err := database.Close(); err != nil {
			tb.Errorf("close the test database: %v", err)
		}
		readerWrites.report(tb)
	})
	if err := database.Migrate(tb.Context(), MigrateOptions(directory)); err != nil {
		tb.Fatalf("migrate the test database: %v", err)
	}
	return database
}

// WriteMigrated writes a copy of the migrated template at path, for a test whose code under test
// opens and migrates the file itself (a served instance's persistent.db): that migration then finds
// nothing to do, rather than replaying every step for each instance a test starts.
func WriteMigrated(tb testing.TB, path string) {
	tb.Helper()
	contents, err := migratedContents(tb)
	if err != nil {
		tb.Fatalf("make the migrated template: %v", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		tb.Fatalf("copy the migrated template: %v", err)
	}
}

// OpenCache returns a new cache.db in a temporary directory, closed when the test ends.
func OpenCache(tb testing.TB) *store.Cache {
	tb.Helper()
	options, readerWrites := OpenOptions(), &readerWrites{}
	options.OnReaderWrite = readerWrites.note
	cache, err := store.OpenCache(tb.Context(), filepath.Join(tb.TempDir(), "cache.db"), options)
	if err != nil {
		tb.Fatalf("open a test cache: %v", err)
	}
	tb.Cleanup(func() {
		if err := cache.Close(); err != nil {
			tb.Errorf("close the test cache: %v", err)
		}
		readerWrites.report(tb)
	})
	return cache
}

// InsertUser adds a live user with the named built-in role and no password, and returns its id.
func InsertUser(tb testing.TB, database *store.Database, name, role string) int64 {
	tb.Helper()
	ctx := tb.Context()
	var id int64
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		found, err := queries.GetRoleByName(ctx, role)
		if err != nil {
			return fmt.Errorf("find role %q: %w", role, err)
		}
		id, err = queries.InsertUser(ctx, sqlcgen.InsertUserParams{Name: name, RoleID: found.ID, CreatedAt: 1})
		if err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		return nil
	})
	if err != nil {
		tb.Fatalf("insert user %q: %v", name, err)
	}
	return id
}

// ExecRaw runs statements directly on the database file at path, bypassing the store, to build
// states the store itself never produces: a file a newer release upgraded, a foreign file. It
// waits for a lock as the store's own connections do: with none, a lock held for an instant just
// after a server stopped failed TestServe_DiscardsACacheFromAnotherVersionAndBuildsItAgain about one
// run in fifty (2026-10-06). A test's calls on one file share one connection, as QueryRaw's do
// (below): a new connection parses the whole schema before its first statement, about 85 times the
// cost of the statement itself (audit 2026-10-25, G1). A path whose file is replaced between calls
// (a restore, a cache's discard) is noticed and opens a fresh connection, never a stale one.
func ExecRaw(tb testing.TB, path string, statements ...string) {
	tb.Helper()
	pool, release := rawConnectionOf(tb, path, true)
	defer release()
	for _, statement := range statements {
		if _, err := pool.ExecContext(tb.Context(), statement); err != nil {
			tb.Fatalf("exec %q on %s: %v", statement, path, err)
		}
	}
}

// QueryRaw reads every row a query returns from the database file at path, bypassing the store,
// each row's columns as text joined by "|": for a test that compares two databases' contents. A
// test's reads of one file share one connection, closed when the test ends: a new connection
// parses the whole schema before its first query, about 85 times the cost of the query itself
// (audit 2026-10-25, G1), and some tests read a file table by table, step by step.
func QueryRaw(tb testing.TB, path, query string) []string {
	tb.Helper()
	pool, release := rawConnectionOf(tb, path, false)
	defer release()
	rows, err := pool.QueryContext(tb.Context(), query)
	if err != nil {
		tb.Fatalf("query %q on %s: %v", query, path, err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // a read; rows.Err is checked
	columns, err := rows.Columns()
	if err != nil {
		tb.Fatalf("read the columns of %q: %v", query, err)
	}
	var read []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			tb.Fatalf("read a row of %q: %v", query, err)
		}
		texts := make([]string, len(values))
		for index, value := range values {
			if bytes, isBytes := value.([]byte); isBytes {
				value = fmt.Sprintf("%x", bytes)
			}
			texts[index] = fmt.Sprint(value)
		}
		read = append(read, strings.Join(texts, "|"))
	}
	if err := rows.Err(); err != nil {
		tb.Fatalf("read %q: %v", query, err)
	}
	return read
}

// rawConnection is ExecRaw's or QueryRaw's connection to one file, for one test, and the file it
// was opened on.
type rawConnection struct {
	pool   *sql.DB
	opened os.FileInfo
}

// rawConnectionKey names a rawConnection: the test it serves, the path it reads or writes, and
// whether it writes. ExecRaw and QueryRaw never share one connection for the same path: a writer
// cannot carry QueryRaw's query_only pragma, and a reader must not carry ExecRaw's absence of it.
type rawConnectionKey struct {
	tb       testing.TB
	path     string
	writable bool
}

// rawConnections are ExecRaw's and QueryRaw's open connections, each closed by the cleanup of the
// test it serves.
//
//nolint:gochecknoglobals // a test's connection must outlive one call, and a TB carries no values
var rawConnections struct {
	mutex  sync.Mutex
	byTest map[rawConnectionKey]*rawConnection
}

// rawConnectionOf is the test's connection to path, for ExecRaw (writable) or QueryRaw (not): the
// one it opened before, unless another file has taken the path since, which it closes and opens
// the new one in its place. With no file at path yet, the connection serves the one call, which
// then releases it.
func rawConnectionOf(tb testing.TB, path string, writable bool) (pool *sql.DB, release func()) {
	tb.Helper()
	key := rawConnectionKey{tb: tb, path: path, writable: writable}
	rawConnections.mutex.Lock()
	defer rawConnections.mutex.Unlock()
	current, statErr := os.Stat(path)
	if conn, isOpen := rawConnections.byTest[key]; isOpen {
		if statErr == nil && os.SameFile(conn.opened, current) {
			return conn.pool, func() {}
		}
		closeRawConnection(tb, path, conn.pool)
		delete(rawConnections.byTest, key)
	}
	dsn := path + "?_pragma=busy_timeout(15000)"
	if !writable {
		dsn += "&_pragma=query_only(1)"
	}
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		tb.Fatalf("open %s: %v", path, err)
	}
	pool.SetMaxOpenConns(1) // one connection, held open: it keeps the file it was opened on
	pool.SetMaxIdleConns(1)
	if statErr != nil {
		return pool, func() { closeRawConnection(tb, path, pool) }
	}
	if rawConnections.byTest == nil {
		rawConnections.byTest = map[rawConnectionKey]*rawConnection{}
	}
	rawConnections.byTest[key] = &rawConnection{pool: pool, opened: current}
	tb.Cleanup(func() {
		rawConnections.mutex.Lock()
		defer rawConnections.mutex.Unlock()
		if conn, isOpen := rawConnections.byTest[key]; isOpen && conn.pool == pool {
			delete(rawConnections.byTest, key)
			closeRawConnection(tb, path, pool)
		}
	})
	return pool, func() {}
}

// closeRawConnection closes one of ExecRaw's or QueryRaw's connections, failing the test if it
// cannot.
func closeRawConnection(tb testing.TB, path string, pool *sql.DB) {
	tb.Helper()
	if err := pool.Close(); err != nil {
		tb.Errorf("close %s: %v", path, err)
	}
}

// DiskFullError is the error SQLite answers a write with when its disk has no room: a real one,
// from a scratch database held to the pages it has, for a test of what a full disk does to a
// package that writes (D231 §5).
func DiskFullError(tb testing.TB) error {
	tb.Helper()
	pool, err := sql.Open("sqlite", filepath.Join(tb.TempDir(), "full.db"))
	if err != nil {
		tb.Fatalf("open a scratch database: %v", err)
	}
	defer func() {
		if closeErr := pool.Close(); closeErr != nil {
			tb.Errorf("close the scratch database: %v", closeErr)
		}
	}()
	pool.SetMaxOpenConns(1) // max_page_count holds for the one connection that set it
	for _, statement := range []string{"CREATE TABLE filler (bytes BLOB)", "PRAGMA max_page_count = 2"} {
		if _, prepareErr := pool.ExecContext(tb.Context(), statement); prepareErr != nil {
			tb.Fatalf("prepare the scratch database: %v", prepareErr)
		}
	}
	for range 100 {
		_, fillErr := pool.ExecContext(tb.Context(), "INSERT INTO filler (bytes) VALUES (zeroblob(4096))")
		if fillErr != nil {
			if !store.IsStorageFailure(fillErr) {
				tb.Fatalf("filling the scratch database failed otherwise than full: %v", fillErr)
			}
			return fmt.Errorf("write to a full database: %w", fillErr)
		}
	}
	tb.Fatal("the scratch database never filled")
	return nil
}

// readerWrites are the writes made under a request that only reads (waiting.ForAReader), which
// fail the test that made them (D303, DL10): nothing a GET or a HEAD does may take the writer.
type readerWrites struct {
	mutex   sync.Mutex
	callers []string
}

// note records one such write, by the function outside the store that made it.
func (writes *readerWrites) note(_, caller string) {
	writes.mutex.Lock()
	defer writes.mutex.Unlock()
	if _, isAllowed := store.ReaderWriteAllowed(caller); !isAllowed {
		writes.callers = append(writes.callers, caller)
	}
}

// report fails the test for each write it recorded.
func (writes *readerWrites) report(tb testing.TB) {
	tb.Helper()
	writes.mutex.Lock()
	defer writes.mutex.Unlock()
	for _, caller := range slices.Compact(slices.Sorted(slices.Values(writes.callers))) {
		tb.Errorf("a request that only reads wrote to the database, through %s: reading never writes "+
			"(D303, DL10); keep it in memory and write it later, or list it in the store's "+
			"readerWritesAllowed with why", caller)
	}
}

// checkInvariants fails the test whose database no longer keeps one of persistent.db's invariants
// (D303, M7): whatever a test did, the data it left must be data the server could hold.
func checkInvariants(tb testing.TB, database *store.Database) {
	tb.Helper()
	violations, err := database.CheckInvariants(context.WithoutCancel(tb.Context()))
	if errors.Is(err, store.ErrClosed) {
		return // the test closed it itself, to see what a closed database does
	}
	if err != nil {
		tb.Errorf("check the test database's invariants: %v", err)
	}
	for _, violation := range violations {
		tb.Errorf("the test left its database breaking an invariant (D303, M7): %s, at %v",
			violation.Invariant, violation.Rows)
	}
}
