// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"strings"
	"testing"
)

// Step 0084 adds the system library (D435): one row at most, by the kind 'system', naming a library
// that version_library, and so every list of titles, leaves out. A library that is not the system
// library is listed as it was.
func TestMigrate_Step0084LeavesTheSystemLibraryOutOfVersionLibrary(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0084 := stepIndex(t, steps, 84)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0084], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at) VALUES
		 (1, 'Clips', 'other', 'nfo', 'manual', 0, 1), (2, 'System', 'other', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/media'), (2, 1, '/own')`,
		"INSERT INTO item (id, kind, identification_state, added_at) VALUES (20, 'other', 'identified', 1)",
		"INSERT INTO item (id, kind, identification_state, added_at) VALUES (21, 'other', 'identified', 1)",
		"INSERT INTO work (id, kind) VALUES (20, 'other'), (21, 'other')",
		"INSERT INTO version (id, work_id, created_at) VALUES (30, 20, 1), (31, 21, 1)",
		"INSERT INTO file (id) VALUES (10), (11)",
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at) VALUES
		 (10, 10, 1, '/media/a.mkv', CAST('/media/a.mkv' AS BLOB), 'read', 1, 1),
		 (11, 11, 1, '/own/b.mp4', CAST('/own/b.mp4' AS BLOB), 'read', 1, 1)`,
		`INSERT INTO file_filing (file_id, library_kind, filing, version_id) VALUES
		 (10, 'other', 'listed', 30), (11, 'other', 'listed', 31)`)
	listed := func() string {
		return fmt.Sprint(queryRows(t, database, "SELECT version_id, library_id FROM version_library ORDER BY 1", 2))
	}
	if got := listed(); got != "[[30 1] [31 2]]" {
		t.Fatalf("before the step, version_library = %s; want both libraries' videos", got)
	}
	if err = database.migrate(t.Context(), steps[:step0084+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if got := listed(); got != "[[30 1] [31 2]]" {
		t.Errorf("after the step, with no system library, version_library = %s; want it unchanged", got)
	}
	execRaw(t, database, "INSERT INTO system_library (kind, library_id) VALUES ('system', 2)")
	if got := listed(); got != "[[30 1]]" {
		t.Errorf("with library 2 the system library, version_library = %s; want only library 1's video", got)
	}
	insert := "INSERT INTO system_library (kind, library_id) VALUES (?, 1)"
	_, err = database.writer.ExecContext(t.Context(), insert, "system")
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("a second system library = %v; want it refused, one per server", err)
	}
	_, err = database.writer.ExecContext(t.Context(), insert, "films")
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("a library of another kind = %v; want it refused by the kind's check", err)
	}
}
