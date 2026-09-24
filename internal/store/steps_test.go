// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

func TestMigrate_Step0007KeepsEveryStoredResponseAsItsItemsOwn(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:6], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at) "+
			"VALUES (1, 'Films', 'movies', 'tmdb', 'manual', 0, 1)",
		"INSERT INTO item (id, kind, library_id, created_at) VALUES (5, 'title', 1, 1)",
		"INSERT INTO title (id, kind, ident) VALUES (5, 'movie', 'identified')",
		"INSERT INTO provider_response (title_id, provider, fetched_at, body) VALUES (5, 'tmdb', 7, X'01')")
	if err = database.migrate(t.Context(), steps[:7], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, "SELECT item_id, provider, part, fetched_at, hex(body) FROM provider_response", 5)
	if fmt.Sprint(rows) != "[[5 tmdb  7 01]]" {
		t.Errorf("after step 0007, provider_response holds %v; want the film's response, as its item's '' part", rows)
	}
	state, err := database.State(t.Context())
	if err != nil || state.CompatibleVersion != 7 {
		t.Errorf("compatible_version = %d (%v); step 0007 renames a column an older binary reads",
			state.CompatibleVersion, err)
	}
}
