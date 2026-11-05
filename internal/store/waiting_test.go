// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// What waits for a person's match is counted as "Needs a match" lists it: an ambiguous title or
// series, or one a search found nothing for, with a live file a library lists; never an episode, a
// title not searched yet, or one with no live file (D209; D303's count, the access audit's A7; D363).
func TestCountWaitingForAPerson_CountsWhatTheListShows(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database, "PRAGMA foreign_keys = OFF",
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movie', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/')")
	title := func(id int, kind, state, source string, hasFile bool) {
		t.Helper()
		execRaw(t, database,
			fmt.Sprintf("INSERT INTO item (id, kind, added_at, identification_state, followed_source) "+
				"VALUES (%d, '%s', 1, '%s', %s)", id, kind, state, source),
			fmt.Sprintf("INSERT INTO title (id, kind) VALUES (%d, '%s')", id, kind),
			fmt.Sprintf("INSERT INTO version (id, title_id, created_at) VALUES (%d, %d, 1)", id, id))
		if hasFile {
			execRaw(t, database, fmt.Sprintf("INSERT INTO file (id, node_id, path, filing, "+
				"first_seen_at, last_seen_at, reported_path_raw) "+
				"VALUES (%d, 1, '/p%d', 'video', 1, 1, CAST('/p%d' AS BLOB))",
				id, id, id),
				fmt.Sprintf("INSERT INTO file_filing (file_id, library_kind, filing, version_id) "+
					"VALUES (%d, 'movie', 'listed', %d)", id, id))
		}
	}
	title(1, "movie", "ambiguous", "NULL", true)        // counted
	title(2, "movie", "unidentified", "'none'", true)   // counted: a search found nothing
	title(3, "episode", "unidentified", "'none'", true) // its series' to count
	title(4, "movie", "unidentified", "NULL", true)     // not searched yet
	title(5, "movie", "ambiguous", "NULL", false)       // no live file
	title(6, "episode", "identified", "'tmdb'", true)   // the series' episode
	title(7, "episode", "identified", "'tmdb'", true)
	execRaw(t, database,
		"INSERT INTO item (id, kind, added_at, identification_state, followed_source) VALUES "+
			"(20, 'series', 1, 'ambiguous', NULL), (21, 'series', 1, 'unidentified', 'none')",
		"INSERT INTO collection (id, kind, created_by) VALUES (20, 'series', 'scanner'), (21, 'series', 'scanner')",
		"INSERT INTO collection_member (collection_id, member_title) VALUES (20, 6), (21, 7)")
	count, err := database.Read().CountWaitingForAPerson(t.Context())
	if err != nil || count != 4 {
		t.Errorf("counted %d, %v; want 4: the ambiguous film, the film nothing was found for, and both series",
			count, err)
	}
}
