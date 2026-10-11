// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"slices"
	"testing"
)

// The id rule's checker, seen to fail on a schema that breaks it (D190 §3).
func TestFindReusableIDs_FindsARowIDThatIsNeitherAutoincrementNorBorrowed(t *testing.T) {
	t.Parallel()
	schema := "-- table item\nCREATE TABLE item (\n  id INTEGER PRIMARY KEY AUTOINCREMENT,\n" +
		"  kind TEXT CHECK (kind IN ('a',\n'b')\n)\n);\n\n-- table title\n" +
		"CREATE TABLE title (\n  id INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT\n);\n" +
		"CREATE TABLE tag (\n  id   INTEGER PRIMARY KEY,\n  name TEXT\n);\n" +
		"CREATE TABLE schema_step (\n  version INTEGER PRIMARY KEY,\n  sha256 TEXT\n);\n" +
		"CREATE TABLE 'item_fts_data'(id INTEGER PRIMARY KEY, block BLOB);\n" +
		"CREATE TABLE pair (\n  a INTEGER NOT NULL,\n  PRIMARY KEY (a)\n) WITHOUT ROWID;\n"
	var where []string
	for _, found := range findReusableIDs(schema) {
		where = append(where, found.where)
	}
	if !slices.Equal(where, []string{"tag.id"}) {
		t.Errorf("reusable ids = %v, want [tag.id] alone", where)
	}
}
