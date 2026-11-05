// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0082 makes a live node's name unique (D432 section 6). A database that already holds two live
// nodes of one name migrates: the earliest, the one a hello reaches, keeps the name, and each later
// one is renamed "<name> (<id>)", its rows kept. A removed node's name stays free to enrol a new one,
// and a second live node of a name is refused from then on.
func TestMigrate_Step0082KeepsTheEarliestLiveNodeOfANameAndRenamesTheRest(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0082 := stepIndex(t, steps, 82)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0082], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		`INSERT INTO node (id, name, enrolled_at, removed_at) VALUES (1, 'nas', 1, 5), (2, 'nas', 6, NULL),
		 (3, 'nas', 7, NULL), (4, 'pi', 1, NULL), (5, 'nas', 8, NULL)`,
		"INSERT INTO file (id) VALUES (1)",
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at)
		 VALUES (1, 1, 3, '/a.mkv', CAST('/a.mkv' AS BLOB), 'read', 1, 1)`)
	if err = database.migrate(t.Context(), steps[:step0082+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(queryRows(t, database,
		"SELECT id, name, coalesce(removed_at, '') FROM node ORDER BY id", 3))
	if want := "[[1 nas 5] [2 nas ] [3 nas (3) ] [4 pi ] [5 nas (5) ]]"; got != want {
		t.Errorf("the nodes = %s; want %s", got, want)
	}
	if got := fmt.Sprint(queryRows(t, database, "SELECT node_id FROM place", 1)); got != "[[3]]" {
		t.Errorf("the renamed node's place = %s; want it still on node 3", got)
	}
	_, err = database.writer.ExecContext(t.Context(), "INSERT INTO node (name, enrolled_at) VALUES ('pi', 9)")
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("a second live node called pi = %v; want it refused as not unique", err)
	}
	execRaw(t, database, "UPDATE node SET removed_at = 9 WHERE name = 'pi'",
		"INSERT INTO node (name, enrolled_at) VALUES ('pi', 10)")
}
