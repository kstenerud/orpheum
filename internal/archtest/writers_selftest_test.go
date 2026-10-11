// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"maps"
	"testing"
)

// The writer rule's parts, each seen to fail on input that breaks it (D190 §3).

func TestFindWriteQueries_FindsTheTableEachWriteNames(t *testing.T) {
	t.Parallel()
	queries := "-- name: GetFile :one\n-- An UPDATE in a comment writes nothing.\n" +
		"SELECT id FROM file WHERE id = ?;\n\n" +
		"-- name: InsertGrant :exec\nINSERT INTO \"grant\" (user_id) VALUES (?);\n\n" +
		"-- name: KeepNode :exec\nINSERT OR IGNORE INTO node (name) VALUES (?);\n\n" +
		"-- name: RetireArtwork :exec\nUPDATE artwork SET removed_at = ? WHERE id IN (SELECT id FROM item);\n\n" +
		"-- name: DeleteSortKeys :exec\nDELETE FROM sort_key WHERE item_id = ?;\n"
	got := findWriteQueries([]sourceFile{{path: "a.sql", contents: queries}}, sqlcgenPath)
	want := map[string]string{
		sqlcgenPath + ".InsertGrant":    "grant",
		sqlcgenPath + ".KeepNode":       "node",
		sqlcgenPath + ".RetireArtwork":  "artwork",
		sqlcgenPath + ".DeleteSortKeys": "sort_key",
	}
	if !maps.Equal(got, want) {
		t.Errorf("write queries = %v, want %v", got, want)
	}
}

func TestReadTableOwners_ReadsEveryRowOfTheOwnersTable(t *testing.T) {
	t.Parallel()
	plan := "| Table | Database | Writing package | Notes |\n| --- | --- | --- | --- |\n" +
		"| `file` | persistent.db | `internal/core/library` | |\n" +
		"| `item_fts`, `item_fts_tri` | persistent.db | `internal/core/identify` | triggers |\n" +
		"| `tag` | persistent.db | none yet | |\n" +
		"| `job` | cache.db | `internal/core/jobs` | |\n"
	want := map[string]string{
		"file": "internal/core/library", "item_fts": "internal/core/identify",
		"item_fts_tri": "internal/core/identify", "tag": "", "job": "internal/core/jobs",
	}
	if got := readTableOwners(plan); !maps.Equal(got, want) {
		t.Errorf("owners = %v, want %v", got, want)
	}
}

func TestFindTablesWithoutOneWriter_CatchesASecondWriterAWrongOneAndAnUnlistedTable(t *testing.T) {
	t.Parallel()
	tableByQuery := map[string]string{
		sqlcgenPath + ".SetFileFiling": "file", sqlcgenPath + ".InsertFile": "file",
		sqlcgenPath + ".InsertTag": "tag", cachegenPath + ".InsertOther": "other",
	}
	owners := map[string]string{"file": "internal/core/library", "tag": ""}
	clean := map[string][]string{
		corePath + "/library":  {sqlcgenPath + ".InsertFile", sqlcgenPath + ".SetFileFiling"},
		corePath + "/identify": {sqlcgenPath + ".GetFile"},
	}
	if got := findTablesWithoutOneWriter(clean, tableByQuery, owners); len(got) != 0 {
		t.Errorf("one writer, the owner, and a read elsewhere: got %v, want none", got)
	}
	cases := map[string]map[string][]string{
		"a second writer": {
			corePath + "/library":  {sqlcgenPath + ".InsertFile"},
			corePath + "/identify": {sqlcgenPath + ".SetFileFiling"},
		},
		"a writer that is not the owner":    {corePath + "/identify": {sqlcgenPath + ".SetFileFiling"}},
		"a writer of a table with none yet": {corePath + "/auth": {sqlcgenPath + ".InsertTag"}},
		"a table the owners table lacks":    {corePath + "/auth": {cachegenPath + ".InsertOther"}},
	}
	for name, queriesByPackage := range cases {
		if got := findTablesWithoutOneWriter(queriesByPackage, tableByQuery, owners); len(got) != 1 {
			t.Errorf("%s: got %v, want one violation", name, got)
		}
	}
}
