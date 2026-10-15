// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"slices"
	"strings"
	"testing"
)

// A migrated, empty database keeps every invariant, and each one invariants.sql names is read.
func TestCheckInvariants_AnEmptyDatabaseKeepsThemAll(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	violations, err := database.CheckInvariants(t.Context())
	if err != nil || len(violations) != 0 {
		t.Errorf("an empty database: %+v, %v; want no violation", violations, err)
	}
	if len(invariants()) < 2 {
		t.Errorf("invariants.sql yields %d invariants; want each of its queries", len(invariants()))
	}
}

// An invariant that does not hold is named with the rows where it fails, and so are a dangling
// foreign key and a full-text index that disagrees with its text (D303, M7).
func TestCheckInvariants_NamesEachThatDoesNotHold(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		"INSERT INTO item (id, kind, added_at) VALUES (7, 'collection', 1)",
		"INSERT INTO title (id, kind, identification_state) VALUES (7, 'movie', 'unidentified')",
		"PRAGMA foreign_keys = OFF",
		"INSERT INTO item (id, kind, library_id, added_at) VALUES (8, 'title', 999, 1)",
		"PRAGMA foreign_keys = ON",
		"INSERT INTO item_fts (rowid, name, search_alternates, overview) VALUES (42, 'ghost', 'ghost', '')",
	)
	violations, err := database.CheckInvariants(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var named []string
	for _, violation := range violations {
		named = append(named, violation.Invariant+": "+strings.Join(violation.Rows, ","))
	}
	slices.Sort(named)
	want := []string{
		"every foreign key names a row that exists", "item_fts agrees",
		"a title's subtype row is a title item, and a collection's a collection item: 7",
	}
	for _, prefix := range want {
		if !slices.ContainsFunc(named, func(each string) bool { return strings.HasPrefix(each, prefix) }) {
			t.Errorf("violations %q; want one starting %q", named, prefix)
		}
	}
}
