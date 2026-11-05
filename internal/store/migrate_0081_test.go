// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0081 adds a place's mark that a repoint moved it and no scan has read it since (D432 section
// 6): every place is confirmed when the step runs, with its modified time as it was, and the mark
// holds only 0 or 1.
func TestMigrate_Step0081LeavesEveryPlaceConfirmedWithItsModifiedTime(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0081 := stepIndex(t, steps, 81)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0081], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO file (id) VALUES (1), (2)",
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, modified_at, filing, first_seen_at,
		 last_seen_at) VALUES (1, 1, 1, '/a.mkv', CAST('/a.mkv' AS BLOB), 7, 'read', 1, 1),
		 (2, 2, 1, '/b.mkv', CAST('/b.mkv' AS BLOB), NULL, 'read', 1, 1)`)
	if err = database.migrate(t.Context(), steps[:step0081+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(queryRows(t, database,
		"SELECT id, coalesce(modified_at, ''), is_unconfirmed FROM place ORDER BY id", 3))
	if want := "[[1 7 0] [2  0]]"; got != want {
		t.Errorf("the places = %s; want %s, each confirmed and its modified time kept", got, want)
	}
	_, err = database.writer.ExecContext(t.Context(), "UPDATE place SET is_unconfirmed = 2 WHERE id = 1")
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("marking a place 2 = %v; want the CHECK to refuse it", err)
	}
}
