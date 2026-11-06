// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// foundShelf fills a library with what newest first orders: films 1 to 4 and a series, 10, of two
// episodes, 11 and 12, each with a file found by a listing at first_seen_at, with its own time.
func foundShelf(t *testing.T, database *Database) {
	t.Helper()
	execRaw(t, database,
		rowstest.Node(rowstest.NodeSpec{ID: 1}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1, Name: "Everything", NodeID: 1, RootPath: "/"}),
		rowstest.Title(rowstest.ItemSpec{ID: 1, LibraryID: 1, AddedAt: 4}),
		rowstest.Title(rowstest.ItemSpec{ID: 2, LibraryID: 1, AddedAt: 3}),
		rowstest.Title(rowstest.ItemSpec{ID: 3, LibraryID: 1, AddedAt: 2}),
		rowstest.Title(rowstest.ItemSpec{ID: 4, LibraryID: 1, AddedAt: 1}),
		rowstest.Series(rowstest.ItemSpec{ID: 10, LibraryID: 1, AddedAt: 9}),
		rowstest.Title(rowstest.ItemSpec{ID: 11, Kind: "episode", LibraryID: 1, AddedAt: 9}),
		rowstest.Title(rowstest.ItemSpec{ID: 12, Kind: "episode", LibraryID: 1, AddedAt: 9}),
		rowstest.Series(rowstest.ItemSpec{ID: 20, LibraryID: 1}),
		rowstest.Title(rowstest.ItemSpec{ID: 21, Kind: "episode", LibraryID: 1}),
		rowstest.Ordering(rowstest.OrderingSpec{ID: 1, SeriesID: 10, Kind: "aired"}),
		rowstest.Ordering(rowstest.OrderingSpec{ID: 2, SeriesID: 20, Kind: "aired"}),
		rowstest.Position(rowstest.PositionSpec{OrderingID: 1, TitleID: 11, SortOrder: 1}),
		rowstest.Position(rowstest.PositionSpec{OrderingID: 1, TitleID: 12, SortOrder: 2}),
		rowstest.Position(rowstest.PositionSpec{OrderingID: 2, TitleID: 21, SortOrder: 1}),
		rowstest.Version(rowstest.VersionSpec{ID: 1, TitleID: 1}),
		rowstest.Version(rowstest.VersionSpec{ID: 2, TitleID: 2}),
		rowstest.Version(rowstest.VersionSpec{ID: 3, TitleID: 3}),
		rowstest.Version(rowstest.VersionSpec{ID: 4, TitleID: 4}),
		rowstest.Version(rowstest.VersionSpec{ID: 11, TitleID: 11}),
		rowstest.Version(rowstest.VersionSpec{ID: 12, TitleID: 12}),
		rowstest.Version(rowstest.VersionSpec{ID: 21, TitleID: 21}),
		// The first scan, at 100, found films 1 to 3 and the series' first episode; the second, at 200,
		// film 4, whose file is older than any, and the other series' episode.
		foundFile(1, "/m/1.mkv", 30, 100),
		foundFile(2, "/m/2.mkv", 50, 100),
		foundFile(3, "/m/3.mkv", 50, 100),
		foundFile(4, "/m/4.mkv", 5, 200),
		foundFile(11, "/s/1.mkv", 70, 100),
		foundFile(21, "/t/1.mkv", 1, 200))
}

// foundFile is a file a listing found at foundAt, modified at modifiedAt, listed as the version
// whose id is its own.
func foundFile(id int64, path string, modifiedAt, foundAt int64) string {
	return rowstest.File(rowstest.FileSpec{
		ID: id, NodeID: 1, Path: path, ModifiedAt: modifiedAt, FirstSeenAt: foundAt,
		LibraryKind: "movie", VersionID: id,
	})
}

// newestFilms is every page of the films newest first, two to a page, as ids.
func newestFilms(t *testing.T, database *Database) string {
	t.Helper()
	params := sqlcgen.ListFilmsByAddedParams{
		BeforeFoundAt: 1 << 62, BeforeModifiedAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 2, IsEveryLibrary: 1,
		LibraryIdsJSON: "[]",
	}
	var ids []int64
	for {
		rows, err := database.Read().ListFilmsByAdded(t.Context(), params)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			ids = append(ids, row.ItemID)
		}
		if len(rows) < 2 {
			return fmt.Sprint(ids)
		}
		last := rows[len(rows)-1]
		params.BeforeFoundAt, params.BeforeModifiedAt = last.FoundAt, last.FoundFileModifiedAt
		params.BeforeID = last.ItemID
	}
}

// newestSeries is the series newest first, as ids.
func newestSeries(t *testing.T, database *Database) string {
	t.Helper()
	rows, err := database.Read().ListSeriesByAdded(t.Context(), sqlcgen.ListSeriesByAddedParams{
		BeforeFoundAt: 1 << 62, BeforeModifiedAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 10, IsEveryLibrary: 1,
		LibraryIdsJSON: "[]",
	})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, row := range rows {
		ids = append(ids, row.ItemID)
	}
	return fmt.Sprint(ids)
}

// Newest first is the scan that found it, most recent first, then its file's own time, newest
// first, then the id, whenever it was added: a page's cursor parts two films found by one scan
// with one time (the owner, 2026-10-25; step 0058).
func TestFound_FilmsNewestFirstByTheScanThatFoundThemThenTheirFilesTime(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	foundShelf(t, database)
	if got, want := newestFilms(t, database), "[4 3 2 1]"; got != want {
		t.Errorf("films newest first = %s; want %s: the later scan's first, then each file's time, then the id",
			got, want)
	}
	// A second version found later does not make a film new again; its first file gone, it is as new
	// as the one it has left.
	execRaw(t, database, rowstest.Version(rowstest.VersionSpec{ID: 5, TitleID: 1}),
		foundFile(5, "/m/1-4k.mkv", 60, 300))
	if got, want := newestFilms(t, database), "[4 3 2 1]"; got != want {
		t.Errorf("with film 1's second version found later = %s; want %s", got, want)
	}
	execRaw(t, database, `UPDATE place SET removed_at = 400 WHERE id = 1`)
	if got, want := newestFilms(t, database), "[1 4 3 2]"; got != want {
		t.Errorf("with film 1's first file gone = %s; want %s", got, want)
	}
}

// A series is as new as its newest episode: one found by a later scan brings it first, and it goes
// back when that episode's file goes (step 0058).
func TestFound_ASeriesIsAsNewAsItsNewestEpisode(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	foundShelf(t, database)
	if got, want := newestSeries(t, database), "[20 10]"; got != want {
		t.Errorf("series newest first = %s; want %s", got, want)
	}
	execRaw(t, database,
		rowstest.File(rowstest.FileSpec{
			ID: 12, NodeID: 1, Path: "/s/2.mkv", ModifiedAt: 2, FirstSeenAt: 300, LibraryKind: "movie",
		}),
		`UPDATE file_filing SET version_id = 12 WHERE file_id = 12`)
	if got, want := newestSeries(t, database), "[10 20]"; got != want {
		t.Errorf("with series 10's second episode found by a later scan = %s; want %s", got, want)
	}
	execRaw(t, database, `UPDATE place SET removed_at = 400 WHERE id = 12`)
	if got, want := newestSeries(t, database), "[20 10]"; got != want {
		t.Errorf("with that episode's file gone = %s; want %s", got, want)
	}
	execRaw(t, database, `DELETE FROM position WHERE ordering_id = 2`,
		rowstest.Position(rowstest.PositionSpec{OrderingID: 1, TitleID: 21, SortOrder: 3}))
	got := fmt.Sprint(queryRows(t, database, "SELECT id, found_at FROM item WHERE kind = 'series' ORDER BY id", 2))
	if want := "[[10 200] [20 0]]"; got != want {
		t.Errorf("with series 20's episode moved to 10 = %s; want %s", got, want)
	}
	// What a write sets again as it was changes nothing derived (D303, DL9).
	if changes := writerChangesOf(t.Context(), t, database,
		"UPDATE place SET modified_at = modified_at"); changes != 7 {
		t.Errorf("setting the files as they were changed %d rows; want the 7 files alone", changes)
	}
}
