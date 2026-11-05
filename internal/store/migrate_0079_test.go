// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0079 makes a group a row (D426 section 5). Every group a position, a name or a season's poster
// used becomes one, in its ordering; a position with no group names none; a season's poster names its
// series' aired group, the aired order made for a series that had lost it; a season's poster on an
// item that is no series goes. Every other id is kept, and the foreign key refuses a group no row is.
func TestMigrate_Step0079MakesEachGroupARowAndKeepsEveryPlace(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0079 := stepIndex(t, steps, 79)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0079], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		`INSERT INTO item (id, kind, added_at, identification_state) VALUES (1, 'series', 1, 'identified'),
		 (2, 'episode', 1, 'identified'), (3, 'episode', 1, 'identified'), (4, 'series', 1, 'identified'),
		 (5, 'movie', 1, 'identified')`,
		`INSERT INTO "sequence" (id, kind, created_by) VALUES (1, 'series', 'scanner'), (4, 'series', 'scanner')`,
		`INSERT INTO "work" (id, kind) VALUES (2, 'episode'), (3, 'episode'), (5, 'movie')`,
		`INSERT INTO ordering (id, sequence_id, name, kind) VALUES (1, 1, 'aired', 'aired')`,
		`INSERT INTO ordering (id, sequence_id, name, provider, kind) VALUES (2, 1, 'tmdb:dvd', 'tmdb', 'dvd')`,
		`INSERT INTO position (ordering_id, work_id, group_number, number_in_group, sort_order) VALUES
		 (1, 2, 1, 1, 10001), (1, 3, NULL /* no season */, NULL, 9999),
		 (2, 2, 1, 1, 10001), (2, 3, 2, 1, 20001)`,
		`INSERT INTO group_text (ordering_id, group_number, language, name) VALUES
		 (1, 1, 'en', 'Season 1'), (2, 3, 'en', 'Disc 3')`,
		`INSERT INTO artwork (id, item_id, kind, source, group_number) VALUES
		 (1, 1, 'poster', 'tmdb', NULL), (2, 1, 'poster', 'tmdb', 1), (3, 1, 'poster', 'tmdb', 2),
		 (4, 4, 'poster', 'tmdb', 0), (5, 5, 'poster', 'tmdb', 1)`)
	if err = database.migrate(t.Context(), steps[:step0079+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		what, query string
		columns     int
		want        string
	}{
		{
			"the orderings, the aired one made for the series that had lost it",
			"SELECT id, sequence_id, name FROM ordering ORDER BY id", 3,
			"[[1 1 aired] [2 1 tmdb:dvd] [3 4 aired]]",
		},
		{
			"the groups, each used number once in its ordering",
			"SELECT id, ordering_id, number FROM grp ORDER BY id", 3,
			"[[1 1 1] [2 1 2] [3 2 1] [4 2 2] [5 2 3] [6 3 0]]",
		},
		{
			"the positions, the unnumbered one in none",
			"SELECT ordering_id, work_id, coalesce(grp_id, ''), coalesce(number_in_group, ''), sort_order " +
				"FROM position ORDER BY ordering_id, work_id", 5,
			"[[1 2 1 1 10001] [1 3   9999] [2 2 3 1 10001] [2 3 4 1 20001]]",
		},
		{
			"the names", "SELECT grp_id, language, name FROM group_text ORDER BY grp_id", 3,
			"[[1 en Season 1] [5 en Disc 3]]",
		},
		{
			"the artwork, each season's poster its aired group, the film's gone",
			"SELECT id, coalesce(grp_id, '') FROM artwork ORDER BY id", 2, "[[1 ] [2 1] [3 2] [4 6]]",
		},
		{"the artwork's sequence, kept", "SELECT seq FROM sqlite_sequence WHERE name = 'artwork'", 1, "[[5]]"},
		{"the foreign keys", "PRAGMA foreign_key_check", 4, "[]"},
	}
	for _, check := range checks {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s:\ngot  %s\nwant %s", check.what, got, check.want)
		}
	}
	_, err = database.writer.ExecContext(t.Context(),
		"INSERT INTO position (ordering_id, work_id, grp_id, sort_order) VALUES (2, 5, 99, 1)")
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Errorf("a position in a group no row is was not refused: %v", err)
	}
	_, err = database.writer.ExecContext(t.Context(), "INSERT INTO grp (ordering_id, number) VALUES (1, 1)")
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("a second group 1 in one ordering was not refused: %v", err)
	}
}
