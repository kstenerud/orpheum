// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0085 adds a node's leave to run unconfined (D450 section 1): one per node, naming the admin who
// gave it, and refused for a node or a user that is not there.
func TestMigrate_Step0085KeepsOneUnconfinedLeavePerNodeWithWhoGaveIt(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0085 := stepIndex(t, steps, 85)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0085+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1), (2, 'old', 1)",
		"INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1)",
		"INSERT INTO node_unconfined_leave (node_id, accepted_by_user_id, accepted_at) VALUES (1, 1, 5)")
	if got := fmt.Sprint(queryRows(t, database,
		"SELECT node_id, accepted_by_user_id, accepted_at FROM node_unconfined_leave", 3)); got != "[[1 1 5]]" {
		t.Errorf("node_unconfined_leave = %s; want node 1's leave, given by user 1 at 5", got)
	}
	insert := "INSERT INTO node_unconfined_leave (node_id, accepted_by_user_id, accepted_at) VALUES (?, ?, 6)"
	for _, refused := range []struct {
		nodeID, userID int64
		want           string
	}{{1, 1, "UNIQUE"}, {3, 1, "FOREIGN KEY"}, {2, 2, "FOREIGN KEY"}} {
		_, err = database.writer.ExecContext(t.Context(), insert, refused.nodeID, refused.userID)
		if err == nil || !strings.Contains(err.Error(), refused.want) {
			t.Errorf("a leave for node %d by user %d = %v; want it refused (%s)", refused.nodeID, refused.userID,
				err, refused.want)
		}
	}
}
