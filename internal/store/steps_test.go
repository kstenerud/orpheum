// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestMigrate_Step0007KeepsEveryStoredResponseAsItsItemsOwn(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:6], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at) "+
			"VALUES (1, 'Films', 'movies', 'tmdb', 'manual', 0, 1)",
		"INSERT INTO item (id, kind, library_id, created_at) VALUES (5, 'title', 1, 1)",
		"INSERT INTO title (id, kind, ident) VALUES (5, 'movie', 'identified')",
		"INSERT INTO provider_response (title_id, provider, fetched_at, body) VALUES (5, 'tmdb', 7, X'01')")
	if err = database.migrate(t.Context(), steps[:7], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, "SELECT item_id, provider, part, fetched_at, hex(body) FROM provider_response", 5)
	if fmt.Sprint(rows) != "[[5 tmdb  7 01]]" {
		t.Errorf("after step 0007, provider_response holds %v; want the film's response, as its item's '' part", rows)
	}
	state, err := database.State(t.Context())
	if err != nil || state.CompatibleVersion != 7 {
		t.Errorf("compatible_version = %d (%v); step 0007 renames a column an older binary reads",
			state.CompatibleVersion, err)
	}
}

func TestMigrate_RecordsEveryStepsChecksumAndRefusesOneThatChanged(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, "SELECT count(*), min(version), max(version) FROM schema_step", 3)
	if fmt.Sprint(rows) != fmt.Sprintf("[[%d 1 %d]]", len(steps), len(steps)) {
		t.Fatalf("schema_step holds %v; want every step, the ones before it included", rows)
	}
	if again := database.Migrate(t.Context(), testMigrateOptions(t.TempDir())); again != nil {
		t.Fatalf("a second Migrate over the same steps: %v", again)
	}
	// As a draft step looks once it was applied and then edited before it was committed.
	execRaw(t, database, "UPDATE schema_step SET sha256 = 'a draft' WHERE version = 7")
	err = database.Migrate(t.Context(), testMigrateOptions(t.TempDir()))
	if !errors.Is(err, ErrStepChanged) || !strings.Contains(err.Error(), steps[6].Name) {
		t.Errorf("Migrate over a changed step = %v; want ErrStepChanged naming it", err)
	}
}

func TestMigrate_Step0011KeysEveryFileByItsReportedBytes(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:10], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		"INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at) "+
			"VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)",
		"INSERT INTO file (library_id, node_id, path, path_raw, filing, first_seen, last_seen) VALUES "+
			"(1, 1, '/m/plain.mkv', NULL, 'listed', 1, 1), (1, 1, '/m/a�.mkv', X'2F6D2F61FF2E6D6B76', 'listed', 1, 1)")
	if err = database.migrate(t.Context(), steps[:11], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, "SELECT hex(path_key) FROM file ORDER BY id", 1)
	if fmt.Sprint(rows) != "[[2F6D2F706C61696E2E6D6B76] [2F6D2F61FF2E6D6B76]]" {
		t.Errorf("after step 0011, the keys are %v; want each file's reported bytes", rows)
	}
	// Two live files whose names are one NFC text are two files now.
	execRaw(t, database, "INSERT INTO file (library_id, node_id, path, path_key, filing, first_seen, last_seen) "+
		"VALUES (1, 1, '/m/a�.mkv', X'2F6D2F61FE2E6D6B76', 'listed', 1, 1)")
}

func TestMigrate_Step0012RenamesColumnsKeepingTheirDataAndTheStoredKeys(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:11], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		"INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at) "+
			"VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)",
		"INSERT INTO file (id, library_id, node_id, path, path_key, size, mtime, payload_len, filing, "+
			"error_detail, probe, first_seen, last_seen) "+
			"VALUES (1, 1, 1, '/m/a.mkv', X'2F6D2F612E6D6B76', 2048, 7, 1000, 'errored', "+
			`'{"text":"denied","owner":1000,"group":100,"mode":"0600","count":3}', `+
			`'{"format":{"bitRate":800},"streams":[{"bitRate":700,"audio":{"sampleRate":48000}}]}', 5, 6)`,
		"INSERT INTO item (id, kind, library_id, created_at) VALUES (1, 'title', 1, 1)",
		"INSERT INTO title (id, kind, ident) VALUES (1, 'movie', 'identified')",
		"INSERT INTO item_text (item_id, lang, is_original, name, search_alt) VALUES (1, 'und', 1, 'Sølvi', 'solvi')")
	if err = database.migrate(t.Context(), steps[:12], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	file := queryRows(t, database, "SELECT size_bytes, mtime_ms, payload_bytes, first_seen_at, last_seen_at, "+
		"error_detail, probe FROM file", 7)
	want := `[[2048 7 1000 5 6 {"text":"denied","ownerUid":1000,"groupGid":100,"mode":"0600","repeatCount":3} ` +
		`{"format":{"bitRateBps":800},"streams":[{"bitRateBps":700,"audio":{"sampleRateHz":48000}}]}]]`
	if fmt.Sprint(file) != want {
		t.Errorf("after step 0012 the file row is\n%v\nwant\n%s", file, want)
	}
	if title := queryRows(t, database, "SELECT identification FROM title", 1); fmt.Sprint(title) != "[[identified]]" {
		t.Errorf("the title's identification is %v, want identified", title)
	}
	// The search indexes were rebuilt over the renamed column, and still find the folded name.
	found := queryRows(t, database, "SELECT rowid FROM item_fts WHERE item_fts MATCH 'solvi'", 1)
	if len(found) != 1 {
		t.Errorf("the word index finds %v for the folded name, want the one row", found)
	}
}

func TestMigrate_Step0020RenamesTVSpeakersToBuiltInSpeakersKeepingTheDevicesAndTheirIDs(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:19], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database,
		"INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1)",
		`INSERT INTO device (id, user_id, name, platform, audio_output, first_seen_at, last_seen_at, removed_at)
		 VALUES (3, 1, 'Laptop', 'web', 'tv_speakers', 1, 2, NULL), (5, 1, 'Phone', 'web', 'headphones', 1, 2, NULL),
		        (8, 1, 'Old', 'web', NULL, 1, 2, 3)`,
		"DELETE FROM device WHERE id = 8")
	if err = database.migrate(t.Context(), steps[:20], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, "SELECT id, audio_output FROM device ORDER BY id", 2)
	if fmt.Sprint(rows) != "[[3 built_in_speakers] [5 headphones]]" {
		t.Errorf("after step 0020 the devices are %v; want the laptop's speakers built-in, the rest kept", rows)
	}
	// The id sequence is kept: a device made next never takes an id an earlier one had.
	execRaw(t, database,
		"INSERT INTO device (user_id, name, platform, first_seen_at, last_seen_at) VALUES (1, 'New', 'web', 4, 4)")
	if next := queryRows(t, database, "SELECT max(id) FROM device", 1); fmt.Sprint(next) != "[[9]]" {
		t.Errorf("the next device's id is %v, want 9, after the 8 an earlier device had", next)
	}
}
