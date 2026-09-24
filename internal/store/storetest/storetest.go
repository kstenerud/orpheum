// SPDX-License-Identifier: GPL-3.0-or-later

package storetest

import (
	"database/sql"
	"fmt"
	"path/filepath"
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
	return store.MigrateOptions{SnapshotDirectory: directory, SpaceFactor: 3}
}

// Open returns a new database in a temporary directory, migrated to the current schema and
// closed when the test ends.
func Open(tb testing.TB) *store.Database {
	tb.Helper()
	directory := tb.TempDir()
	database, err := store.Open(tb.Context(), filepath.Join(directory, "library.db"), OpenOptions())
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
