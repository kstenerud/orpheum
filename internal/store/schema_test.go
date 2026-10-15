// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// An item's sort key is read by its item and locale, one key per item in each locale (D303; the
// access audit's A1).
func TestSchema_ASortKeyIsOnePerItemInEachLocaleAndReadByIt(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		"INSERT INTO item (id, kind, added_at) VALUES (7, 'title', 1)",
		"INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (7, 'en', 'movie', x'01')")
	const second = "INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (7, 'en', 'series', x'02')"
	if _, err := database.writer.ExecContext(t.Context(), second); err == nil {
		t.Error("a second key for the item in the locale was kept")
	}
	plan := fmt.Sprint(queryRows(t, database,
		"EXPLAIN QUERY PLAN SELECT listing FROM sort_key WHERE item_id = 7 AND locale = 'en'", 4))
	if !strings.Contains(plan, "PRIMARY KEY (item_id=? AND locale=?)") {
		t.Errorf("an item's key is read by %s; want its primary key, by item and locale", plan)
	}
}

// writerChangesOf is how many rows update changes on the writer, what its triggers write included.
func writerChangesOf(ctx context.Context, t *testing.T, database *Database, update string) int {
	t.Helper()
	before := writerChanges(ctx, t, database)
	if _, err := database.writer.ExecContext(ctx, update); err != nil {
		t.Fatalf("exec %q: %v", update, err)
	}
	return writerChanges(ctx, t, database) - before
}

// An item's text is found by both full-text indexes under its own key, and an update that changes
// nothing they index rewrites neither (D303, DL9; SQLite fires an UPDATE trigger whatever the
// values, and total_changes counts what a trigger writes).
func TestSchema_TheTextIndexesAreRewrittenOnlyOnChange(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	ctx := t.Context()
	execRaw(t, database,
		"INSERT INTO item (id, kind, added_at) VALUES (7, 'title', 1)",
		"INSERT INTO item_text (item_id, language, is_original, name, search_alternates) "+
			"VALUES (7, 'und', 1, 'The General', 'solaris')")
	found := fmt.Sprint(queryRows(t, database,
		"SELECT item_text.item_id FROM item_fts JOIN item_text ON item_text.id = item_fts.rowid "+
			"WHERE item_fts MATCH 'solaris'", 1),
		queryRows(t, database, "SELECT item_text.item_id FROM item_fts_tri JOIN item_text ON item_text.id = "+
			"item_fts_tri.rowid WHERE item_fts_tri MATCH 'olar'", 1))
	if found != "[[7]] [[7]]" {
		t.Errorf("the indexes find %s; want the text in both", found)
	}
	unchanged := writerChangesOf(ctx, t, database, "UPDATE item_text SET name = name, "+
		"search_alternates = search_alternates, overview = overview WHERE item_id = 7")
	overviewOnly := writerChangesOf(ctx, t, database, "UPDATE item_text SET overview = 'A planet.' WHERE item_id = 7")
	renamed := writerChangesOf(ctx, t, database,
		"UPDATE item_text SET name = 'Solyaris', search_alternates = 'solyaris' WHERE item_id = 7")
	if unchanged != 1 || overviewOnly <= unchanged || overviewOnly >= renamed {
		t.Errorf("an unchanged update made %d changes, an overview's %d, a rename %d; want the row alone, then "+
			"the word index rewritten, then both indexes: each more than the last", unchanged, overviewOnly, renamed)
	}
}

// Each index for a path a query takes serves it: a removed payload by library, a library's files by
// path, the errored files, a title's series, a device's grants, every version of a title, and the
// titles waiting for a person (D303, DL1, DL4).
func TestSchema_EachPathsIndexServesIt(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	for query, index := range map[string]string{
		"SELECT id FROM file WHERE library_id = 1 AND payload_hash = x'00' AND removed_at IS NOT NULL " +
			"ORDER BY removed_at DESC LIMIT 1": "file_removed_payload",
		"SELECT id FROM file WHERE library_id = 1 AND removed_at IS NULL ORDER BY path": "file_library",
		"SELECT id FROM file WHERE filing = 'errored' AND removed_at IS NULL " +
			"ORDER BY library_id, path": "file_errored",
		"SELECT collection_id FROM collection_member WHERE member_title = 1": "collection_member_title",
		`SELECT secret_sha256 FROM "grant" WHERE device_id = 1`:              "grant_device",
		"UPDATE version SET title_id = 2 WHERE title_id = 1":                 "version_title",
		"SELECT id FROM title WHERE identification_state = 'unidentified' AND followed_source = 'none' " +
			"AND kind != 'episode'": "title_waiting",
	} {
		plan := fmt.Sprint(queryRows(t, database, "EXPLAIN QUERY PLAN "+query, 4))
		if !strings.Contains(plan, index) {
			t.Errorf("%s reads by %s; want %s", query, plan, index)
		}
	}
}

// A title and a collection are each an item of their own kind, an id at a source has its source, and
// a library's check interval is for checking by interval alone: the engine refuses each otherwise
// (D303, M5).
func TestSchema_RefusesRowsThatBreakTheirRules(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		"INSERT INTO item (id, kind, added_at) VALUES (7, 'collection', 1), (8, 'title', 1)")
	for what, statement := range map[string]string{
		"a title row on a collection item": "INSERT INTO title (id, kind, identification_state) " +
			"VALUES (7, 'movie', 'unidentified')",
		"an id with no source": "INSERT INTO title (id, kind, identification_state, followed_source_id) " +
			"VALUES (8, 'movie', 'identified', '603')",
		"hours for a manual library": "INSERT INTO library (name, kind, metadata_source, change_detection, " +
			"check_every_hours, is_writable, created_at) VALUES ('Films', 'movies', 'nfo', 'manual', 6, 0, 1)",
		"text in an integer column": "INSERT INTO item (kind, added_at) VALUES ('title', 'yesterday')",
	} {
		if _, err := database.writer.ExecContext(t.Context(), statement); err == nil {
			t.Errorf("%s was kept", what)
		}
	}
}

// Every table is STRICT, so a value of the wrong type is refused rather than kept as another type
// that never compares equal: a hash given as hex text, a time as words (D303, M5; the independent
// audit's S8). FTS5's own tables and SQLite's are not ours to declare.
func TestSchema_EveryTableIsStrict(t *testing.T) {
	t.Parallel()
	for _, database := range []*Database{openMigrated(t), openCacheAt(t, t.TempDir()+"/cache.db").database} {
		tables := queryRows(t, database, "SELECT name, strict FROM pragma_table_list "+
			"WHERE schema = 'main' AND type = 'table' AND name NOT LIKE 'sqlite_%' "+
			"AND name NOT GLOB 'item_fts*'", 2)
		if len(tables) == 0 {
			t.Fatal("no table found; the test reads nothing")
		}
		for _, table := range tables {
			if table[1] != "1" {
				t.Errorf("%s (%s) is not STRICT", table[0], database.path)
			}
		}
	}
}
