// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0080 makes a series' members its aired order's places (D426 section 6). No member is lost: one
// with no place there is given one in no season, at its member sort order, an unplaced one after the
// numbered specials, and a series with members and no aired order is given it. A place moved between
// aired orders moves its series' found time, as a member moved did.
func TestMigrate_Step0080KeepsEveryMemberAsAPlaceInItsAiredOrder(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0080 := stepIndex(t, steps, 80)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0080], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		`INSERT INTO item (id, kind, added_at, identification_state) VALUES (1, 'series', 1, 'identified'),
		 (2, 'episode', 1, 'identified'), (3, 'episode', 1, 'identified'), (4, 'episode', 1, 'identified'),
		 (5, 'series', 1, 'identified'), (6, 'episode', 1, 'identified')`,
		`INSERT INTO "sequence" (id, kind, created_by) VALUES (1, 'series', 'scanner'), (5, 'series', 'scanner')`,
		`INSERT INTO "work" (id, kind) VALUES (2, 'episode'), (3, 'episode'), (4, 'episode'), (6, 'episode')`,
		`INSERT INTO ordering (id, sequence_id, name, kind) VALUES (1, 1, 'aired', 'aired')`,
		`INSERT INTO grp (id, ordering_id, number) VALUES (1, 1, 1)`,
		`INSERT INTO position (ordering_id, work_id, grp_id, number_in_group, sort_order) VALUES
		 (1, 2, 1, 1, 10001)`,
		`INSERT INTO sequence_member (sequence_id, member_work, position) VALUES
		 (1, 2, 10001), (1, 3, 10002), (1, 4, NULL), (5, 6, 20001)`)
	if err = database.migrate(t.Context(), steps[:step0080+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		what, query string
		columns     int
		want        string
	}{
		{
			"the orderings, the aired one made for the series that had none",
			"SELECT id, sequence_id, name FROM ordering ORDER BY id", 3, "[[1 1 aired] [2 5 aired]]",
		},
		{
			"the places, a member's kept and every other member placed in no season",
			"SELECT ordering_id, work_id, coalesce(grp_id, ''), sort_order FROM position " +
				"ORDER BY ordering_id, work_id", 4,
			"[[1 2 1 10001] [1 3  10002] [1 4  9999] [2 6  20001]]",
		},
		{
			"the members, read from the aired orders",
			"SELECT sequence_id, member_work FROM series_member ORDER BY sequence_id, member_work", 2,
			"[[1 2] [1 3] [1 4] [5 6]]",
		},
		{
			"the tables, the member table gone", "SELECT count(*) FROM sqlite_schema WHERE name = 'sequence_member'",
			1, "[[0]]",
		},
		{"the foreign keys", "PRAGMA foreign_key_check", 4, "[]"},
	}
	for _, check := range checks {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s:\ngot  %s\nwant %s", check.what, got, check.want)
		}
	}
	// A place moved to another series' aired order moves the series' found time with it.
	execRaw(t, database,
		"INSERT INTO version (id, work_id, created_at) VALUES (6, 6, 1)",
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO file (id) VALUES (6)",
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, modified_at, filing, first_seen_at,
		 last_seen_at) VALUES (6, 6, 1, '/s/6.mkv', CAST('/s/6.mkv' AS BLOB), 7, 'read', 300, 300)`,
		"INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES (6, 'series', 'listed', 6)")
	moves := []struct{ what, statement, want string }{
		{"with series 5's episode found", "SELECT 1", "[[1 0] [5 300]]"},
		{"with it moved to series 1", "UPDATE position SET ordering_id = 1 WHERE work_id = 6", "[[1 300] [5 0]]"},
		{"with its place gone", "DELETE FROM position WHERE work_id = 6", "[[1 0] [5 0]]"},
		{
			"with it placed again", "INSERT INTO position (ordering_id, work_id, sort_order) VALUES (2, 6, 1)",
			"[[1 0] [5 300]]",
		},
	}
	for _, move := range moves {
		execRaw(t, database, move.statement)
		got := fmt.Sprint(queryRows(t, database, "SELECT id, found_at FROM item WHERE kind = 'series' ORDER BY id", 2))
		if got != move.want {
			t.Errorf("%s: found = %s; want %s", move.what, got, move.want)
		}
	}
}
