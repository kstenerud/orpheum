// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0083 finds a file's places by place.file_id (D432 section 6). A film whose file's own place
// is gone and which a later place holds, with an id not the file's, was not found before the step;
// after it the film is found when the file was first found, at the live place's modified time, and
// file_place says the file is read from that place. The triggers keep it so as places are made,
// change file and are deleted.
func TestMigrate_Step0083FindsAFileAtAPlaceWhoseIDIsNotItsOwn(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0083 := stepIndex(t, steps, 83)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0083], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO item (id, kind, identification_state, added_at) VALUES (20, 'movie', 'identified', 1)",
		"INSERT INTO work (id, kind) VALUES (20, 'movie')",
		"INSERT INTO version (id, work_id, created_at) VALUES (30, 20, 1)",
		"INSERT INTO file (id) VALUES (10), (11)",
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, modified_at, filing, first_seen_at,
		 last_seen_at, removed_at) VALUES
		 (10, 10, 1, '/old/a.mkv', CAST('/old/a.mkv' AS BLOB), 7, 'read', 100, 100, 400),
		 (50, 10, 1, '/new/a.mkv', CAST('/new/a.mkv' AS BLOB), 9, 'read', 500, 500, NULL)`,
		"INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES (10, 'movie', 'listed', 30)")
	found := func() string {
		return fmt.Sprint(queryRows(t, database, "SELECT found_at, found_file_modified_at FROM item WHERE id = 20", 2))
	}
	if got := found(); got != "[[0 0]]" {
		t.Fatalf("before the step, the film's found time = %s; want [[0 0]], its file's own place being gone", got)
	}
	if err = database.migrate(t.Context(), steps[:step0083+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if got := found(); got != "[[100 9]]" {
		t.Errorf("after the step, the film's found time = %s; want [[100 9]]: found at 100, read at place 50", got)
	}
	filePlace := fmt.Sprint(queryRows(t, database, "SELECT file_id, place_id FROM file_place ORDER BY file_id", 2))
	if got := filePlace; got != "[[10 50]]" {
		t.Errorf("file_place = %s; want [[10 50]], the file read from its one live place", got)
	}
	changes := []struct{ what, statement, want string }{
		{
			"with another live place found later", `INSERT INTO place (id, file_id, node_id, path, reported_path_raw,
			 modified_at, filing, first_seen_at, last_seen_at) VALUES (60, 10, 1, '/c.mkv', CAST('/c.mkv' AS BLOB), 3,
			 'read', 600, 600)`, "[[100 9]]",
		},
		{"with place 50 holding another file", "UPDATE place SET file_id = 11 WHERE id = 50", "[[100 3]]"},
		{"with the own place deleted", "DELETE FROM place WHERE id = 10", "[[600 3]]"},
		{"with no live place of the file", "UPDATE place SET removed_at = 700 WHERE id = 60", "[[0 0]]"},
	}
	for _, change := range changes {
		execRaw(t, database, change.statement)
		if got := found(); got != change.want {
			t.Errorf("%s: found = %s; want %s", change.what, got, change.want)
		}
	}
}
