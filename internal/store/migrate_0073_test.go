// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// Step 0073 brings link_memory back to persistent.db (D427): a player's measured link is kept where
// a release that rebuilds cache.db cannot lose it, the rest of the database is kept, and cache.db no
// longer defines it.
func TestMigrate_Step0073BringsThePlayersLinkBackToPersistent(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0073 := stepIndex(t, steps, 73)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0073], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, "INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)")
	if err = database.migrate(t.Context(), steps[:step0073+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, `INSERT INTO link_memory (player, network, steadiness, steadiness_at, said_at)
		 VALUES ('device 3', '192.0.2.0/24', 2, 1, 1)`)
	links := fmt.Sprint(queryRows(t, database, "SELECT player, steadiness FROM link_memory", 2))
	if links != "[[device 3 2]]" {
		t.Errorf("persistent.db's links after the step: %s; want [[device 3 2]]", links)
	}
	if nodes := fmt.Sprint(queryRows(t, database, "SELECT id, name FROM node", 2)); nodes != "[[1 nas]]" {
		t.Errorf("the nodes after the step: %s; want [[1 nas]]", nodes)
	}
	cache, err := OpenCache(t.Context(), filepath.Join(t.TempDir(), "cache.db"), testOpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := cache.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	inCache := fmt.Sprint(queryRows(t, cache.database, `SELECT name FROM sqlite_schema
		WHERE name IN ('resource_use', 'link_memory') ORDER BY name`, 1))
	if inCache != "[[resource_use]]" {
		t.Errorf("cache.db's telemetry tables: %s; want resource_use alone", inCache)
	}
}
