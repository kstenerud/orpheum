// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0072 puts an item's kind and the columns every kind shares on the item (D426 §2): each item
// takes its subtype's kind, a title's and a series' year, release date, source, identification and
// found times move to it (here an episode's file's, and its series' by it), a series not yet
// resolved becomes unidentified, a person keeps its row and no identification, a sort key stays
// only where its listing is its item's kind, and the item's id sequence survives. After it, a kind
// changed on the item follows to the subtype and the sort key.
func TestMigrate_Step0072MovesTheKindAndSharedColumnsToTheItem(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0072 := stepIndex(t, steps, 72)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0072], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		`INSERT INTO item (id, kind, added_at, removed_at) VALUES (1, 'title', 1, NULL), (2, 'title', 1, NULL),
		 (3, 'collection', 1, NULL), (4, 'collection', 1, NULL), (5, 'person', 1, NULL), (6, 'title', 1, 9)`,
		`INSERT INTO title (id, kind, year, released_on, followed_source, followed_source_id,
		   identification_state, runtime_ms)
		 VALUES (1, 'movie', 1999, 19990331, 'tmdb', '4801', 'confirmed', 8160000)`,
		`INSERT INTO title (id, kind, year, released_on, identification_state)
		 VALUES (2, 'episode', 1999, 20000101, 'unidentified')`,
		"INSERT INTO title (id, kind, followed_source, identification_state) VALUES (6, 'other', 'none', 'ambiguous')",
		`INSERT INTO collection (id, kind, created_by, year, released_on, followed_source, followed_source_id,
		   identification_state)
		 VALUES (3, 'series', 'scanner', 2008, 20080120, 'tvdb', '81189', 'identified')`,
		"INSERT INTO collection (id, kind, created_by) VALUES (4, 'series', 'scanner')",
		"INSERT INTO person (id, tmdb_id, name_key, picture_ref) VALUES (5, '6384', 'keanu reeves', '/k.jpg')",
		`INSERT INTO sort_key (item_id, locale, listing, collation_key)
		 VALUES (1, 'en', 'movie', x'01'), (3, 'en', 'series', x'03'), (2, 'en', 'movie', x'02')`,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO version (id, title_id, created_at) VALUES (7, 2, 1)",
		`INSERT INTO file (id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at, modified_at)
		 VALUES (8, 1, '/s/e.mkv', CAST('/s/e.mkv' AS BLOB), 'video', 50, 50, 40)`,
		"INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES (8, 'series', 'listed', 7)",
		"INSERT INTO collection_member (collection_id, member_title, position) VALUES (3, 2, 1)",
		"INSERT INTO item (id, kind, added_at) VALUES (8, 'person', 1)",
		"DELETE FROM item WHERE id = 8")
	if err = database.migrate(t.Context(), steps[:step0072+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query   string
		columns int
		want    string
	}{
		{
			`SELECT id, kind, coalesce(year, 0), coalesce(released_on, 0), coalesce(followed_source, ''),
			   coalesce(followed_source_id, ''), coalesce(identification_state, ''), found_at,
			   found_file_modified_at, coalesce(removed_at, 0)
			 FROM item ORDER BY id`, 10,
			"[[1 movie 1999 19990331 tmdb 4801 confirmed 0 0 0] " +
				"[2 episode 1999 0   unidentified 50 40 0] " +
				"[3 series 2008 20080120 tvdb 81189 identified 50 40 0] " +
				"[4 series 0 0   unidentified 0 0 0] " +
				"[5 person 0 0    0 0 0] " +
				"[6 other 0 0 none  ambiguous 0 0 9]]",
		},
		{
			"SELECT id, kind, coalesce(runtime_ms, 0) FROM title ORDER BY id", 3,
			"[[1 movie 8160000] [2 episode 0] [6 other 0]]",
		},
		{"SELECT id, kind, created_by FROM collection ORDER BY id", 3, "[[3 series scanner] [4 series scanner]]"},
		{"SELECT id, kind, tmdb_id, name_key, picture_ref FROM person", 5, "[[5 person 6384 keanu reeves /k.jpg]]"},
		{"SELECT item_id, listing FROM sort_key ORDER BY item_id", 2, "[[1 movie] [3 series]]"},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s after the step: %s; want %s", check.query, got, check.want)
		}
	}
	execRaw(t, database, "UPDATE item SET kind = 'extra' WHERE id = 1",
		"INSERT INTO item (kind, added_at, identification_state) VALUES ('movie', 2, 'unidentified')")
	followed := fmt.Sprint(queryRows(t, database,
		"SELECT (SELECT kind FROM title WHERE id = 1), (SELECT listing FROM sort_key WHERE item_id = 1)", 2))
	if followed != "[[extra extra]]" {
		t.Errorf("a title's kind and its sort key's listing after its item became an extra: %s; want [[extra extra]]",
			followed)
	}
	next := fmt.Sprint(queryRows(t, database, "SELECT max(id) FROM item", 1))
	if next != "[[9]]" {
		t.Errorf("the next item's id: %s; want [[9]], past the highest the sequence gave", next)
	}
}
