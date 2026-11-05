// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// Step 0070 takes the telemetry out of persistent.db (D426 §9): resource_use and link_memory are
// gone from it, and the rest of the database is kept. The cache holds resource_use; link_memory
// returned to persistent.db at step 0073 (D427).
func TestMigrate_Step0070MovesTheTelemetryToTheCache(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0070 := stepIndex(t, steps, 70)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0070], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO resource_use (at, process, scope, held_bytes, cache_bytes) VALUES (1, 'core', 'process', 1, 1)",
		`INSERT INTO link_memory (player, network, steadiness, steadiness_at, said_at)
		 VALUES ('device 3', '192.0.2.0/24', 2, 1, 1)`)
	if err = database.migrate(t.Context(), steps[:step0070+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	tables := fmt.Sprint(queryRows(t, database, `SELECT name FROM sqlite_schema
		WHERE name IN ('resource_use', 'link_memory', 'node') ORDER BY name`, 1))
	if tables != "[[node]]" {
		t.Errorf("persistent.db's tables after the step: %s; want [[node]] alone", tables)
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
		t.Errorf("cache.db's telemetry tables: %s; want resource_use alone (D427)", inCache)
	}
}
