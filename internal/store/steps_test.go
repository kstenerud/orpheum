// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The runner's tests run synthetic steps (migrate_test.go's firstStep and the ones below): until
// migrations are live the real ones are a baseline and the one step after it (D228), or every step
// since production's checkpoint (D272 §5), and what the runner does to any step is tested here so
// it is ready for every step to come.

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

// fifthStep follows fourthStep, for a window of more than one step after the baseline (D272 §5).
const fifthStep = "CREATE TABLE latest (id INTEGER PRIMARY KEY);"

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

// checkpointFile is the file in migrations/ holding the schema version production last ran (D272
// §6): while it exists, the baseline stays at that version and every step after it is kept.
const checkpointFile = "CHECKPOINT"

// windowProblem says how steps break the migration window, or "" when they keep it. Live, every
// step is kept and nothing is held. Before it, while production runs (checkpoint > 0), the baseline
// is the checkpoint, with any number of steps after it (D272 §5); before production, a baseline and
// at most one step (D228).
func windowProblem(steps []Step, isLive bool, checkpoint int) string {
	switch {
	case isLive:
		return ""
	case len(steps) == 0 || !steps[0].IsBaseline:
		return "the first step is not a baseline; until migrations are live, the steps start with one (D228)"
	case checkpoint > 0 && steps[0].Version != checkpoint:
		return fmt.Sprintf("the baseline is at version %d and the checkpoint at %d; while production runs, the "+
			"baseline is the version it last ran, and a fold waits for the owner to move the checkpoint (D272 §5, §6)",
			steps[0].Version, checkpoint)
	case checkpoint == 0 && len(steps) > 2:
		return fmt.Sprintf("the steps are a baseline and %d after it; before production runs, a schema change folds "+
			"the last step into the baseline and adds the change as the one step after it (D228)", len(steps)-1)
	}
	return ""
}

// readCheckpoint is the version migrations/CHECKPOINT holds, or 0 when there is none.
func readCheckpoint(t *testing.T) int {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("migrations", checkpointFile))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	version, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil || version < 1 {
		t.Fatalf("migrations/%s holds %q; want the schema version production last ran", checkpointFile, text)
	}
	return version
}

func TestSteps_KeepTheMigrationWindow(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join("migrations", migrationsLiveMarker))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if problem := windowProblem(steps, err == nil, readCheckpoint(t)); problem != "" {
		t.Error(problem)
	}
}

func TestWindowProblem_HoldsTheBaselineAtTheCheckpointAndOneStepBeforeIt(t *testing.T) {
	t.Parallel()
	baselineAndOne := stepsFrom(t, 3, foldedBaseline(t), fourthStep)
	baselineAndTwo := stepsFrom(t, 3, foldedBaseline(t), fourthStep, fifthStep)
	noBaseline := syntheticSteps(t, firstStep, secondStep)
	for _, tc := range []struct {
		name        string
		steps       []Step
		isLive      bool
		checkpoint  int
		wantProblem string
	}{
		{"before production, a baseline and one step", baselineAndOne, false, 0, ""},
		{"before production, a second step", baselineAndTwo, false, 0, "folds the last step"},
		{"before production, no baseline", noBaseline, false, 0, "not a baseline"},
		{"while production runs, steps since the checkpoint", baselineAndTwo, false, 3, ""},
		{"while production runs, a baseline past the checkpoint", baselineAndOne, false, 2, "waits for the owner"},
		{"while production runs, no baseline", noBaseline, false, 1, "not a baseline"},
		{"live, anything", noBaseline, true, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problem := windowProblem(tc.steps, tc.isLive, tc.checkpoint)
			if tc.wantProblem == "" && problem != "" || !strings.Contains(problem, tc.wantProblem) {
				t.Errorf("windowProblem = %q; want one saying %q", problem, tc.wantProblem)
			}
		})
	}
}

// Step 0037 keeps every grant, with no locate hash or offer key, since no link made before it is
// registered with a discovery, and adds the empty locator table (discovery-core.md K7).
func TestMigrate_Step0037KeepsEachGrantAndAddsWhatADiscoveryFindsTheCoreBy(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, `INSERT INTO "grant" (secret_sha256, kind, scope, issued_at) VALUES (X'0a', 'session',
		'role', 1)`)
	if err = database.migrate(t.Context(), steps[:2], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(queryRows(t, database,
		`SELECT kind, locate_hash IS NULL, offer_key IS NULL, (SELECT count(*) FROM locator) FROM "grant"`, 4))
	if want := "[[session 1 1 0]]"; got != want {
		t.Errorf("the grants after the step are %s, want %s: each kept, no link's hashes, no locators", got, want)
	}
}

// Step 0038 keeps every grant with no sealed link, since the core kept no link's secret before it
// (discovery-core.md K7).
func TestMigrate_Step0038KeepsEachGrantWithNoSealedLink(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:2], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	execRaw(t, database, `INSERT INTO "grant" (secret_sha256, kind, scope, issued_at, locate_hash) VALUES
		(X'0a', 'issued_secret', 'invitation', 1, X'0b')`)
	if err = database.migrate(t.Context(), steps[:3], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(queryRows(t, database, `SELECT kind, locate_hash IS NULL, link_sealed IS NULL FROM "grant"`, 3))
	if want := "[[issued_secret 0 1]]"; got != want {
		t.Errorf("the grants after the step are %s, want %s: each kept, its hashes kept, no link sealed", got, want)
	}
}
