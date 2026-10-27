// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0061 dates each title and series that has a year by its year alone, which a pass then makes
// its source's day, and leaves one with no year undated (D366 §1).
func TestMigrate_Step0061DatesEachTitleAndSeriesByItsYear(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0061 := stepIndex(t, steps, 61)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0061], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO item (id, kind, filing_library_id, added_at) VALUES (1, 'title', 1, 1), (2, 'title', 1, 1),
		 (10, 'collection', 1, 1), (11, 'collection', 1, 1)`,
		`INSERT INTO title (id, kind, year, identification_state) VALUES (1, 'movie', 1995, 'identified'),
		 (2, 'movie', NULL, 'unidentified')`,
		`INSERT INTO collection (id, kind, membership, created_by, year) VALUES (10, 'series', 'explicit', 'scanner',
		 2008), (11, 'series', 'explicit', 'scanner', NULL)`)
	if err = database.migrate(t.Context(), steps[:step0061+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(queryRows(t, database, "SELECT id, coalesce(released_on, 'none') FROM title ORDER BY id", 2),
		queryRows(t, database, "SELECT id, coalesce(released_on, 'none') FROM collection ORDER BY id", 2))
	if want := "[[1 19950000] [2 none]] [[10 20080000] [11 none]]"; got != want {
		t.Errorf("after the step: %s; want %s: a year alone dates each, and no year none", got, want)
	}
}
