// SPDX-License-Identifier: GPL-3.0-or-later

package storetest

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// migratedTemplate is library.db migrated to the current schema once per test process, as bytes:
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
	path := filepath.Join(directory, "library.db")
	if err = os.WriteFile(path, contents, 0o600); err != nil {
		tb.Fatalf("copy the migrated template: %v", err)
	}
	database, err := store.Open(tb.Context(), path, OpenOptions())
	if err != nil {
		tb.Fatalf("open a test database: %v", err)
	}
	tb.Cleanup(func() {
		if err := database.Close(); err != nil {
			tb.Errorf("close the test database: %v", err)
		}
	})
	if err := database.Migrate(tb.Context(), MigrateOptions(directory)); err != nil {
		tb.Fatalf("migrate the test database: %v", err)
	}
	return database
}

// OpenCache returns a new cache.db in a temporary directory, closed when the test ends.
func OpenCache(tb testing.TB) *store.Cache {
	tb.Helper()
	cache, err := store.OpenCache(tb.Context(), filepath.Join(tb.TempDir(), "cache.db"), OpenOptions())
	if err != nil {
		tb.Fatalf("open a test cache: %v", err)
	}
	tb.Cleanup(func() {
		if err := cache.Close(); err != nil {
			tb.Errorf("close the test cache: %v", err)
		}
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
// states the store itself never produces: a file a newer release upgraded, a foreign file.
func ExecRaw(tb testing.TB, path string, statements ...string) {
	tb.Helper()
	pool, err := sql.Open("sqlite", path)
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
