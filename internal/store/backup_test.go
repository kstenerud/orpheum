// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// tableContents renders every row of every table, in a stable order, as text.
func tableContents(t *testing.T, database *Database) map[string]string {
	t.Helper()
	contents := map[string]string{}
	const tables = "SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name"
	for _, table := range queryRows(t, database, tables, 1) {
		name := table[0]
		columns := queryRows(t, database,
			fmt.Sprintf("SELECT name FROM pragma_table_info('%s') ORDER BY cid", name), 1)
		var list []string
		for _, column := range columns {
			list = append(list, "quote(\""+column[0]+"\")")
		}
		selection := strings.Join(list, " || '|' || ")
		// The names come from sqlite_schema and pragma_table_info, not from input.
		query := "SELECT " + selection + " FROM \"" + name + "\" ORDER BY 1"
		var rows []string
		for _, row := range queryRows(t, database, query, 1) {
			rows = append(rows, row[0])
		}
		contents[name] = strings.Join(rows, "\n")
	}
	return contents
}

// loadEveryTable fills a migrated database with testdata/every_table.sql.
func loadEveryTable(t *testing.T, database *Database) {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("testdata", "every_table.sql"))
	if err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, string(fixture))
}

func TestBackup_ARestoredCopyReproducesEveryTable(t *testing.T) {
	t.Parallel()
	source := openMigrated(t)
	loadEveryTable(t, source)
	want := tableContents(t, source)
	for table, rows := range want {
		if rows == "" && !strings.HasPrefix(table, "item_fts") && !strings.HasPrefix(table, "sqlite_") {
			t.Errorf("the fixture leaves %s empty, so the test cannot see it copied", table)
		}
	}

	backup := filepath.Join(t.TempDir(), "backup.sqlite")
	if err := source.BackupInto(t.Context(), backup); err != nil {
		t.Fatalf("BackupInto: %v", err)
	}
	restoredPath := filepath.Join(t.TempDir(), "persistent.db")
	if err := CopyBackup(t.Context(), backup, restoredPath); err != nil {
		t.Fatalf("CopyBackup: %v", err)
	}
	restored, err := Open(t.Context(), restoredPath, testOpenOptions())
	if err != nil {
		t.Fatalf("open the restored copy: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := restored.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	got := tableContents(t, restored)
	if len(got) != len(want) {
		t.Errorf("the restored copy has %d tables, want %d", len(got), len(want))
	}
	for table, rows := range want {
		if got[table] != rows {
			t.Errorf("table %s differs after a backup and restore.\nwant:\n%s\ngot:\n%s", table, rows, got[table])
		}
	}
}

func TestInspectBackup_RefusesAMissingOrForeignFile(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	missing := filepath.Join(directory, "typo.sqlite")
	if _, err := InspectBackup(t.Context(), missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("InspectBackup(missing) error = %v, want not-exist", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Error("inspecting a missing backup created it; a typo must not become an empty database")
	}

	foreignPath := filepath.Join(directory, "foreign.sqlite")
	foreign, err := sql.Open("sqlite", foreignPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = foreign.ExecContext(t.Context(), "CREATE TABLE someone_elses (id INTEGER)")
	if err = errors.Join(err, foreign.Close()); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectBackup(t.Context(), foreignPath); !errors.Is(err, ErrNotOurs) {
		t.Errorf("InspectBackup(foreign) error = %v, want ErrNotOurs", err)
	}
	err = CopyBackup(t.Context(), foreignPath, filepath.Join(directory, "copy.sqlite"))
	if !errors.Is(err, ErrNotOurs) {
		t.Errorf("CopyBackup(foreign) error = %v, want ErrNotOurs", err)
	}

	ours := openMigrated(t)
	backup := filepath.Join(directory, "ours.sqlite")
	if err = ours.BackupInto(t.Context(), backup); err != nil {
		t.Fatal(err)
	}
	inspected, err := InspectBackup(t.Context(), backup)
	if err != nil || inspected.State.Version == 0 {
		t.Errorf("InspectBackup(ours) = %+v, %v; want our schema version", inspected, err)
	}
}

// A backup copies a snapshot while a write holds the writer: it never waits for the writer, and
// the writer never waits for it (D303 §7, the independent audit's S7).
func TestBackupInto_CopiesWhileAWriteHoldsTheWriter(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	holding, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- database.Write(t.Context(), func(*sqlcgen.Queries) error {
			close(holding)
			select {
			case <-release:
			case <-t.Context().Done():
			}
			return nil
		})
	}()
	testutil.Receive(t, holding, "the write to hold the writer")
	// A failure bound only (test-weight.md §25): the copy is CPU-bound, so a passing test never
	// waits for it; raised to testutil.ReceiveLimit's scale after a 5 s bound failed once under a
	// loaded whole-tree run (2026-10-25).
	ctx, cancel := context.WithTimeout(t.Context(), testutil.ReceiveLimit)
	defer cancel()
	err := database.BackupInto(ctx, filepath.Join(t.TempDir(), "backup.db"))
	close(release)
	if err != nil {
		t.Errorf("a backup beside a held writer failed: %v", err)
	}
	if err := testutil.Receive(t, done, "the holding write to finish"); err != nil {
		t.Fatal(err)
	}
}

// A copy that fails SQLite's integrity check is refused: here a row the database's own CHECK
// forbids, which VACUUM INTO copies as it is (D303 §7, M6).
func TestBackupInto_RefusesACopyThatFailsItsIntegrityCheck(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	for _, statement := range []string{
		"PRAGMA ignore_check_constraints = 1",
		"INSERT INTO user (name, role_id, created_at, admin_away) VALUES ('mallory', 1, 1, 'sometimes')",
		"PRAGMA ignore_check_constraints = 0",
	} {
		if _, err := database.writer.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	err := database.BackupInto(t.Context(), backup)
	if !errors.Is(err, ErrDamagedCopy) {
		t.Errorf("a backup of a row its CHECK forbids: %v, want ErrDamagedCopy", err)
	}
}
