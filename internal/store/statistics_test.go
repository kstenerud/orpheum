// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// The committed statistics are current: each row names a table and an index the schema has, and a
// table with statistics has a row for each of its indexes, so an index added since they were
// written fails until they are written again (D303 §3, DL15).
func TestStatistics_NameTheSchemasIndexesAndNoOthers(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	rows, err := parseStatistics(persistentStatistics)
	if err != nil || len(rows) == 0 {
		t.Fatalf("the committed statistics: %d rows, %v", len(rows), err)
	}
	indexesOf := schemaIndexes(t, database)
	tables := map[string]bool{}
	for _, table := range queryRows(t, database, "SELECT name FROM sqlite_schema WHERE type = 'table'", 1) {
		tables[table[0]] = true
	}
	covered := map[string]map[string]bool{}
	for _, row := range rows {
		if !tables[row.table] {
			t.Errorf("the statistics name table %s, which the schema has not", row.table)
		}
		if row.index != "" && !strings.HasPrefix(row.index, "sqlite_autoindex_") && !indexesOf[row.table][row.index] &&
			row.index != row.table {
			t.Errorf("the statistics name index %s on %s, which the schema has not", row.index, row.table)
		}
		if covered[row.table] == nil {
			covered[row.table] = map[string]bool{}
		}
		covered[row.table][row.index] = true
	}
	for table, indexes := range covered {
		for index := range indexesOf[table] {
			if !indexes[index] {
				t.Errorf("the statistics have %s but not its index %s; write them again with go test -tags "+
					"integration -run TestReferenceScale ./internal/store -update-statistics", table, index)
			}
		}
	}
}

// A reader plans by the committed statistics, though it opened before they were loaded: SQLite
// gives them only to a connection opened after them, so the loader closes the idle readers (D303
// §3). By them an item's shown artwork starts from the artwork, a few rows an item, where a planner
// without them starts from the item (the series list, which read the few series by them before
// step 0072, now walks sort_key either way: no table of series alone is left to read).
func TestStatistics_AReaderOpenedBeforeThemPlansByThem(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	for _, query := range queriesIn(t, "queries") {
		if query.name != "ListShownArtwork" {
			continue
		}
		plan := fmt.Sprint(planOf(t, database.readers, query))
		if !strings.HasPrefix(plan, "[SEARCH artwork") {
			t.Errorf("a reader plans an item's artwork as %s; want it to start from the artwork, as the "+
				"statistics say", plan)
		}
		return
	}
	t.Fatal("no query ListShownArtwork")
}

// schemaIndexes are the schema's own indexes, by table.
func schemaIndexes(t *testing.T, database *Database) map[string]map[string]bool {
	t.Helper()
	indexesOf := map[string]map[string]bool{}
	for _, index := range queryRows(t, database, "SELECT tbl_name, name FROM sqlite_schema WHERE type = 'index' "+
		"AND name NOT LIKE 'sqlite_autoindex_%'", 2) {
		if indexesOf[index[0]] == nil {
			indexesOf[index[0]] = map[string]bool{}
		}
		indexesOf[index[0]][index[1]] = true
	}
	return indexesOf
}
