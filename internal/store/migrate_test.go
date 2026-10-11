// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// updateGolden regenerates the golden schema. It is never set by default or in CI; the golden changes
// only as the subject of its own commit (standards/testing.md §8).
var updateGolden = flag.Bool("update", false, "rewrite testdata/golden from the current schema and print what changed")

// dumpSchema renders a database's schema as text for comparison: the bookkeeping integers, the
// id sequences, and every schema object's SQL in a stable order.
func dumpSchema(t *testing.T, database *Database) string {
	t.Helper()
	var applicationID int64
	err := database.readers.QueryRowContext(t.Context(), "PRAGMA application_id").Scan(&applicationID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := database.State(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	fmt.Fprintf(&text, "-- application_id %d\n-- user_version %d\n-- compatible_version %d\n",
		applicationID, state.Version, state.CompatibleVersion)
	for _, row := range queryRows(t, database, "SELECT name, seq FROM sqlite_sequence ORDER BY name", 2) {
		fmt.Fprintf(&text, "-- sqlite_sequence %s %s\n", row[0], row[1])
	}
	const objects = "SELECT type, name, sql FROM sqlite_schema WHERE sql IS NOT NULL ORDER BY type, name"
	for _, row := range queryRows(t, database, objects, 3) {
		fmt.Fprintf(&text, "\n-- %s %s\n%s;\n", row[0], row[1], row[2])
	}
	return text.String()
}

// queryRows runs a query on the readers and returns each row's columns as text.
func queryRows(t *testing.T, database *Database, query string, columns int) [][]string {
	t.Helper()
	rows, err := database.readers.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	var result [][]string
	for rows.Next() {
		values := make([]string, columns)
		pointers := make([]any, columns)
		for index := range values {
			pointers[index] = &values[index]
		}
		if scanErr := rows.Scan(pointers...); scanErr != nil {
			t.Fatal(scanErr)
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

// rewriteGolden writes contents to path for -update, and says whether it changed.
func rewriteGolden(t *testing.T, path, contents string) {
	t.Helper()
	previous, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if string(previous) != contents {
		t.Logf("rewriting %s: it changed", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestMigrate_FreshSchemaEqualsTheGolden compares a new database with the golden of its version:
// at the baseline, built by it alone, and at the current version. Until migrations are live the
// two are the baseline and the one step after it (D228), and their goldens' difference is what
// that step does.
func TestMigrate_FreshSchemaEqualsTheGolden(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	built := [][]Step{steps}
	if steps[0].IsBaseline && len(steps) > 1 {
		built = append(built, steps[:1])
	}
	for _, through := range built {
		database := openTemporary(t)
		if err := database.migrate(t.Context(), through, testMigrateOptions(t.TempDir())); err != nil {
			t.Fatal(err)
		}
		got := dumpSchema(t, database)
		golden := filepath.Join("testdata", "golden", fmt.Sprintf("schema-v%04d.sql", lastVersion(through)))
		if *updateGolden {
			rewriteGolden(t, golden, got)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("the baseline and the current version each have a golden, and %s cannot be read: %v", golden, err)
		}
		if got != string(want) {
			t.Errorf("a fresh schema differs from %s; a schema change is a step, and the goldens are rewritten "+
				"with -update (D228).\ngot:\n%s", golden, got)
		}
	}
}

// syntheticSteps builds a step list from SQL texts, numbered from 1.
func syntheticSteps(t *testing.T, texts ...string) []Step {
	t.Helper()
	return stepsFrom(t, 1, texts...)
}

// stepsFrom builds a step list from SQL texts, numbered from first: a baseline's version, when the
// first text is one.
func stepsFrom(t *testing.T, first int, texts ...string) []Step {
	t.Helper()
	files := fstest.MapFS{}
	for index, text := range texts {
		files[fmt.Sprintf("steps/%04d_step.sql", first+index)] = &fstest.MapFile{Data: []byte(text)}
	}
	steps, err := readSteps(files, "steps")
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

const (
	// A first schema, firstStep: the bookkeeping tables every schema has, and a table with rows.
	firstStep = raisesCompatibleDirective + "\n" +
		"CREATE TABLE schema_meta (id INTEGER PRIMARY KEY CHECK (id = 1), compatible_version INTEGER NOT NULL);\n" +
		"CREATE TABLE schema_step (version INTEGER PRIMARY KEY, sha256 TEXT NOT NULL);\n" +
		"CREATE TABLE thing (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL);\n" +
		"INSERT INTO thing (name) VALUES ('first'), ('second');"
	secondStep = "CREATE TABLE part (id INTEGER PRIMARY KEY AUTOINCREMENT, " +
		"thing_id INTEGER NOT NULL REFERENCES thing(id) ON DELETE RESTRICT);\n" +
		"INSERT INTO part (thing_id) VALUES (1);"
	thirdStep = "CREATE VIEW live_thing AS SELECT id, name FROM thing;"
)

func TestMigrate_ResumesAfterAnInterruption(t *testing.T) {
	t.Parallel()
	steps := syntheticSteps(t, firstStep, secondStep, thirdStep)

	uninterrupted := openTemporary(t)
	if err := uninterrupted.migrate(t.Context(), steps, testMigrateOptions(t.TempDir())); err != nil {
		t.Fatalf("uninterrupted migrate: %v", err)
	}

	interrupted := openTemporary(t)
	snapshots := t.TempDir()
	ctx, kill := context.WithCancel(t.Context())
	err := interrupted.migrate(ctx, steps, MigrateOptions{
		SnapshotDirectory:         snapshots,
		FreeSpaceDatabaseMultiple: 3,
		OnBeforeStep: func(step, _ int) {
			if step == 3 {
				kill()
			}
		},
	})
	stepError, isStepError := errors.AsType[*StepError](err)
	if !isStepError || stepError.Step.Version != 3 {
		t.Fatalf("interrupted migrate error = %v, want a StepError for step 3", err)
	}
	state, err := interrupted.State(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 2 {
		t.Fatalf("after the interruption, version = %d, want 2: each step commits on its own", state.Version)
	}

	var resumedAt []int
	err = interrupted.migrate(t.Context(), steps, MigrateOptions{
		SnapshotDirectory:         snapshots,
		FreeSpaceDatabaseMultiple: 3,
		OnBeforeStep:              func(step, total int) { resumedAt = append(resumedAt, step, total) },
	})
	if err != nil {
		t.Fatalf("resumed migrate: %v", err)
	}
	if fmt.Sprint(resumedAt) != "[1 1]" {
		t.Errorf("the resumed run's progress = %v, want only the one remaining step, [1 1]", resumedAt)
	}
	if got, want := dumpSchema(t, interrupted), dumpSchema(t, uninterrupted); got != want {
		t.Errorf("an interrupted and resumed migration ended differently.\nresumed:\n%s\nuninterrupted:\n%s", got, want)
	}
}

func TestMigrate_RollsBackAStepThatFailsHalfway(t *testing.T) {
	t.Parallel()
	steps := syntheticSteps(t, firstStep, "CREATE TABLE half (id INTEGER);\nINSERT INTO no_such_table VALUES (1);")
	database := openTemporary(t)
	err := database.migrate(t.Context(), steps, testMigrateOptions(t.TempDir()))
	stepError, isStepError := errors.AsType[*StepError](err)
	if !isStepError || !strings.Contains(stepError.Error(), "no_such_table") {
		t.Fatalf("migrate error = %v, want a StepError naming the failing statement", err)
	}
	assertVersion(t, database, 1)
	var halves int64
	err = database.readers.QueryRowContext(t.Context(),
		"SELECT count(*) FROM sqlite_schema WHERE name = 'half'").Scan(&halves)
	if err != nil {
		t.Fatal(err)
	}
	if halves != 0 {
		t.Error("the failed step's first statement survived; a step must roll back whole")
	}
}

func TestMigrate_RefusesAStepThatBreaksWhatSQLCannotCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		step  string
		wants string
	}{
		{
			name:  "a dangling foreign key",
			step:  "INSERT INTO part (thing_id) VALUES (99);",
			wants: "references a missing row",
		},
		{
			name:  "an id sequence moved backwards",
			step:  "UPDATE sqlite_sequence SET seq = 0 WHERE name = 'thing';",
			wants: "went down",
		},
		{
			name:  "a view over a dropped table",
			step:  "DROP TABLE part; ALTER TABLE thing RENAME TO old_thing;",
			wants: "no longer compiles",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			database := openTemporary(t)
			good := syntheticSteps(t, firstStep, secondStep, thirdStep)
			if err := database.migrate(t.Context(), good, testMigrateOptions(t.TempDir())); err != nil {
				t.Fatalf("the good steps: %v", err)
			}
			execRaw(t, database, "PRAGMA legacy_alter_table = ON")
			bad := syntheticSteps(t, firstStep, secondStep, thirdStep, tc.step)
			err := database.migrate(t.Context(), bad, testMigrateOptions(t.TempDir()))
			stepError, isStepError := errors.AsType[*StepError](err)
			if !isStepError || !strings.Contains(stepError.Error(), tc.wants) {
				t.Errorf("migrate error = %v, want a StepError saying %q", err, tc.wants)
			}
			assertVersion(t, database, 3)
		})
	}
}

func TestMigrate_RefusesANewerDatabaseAndNamesTheSnapshot(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	version, err := SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	snapshots := t.TempDir()

	// A newer release that only added: it left the file usable by us, so we run.
	execRaw(t, database, fmt.Sprintf("PRAGMA user_version = %d", version+1))
	err = database.Migrate(t.Context(), MigrateOptions{SnapshotDirectory: snapshots, FreeSpaceDatabaseMultiple: 3})
	if err != nil {
		t.Fatalf("a newer but compatible database: %v, want it to run", err)
	}

	// A newer release that reshaped: refused, with the way back named.
	execRaw(t, database, fmt.Sprintf("UPDATE schema_meta SET compatible_version = %d WHERE id = 1", version+1))
	err = database.Migrate(t.Context(), MigrateOptions{SnapshotDirectory: snapshots, FreeSpaceDatabaseMultiple: 3})
	refusal, isRefusal := errors.AsType[*NewerDatabaseError](err)
	if !isRefusal {
		t.Fatalf("migrate error = %v, want a NewerDatabaseError", err)
	}
	wantSnapshot := filepath.Join(snapshots, fmt.Sprintf("pre-migrate-v%d.sqlite", version))
	if !strings.Contains(refusal.Error(), wantSnapshot) {
		t.Errorf("the refusal does not name the snapshot %s:\n%v", wantSnapshot, err)
	}
	assertVersion(t, database, version+1)
}

func TestMigrate_KeepsTheSnapshotAfterSuccess(t *testing.T) {
	t.Parallel()
	database := openTemporary(t)
	snapshots := t.TempDir()
	options := MigrateOptions{SnapshotDirectory: snapshots, FreeSpaceDatabaseMultiple: 3}
	if err := database.migrate(t.Context(), syntheticSteps(t, firstStep), options); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(snapshots)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("migrating a new file wrote snapshots %v; there was nothing to keep", entries)
	}

	err = database.migrate(t.Context(), syntheticSteps(t, firstStep, secondStep), options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(snapshots, "pre-migrate-v1.sqlite")
	inspected, err := InspectBackup(t.Context(), snapshot)
	if err != nil {
		t.Fatalf("the snapshot taken before step 2 is not a usable backup after success: %v", err)
	}
	if inspected.State.Version != 1 {
		t.Errorf("snapshot version = %d, want 1, the version it was taken from", inspected.State.Version)
	}
}

func assertVersion(t *testing.T, database *Database, want int) {
	t.Helper()
	state, err := database.State(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != want {
		t.Errorf("user_version = %d, want %d", state.Version, want)
	}
}

func TestReadSteps_RefusesAGapOrAMisnamedStep(t *testing.T) {
	t.Parallel()
	baseline := &fstest.MapFile{Data: []byte(baselineDirective + " " + strings.Repeat("ab", 32) + "\n")}
	cases := map[string]fstest.MapFS{
		"a gap":                  {"steps/0001_a.sql": {}, "steps/0003_c.sql": {}},
		"a misnamed step":        {"steps/0001_a.sql": {}, "steps/2_b.sql": {}},
		"a first step after one": {"steps/0002_b.sql": {}, "steps/0003_c.sql": {}},
		"a baseline not first":   {"steps/0001_a.sql": {}, "steps/0002_b.sql": baseline},
		"a baseline with no checksum": {
			"steps/0004_baseline.sql": {Data: []byte(baselineDirective + "\n")}, "steps/0005_e.sql": {},
		},
	}
	for name, files := range cases {
		if _, err := readSteps(files, "steps"); err == nil {
			t.Errorf("%s: readSteps succeeded, want a refusal", name)
		}
	}
	steps, err := readSteps(fstest.MapFS{"steps/0001_a.sql": {}, "steps/0002_b.sql": {}}, "steps")
	if err != nil || len(steps) != 2 {
		t.Errorf("two good steps: %d steps, %v", len(steps), err)
	}
	steps, err = readSteps(fstest.MapFS{"steps/0004_baseline.sql": baseline, "steps/0005_e.sql": {}}, "steps")
	if err != nil || len(steps) != 2 || !steps[0].IsBaseline || steps[0].FoldedChecksum != strings.Repeat("ab", 32) ||
		steps[1].Version != 5 {
		t.Errorf("a baseline at 4 and step 5: %+v, %v", steps, err)
	}
}

// TestSteps_MatchTheCommittedChecksums fails when a committed step is edited, renamed or deleted,
// even outside an agent session where the pre-edit hook would have refused it
// (standards/sql.md §5). A new step adds its line to migrations/SHA256SUMS.
func TestSteps_MatchTheCommittedChecksums(t *testing.T) {
	t.Parallel()
	listed, err := os.ReadFile(filepath.Join("migrations", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(listed)), "\n") {
		sum, name, found := strings.Cut(line, "  ")
		if !found {
			t.Fatalf("SHA256SUMS line %q: want \"<sha256>  <file>\"", line)
		}
		want[name] = sum
	}
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) == 0 {
		t.Fatal("no migration steps; the check would pass vacuously")
	}
	for _, step := range steps {
		sum := sha256.Sum256([]byte(step.SQL))
		switch listedSum, found := want[step.Name]; {
		case !found:
			t.Errorf("%s is not in migrations/SHA256SUMS; add its line: %x  %s", step.Name, sum, step.Name)
		case listedSum != hex.EncodeToString(sum[:]):
			t.Errorf("%s differs from its committed checksum: a merged step is never edited; add a new step",
				step.Name)
		}
		delete(want, step.Name)
	}
	for name := range want {
		t.Errorf("migrations/SHA256SUMS lists %s, which is gone: a merged step is never deleted or renamed", name)
	}
}
