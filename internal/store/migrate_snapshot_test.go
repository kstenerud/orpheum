// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// removals records the snapshot files the runner asks its caller to remove. The store removes no
// file itself (D144), and its tests may not either, so the file stays where it is.
type removals struct{ names []string }

func (recorded *removals) remove(name string) error {
	recorded.names = append(recorded.names, name)
	return nil
}

// A snapshot file found under the name, left by a stop that cut a copy short, is not trusted as the
// way back: it is checked, and one that is not a whole snapshot of the version is removed, to be
// written again (D181). The fake removal leaves it, so the copy then finds its name taken.
func TestMigrate_RemovesASnapshotFoundDamagedBeforeTrustingIt(t *testing.T) {
	t.Parallel()
	database := openTemporary(t)
	if err := database.migrate(t.Context(), syntheticSteps(t, firstStep), testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	snapshots := t.TempDir()
	snapshot := filepath.Join(snapshots, "pre-migrate-v1.sqlite")
	if err := os.WriteFile(snapshot, []byte("SQLite format 3\x00 and then the stop came"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorded := &removals{}
	options := testMigrateOptions(snapshots)
	options.RemoveSnapshotFile = recorded.remove
	err := database.migrate(t.Context(), syntheticSteps(t, firstStep, secondStep), options)
	if fmt.Sprint(recorded.names) != "[pre-migrate-v1.sqlite]" {
		t.Errorf("removed %v, want the damaged leftover [pre-migrate-v1.sqlite]", recorded.names)
	}
	if !errors.Is(err, ErrCopyExists) {
		t.Errorf("migrate over a damaged leftover the fake did not remove: %v, want ErrCopyExists, "+
			"the leftover not taken as the snapshot", err)
	}
	assertVersion(t, database, 1)
}

// A whole snapshot of the version found under the name is the way back, kept as it is.
func TestMigrate_KeepsAWholeSnapshotFoundOfTheVersion(t *testing.T) {
	t.Parallel()
	database := openTemporary(t)
	if err := database.migrate(t.Context(), syntheticSteps(t, firstStep), testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	snapshots := t.TempDir()
	if err := database.BackupInto(t.Context(), filepath.Join(snapshots, "pre-migrate-v1.sqlite")); err != nil {
		t.Fatal(err)
	}
	recorded := &removals{}
	options := testMigrateOptions(snapshots)
	options.RemoveSnapshotFile = recorded.remove
	if err := database.migrate(t.Context(), syntheticSteps(t, firstStep, secondStep), options); err != nil {
		t.Fatalf("migrate over a whole snapshot of the version: %v", err)
	}
	if len(recorded.names) != 0 {
		t.Errorf("removed %v, a whole snapshot of the version", recorded.names)
	}
	assertVersion(t, database, 2)
}

// A copy the integrity check refuses is removed, so the next start does not find it and take it as
// the way back.
func TestMigrate_RemovesASnapshotItsCheckRefused(t *testing.T) {
	t.Parallel()
	database := openTemporary(t)
	checked := firstStep + "\nCREATE TABLE counted (n INTEGER NOT NULL CHECK (n > 0));"
	if err := database.migrate(t.Context(), syntheticSteps(t, checked), testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"PRAGMA ignore_check_constraints = 1",
		"INSERT INTO counted (n) VALUES (-1)",
		"PRAGMA ignore_check_constraints = 0",
	} {
		if _, err := database.writer.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	recorded := &removals{}
	options := testMigrateOptions(t.TempDir())
	options.RemoveSnapshotFile = recorded.remove
	err := database.migrate(t.Context(), syntheticSteps(t, checked, secondStep), options)
	if !errors.Is(err, ErrDamagedCopy) {
		t.Fatalf("migrate with a copy its check refuses: %v, want ErrDamagedCopy", err)
	}
	assertVersion(t, database, 1)
	if fmt.Sprint(recorded.names) != "[pre-migrate-v1.sqlite]" {
		t.Errorf("removed %v, want the refused copy [pre-migrate-v1.sqlite]", recorded.names)
	}
}
