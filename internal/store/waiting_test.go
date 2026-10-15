// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// What waits for a person's match is counted as "Needs a match" lists it: an ambiguous title or
// series, or one a search found nothing for, with a live file; never an episode, a title not
// searched yet, or one with no live file (D209; D303's count, the access audit's A7).
func TestCountWaitingForAPerson_CountsWhatTheListShows(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	execRaw(t, database, "PRAGMA foreign_keys = OFF")
	title := func(id int, kind, state, source string, hasFile bool) {
		t.Helper()
		execRaw(t, database,
			fmt.Sprintf("INSERT INTO item (id, kind, added_at) VALUES (%d, 'title', 1)", id),
			fmt.Sprintf("INSERT INTO title (id, kind, identification_state, followed_source) "+
				"VALUES (%d, '%s', '%s', %s)", id, kind, state, source),
			fmt.Sprintf("INSERT INTO version (id, title_id, created_at) VALUES (%d, %d, 1)", id, id))
		if hasFile {
			execRaw(t, database, fmt.Sprintf("INSERT INTO file (library_id, node_id, path, filing, version_id, "+
				"first_seen_at, last_seen_at, reported_path_raw) VALUES (1, 1, 'p%d', 'listed', %d, 1, 1, x'%02x')",
				id, id, id))
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
		"INSERT INTO item (id, kind, added_at) VALUES (20, 'collection', 1), (21, 'collection', 1)",
		"INSERT INTO collection (id, kind, membership, created_by, identification_state, followed_source) VALUES "+
			"(20, 'series', 'explicit', 'scanner', 'ambiguous', NULL), "+
			"(21, 'series', 'explicit', 'scanner', 'unidentified', 'none')",
		"INSERT INTO collection_member (collection_id, member_title) VALUES (20, 6), (21, 7)")
	count, err := database.Read().CountWaitingForAPerson(t.Context())
	if err != nil || count != 4 {
		t.Errorf("counted %d, %v; want 4: the ambiguous film, the film nothing was found for, and both series",
			count, err)
	}
}
