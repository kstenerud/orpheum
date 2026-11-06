// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/rowstest"
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
		rowstest.Title(rowstest.ItemSpec{ID: 7, RemovedAt: 2, IdentificationState: "unidentified"}),
		rowstest.Version(rowstest.VersionSpec{ID: 5, TitleID: 7}),
		rowstest.File(rowstest.FileSpec{ID: 1, NodeID: 1, Path: "/m/f.mkv", LibraryKind: "movie", VersionID: 5}),
		"PRAGMA foreign_keys = ON",
		"PRAGMA foreign_keys = OFF",
		rowstest.Item(rowstest.ItemSpec{ID: 8, LibraryID: 999}),
		"PRAGMA foreign_keys = ON",
		"INSERT INTO item_fts (rowid, name, search_alternates, overview) VALUES (42, 'ghost', 'ghost', '')",
		rowstest.Series(rowstest.ItemSpec{ID: 9, IdentificationState: "identified"}),
		rowstest.Ordering(rowstest.OrderingSpec{ID: 1, SeriesID: 9}),
		rowstest.Ordering(rowstest.OrderingSpec{ID: 2, SeriesID: 9, Name: "tmdb:dvd"}),
		"INSERT INTO grp (id, ordering_id, number) VALUES (1, 2, 1)",
		rowstest.Position(rowstest.PositionSpec{OrderingID: 1, TitleID: 7, GroupID: 1, SortOrder: 1}),
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
		"a group is its ordering's, and a season's poster its series' aired order's: position",
	}
	for _, prefix := range want {
		if !slices.ContainsFunc(named, func(each string) bool { return strings.HasPrefix(each, prefix) }) {
			t.Errorf("violations %q; want one starting %q", named, prefix)
		}
	}
}
