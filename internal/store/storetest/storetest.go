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
// run in fifty (2026-10-06).
func ExecRaw(tb testing.TB, path string, statements ...string) {
	tb.Helper()
	pool, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(15000)")
	if err != nil {
		tb.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if closeErr := pool.Close(); closeErr != nil {
			tb.Errorf("close %s: %v", path, closeErr)
		}
	}()
	for _, statement := range statements {
		if _, err := pool.ExecContext(tb.Context(), statement); err != nil {
			tb.Fatalf("exec %q on %s: %v", statement, path, err)
		}
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
	if _, isAllowed := readerWritesAllowed()[shortName(caller)]; !isAllowed {
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
			"(D303, DL10); keep it in memory and write it later, or list it in storetest's "+
			"readerWritesAllowed with why", caller)
	}
}

// shortName is a function's name without its package path: "auth.(*Service).Authenticate".
func shortName(function string) string {
	return function[strings.LastIndex(function, "/")+1:]
}

// readerWritesAllowed are the writes a reading request may make, by the function that makes them,
// each with why. An entry naming D303 is a defect the redesign removes.
func readerWritesAllowed() map[string]string {
	return map[string]string{
		"settings.(*Service).heal": "a setting row removed while the server runs is written back once, with its " +
			"starting value, the first time it is read (D202); every start writes them all",
		// A play's GET asks for the work it plays, on cache.db's writer, once a run, never once a
		// chunk: the person waits on it, and nothing else can ask for it (D234 §3, D247 §10).
		"jobqueue.(*Queue[...]).Enqueue": "a play's GET queues the run of chunks, the read-through or the " +
			"text it plays",
		"jobqueue.(*Queue[...]).Cancel": "a play's GET cancels the run of chunks a seek went past (D250 §5)",
		"chunks.(*Service).storeIDOf":   "a play's GET names the chunk store its first run makes",
		"renditions.(*Service).want":    "a play's GET asks for the prepared copy it plays",
		"renditions.(*Service).redo":    "a play's GET asks again for a prepared copy that failed",
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
