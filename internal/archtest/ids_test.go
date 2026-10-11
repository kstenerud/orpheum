// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"
)

// goldenDirectory is where persistent.db's schema goldens are, relative to internal/store.
const goldenDirectory = "testdata/golden"

// statementStart is where a schema's statement or comment begins: each starts a line.
var statementStart = regexp.MustCompile(`(?m)^(?:CREATE |INSERT |-- )`)

// createTable is a table a statement creates, and its definition. A table SQLite's FTS5 module
// creates for itself has its name quoted with ', and is not its own.
var createTable = regexp.MustCompile(`(?s)^CREATE TABLE "?(\w+)"? \((.*)`)

// rowIDColumn is a column that is the table's rowid: an INTEGER PRIMARY KEY, and the rest of its
// definition.
var rowIDColumn = regexp.MustCompile(`(?m)^\s*(\w+)\s+INTEGER PRIMARY KEY\b(.*)$`)

// neverGivenTwice is a rowid's definition that no other row can be given: AUTOINCREMENT, or one
// borrowed from the row it extends.
var neverGivenTwice = regexp.MustCompile(`\bAUTOINCREMENT\b|\bREFERENCES\b`)

// goldenName is a schema golden's file name, which says its version.
var goldenName = regexp.MustCompile(`^schema-v(\d+)\.sql$`)

// ownRowIDs are the tables whose rowid is not an id anything outside the database holds: the
// runner's one row, and its record of each step by number.
var ownRowIDs = []string{"schema_meta", "schema_step"}

// findReusableIDs lists each table of schema whose rowid SQLite may give again after the row
// holding it is deleted: an INTEGER PRIMARY KEY with neither AUTOINCREMENT, which never gives a
// number twice, nor REFERENCES, which takes the id another table gave.
func findReusableIDs(schema string) []violation {
	var found []violation
	for _, table := range createdTables(schema) {
		name, columns := table[1], table[2]
		if slices.Contains(ownRowIDs, name) {
			continue
		}
		for _, column := range rowIDColumn.FindAllStringSubmatch(columns, -1) {
			if neverGivenTwice.MatchString(column[2]) {
				continue
			}
			found = append(found, violation{
				where: name + "." + column[1],
				detail: "is an INTEGER PRIMARY KEY without AUTOINCREMENT: SQLite may give its number again " +
					"once the newest row is deleted, and an id the API gave out would name another row (D243 §5)",
			})
		}
	}
	return found
}

// createdTables are the tables schema creates, each its statement's match of createTable: the
// schema cut where each statement or comment begins, so one table's text never runs into the next.
func createdTables(schema string) [][]string {
	starts := statementStart.FindAllStringIndex(schema, -1)
	var tables [][]string
	for index, start := range starts {
		end := len(schema)
		if index+1 < len(starts) {
			end = starts[index+1][0]
		}
		if table := createTable.FindStringSubmatch(schema[start[0]:end]); table != nil {
			tables = append(tables, table)
		}
	}
	return tables
}

// newestGolden is the text of the newest schema golden in directory.
func newestGolden(t *testing.T, directory string) string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	newest, newestVersion := "", -1
	for _, entry := range entries {
		matched := goldenName.FindStringSubmatch(entry.Name())
		if matched == nil {
			continue
		}
		version, parseErr := strconv.Atoi(matched[1])
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if version > newestVersion {
			newest, newestVersion = entry.Name(), version
		}
	}
	if newest == "" {
		t.Fatalf("no schema golden in %s; the rule would check nothing", directory)
	}
	schema, err := os.ReadFile(filepath.Join(directory, newest))
	if err != nil {
		t.Fatal(err)
	}
	return string(schema)
}

// TestArch_IDsAreNeverGivenTwice holds that an id persistent.db gives a row, which the API gives out
// and another server may hold (D243 §5), is never given to another row: every table's own rowid is
// AUTOINCREMENT, or borrowed from the row it extends.
func TestArch_IDsAreNeverGivenTwice(t *testing.T) {
	t.Parallel()
	var storeDirectory string
	for _, listed := range loadModulePackages(t) {
		if listed.ImportPath == storePath {
			storeDirectory = listed.Dir
		}
	}
	if storeDirectory == "" {
		t.Fatalf("go list did not find %s", storePath)
	}
	schema := newestGolden(t, filepath.Join(storeDirectory, goldenDirectory))
	if len(createdTables(schema)) == 0 {
		t.Fatal("the newest golden creates no table this rule can read; it would pass vacuously")
	}
	reportViolations(t, findReusableIDs(schema))
}
