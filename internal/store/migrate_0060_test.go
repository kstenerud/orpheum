// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// productionFiles are production's files at 0059: a film, its NFO, an extra, one unread, one
// skipped, one errored, a removed one and an episode.
//
//nolint:dupword // a row's NULL columns follow one another
const productionFiles = `
INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, modified_at, payload_hash,
 filing, filing_reason, version_id, part_index, identified_from_path, sidecar_kind, attached_to_file,
 first_seen_at, last_seen_at, removed_at) VALUES
 (1, 1, 1, '/tank/media/video/movies/Gold Rush (1999)/Gold Rush (1999).mkv',
  CAST('/tank/media/video/movies/Gold Rush (1999)/Gold Rush (1999).mkv' AS BLOB), 100, 30, X'01', 'listed', NULL,
  1, 1, '/tank/media/video/movies/Gold Rush (1999)/Gold Rush (1999).mkv', NULL, NULL, 100, 100, NULL),
 (2, 1, 1, '/tank/media/video/movies/Gold Rush (1999)/Gold Rush (1999).nfo',
  CAST('/tank/media/video/movies/Gold Rush (1999)/Gold Rush (1999).nfo' AS BLOB), 1, 30, NULL, 'attached',
  'an NFO beside its video', NULL, NULL, NULL, 'nfo', 1, 100, 100, NULL),
 (3, 1, 1, '/tank/media/video/movies/Gold Rush (1999)/Extras/Making of.mkv',
  CAST('/tank/media/video/movies/Gold Rush (1999)/Extras/Making of.mkv' AS BLOB), 10, 30, X'03', 'extra',
  'in an Extras folder', 2, NULL, NULL, NULL, NULL, 100, 100, NULL),
 (4, 1, 1, '/tank/media/video/movies/new.mkv', CAST('/tank/media/video/movies/new.mkv' AS BLOB), NULL, 40,
  NULL, 'unidentified', 'not identified yet', NULL, NULL, NULL, NULL, NULL, 110, 110, NULL),
 (5, 1, 1, '/tank/media/video/movies/a.txt', CAST('/tank/media/video/movies/a.txt' AS BLOB), 1, 40,
  NULL, 'skipped', 'not a video', NULL, NULL, NULL, NULL, NULL, 110, 110, NULL),
 (6, 1, 1, '/tank/media/video/movies/Locked', CAST('/tank/media/video/movies/Locked' AS BLOB), NULL,
  NULL, NULL, 'errored', 'EACCES', NULL, NULL, NULL, NULL, NULL, 110, 110, NULL),
 (7, 1, 1, '/tank/media/video/movies/Gold Rush (1999)/old.mkv',
  CAST('/tank/media/video/movies/Gold Rush (1999)/old.mkv' AS BLOB), 100, 20, X'07', 'listed', NULL,
  7, 1, NULL, NULL, NULL, 90, 90, 95),
 (11, 2, 1, '/tank/media/video/shows/Show/Season 01/Show - S01E01.mkv',
  CAST('/tank/media/video/shows/Show/Season 01/Show - S01E01.mkv' AS BLOB), 100, 50, X'0B', 'listed',
  NULL, 11, 1, '/tank/media/video/shows/Show/Season 01/Show - S01E01.mkv', NULL, NULL, 100, 100, NULL)`

// productionAt0059 fills a database at step 0059 as nas.example's stands (D363 §12): a films library over
// /tank/media/video/movies and a series library over /tank/media/video/shows on one node, a library
// deleted before, a film with its NFO and an extra, a series' episode, a file not identified yet, one
// skipped, one that could not be read, a removed file that was watched, and a person's watching,
// choices, list and corrections.
func productionAt0059(t *testing.T, database *Database) {
	t.Helper()
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at, removed_at)
		 VALUES (1, 'Films', 'movies', 'nfo_then_tmdb', 'watch', 0, 1, NULL),
		        (2, 'Series', 'shows', 'tmdb', 'watch', 0, 1, NULL),
		        (3, 'Old', 'movies', 'nfo', 'manual', 0, 1, 50)`,
		`INSERT INTO library_root (library_id, node_id, root_path, retired_at)
		 VALUES (1, 1, '/tank/media/video/movies', NULL), (2, 1, '/tank/media/video/shows', NULL),
		        (3, 1, '/tank/media/old', 50)`,
		"INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1), (2, 'partner', 2, 1)",
		"INSERT INTO role_library (role_id, library_id, may_rebroadcast) VALUES (2, 1, 0)",
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (1, 'title', 1, 1), (2, 'title', 1, 1),
		 (10, 'collection', 2, 1), (11, 'title', 2, 1)`,
		`INSERT INTO title (id, kind, identification_state, followed_source, followed_source_id)
		 VALUES (1, 'movie', 'confirmed', 'tmdb', '4801'), (2, 'extra', 'identified', 'none', NULL),
		        (11, 'episode', 'identified', 'tmdb', '1')`,
		`INSERT INTO collection (id, kind, membership, created_by) VALUES (10, 'series', 'explicit', 'scanner')`,
		`INSERT INTO collection_member (collection_id, member_title, position) VALUES (10, 11, 1)`,
		`INSERT INTO version (id, title_id, created_at) VALUES (1, 1, 1), (2, 2, 1), (11, 11, 1), (7, 1, 1)`,
		productionFiles,
		`INSERT INTO file_nfo (file_id, body) VALUES (2, CAST('<movie><title>The Gold Rush</title></movie>' AS BLOB))`,
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, position_read_at)
		 VALUES (1, 1, 600000, 3, 200, 200), (2, 11, 120000, 1, 210, 210), (1, 7, 1000, 1, 92, 92)`,
		`INSERT INTO play_span (user_id, file_id, from_position_ms, to_position_ms, started_at, ended_at)
		 VALUES (1, 1, 0, 600000, 100, 200)`,
		`INSERT INTO play_event (user_id, file_id, kind, position_ms, happened_at) VALUES (1, 1, 'started', 0, 100)`,
		`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
		 VALUES (1, 'file', 1, 'subtitle_offset_ms', '250', 150), (1, 'library', 1, 'audio_language', '"en"', 150)`,
		`INSERT INTO watchlist (user_id, item_id, added_at) VALUES (2, 10, 1)`,
		`INSERT INTO field_override (item_id, field, language, override_value, origin, set_by_user_id, set_at)
		 VALUES (1, 'name', '', 'The Gold Rush', 'typed', 1, 1)`,
		`INSERT INTO progress_hide (user_id, item_id, hidden_at) VALUES (2, 1, 300)`)
}

// D363 §12: step 0060 carries production's database to the new model, nothing a person did lost. Each
// file's library becomes a filing of its kind and a coverage of its place: the films library lists the
// film and its extra, the series library the series, a person reaching one library reaches only its
// files, and every watching, choice, list entry and correction is where it was.
func TestMigrate_Step0060CarriesAProductionDatabaseToPlacesAndCoverage(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0060 := stepIndex(t, steps, 60)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0060], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	productionAt0059(t, database)
	const foundQuery = "SELECT id, found_at, found_file_modified_at FROM title ORDER BY id"
	beforeFound := fmt.Sprint(queryRows(t, database, foundQuery, 3))
	if err = database.migrate(t.Context(), steps[:step0060+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		what, query string
		columns     int
		want        string
	}{
		{
			"each file's own filing, the scan's",
			"SELECT id, filing, coalesce(filing_reason, '-') FROM file ORDER BY id", 3,
			"[[1 video -] [2 attached an NFO beside its video] [3 video -] [4 video -] " +
				"[5 skipped not a video] [6 errored EACCES] [7 video -] [11 video -]]",
		},
		{
			"how its library's kind filed each video, its version and the path it was identified from",
			"SELECT file_id, library_kind, filing, coalesce(filing_reason, '-'), coalesce(version_id, '-'), " +
				"coalesce(part_index, '-'), coalesce(identified_from_path, '-') FROM file_filing ORDER BY file_id", 7,
			"[[1 movies listed - 1 1 /tank/media/video/movies/Gold Rush (1999)/Gold Rush (1999).mkv] " +
				"[3 movies extra in an Extras folder 2 - -] [4 movies unidentified not identified yet - - -] " +
				"[7 movies listed - 7 1 -] " +
				"[11 shows listed - 11 1 /tank/media/video/shows/Show/Season 01/Show - S01E01.mkv]]",
		},
		{
			"the library whose pass files each item",
			"SELECT id, filing_library_id FROM item ORDER BY id", 2,
			"[[1 1] [2 1] [10 2] [11 2]]",
		},
		{
			"what each person watched, chose, listed and corrected",
			"SELECT (SELECT group_concat(user_id || ':' || file_id || ':' || position_ms, ' ') FROM play_position), " +
				"(SELECT count(*) FROM play_span) || (SELECT count(*) FROM play_event), " +
				"(SELECT group_concat(scope || scope_id, ' ') FROM preference), " +
				"(SELECT count(*) FROM watchlist) || (SELECT count(*) FROM field_override) || " +
				"(SELECT count(*) FROM progress_hide)", 4,
			"[[1:1:600000 1:7:1000 2:11:120000 11 file1 library1 111]]",
		},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s: %s; want %s", check.what, got, check.want)
		}
	}
	if after := fmt.Sprint(queryRows(t, database, foundQuery, 3)); after != beforeFound {
		t.Errorf("newest first changed: %s; want %s as before", after, beforeFound)
	}
	queries := database.Read()
	films, err := queries.ListFilmsByAdded(t.Context(), sqlcgen.ListFilmsByAddedParams{
		BeforeFoundAt: 1 << 62, BeforeModifiedAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 10, LibraryIdsJSON: "[1]",
	})
	if err != nil || len(films) != 1 || films[0].ItemID != 1 {
		t.Errorf("the partner's films = %+v, %v; want the film, which the films library lists", films, err)
	}
	series, err := queries.ListSeriesByAdded(t.Context(), sqlcgen.ListSeriesByAddedParams{
		BeforeFoundAt: 1 << 62, BeforeModifiedAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 10, LibraryIdsJSON: "[1]",
	})
	if err != nil || len(series) != 0 {
		t.Errorf("the partner's series = %+v, %v; want none: they reach the films library alone", series, err)
	}
	for _, reached := range []struct {
		fileID int64
		json   string
		want   int64
	}{{1, "[1]", 1}, {11, "[1]", 0}, {11, "[2]", 1}, {1, "[3]", 0}} {
		isReached, err := queries.IsFileReached(t.Context(), sqlcgen.IsFileReachedParams{
			FileID: reached.fileID, LibraryIdsJSON: reached.json,
		})
		if err != nil || isReached != reached.want {
			t.Errorf("file %d reached by %s = %d, %v; want %d", reached.fileID, reached.json, isReached, err,
				reached.want)
		}
	}
}

// D363: the step refuses a mixed library, the kind it removes, loudly, and leaves the database as it
// was: none exists in any database, and one would be a library's files given no kind.
func TestMigrate_Step0060RefusesAMixedLibrary(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0060 := stepIndex(t, steps, 60)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0060], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, `INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable,
		created_at) VALUES (1, 'Everything', 'mixed', 'nfo', 'manual', 0, 1)`)
	err = database.migrate(t.Context(), steps[:step0060+1], testMigrateOptions(t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "D363 removes the mixed kind of library") {
		t.Errorf("migrating a mixed library: %v; want the step refused, saying why", err)
	}
	if kind := fmt.Sprint(queryRows(t, database, "SELECT kind FROM library", 1)); kind != "[[mixed]]" {
		t.Errorf("after the refusal the library is %s; want it as it was", kind)
	}
}
