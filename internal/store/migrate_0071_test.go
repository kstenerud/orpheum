// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0071 drops the schema nothing writes or reads (D426 §8): the versions, extras and files it
// rebuilds keep their rows and their id sequences, and the tables, columns, index and views it names
// are gone.
func TestMigrate_Step0071DropsTheDeadSchemaAndKeepsEveryRow(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0071 := stepIndex(t, steps, 71)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0071], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO item (id, kind, added_at) VALUES (1, 'title', 1), (2, 'title', 1)",
		`INSERT INTO title (id, kind, identification_state)
		 VALUES (1, 'movie', 'identified'), (2, 'extra', 'identified')`,
		"INSERT INTO version (id, title_id, created_at) VALUES (7, 1, 1)",
		`INSERT INTO extra_relation (owner_title, extra_title, extra_type, asserted_by)
		 VALUES (1, 2, 'trailer', 'scanner')`,
		`INSERT INTO file (id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at, removed_at)
		 VALUES (8, 1, '/m/a.mkv', CAST('/m/a.mkv' AS BLOB), 'video', 1, 2, NULL),
		        (9, 1, '/m/b.mkv', CAST('/m/b.mkv' AS BLOB), 'skipped', 1, 2, 3)`)
	if err = database.migrate(t.Context(), steps[:step0071+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query   string
		columns int
		want    string
	}{
		{"SELECT id, title_id FROM version", 2, "[[7 1]]"},
		{"SELECT owner_title, extra_title, extra_type FROM extra_relation", 3, "[[1 2 trailer]]"},
		{
			"SELECT id, path, filing, coalesce(removed_at, 0) FROM file ORDER BY id", 4,
			"[[8 /m/a.mkv video 0] [9 /m/b.mkv skipped 3]]",
		},
		{`SELECT name FROM sqlite_schema WHERE name IN ('edition', 'edition_text', 'tag', 'item_tag', 'user_deny',
		  'favourite', 'user_rating', 'watchlist', 'fetched_subtitle_video', 'live_item', 'live_file')`, 1, "[]"},
		{`SELECT name FROM pragma_table_info('title') WHERE name IN ('original_language', 'media_kind')
		  UNION ALL SELECT name FROM pragma_table_info('collection') WHERE name IN ('predicate', 'membership')
		  UNION ALL SELECT name FROM pragma_table_info('file') WHERE name = 'is_identity_confirmed'
		  UNION ALL SELECT name FROM pragma_table_info('version') WHERE name = 'edition_id'
		  UNION ALL SELECT name FROM pragma_table_info('item') WHERE name = 'origin_server'`, 1, "[]"},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s after the step: %s; want %s", check.query, got, check.want)
		}
	}
	execRaw(t, database, "INSERT INTO version (title_id, created_at) VALUES (1, 2)",
		`INSERT INTO file (node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at)
		 VALUES (1, '/m/c.mkv', CAST('/m/c.mkv' AS BLOB), 'video', 1, 1)`)
	next := fmt.Sprint(queryRows(t, database, `SELECT (SELECT max(id) FROM version), (SELECT max(id) FROM file)`, 2))
	if next != "[[8 10]]" {
		t.Errorf("the next version's and file's ids: %s; want [[8 10]], past the highest each sequence gave", next)
	}
}
