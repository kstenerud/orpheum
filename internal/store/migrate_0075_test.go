// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0075 stores a library's roots, each root's scan, state and mounts, the fields a person set
// by hand and an extra's tie to its work by their key alone (WITHOUT ROWID, D426 §10), and gives
// a field set by hand, a library, a user, a role and a setting a revision (sql.md M1). Every row
// is kept, at revision 0, a person's own state among them.
func TestMigrate_Step0075KeepsEveryRowWithoutTheRowidAndAtRevisionZero(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0075 := stepIndex(t, steps, 75)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0075], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO role (id, name, capabilities) VALUES (99, 'step 0075', '[]')",
		"INSERT INTO user (id, name, role_id, created_at) VALUES (5, 'dan', 99, 1)",
		"INSERT INTO node (id, name, enrolled_at) VALUES (3, 'nas', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (2, 'Films', 'movie', 'tmdb', 'watch', 0, 1)`,
		"INSERT INTO library_root (library_id, node_id, root_path) VALUES (2, 3, '/media/films')",
		"INSERT INTO root_scan (node_id, root_path, completed_at) VALUES (3, '/media/films', 40)",
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (3, '/media/films', 'complete', 40)`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (3, '/media/films', '/media/films', 77, 1)`,
		`INSERT INTO item (id, kind, identification_state, added_at) VALUES (1, 'movie', 'identified', 1),
		 (4, 'extra', 'identified', 1)`,
		"INSERT INTO work (id, kind) VALUES (1, 'movie'), (4, 'extra')",
		`INSERT INTO field_override (item_id, field, override_value, origin, set_by_user_id, set_at)
		 VALUES (1, 'title', 'Wing', 'typed', 5, 50)`,
		`INSERT INTO extra_relation (owner_work, extra_work, extra_type, asserted_by)
		 VALUES (1, 4, 'trailer', 'scanner')`,
		"INSERT INTO setting (key, value_json, set_by_user_id, set_at) VALUES ('step.0075', '1', 5, 60)",
		"INSERT INTO preference (user_id, scope, axis, value_json, set_at) VALUES (5, 'user', 'subtitles', 'true', 70)",
		"INSERT INTO work_watched (user_id, work_id, watched_at) VALUES (5, 1, 100)")
	if err = database.migrate(t.Context(), steps[:step0075+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query   string
		columns int
		want    string
	}{
		{"SELECT library_id, node_id, root_path FROM library_root", 3, "[[2 3 /media/films]]"},
		{"SELECT node_id, root_path, completed_at FROM root_scan", 3, "[[3 /media/films 40]]"},
		{"SELECT node_id, last_scan_outcome, last_scan_at FROM root_state", 3, "[[3 complete 40]]"},
		{"SELECT mount_path, filesystem_device, is_present FROM root_mount", 3, "[[/media/films 77 1]]"},
		{
			"SELECT item_id, field, override_value, set_by_user_id, revision FROM field_override", 5,
			"[[1 title Wing 5 0]]",
		},
		{"SELECT owner_work, extra_work, extra_type FROM extra_relation", 3, "[[1 4 trailer]]"},
		{"SELECT name, revision FROM library", 2, "[[Films 0]]"},
		{"SELECT name, role_id, revision FROM user WHERE id = 5", 3, "[[dan 99 0]]"},
		{"SELECT name, revision FROM role WHERE id = 99", 2, "[[step 0075 0]]"},
		{"SELECT value_json, set_by_user_id, revision FROM setting WHERE key = 'step.0075'", 3, "[[1 5 0]]"},
		{"SELECT axis, value_json FROM preference WHERE user_id = 5", 2, "[[subtitles true]]"},
		{"SELECT work_id, watched_at FROM work_watched WHERE user_id = 5", 2, "[[1 100]]"},
		{
			`SELECT name FROM pragma_table_list WHERE wr = 1 AND name IN ('library_root', 'root_scan',
			  'root_state', 'root_mount', 'field_override', 'extra_relation') ORDER BY name`, 1,
			"[[extra_relation] [field_override] [library_root] [root_mount] [root_scan] [root_state]]",
		},
		{"SELECT name FROM sqlite_schema WHERE name = 'library_root_node'", 1, "[[library_root_node]]"},
		{"SELECT name FROM sqlite_temp_schema", 1, "[]"},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s:\ngot  %s\nwant %s", check.query, got, check.want)
		}
	}
	if problems := fmt.Sprint(queryRows(t, database, "PRAGMA foreign_key_check", 4)); problems != "[]" {
		t.Errorf("foreign keys broken by the rebuilds: %s", problems)
	}
}
