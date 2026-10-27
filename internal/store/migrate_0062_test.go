// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0062 keeps watched what was finished until it (D370 §1): a position past the household's
// finished threshold of a known length, a file played to its end, and what a deleted file left as
// completed on its version; a position short of the threshold, a file of unknown length, and an extra
// are not watched.
func TestMigrate_Step0062KeepsWatchedWhatWasFinished(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0062 := stepIndex(t, steps, 62)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0062], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1), (2, 'partner', 2, 1)",
		// The household finishes a film past 95%, not the starting 90%.
		`INSERT INTO setting (key, value_json, set_at, is_modified) VALUES ('finished_after_percent', '95', 1, 1)`,
		`INSERT INTO item (id, kind, filing_library_id, added_at) VALUES (1, 'title', 1, 1), (2, 'title', 1, 1),
		 (3, 'title', 1, 1), (4, 'title', 1, 1), (5, 'title', 1, 1), (6, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (1, 'movie', 'identified'),
		 (2, 'movie', 'identified'), (3, 'movie', 'identified'), (4, 'extra', 'identified'),
		 (5, 'movie', 'identified'), (6, 'other', 'identified')`,
		`INSERT INTO version (id, title_id, created_at) VALUES (1, 1, 1), (2, 2, 1), (3, 3, 1), (4, 4, 1),
		 (5, 5, 1), (6, 6, 1)`,
		`INSERT INTO file (id, node_id, path, first_seen_at, last_seen_at) VALUES (1, 1, '/a', 1, 1),
		 (2, 1, '/b', 1, 1), (3, 1, '/c', 1, 1), (4, 1, '/d', 1, 1), (6, 1, '/f', 1, 1)`,
		`INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES (1, 'movies', 'listed', 1),
		 (2, 'movies', 'listed', 2), (3, 'movies', 'listed', 3), (4, 'movies', 'extra', 4),
		 (6, 'other', 'listed', 6)`,
		`INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (1, '{}', 1000), (2, '{}', 1000),
		 (3, '{}', 1000), (4, '{}', 1000), (6, '{}', 0)`,
		// Film 1 past 95%; film 3 at 92%, short of it; the extra past it; the other video of unknown
		// length far along, and played to its end by the partner; film 2 played to its end.
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, position_read_at)
		 VALUES (1, 1, 970, 1, 500, 500), (1, 3, 920, 1, 510, 510), (1, 4, 990, 1, 520, 520),
		        (1, 6, 999999, 1, 530, 530)`,
		`INSERT INTO play_event (user_id, file_id, kind, position_ms, happened_at)
		 VALUES (2, 2, 'completed', 1000, 600), (2, 6, 'completed', 5000, 610), (1, 2, 'started', 0, 400)`,
		// Film 5's file was deleted, finished twice by the owner.
		`INSERT INTO purged_watching (user_id, version_id, completed_count, watched_ms, last_watched_at)
		 VALUES (1, 5, 2, 1000, 700), (2, 5, 0, 300, 710)`)
	if err = database.migrate(t.Context(), steps[:step0062+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	const watched = "SELECT user_id, title_id, watched_at, coalesce(marked_at, 'none') FROM title_watched " +
		"ORDER BY user_id, title_id"
	got := fmt.Sprint(queryRows(t, database, watched, 4))
	if want := "[[1 1 500 none] [1 5 700 none] [2 2 600 none] [2 6 610 none]]"; got != want {
		t.Errorf("watched after the step: %s; want %s", got, want)
	}
}
