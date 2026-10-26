// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0059 keeps every library and title as it was, the libraries' id sequence too, and gives
// both kinds other, an other library held to NFO files alone (D349).
func TestMigrate_Step0059KeepsEveryLibraryAndTitleAndAddsTheKindOther(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0059 := stepIndex(t, steps, 59)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0059], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, `INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable,
		created_at) VALUES (1, 'Films', 'movies', 'tmdb', 'manual', 0, 1), (3, 'Shows', 'shows', 'nfo', 'manual', 0, 1),
		(4, 'Gone', 'mixed', 'nfo', 'manual', 0, 1)`,
		"DELETE FROM library WHERE id = 4",
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (1, 'title', 1, 1), (2, 'title', 3, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (1, 'movie', 'identified'),
		(2, 'episode', 'unidentified')`)
	if err = database.migrate(t.Context(), steps[:step0059+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(queryRows(t, database, "SELECT id, name, kind, metadata_source FROM library ORDER BY id", 4),
		queryRows(t, database, "SELECT id, kind, identification_state FROM title ORDER BY id", 3))
	want := "[[1 Films movies tmdb] [3 Shows shows nfo]] [[1 movie identified] [2 episode unidentified]]"
	if got != want {
		t.Errorf("after the step: %s; want %s", got, want)
	}
	execRaw(t, database, `INSERT INTO library (name, kind, metadata_source, change_detection, is_writable, created_at)
		VALUES ('Home movies', 'other', 'nfo', 'manual', 0, 2)`,
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (3, 'title', 5, 2)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (3, 'other', 'identified')`)
	if next := fmt.Sprint(queryRows(t, database, "SELECT max(id) FROM library", 1)); next != "[[5]]" {
		t.Errorf("the next library's id is %s, want 5, after the 4 a deleted library had", next)
	}
	_, err = database.writer.ExecContext(t.Context(), `INSERT INTO library (name, kind, metadata_source,
		change_detection, is_writable, created_at) VALUES ('Clips', 'other', 'nfo_then_tmdb', 'manual', 0, 3)`)
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("an other library that would be matched at TMDB: %v, want the check to refuse it", err)
	}
}
