// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0074 renames title to work and collection to sequence (D426 §3), with every table, column,
// index and trigger named for them: a person's watched works, a series' members and orderings, an
// episode's position, a version's work and an extra's owner keep their rows under the new names, and
// the old names are gone.
func TestMigrate_Step0074KeepsEveryRowUnderWorkAndSequence(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0074 := stepIndex(t, steps, 74)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0074], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO role (id, name, capabilities) VALUES (99, 'step 0074', '[]')",
		"INSERT INTO user (id, name, role_id, created_at) VALUES (5, 'dan', 99, 1)",
		`INSERT INTO item (id, kind, identification_state, added_at) VALUES (1, 'movie', 'identified', 1),
		 (2, 'episode', 'identified', 1), (3, 'series', 'identified', 1), (4, 'extra', 'identified', 1)`,
		`INSERT INTO title (id, kind, runtime_ms) VALUES (1, 'movie', 8160000), (2, 'episode', NULL),
		 (4, 'extra', NULL)`,
		"INSERT INTO collection (id, kind, created_by) VALUES (3, 'series', 'scanner')",
		"INSERT INTO collection_member (collection_id, member_title, position) VALUES (3, 2, 1)",
		"INSERT INTO ordering (id, collection_id, name) VALUES (6, 3, 'aired')",
		`INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order)
		 VALUES (6, 2, 1, 1, 1)`,
		"INSERT INTO version (id, title_id, created_at) VALUES (7, 1, 1)",
		`INSERT INTO extra_relation (owner_title, extra_title, extra_type, asserted_by)
		 VALUES (1, 4, 'trailer', 'scanner')`,
		"INSERT INTO title_watched (user_id, title_id, watched_at) VALUES (5, 1, 100), (5, 2, 200)")
	if err = database.migrate(t.Context(), steps[:step0074+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query   string
		columns int
		want    string
	}{
		{
			"SELECT id, kind, coalesce(runtime_ms, 0) FROM work ORDER BY id", 3,
			"[[1 movie 8160000] [2 episode 0] [4 extra 0]]",
		},
		{"SELECT id, kind, created_by FROM sequence", 3, "[[3 series scanner]]"},
		{"SELECT sequence_id, member_work, coalesce(member_sequence, 0) FROM sequence_member", 3, "[[3 2 0]]"},
		{"SELECT id, sequence_id, name FROM ordering", 3, "[[6 3 aired]]"},
		{"SELECT ordering_id, work_id, group_number, number_in_group FROM position", 4, "[[6 2 1 1]]"},
		{"SELECT id, work_id FROM version", 2, "[[7 1]]"},
		{"SELECT owner_work, extra_work, extra_type FROM extra_relation", 3, "[[1 4 trailer]]"},
		{"SELECT user_id, work_id, watched_at FROM work_watched ORDER BY work_id", 3, "[[5 1 100] [5 2 200]]"},
		{"SELECT id, name FROM user WHERE id = 5", 2, "[[5 dan]]"},
		{
			`SELECT name FROM sqlite_schema WHERE name IN ('title', 'collection', 'collection_member',
			  'title_watched', 'collection_member_title', 'position_title', 'version_title',
			  'collection_member_found_ad', 'collection_member_found_ai', 'collection_member_found_au')`, 1, "[]",
		},
		{
			`SELECT name FROM sqlite_schema WHERE name IN ('sequence_member_work', 'position_work', 'version_work',
			  'sequence_member_found_ad', 'sequence_member_found_ai', 'sequence_member_found_au') ORDER BY name`, 1,
			"[[position_work] [sequence_member_found_ad] [sequence_member_found_ai] [sequence_member_found_au] " +
				"[sequence_member_work] [version_work]]",
		},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s:\ngot  %s\nwant %s", check.query, got, check.want)
		}
	}
	if problems := fmt.Sprint(queryRows(t, database, "PRAGMA foreign_key_check", 4)); problems != "[]" {
		t.Errorf("foreign keys broken by the renames: %s", problems)
	}
}
