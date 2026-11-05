// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0077 keeps one view of which library covers a place, library_place (D426 §4): place_library,
// which wrote the same coverage from the place's side, goes, and version_library reads
// library_place. The one view says what the three said: a folder holds the places below it and no
// place beside it whose name only begins as its own does, a folder of "/" holds every place on its
// node, a removed place, a retired folder and a removed library cover nothing, a copy's place is
// its file's, and a version is listed by a library of the kind that filed it.
func TestMigrate_Step0077KeepsOneCoverageViewThatSaysWhatTheThreeSaid(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0077 := stepIndex(t, steps, 77)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0077+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (3, 'nas', 1), (4, 'pi', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at, removed_at)
		 VALUES (1, 'Films', 'movie', 'tmdb', 'watch', 0, 1, NULL),
		 (2, 'Everything', 'other', 'nfo', 'watch', 0, 1, NULL), (5, 'Gone', 'movie', 'tmdb', 'watch', 0, 1, 9)`,
		`INSERT INTO library_root (library_id, node_id, root_path, retired_at) VALUES
		 (1, 3, '/media/films', NULL), (1, 3, '/media/old', 8), (2, 4, '/', NULL), (5, 3, '/media', NULL)`,
		`INSERT INTO file (id, node_id, path, reported_path_raw, first_seen_at, last_seen_at, removed_at, filing,
		 copy_of)
		 VALUES (10, 3, '/media/films/Wing.mkv', CAST('/media/films/Wing.mkv' AS BLOB), 1, 1, NULL, 'video', NULL),
		 (11, 3, '/media/films2/Greed.mkv', CAST('/media/films2/Greed.mkv' AS BLOB), 1, 1, NULL, 'video', NULL),
		 (12, 3, '/media/films/Gone.mkv', CAST('/media/films/Gone.mkv' AS BLOB), 1, 1, 5, 'video', NULL),
		 (13, 3, '/media/old/Old.mkv', CAST('/media/old/Old.mkv' AS BLOB), 1, 1, NULL, 'video', NULL),
		 (14, 4, '/srv/Wing.mkv', CAST('/srv/Wing.mkv' AS BLOB), 2, 2, NULL, 'video', NULL)`,
		"UPDATE file SET filing = 'copy', copy_of = 10 WHERE id = 14",
		"INSERT INTO item (id, kind, identification_state, added_at) VALUES (20, 'movie', 'identified', 1)",
		"INSERT INTO work (id, kind) VALUES (20, 'movie')",
		"INSERT INTO version (id, work_id, created_at) VALUES (30, 20, 1)",
		`INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES
		 (10, 'movie', 'listed', 30), (10, 'other', 'listed', 30)`)
	for _, check := range []struct {
		query   string
		columns int
		want    string
	}{
		{
			`SELECT place_id, file_id, library_id, library_kind, root_path FROM library_place
			 ORDER BY place_id, library_id`, 5,
			"[[10 10 1 movie /media/films] [14 10 2 other /]]",
		},
		{
			"SELECT version_id, file_id, place_id, library_id FROM version_library ORDER BY place_id", 4,
			"[[30 10 10 1] [30 10 14 2]]",
		},
		{
			"SELECT name FROM sqlite_schema WHERE type = 'view' AND name LIKE '%library%' ORDER BY name", 1,
			"[[library_place] [version_library]]",
		},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s:\ngot  %s\nwant %s", check.query, got, check.want)
		}
	}
}
