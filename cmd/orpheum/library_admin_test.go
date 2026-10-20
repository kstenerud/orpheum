// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The server's library admin service names as many of a held-back scan's removals as its setting
// says (the constants sweep, 2026-10-20).
func TestLibraryAdminOf_NamesAsManyRemovalsAsTheServersSetting(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	var opened openedServices
	opened.settings = settingstest.Seeded(t, database)
	settingstest.Set(t, opened.settings, settings.ScanWithheldRemovalsNamed, "1")
	storetest.ExecRaw(t, database.Path(),
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/media')",
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at)
		 VALUES (1, 1, 1, '/media/a.mkv', CAST('/media/a.mkv' AS BLOB), 'skipped', 1, 1),
		        (2, 1, 1, '/media/b.mkv', CAST('/media/b.mkv' AS BLOB), 'skipped', 1, 1)`,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at,
		 withheld_reason) VALUES (1, '/media', 'withheld', 9000, 5000, 'the scan would remove 2 of 2 files')`)
	waiting, err := libraryAdminOf(database, opened).Waiting(t.Context())
	if err != nil || len(waiting) != 1 || waiting[0].FileCount != 2 ||
		!slices.Equal(waiting[0].FirstPaths, []string{"/media/a.mkv"}) {
		t.Errorf("waiting = %+v (%v); want both counted and the first alone named", waiting, err)
	}
}
