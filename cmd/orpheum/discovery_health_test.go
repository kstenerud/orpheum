// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The core's health for its discovery is whether it reads its database: an open one passes, and a
// closed or missing one fails, so the core lets its realm go to a copy that can serve (D437 §2).
func TestDatabaseHealth_PassesWhileTheDatabaseReadsAndFailsOnceItDoesNot(t *testing.T) {
	t.Parallel()
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	healthy := databaseHealth(database)
	if err = healthy(t.Context()); err != nil {
		t.Errorf("an open database reads as unhealthy: %v", err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	if healthy(t.Context()) == nil {
		t.Error("a closed database reads as healthy")
	}
	if databaseHealth(nil)(t.Context()) == nil {
		t.Error("no database reads as healthy")
	}
}
