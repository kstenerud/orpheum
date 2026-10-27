// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// deleteFrom is a query's DELETE and the table it deletes from.
var deleteFrom = regexp.MustCompile(`(?is)\bDELETE\s+FROM\s+"?(\w+)"?`)

// childIndexesAllowedMissing are the foreign keys whose parent can be deleted, or that cascade,
// with no index on their child column, each with why. A new one fails until it is listed here.
func childIndexesAllowedMissing() map[string]string {
	return map[string]string{
		"user(role_id)": "deleting a role reads its people, and a household has a handful",
		"file(copy_of)": "file_copy holds the copies alone, WHERE copy_of IS NOT NULL, which the lookup of a " +
			"purged file's copies, copy_of = ?, implies; a full index would tell the planner every file is " +
			"a copy of one, the NULLs counted as one value (D363)",
	}
}

// A foreign key whose parent can be deleted, or whose action cascades or sets null, has an index
// on its child columns: deleting the parent looks up its children, and with no index reads the
// whole child table for each parent row (D303, DL3 as the independent audit amended it). A parent
// is deleted when a query deletes from its table.
func TestSchema_AForeignKeyWhoseParentCanGoHasAChildIndex(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	deleted := map[string]bool{}
	for _, directory := range []string{"queries", filepath.Join("cache", "queries")} {
		for _, query := range queriesIn(t, directory) {
			for _, match := range deleteFrom.FindAllStringSubmatch(query.sql, -1) {
				deleted[match[1]] = true
			}
		}
	}
	allowed := childIndexesAllowedMissing()
	tables := queryRows(t, database, "SELECT name FROM sqlite_schema WHERE type = 'table' "+
		"AND name NOT LIKE 'sqlite_%' AND name NOT GLOB 'item_fts*'", 1)
	for _, table := range tables {
		keys := queryRows(t, database, "SELECT \"table\", group_concat(\"from\", ','), on_delete "+
			"FROM pragma_foreign_key_list('"+table[0]+"') GROUP BY id", 3)
		for _, key := range keys {
			parent, columns, onDelete := key[0], key[1], key[2]
			if !deleted[parent] && onDelete != "CASCADE" && onDelete != "SET NULL" {
				continue
			}
			name := table[0] + "(" + columns + ")"
			_, isAllowed := allowed[name]
			delete(allowed, name)
			if !leadsAnIndex(t, database, table[0], strings.Split(columns, ",")) && !isAllowed {
				t.Errorf("%s -> %s (%s) has no index on its child columns: deleting a parent reads the whole "+
					"table; add one, or list it in childIndexesAllowedMissing with why", name, parent, onDelete)
			}
		}
	}
	for name := range allowed {
		t.Errorf("childIndexesAllowedMissing names %s, which needs no index or has one", name)
	}
}

// leadsAnIndex is whether some full index on table, or its primary key, begins with columns, in
// any order. A partial index does not count: a parent's delete cannot use one unless it implies
// its condition.
func leadsAnIndex(t *testing.T, database *Database, table string, columns []string) bool {
	t.Helper()
	slices.Sort(columns)
	for _, index := range queryRows(t, database, "SELECT name, partial FROM pragma_index_list('"+table+"')", 2) {
		if index[1] == "1" {
			continue
		}
		leading := queryRows(t, database, "SELECT name FROM pragma_index_info('"+index[0]+"') ORDER BY seqno", 1)
		if len(leading) < len(columns) {
			continue
		}
		var names []string
		for _, column := range leading[:len(columns)] {
			names = append(names, column[0])
		}
		slices.Sort(names)
		if slices.Equal(names, columns) {
			return true
		}
	}
	primary := queryRows(t, database, "SELECT name FROM pragma_table_info('"+table+"') WHERE pk > 0 ORDER BY pk", 1)
	if len(primary) >= len(columns) {
		var names []string
		for _, column := range primary[:len(columns)] {
			names = append(names, column[0])
		}
		slices.Sort(names)
		return slices.Equal(names, columns)
	}
	return false
}
