// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0101 keeps every library's hours between checks, whatever it notices changes by (D541 §1): a
// library checked on an interval keeps its own, one watching or checked when asked, which had none,
// takes 6, a new library's starting value; the next library made is given an id never given before;
// and it raises the compatible version, so an older binary, which writes a library with no hours,
// refuses the file.
func TestMigrate_EveryLibraryHasItsHoursBetweenChecksAfterStep0101(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	var before []Step
	for _, step := range steps {
		if step.Version < 101 {
			before = append(before, step)
		}
	}
	database := openTemporary(t)
	if err := database.migrate(t.Context(), before, testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, check_every_hours, is_writable,
		created_at) VALUES (1, 'Films', 'movie', 'nfo', 'interval', 3, 0, 1)`,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at, removed_at)
		VALUES (2, 'Shows', 'series', 'tmdb', 'watch', 1, 2, NULL), (3, 'Gone', 'other', 'nfo', 'manual', 0, 3, 9),
		(4, 'Last', 'movie', 'nfo', 'manual', 0, 4, NULL)`,
		"DELETE FROM library WHERE id = 4", // the sequence stays at 4
	)
	if err := database.migrate(t.Context(), steps, testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, `SELECT id, name, change_detection, check_every_hours, quote(removed_at)
		FROM library ORDER BY id`, 5)
	want := "[[1 Films interval 3 NULL] [2 Shows watch 6 NULL] [3 Gone manual 6 9]]"
	if got := fmt.Sprint(rows); got != want {
		t.Errorf("after step 0101 the libraries are %q; want %q: an interval's hours kept, 6 for the rest", got, want)
	}
	execRaw(t, database, `INSERT INTO library (name, kind, metadata_source, change_detection, check_every_hours,
		is_writable, created_at) VALUES ('Next', 'movie', 'nfo', 'watch', 6, 0, 5)`)
	if got := fmt.Sprint(queryRows(t, database, "SELECT id FROM library WHERE name = 'Next'", 1)); got != "[[5]]" {
		t.Errorf("the library made after step 0101 has id %s; want [[5]], past every id given before", got)
	}
	if state, err := database.State(t.Context()); err != nil || state.CompatibleVersion < 101 {
		t.Errorf("after step 0101, state = %+v (%v); want compatible_version 101 at least", state, err)
	}
}
