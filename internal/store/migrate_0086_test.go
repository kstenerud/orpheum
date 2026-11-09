// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"testing"
)

// Step 0086 renames the names a database keeps that the API says, each to its camelCase (the owner,
// 2026-11-09): a role's capabilities, each whole within its comma list, a device secret's grant scope,
// a setting's key, a retired one too, and the skip rule or error code a node gave a file it did not
// file. A name with no underscore, and a filing reason that is prose, stay as they were.
func TestMigrate_Step0086ReadsBackEveryKeptNameInCamelCase(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	step0086 := stepIndex(t, steps, 86)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:step0086], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		`INSERT INTO role (id, is_builtin, capabilities, name) VALUES
		 (3, 0, 'download,remote_access,edit_metadata', 'helper')`,
		`INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1)`,
		`INSERT INTO device (id, user_id, name, platform, added_at, last_signed_in_at)
		 VALUES (1, 1, 'TV', 'webos', 1, 1)`,
		`INSERT INTO "grant" (secret_sha256, kind, user_id, scope, issued_at, device_id) VALUES
		 (X'01', 'issued_secret', 1, 'device_secret', 1, 1), (X'02', 'session', 1, 'role', 1, 1)`,
		`INSERT INTO setting (key, value_json, set_at, is_modified) VALUES
		 ('finished_after_percent', '95', 1, 1), ('log_level', '"debug"', 1, 1),
		 ('resume_min_percent', '5', 1, 1), ('a_typo_no_release_reads', '1', 1, 1)`,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1)",
		"INSERT INTO file (id) VALUES (1), (2)",
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, filing, filing_reason,
		 first_seen_at, last_seen_at) VALUES
		 (1, 1, 1, '/a', CAST('/a' AS BLOB), 'skipped', 'disc_folder_not_supported_yet', 1, 1),
		 (2, 2, 1, '/b', CAST('/b' AS BLOB), 'skipped', 'not_found: the file is gone', 1, 1)`)
	if err = database.migrate(t.Context(), steps[:step0086+1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	roles := fmt.Sprint(queryRows(t, database, "SELECT id, capabilities FROM role ORDER BY id", 2))
	want := "[[1 administer,deleteMedia,download,editMetadata,hostWatchTogether,remoteAccess] " +
		"[2 hostWatchTogether,remoteAccess] [3 download,remoteAccess,editMetadata]]"
	if roles != want {
		t.Errorf("the roles after the step: %s; want %s", roles, want)
	}
	grants := fmt.Sprint(queryRows(t, database, `SELECT kind, scope FROM "grant" ORDER BY secret_sha256`, 2))
	if want := "[[issued_secret deviceSecret] [session role]]"; grants != want {
		t.Errorf("the grants after the step: %s; want %s", grants, want)
	}
	settings := fmt.Sprint(queryRows(t, database, "SELECT key, value_json FROM setting ORDER BY key", 2))
	if want := `[[a_typo_no_release_reads 1] [finishedAfterPercent 95] [logLevel "debug"] ` +
		`[resumeMinPercent 5]]`; settings != want {
		t.Errorf("the settings after the step: %s; want %s", settings, want)
	}
	places := fmt.Sprint(queryRows(t, database, "SELECT id, filing_reason FROM place ORDER BY id", 2))
	if want := "[[1 discFolderNotSupportedYet] [2 not_found: the file is gone]]"; places != want {
		t.Errorf("the places after the step: %s; want %s", places, want)
	}
}
