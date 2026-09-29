// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"strings"
	"testing"
)

// A migration that the free space could not hold is refused before anything is written: no
// snapshot, no step, the database as it was (D228 §4).
func TestMigrate_RefusesWhenTheFreeSpaceCannotHoldItAndChangesNothing(t *testing.T) {
	t.Parallel()
	database := openTemporary(t)
	if err := database.migrate(t.Context(), syntheticSteps(t, firstStep), testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	before := dumpSchema(t, database)
	snapshots := t.TempDir()
	err := database.migrate(t.Context(), syntheticSteps(t, firstStep, secondStep),
		MigrateOptions{SnapshotDirectory: snapshots, FreeSpaceDatabaseMultiple: 1 << 40})
	if err == nil || !strings.Contains(err.Error(), "not enough free space") ||
		!strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("a migration past the free space: %v, want it refused, with the numbers", err)
	}
	assertVersion(t, database, 1)
	if after := dumpSchema(t, database); after != before {
		t.Errorf("the refused migration changed the schema.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if entries, readErr := os.ReadDir(snapshots); readErr != nil || len(entries) != 0 {
		t.Errorf("the refused migration left snapshots %v, %v; want none", entries, readErr)
	}
}
