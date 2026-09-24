// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if rows == "" && !strings.HasPrefix(table, "item_fts") {
			t.Errorf("the fixture leaves %s empty, so the test cannot see it copied", table)
		}
	}

	backup := filepath.Join(t.TempDir(), "backup.sqlite")
	if err := source.BackupInto(t.Context(), backup); err != nil {
		t.Fatalf("BackupInto: %v", err)
	}
	restoredPath := filepath.Join(t.TempDir(), "library.db")
	if err := CopyBackup(t.Context(), backup, restoredPath); err != nil {
		t.Fatalf("CopyBackup: %v", err)
	}
	restored, err := Open(t.Context(), restoredPath)
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
