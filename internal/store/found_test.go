// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// foundShelf fills a library with what newest first orders: films 1 to 4 and a series, 10, of two
// episodes, 11 and 12, each with a file found by a listing at first_seen_at, with its own time.
func foundShelf(t *testing.T, database *Database) {
	t.Helper()
	execRaw(t, database,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Everything', 'mixed', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)`,
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (1, 'title', 1, 4), (2, 'title', 1, 3),
		 (3, 'title', 1, 2), (4, 'title', 1, 1), (10, 'collection', 1, 9), (11, 'title', 1, 9), (12, 'title', 1, 9),
		 (20, 'collection', 1, 1), (21, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (1, 'movie', 'identified'),
		 (2, 'movie', 'identified'), (3, 'movie', 'identified'), (4, 'movie', 'identified'),
		 (11, 'episode', 'identified'), (12, 'episode', 'identified'), (21, 'episode', 'identified')`,
		`INSERT INTO collection (id, kind, membership, created_by) VALUES (10, 'series', 'explicit', 'scanner'),
		 (20, 'series', 'explicit', 'scanner')`,
		`INSERT INTO collection_member (collection_id, member_title, position) VALUES (10, 11, 1), (10, 12, 2),
		 (20, 21, 1)`,
		`INSERT INTO version (id, title_id, created_at) VALUES (1, 1, 1), (2, 2, 1), (3, 3, 1), (4, 4, 1),
		 (11, 11, 1), (12, 12, 1), (21, 21, 1)`,
		// The first scan, at 100, found films 1 to 3 and the series' first episode; the second, at 200,
		// film 4, whose file is older than any, and the other series' episode.
		`INSERT INTO file (id, library_id, node_id, path, modified_at, filing, version_id, first_seen_at,
		 last_seen_at) VALUES
		 (1, 1, 1, '/m/1.mkv', 30, 'listed', 1, 100, 100), (2, 1, 1, '/m/2.mkv', 50, 'listed', 2, 100, 100),
		 (3, 1, 1, '/m/3.mkv', 50, 'listed', 3, 100, 100), (4, 1, 1, '/m/4.mkv', 5, 'listed', 4, 200, 200),
		 (11, 1, 1, '/s/1.mkv', 70, 'listed', 11, 100, 100), (21, 1, 1, '/t/1.mkv', 1, 'listed', 21, 200, 200)`)
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
	execRaw(t, database, `INSERT INTO version (id, title_id, created_at) VALUES (5, 1, 1)`,
		`INSERT INTO file (id, library_id, node_id, path, modified_at, filing, version_id, first_seen_at,
		 last_seen_at) VALUES (5, 1, 1, '/m/1-4k.mkv', 60, 'listed', 5, 300, 300)`)
	if got, want := newestFilms(t, database), "[4 3 2 1]"; got != want {
		t.Errorf("with film 1's second version found later = %s; want %s", got, want)
	}
	execRaw(t, database, `UPDATE file SET removed_at = 400 WHERE id = 1`)
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
	execRaw(t, database, `INSERT INTO file (id, library_id, node_id, path, modified_at, filing, first_seen_at,
		 last_seen_at) VALUES (12, 1, 1, '/s/2.mkv', 2, 'listed', 300, 300)`,
		`UPDATE file SET version_id = 12 WHERE id = 12`)
	if got, want := newestSeries(t, database), "[10 20]"; got != want {
		t.Errorf("with series 10's second episode found by a later scan = %s; want %s", got, want)
	}
	execRaw(t, database, `UPDATE file SET removed_at = 400 WHERE id = 12`)
	if got, want := newestSeries(t, database), "[20 10]"; got != want {
		t.Errorf("with that episode's file gone = %s; want %s", got, want)
	}
	execRaw(t, database, `DELETE FROM collection_member WHERE collection_id = 20`,
		`INSERT INTO collection_member (collection_id, member_title, position) VALUES (10, 21, 3)`)
	got := fmt.Sprint(queryRows(t, database, "SELECT id, found_at FROM collection ORDER BY id", 2))
	if want := "[[10 200] [20 0]]"; got != want {
		t.Errorf("with series 20's episode moved to 10 = %s; want %s", got, want)
	}
	// What a write sets again as it was changes nothing derived (D303, DL9).
	if changes := writerChangesOf(t.Context(), t, database,
		"UPDATE file SET modified_at = modified_at, version_id = version_id"); changes != 7 {
		t.Errorf("setting the files as they were changed %d rows; want the 7 files alone", changes)
	}
}
