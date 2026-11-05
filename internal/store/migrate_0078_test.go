// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0078 splits a file from its places (D426 section 4). Every id is kept: a row that was no copy
// becomes a file and a place with its id; a confirmed copy becomes a place holding the file it was a
// copy of, its own hash dropped and a sidecar's track beside it re-pointed at that file; the sidecar
// stays attached to the copy's place. The CHECK refuses a read place holding no file.
func TestMigrate_Step0078KeepsEveryIDAndMakesEachCopyAPlace(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0078 := stepIndex(t, steps, 78)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0078], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO file (id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing,
		 first_seen_at, last_seen_at) VALUES (1, 1, '/m/a.mkv', CAST('/m/a.mkv' AS BLOB), 10, X'aa', 'video', 1, 9)`,
		`INSERT INTO file (id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing, copy_of,
		 filing_reason, first_seen_at, last_seen_at)
		 VALUES (2, 1, '/n/a.mkv', CAST('/n/a.mkv' AS BLOB), 10, X'aa', 'copy', 1, 'same bytes', 2, 9)`,
		`INSERT INTO file (id, node_id, path, reported_path_raw, size_bytes, payload_hash, filing, sidecar_kind,
		 attached_to_file, first_seen_at, last_seen_at)
		 VALUES (3, 1, '/n/a.en.srt', CAST('/n/a.en.srt' AS BLOB), 5, X'bb', 'attached', 'subtitle', 2, 2, 9)`,
		`INSERT INTO file (id, node_id, path, reported_path_raw, filing, filing_reason, first_seen_at,
		 last_seen_at, removed_at) VALUES
		 (4, 1, '/m/b.txt', CAST('/m/b.txt' AS BLOB), 'skipped', 'not media', 3, 9, NULL),
		 (5, 1, '/m/c.mkv', CAST('/m/c.mkv' AS BLOB), 'errored', 'permission_denied', 4, 9, 8)`,
		`INSERT INTO whole_file_hash (file_id, sha256, size_bytes, modified_at, failure, hashed_at)
		 VALUES (1, X'0f', 10, 1, NULL, 5), (2, X'0f', 10, 1, NULL, 6)`,
		`INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, disposition_flags) VALUES
		 (1, 1, 1, 0, 'video', 'h264', 0), (2, 3, 2, 0, 'subtitle', 'subrip', 0)`)
	if err = database.migrate(t.Context(), steps[:step0078+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		what, query string
		columns     int
		want        string
	}{
		{
			"the files, the copy's gone", "SELECT id, coalesce(size_bytes, ''), hex(payload_hash), " +
				"coalesce(sidecar_kind, '') FROM file ORDER BY id", 4,
			"[[1 10 AA ] [3 5 BB subtitle] [4   ] [5   ]]",
		},
		{
			"the places, every id kept", "SELECT id, file_id, path, filing, coalesce(filing_reason, ''), " +
				"coalesce(attached_to_place, ''), coalesce(removed_at, '') FROM place ORDER BY id", 7,
			"[[1 1 /m/a.mkv read   ] [2 1 /n/a.mkv read same bytes  ] [3 3 /n/a.en.srt read  2 ] " +
				"[4 4 /m/b.txt skipped not media  ] [5 5 /m/c.mkv errored permission_denied  8]]",
		},
		{"the hashes, the copy's dropped", "SELECT file_id FROM whole_file_hash", 1, "[[1]]"},
		{
			"the tracks, the sidecar's re-pointed at the file", "SELECT id, bytes_file, video_file FROM stream " +
				"ORDER BY id", 3, "[[1 1 1] [2 3 1]]",
		},
		{
			"the sequences, past every id kept", "SELECT name, seq FROM sqlite_sequence " +
				"WHERE name IN ('file', 'place') ORDER BY name", 2, "[[file 5] [place 5]]",
		},
		{"the foreign keys", "PRAGMA foreign_key_check", 4, "[]"},
	}
	for _, check := range checks {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s:\ngot  %s\nwant %s", check.what, got, check.want)
		}
	}
	_, err = database.writer.ExecContext(t.Context(),
		"INSERT INTO place (node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at) "+
			"VALUES (1, '/m/d.mkv', CAST('/m/d.mkv' AS BLOB), 'read', 1, 1)")
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("a read place holding no file was not refused by the CHECK: %v", err)
	}
	_, err = database.writer.ExecContext(t.Context(),
		"INSERT INTO place (file_id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at) "+
			"VALUES (1, 1, '/m/e.mkv', CAST('/m/e.mkv' AS BLOB), 'copy', 1, 1)")
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("a place filed 'copy', a word the API reads and no place stores, was not refused: %v", err)
	}
}
