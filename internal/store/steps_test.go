// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The runner's tests run synthetic steps (migrate_test.go's firstStep and the ones below): until
// migrations are live the real ones are a baseline and the one step after it (D228), and what the
// runner does to any step is tested here so it is ready for every step to come.

func TestMigrate_RecordsEveryStepsChecksumAndRefusesOneThatChanged(t *testing.T) {
	t.Parallel()
	built := openMigrated(t)
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	var want [][]string
	for _, step := range steps {
		want = append(want, []string{fmt.Sprint(step.Version), step.Checksum})
	}
	if rows := queryRows(t, built, "SELECT version, sha256 FROM schema_step ORDER BY version", 2); fmt.Sprint(rows) !=
		fmt.Sprint(want) {
		t.Errorf("schema_step holds %v; want every step's checksum, the baseline's at its version: %v", rows, want)
	}

	synthetic := syntheticSteps(t, firstStep, secondStep, thirdStep)
	database := openTemporary(t)
	if err = database.migrate(t.Context(), synthetic, testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, "SELECT count(*), min(version), max(version) FROM schema_step", 3)
	if fmt.Sprint(rows) != "[[3 1 3]]" {
		t.Fatalf("schema_step holds %v; want every step", rows)
	}
	if again := database.migrate(t.Context(), synthetic, testMigrateOptions(t.TempDir())); again != nil {
		t.Fatalf("a second migrate over the same steps: %v", again)
	}
	// As a draft step looks once it was applied and then edited before it was committed.
	execRaw(t, database, "UPDATE schema_step SET sha256 = 'a draft' WHERE version = 2")
	err = database.migrate(t.Context(), synthetic, testMigrateOptions(t.TempDir()))
	if !errors.Is(err, ErrStepChanged) || !strings.Contains(err.Error(), synthetic[1].Name) {
		t.Errorf("migrate over a changed step = %v; want ErrStepChanged naming it", err)
	}
}

// foldedBaseline is a baseline at version 3 that folds firstStep, secondStep and thirdStep: what
// the three build, in one step, with the third's checksum as the one it folded.
func foldedBaseline(t *testing.T) string {
	t.Helper()
	third := syntheticSteps(t, firstStep, secondStep, thirdStep)[2]
	return baselineDirective + " " + third.Checksum + "\n" + firstStep + "\n" + secondStep + "\n" + thirdStep
}

// fourthStep is the one step after the folded baseline.
const fourthStep = "CREATE TABLE later (id INTEGER PRIMARY KEY);"

func TestMigrate_ABaselineBuildsANewDatabaseAtItsVersion(t *testing.T) {
	t.Parallel()
	steps := stepsFrom(t, 3, foldedBaseline(t), fourthStep)
	if !steps[0].IsBaseline || steps[0].Version != 3 || steps[1].Version != 4 {
		t.Fatalf("read the steps as %+v; want a baseline at 3 and step 4", steps)
	}
	database := openTemporary(t)
	var progress []int
	options := testMigrateOptions(t.TempDir())
	options.OnBeforeStep = func(step, total int) { progress = append(progress, step, total) }
	if err := database.migrate(t.Context(), steps, options); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(progress) != "[1 2 2 2]" {
		t.Errorf("progress = %v; want the baseline and the step after it, [1 2 2 2]", progress)
	}
	state, err := database.State(t.Context())
	if err != nil || state.Version != 4 || state.CompatibleVersion != 3 {
		t.Errorf("state = %+v (%v); want version 4, compatible_version 3, the baseline's", state, err)
	}
	if rows := queryRows(t, database, "SELECT version FROM schema_step ORDER BY version", 1); fmt.Sprint(rows) !=
		"[[3] [4]]" {
		t.Errorf("schema_step's versions are %v; want the baseline's and the step's, [[3] [4]]", rows)
	}
	if rows := queryRows(t, database, "SELECT id, name FROM live_thing ORDER BY id", 2); fmt.Sprint(rows) !=
		"[[1 first] [2 second]]" {
		t.Errorf("the baseline's rows are %v; want the ones its folded steps made", rows)
	}
	reopened, err := Open(t.Context(), database.Path(), testOpenOptions())
	if err != nil {
		t.Fatalf("a file a baseline built is refused as not ours: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrate_ADatabaseMigratedThroughTheFoldedStepsCarriesOn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                 string
		atVersion            int
		isBuiltByTheBaseline bool
		afterwards           string
		wantErr              error
		wantInMessage        string
		wantVersion          int
	}{
		{"one at the baseline's version, through the steps it folded", 3, false, "", nil, "", 4},
		{"one the baseline built", 3, true, "", nil, "", 4},
		{
			"one whose record at the baseline's version is neither", 3, false,
			"UPDATE schema_step SET sha256 = 'other' WHERE version = 3", ErrStepChanged, "make dev-reset", 3,
		},
		{
			"one below the baseline's version", 2, false, "", ErrBeforeBaseline,
			"`make dev-reset`, and the next `make dev`", 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			database := openTemporary(t)
			built := syntheticSteps(t, firstStep, secondStep, thirdStep)[:tc.atVersion]
			if tc.isBuiltByTheBaseline {
				built = stepsFrom(t, 3, foldedBaseline(t))
			}
			if err := database.migrate(t.Context(), built, testMigrateOptions(t.TempDir())); err != nil {
				t.Fatal(err)
			}
			if tc.afterwards != "" {
				execRaw(t, database, tc.afterwards)
			}
			err := database.migrate(t.Context(), stepsFrom(t, 3, foldedBaseline(t), fourthStep),
				testMigrateOptions(t.TempDir()))
			if !errors.Is(err, tc.wantErr) || (err != nil && !strings.Contains(err.Error(), tc.wantInMessage)) {
				t.Errorf("migrate = %v; want %v, saying %q", err, tc.wantErr, tc.wantInMessage)
			}
			assertVersion(t, database, tc.wantVersion)
		})
	}
}

func TestMigrate_ADataTransformingStepKeepsTheRowsAndRaisesTheCompatibleVersion(t *testing.T) {
	t.Parallel()
	database := openTemporary(t)
	if err := database.migrate(t.Context(), syntheticSteps(t, firstStep, secondStep, thirdStep),
		testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	state, err := database.State(t.Context())
	if err != nil || state.CompatibleVersion != 1 {
		t.Errorf("after additive steps, compatible_version = %d (%v); want 1, the first step's",
			state.CompatibleVersion, err)
	}
	execRaw(t, database, "INSERT INTO thing (name) VALUES ('third')")
	transform := raisesCompatibleDirective + "\n" +
		"ALTER TABLE thing RENAME COLUMN name TO label;\n" +
		"ALTER TABLE thing ADD COLUMN detail_json TEXT;\n" +
		"UPDATE thing SET detail_json = json_object('wasNamed', label), label = upper(label);"
	err = database.migrate(t.Context(), syntheticSteps(t, firstStep, secondStep, thirdStep, transform, fourthStep),
		testMigrateOptions(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	rows := queryRows(t, database, "SELECT id, label, detail_json FROM thing ORDER BY id", 3)
	want := `[[1 FIRST {"wasNamed":"first"}] [2 SECOND {"wasNamed":"second"}] [3 THIRD {"wasNamed":"third"}]]`
	if fmt.Sprint(rows) != want {
		t.Errorf("after the transforming step, the rows are\n%v\nwant\n%s", rows, want)
	}
	state, err = database.State(t.Context())
	if err != nil || state.CompatibleVersion != 4 || state.Version != 5 {
		t.Errorf("state = %+v (%v); want version 5, compatible_version 4: the renaming step raises it, "+
			"the additive one after it does not", state, err)
	}
}

// rebuildThing rebuilds thing with a new CHECK, in the documented pattern (migrations.md), and
// carries its id sequence over unless told not to.
func rebuildThing(isSequenceKept bool) string {
	keepSequence := ""
	if isSequenceKept {
		keepSequence = "UPDATE sqlite_sequence SET seq = (SELECT seq FROM sqlite_sequence WHERE name = 'thing') " +
			"WHERE name = 'thing_new';\n"
	}
	return "DROP VIEW live_thing;\n" +
		"CREATE TABLE thing_new (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL CHECK (name <> ''));\n" +
		"INSERT INTO thing_new (id, name) SELECT id, name FROM thing;\n" +
		keepSequence +
		"DROP TABLE thing;\n" +
		"ALTER TABLE thing_new RENAME TO thing;\n" +
		"CREATE VIEW live_thing AS SELECT id, name FROM thing;"
}

func TestMigrate_ARebuildKeepsTheIDsAndTheIDSequence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		isSequenceKept bool
		wantRefusal    string
	}{
		{"carrying the sequence over", true, ""},
		{"losing the sequence", false, "went down"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			database := openTemporary(t)
			good := syntheticSteps(t, firstStep, secondStep, thirdStep)
			if err := database.migrate(t.Context(), good, testMigrateOptions(t.TempDir())); err != nil {
				t.Fatal(err)
			}
			// The last thing made is gone: its id must never be issued again (D173).
			execRaw(t, database, "INSERT INTO thing (name) VALUES ('third'), ('fourth')",
				"DELETE FROM thing WHERE id = 4")
			err := database.migrate(t.Context(), syntheticSteps(t, firstStep, secondStep, thirdStep,
				rebuildThing(tc.isSequenceKept)), testMigrateOptions(t.TempDir()))
			if tc.wantRefusal != "" {
				stepError, isStepError := errors.AsType[*StepError](err)
				if !isStepError || !strings.Contains(stepError.Error(), tc.wantRefusal) {
					t.Errorf("a rebuild that loses the id sequence: migrate = %v; want a StepError saying %q",
						err, tc.wantRefusal)
				}
				assertVersion(t, database, 3)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertRebuiltThingKeptItsIDs(t, database)
		})
	}
}

// assertRebuiltThingKeptItsIDs checks that the rebuilt thing kept every row's id, and issues
// the next id after the highest it ever issued.
func assertRebuiltThingKeptItsIDs(t *testing.T, database *Database) {
	t.Helper()
	if rows := queryRows(t, database, "SELECT id, name FROM live_thing ORDER BY id", 2); fmt.Sprint(rows) !=
		"[[1 first] [2 second] [3 third]]" {
		t.Errorf("after the rebuild, the things are %v; want every one kept with its id", rows)
	}
	execRaw(t, database, "INSERT INTO thing (name) VALUES ('fifth')")
	if next := queryRows(t, database, "SELECT max(id) FROM thing", 1); fmt.Sprint(next) != "[[5]]" {
		t.Errorf("the next thing's id is %v, want 5, after the 4 an earlier thing had", next)
	}
}

// migrationsLiveMarker is the file in migrations/ whose presence says migrations are live (D228):
// from then on the baseline is frozen and every schema change is a new step.
const migrationsLiveMarker = "LIVE"

func TestSteps_AreABaselineAndOneStepUntilMigrationsAreLive(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) == 0 {
		t.Fatal("there is no migration step")
	}
	_, err = os.Stat(filepath.Join("migrations", migrationsLiveMarker))
	switch {
	case errors.Is(err, os.ErrNotExist):
		if !steps[0].IsBaseline || len(steps) > 2 {
			t.Errorf("the steps are %d, the first a baseline: %t; until migrations are live, a schema change "+
				"folds the last step into the baseline and adds the change as the one step after it (D228)",
				len(steps), steps[0].IsBaseline)
		}
	case err != nil:
		t.Fatal(err)
	}
}

func TestMigrate_Step0028KeepsAWorksLastChoiceAndWhatStartedFilesUsedAndSpellsOutSettingUnits(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	// A series (item 20) with episodes 21, 22 and 23 (files 1, 2 and 3; file 5 is episode 21's other
	// copy) and a film (item 30, file 4). English was chosen in file 1 and carried to 2 and 3, of
	// which only 3 was started; forced French was chosen later in file 5; the film's subtitles were
	// turned off.
	const english = `{"streamId":7,"track":{"language":"en","title":"Full","isHearingImpaired":true}}`
	execRaw(t, database,
		"INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1), (2, 'partner', 2, 1)",
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		"INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at) "+
			"VALUES (1, 'All', 'mixed', 'nfo', 'manual', 0, 1)",
		`INSERT INTO item (id, kind, library_id, added_at) VALUES
		 (20, 'collection', 1, 1), (21, 'title', 1, 1), (22, 'title', 1, 1), (23, 'title', 1, 1), (30, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (21, 'episode', 'identified'),
		 (22, 'episode', 'identified'), (23, 'episode', 'identified'), (30, 'movie', 'identified')`,
		"INSERT INTO collection (id, kind, membership, created_by) VALUES (20, 'series', 'explicit', 'scanner')",
		"INSERT INTO collection_member (collection_id, member_title) VALUES (20, 21), (20, 22), (20, 23)",
		"INSERT INTO version (id, title_id, created_at) VALUES "+
			"(1, 21, 1), (2, 22, 1), (3, 23, 1), (4, 30, 1), (5, 21, 1)",
		`INSERT INTO file (id, library_id, node_id, path, filing, version_id, first_seen_at, last_seen_at) VALUES
		 (1, 1, 1, '/1', 'listed', 1, 1, 1), (2, 1, 1, '/2', 'listed', 2, 1, 1), (3, 1, 1, '/3', 'listed', 3, 1, 1),
		 (4, 1, 1, '/4', 'listed', 4, 1, 1), (5, 1, 1, '/5', 'listed', 5, 1, 1)`,
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at) VALUES (1, 3, 60000, 1, 12)`,
		`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at) VALUES
		 (1, 'file', 1, 'subtitle_choice', '`+english+`', 10),
		 (1, 'file', 2, 'subtitle_choice', '{"streamId":8,"isCarriedFromWork":true,"track":{"language":"en"}}', 10),
		 (1, 'file', 3, 'subtitle_choice', '{"streamId":9,"isCarriedFromWork":true,"track":{"language":"en"}}', 10),
		 (1, 'file', 5, 'subtitle_choice', '{"streamId":6,"track":{"language":"fr","isForced":true}}', 20),
		 (1, 'file', 4, 'subtitle_choice', '{"isOff":true}', 30),
		 (1, 'file', 4, 'subtitle_offset_ms', '500', 30),
		 (2, 'user', 0, 'levelling', 'false', 5)`,
		`INSERT INTO setting (scope, user_id, key, value_json, is_modified, set_at) VALUES
		 ('server', NULL, 'job_first_retry_delay_s', '30', 1, 1), ('server', NULL, 'tmdb_refresh_after_d', '7', 0, 1),
		 ('server', NULL, 'signin_failure_limit', '5', 0, 1), ('user', 1, 'old_key', '1', 1, 1)`)
	if err = database.migrate(t.Context(), steps[:2], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query   string
		columns int
		want    string
	}{
		{
			"SELECT user_id, scope, scope_id, axis, value_json, set_at FROM preference " +
				"ORDER BY user_id, scope, scope_id, axis",
			6,
			"[[1 file 1 subtitle_used " +
				`{"streamId":7,"track":{"language":"en","containerTitle":"Full","kind":"hearing_impaired"}} 10] ` +
				`[1 file 3 subtitle_used ` +
				`{"streamId":9,"track":{"language":"en","containerTitle":"","kind":"plain"}} 10] ` +
				`[1 file 4 subtitle_offset_ms 500 30] [1 file 4 subtitle_used {"isOff":true} 30] ` +
				`[1 file 5 subtitle_used ` +
				`{"streamId":6,"track":{"language":"fr","containerTitle":"","kind":"forced"}} 20] ` +
				`[1 work 20 subtitle_language {"language":"fr","containerTitle":"","kind":"forced"} 20] ` +
				`[1 work 30 subtitle_language {"isOff":true} 30] [2 user 0 levelling false 5]]`,
		},
		{
			"SELECT key, value_json, is_modified FROM setting ORDER BY key", 3,
			"[[job_first_retry_delay_seconds 30 1] [signin_failure_limit 5 0] [tmdb_refresh_after_days 7 0]]",
		},
	} {
		if got := fmt.Sprint(queryRows(t, database, check.query, check.columns)); got != check.want {
			t.Errorf("after step 0028, %s =\n%s\nwant\n%s (D230)", check.query, got, check.want)
		}
	}
}
