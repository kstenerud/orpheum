// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0076 names a preference set on a film or a whole series by the scope 'item', not 'work'
// (D428). Every stored preference is kept, and the ones at 'work' move to 'item', their value and
// time kept; the CHECK refuses the old word.
func TestMigrate_Step0076MovesEveryWorkPreferenceToTheScopeItem(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0076 := stepIndex(t, steps, 76)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0076], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO role (id, name, capabilities) VALUES (99, 'step 0076', '[]')",
		"INSERT INTO user (id, name, role_id, created_at) VALUES (5, 'dan', 99, 1)",
		`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at) VALUES
		 (5, 'user', 0, 'levelling', 'true', 10),
		 (5, 'work', 20, 'episode_ordering', '"tmdb:dvd"', 20),
		 (5, 'work', 21, 'audio_language', '{"language":"en"}', 30),
		 (5, 'file', 7, 'subtitle_offset_ms', '250', 40)`)
	if err = database.migrate(t.Context(), steps[:step0076+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	const query = "SELECT scope, scope_id, axis, value_json, set_at FROM preference ORDER BY set_at"
	want := `[[user 0 levelling true 10] [item 20 episode_ordering "tmdb:dvd" 20] ` +
		`[item 21 audio_language {"language":"en"} 30] [file 7 subtitle_offset_ms 250 40]]`
	if got := fmt.Sprint(queryRows(t, database, query, 5)); got != want {
		t.Errorf("the preferences:\ngot  %s\nwant %s", got, want)
	}
	_, err = database.writer.ExecContext(t.Context(),
		"INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at) "+
			"VALUES (5, 'work', 1, 'a', '1', 1)")
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("a preference at the old scope 'work' was not refused by the CHECK: %v", err)
	}
	if problems := fmt.Sprint(queryRows(t, database, "PRAGMA foreign_key_check", 4)); problems != "[]" {
		t.Errorf("foreign keys broken by the rebuild: %s", problems)
	}
}
