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
	if len(invariants()) < 1 {
		t.Errorf("invariants.sql yields %d invariants; want each of its queries", len(invariants()))
	}
}

// An invariant that does not hold is named with the rows where it fails, and so are a dangling
// foreign key and a full-text index that disagrees with its text (D303, M7).
func TestCheckInvariants_NamesEachThatDoesNotHold(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		"PRAGMA foreign_keys = OFF",
		"INSERT INTO item (id, kind, added_at, removed_at, identification_state) "+
			"VALUES (7, 'movie', 1, 2, 'unidentified')",
		"INSERT INTO work (id, kind) VALUES (7, 'movie')",
		"INSERT INTO version (id, work_id, created_at) VALUES (5, 7, 1)",
		"INSERT INTO file (id, node_id, path, filing, first_seen_at, last_seen_at) "+
			"VALUES (1, 1, '/m/f.mkv', 'video', 1, 1)",
		"INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES (1, 'movie', 'listed', 5)",
		"PRAGMA foreign_keys = ON",
		"PRAGMA foreign_keys = OFF",
		"INSERT INTO item (id, kind, filing_library_id, added_at, identification_state) "+
			"VALUES (8, 'movie', 999, 1, 'identified')",
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
		"no live version belongs to a removed title: 5",
	}
	for _, prefix := range want {
		if !slices.ContainsFunc(named, func(each string) bool { return strings.HasPrefix(each, prefix) }) {
			t.Errorf("violations %q; want one starting %q", named, prefix)
		}
	}
}
