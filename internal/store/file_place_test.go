// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A file is found, listed and played at its live place by place.file_id, not at the place whose id
// is the file's (D432 section 6): film 1's file has its own place, 1, gone, and lives at place 7.
// Its title lists it there, counts it, plays it from there and is found when the file first was.
func TestFilePlace_AFileIsListedAndPlayedAtAPlaceWhoseIDIsNotItsOwn(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movie', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/m')",
		`INSERT INTO item (id, kind, filing_library_id, added_at, identification_state)
		 VALUES (1, 'movie', 1, 1, 'identified')`,
		"INSERT INTO work (id, kind) VALUES (1, 'movie')",
		"INSERT INTO version (id, work_id, created_at) VALUES (1, 1, 1)",
		"INSERT INTO file (id, size_bytes) VALUES (1, 5)",
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, modified_at, filing, first_seen_at,
		 last_seen_at, removed_at) VALUES
		 (1, 1, 1, '/m/a.mkv', CAST('/m/a.mkv' AS BLOB), 30, 'read', 100, 100, 300),
		 (7, 1, 1, '/m/b.mkv', CAST('/m/b.mkv' AS BLOB), 40, 'read', 200, 200, NULL)`,
		"INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES (1, 'movie', 'listed', 1)")
	ctx := t.Context()
	queries := database.Read()
	params := sqlcgen.ListWorkFilesParams{WorkID: 1, IsEveryLibrary: 1, LibraryIdsJSON: "[]"}
	listed, err := queries.ListWorkFiles(ctx, params)
	if err != nil || len(listed) != 1 {
		t.Fatalf("the title's files = %+v, %v; want file 1 at place 7", listed, err)
	}
	live, err := queries.ListLiveFilesOfWork(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	count, err := queries.CountLiveFilesOfWork(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	playable, err := queries.GetPlayableFile(ctx, 1)
	if err != nil {
		t.Fatalf("the file to play: %v; want it at place 7", err)
	}
	checks := []struct{ what, got, want string }{
		{
			"the title's file", fmt.Sprintf("%d %d %s", listed[0].FileID, listed[0].PlaceID, listed[0].Path),
			"1 7 /m/b.mkv",
		},
		{"its live files", fmt.Sprintf("%+v", live), "[{ID:1 Path:/m/b.mkv LibraryKind:movie DurationMs:0}]"},
		{"its count of live files", fmt.Sprint(count), "1"},
		{"where it plays from", string(playable.ReportedPathRaw), "/m/b.mkv"},
		{
			"when it was found", fmt.Sprint(queryRows(t, database,
				"SELECT found_at, found_file_modified_at FROM item WHERE id = 1", 2)), "[[100 40]]",
		},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %s; want %s", check.what, check.got, check.want)
		}
	}
}
