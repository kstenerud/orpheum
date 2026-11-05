// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0068 makes a library's kind singular (D416 §5, D425 §5): each library and each file's filing
// keeps its rows with movies read as movie and shows as series, other as it was, and the library's id
// sequence carries on past the highest id it gave.
func TestMigrate_Step0068MakesEveryLibraryKindSingular(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0068 := stepIndex(t, steps, 68)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0068], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'tmdb', 'manual', 0, 1), (2, 'Series', 'shows', 'tmdb', 'manual', 0, 1),
		        (7, 'Home', 'other', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO file (id, node_id, path, first_seen_at, last_seen_at) VALUES (1, 1, '/a', 1, 1),
		 (2, 1, '/b', 1, 1), (3, 1, '/c', 1, 1)`,
		`INSERT INTO file_filing (file_id, library_kind, filing) VALUES (1, 'movies', 'listed'),
		 (2, 'shows', 'unidentified'), (3, 'other', 'listed'), (3, 'movies', 'extra')`)
	if err = database.migrate(t.Context(), steps[:step0068+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	libraries := fmt.Sprint(queryRows(t, database, "SELECT id, name, kind FROM library ORDER BY id", 3))
	if want := "[[1 Films movie] [2 Series series] [7 Home other]]"; libraries != want {
		t.Errorf("the libraries after the step: %s; want %s", libraries, want)
	}
	filings := fmt.Sprint(queryRows(t, database,
		"SELECT file_id, library_kind, filing FROM file_filing ORDER BY file_id, library_kind", 3))
	if want := "[[1 movie listed] [2 series unidentified] [3 movie extra] [3 other listed]]"; filings != want {
		t.Errorf("the filings after the step: %s; want %s", filings, want)
	}
	execRaw(t, database, `INSERT INTO library (name, kind, metadata_source, change_detection, is_writable,
		created_at) VALUES ('Next', 'movie', 'tmdb', 'manual', 0, 1)`)
	next := fmt.Sprint(queryRows(t, database, "SELECT id FROM library WHERE name = 'Next'", 1))
	if next != "[[8]]" {
		t.Errorf("the next library's id: %s; want [[8]], past the highest the sequence gave", next)
	}
}
