// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0069 names the schema as the API does (D426 §7): each credit keeps its job, each artwork its
// group, each stored listing of a season is a group's, a user's correction of an other video's time
// is the user's, each hidden item stays hidden, and the invitations and preferences, rebuilt for
// their comments, keep their rows and the invitations' id sequence.
func TestMigrate_Step0069KeepsEveryRowUnderItsNewName(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0069 := stepIndex(t, steps, 69)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0069], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO role (id, name, capabilities) VALUES (9, 'guest', '')",
		"INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'ann', 9, 1)",
		`INSERT INTO item (id, kind, added_at) VALUES (1, 'title', 1), (2, 'collection', 1), (3, 'person', 1),
		 (4, 'title', 1)`,
		`INSERT INTO title (id, kind, identification_state)
		 VALUES (1, 'movie', 'identified'), (4, 'other', 'identified')`,
		"INSERT INTO collection (id, kind, membership, created_by) VALUES (2, 'series', 'explicit', 'scanner')",
		"INSERT INTO person (id, name_key) VALUES (3, 'ann')",
		"INSERT INTO credit (item_id, role, ordinal, person_id) VALUES (1, 'director', 0, 3)",
		"INSERT INTO artwork (item_id, kind, source, season_number) VALUES (2, 'poster', 'tmdb', 3)",
		`INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
		 VALUES (2, 'tmdb', '', 1, 1, X'00'), (2, 'tmdb', 'season/3', 1, 1, X'00'),
		        (2, 'tmdb', 'episode_group/abc', 1, 1, X'00')`,
		`INSERT INTO other_video (title_id, recorded_at, recorded_from, file_name, file_name_key)
		 VALUES (4, 5, 'person', 'a.mkv', X'61')`,
		"INSERT INTO progress_hide (user_id, item_id, hidden_at) VALUES (1, 1, 7)",
		"INSERT INTO invitation (id, role_id, admin_note, issued_at) VALUES (5, 9, 'for bo', 1)",
		`INSERT INTO preference (user_id, scope, axis, value_json, set_at)
		 VALUES (1, 'user', 'levelling', 'true', 1)`)
	if err = database.migrate(t.Context(), steps[:step0069+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query   string
		columns int
		want    string
	}{
		{"SELECT item_id, job, person_id FROM credit", 3, "[[1 director 3]]"},
		{"SELECT item_id, group_number FROM artwork", 2, "[[2 3]]"},
		{"SELECT part FROM provider_response ORDER BY part", 1, "[[] [episode_group/abc] [group/3]]"},
		{"SELECT title_id, recorded_from FROM other_video", 2, "[[4 user]]"},
		{"SELECT user_id, item_id, hidden_at FROM item_hidden", 3, "[[1 1 7]]"},
		{"SELECT id, admin_note FROM invitation", 2, "[[5 for bo]]"},
		{"SELECT user_id, axis, value_json FROM preference", 3, "[[1 levelling true]]"},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("%s after the step: %s; want %s", check.query, got, check.want)
		}
	}
	execRaw(t, database, "INSERT INTO invitation (role_id, admin_note, issued_at) VALUES (9, 'next', 1)")
	next := fmt.Sprint(queryRows(t, database, "SELECT id FROM invitation WHERE admin_note = 'next'", 1))
	if next != "[[6]]" {
		t.Errorf("the next invitation's id: %s; want [[6]], past the highest the sequence gave", next)
	}
}
