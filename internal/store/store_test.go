// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// testOpenOptions are the store options these tests use; in production they come from the
// config file. They skip the disk's syncs, as storetest.OpenOptions do (test-weight.md §35).
func testOpenOptions() OpenOptions {
	return OpenOptions{MinimumReaders: 4, SkipsDiskSyncs: true}
}

// testMigrateOptions are the runner options these tests use, snapshotting into directory.
func testMigrateOptions(directory string) MigrateOptions {
	return MigrateOptions{SnapshotDirectory: directory, FreeSpaceDatabaseMultiple: 3}
}

// openTemporary opens a new database in the test's temporary directory.
func openTemporary(t *testing.T) *Database {
	t.Helper()
	database, err := Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), testOpenOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return database
}

// openMigrated opens a new database and migrates it to this binary's schema.
func openMigrated(t *testing.T) *Database {
	t.Helper()
	database := openTemporary(t)
	if err := database.Migrate(t.Context(), testMigrateOptions(t.TempDir())); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return database
}

// execRaw runs statements on the writer, bypassing the store's API, to build states the store
// itself would never produce.
func execRaw(t *testing.T, database *Database, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := database.writer.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
}

func TestOpen_RefusesAFileThatIsNotOurs(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "foreign.db")
	foreign, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = foreign.ExecContext(t.Context(), "CREATE TABLE someone_elses (id INTEGER)")
	if err != nil {
		t.Fatal(err)
	}
	err = foreign.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = Open(t.Context(), path, testOpenOptions())
	if !errors.Is(err, ErrNotOurs) {
		t.Errorf("Open(foreign file) error = %v, want ErrNotOurs", err)
	}

	// Positive siblings: an empty file and one of ours both open.
	empty := openTemporary(t)
	err = empty.Migrate(t.Context(), MigrateOptions{FreeSpaceDatabaseMultiple: 3})
	if err != nil {
		t.Fatalf("Migrate a new file: %v", err)
	}
	reopened, err := Open(t.Context(), empty.Path(), testOpenOptions())
	if err != nil {
		t.Fatalf("reopen our own file: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpen_RefusesAPathWithAQuestionMark(t *testing.T) {
	t.Parallel()
	_, err := Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db?mode=memory"), testOpenOptions())
	if err == nil || !strings.Contains(err.Error(), "may not contain '?'") {
		t.Errorf("Open error = %v, want the '?' refusal", err)
	}
}

func TestVerifyPragmas_RefusesAConnectionWithoutThem(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "plain.db")
	plain, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := plain.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	err = verifyPragmas(t.Context(), plain, synchronousNormal)
	if err == nil || !strings.Contains(err.Error(), "journal_mode") {
		t.Errorf("verifyPragmas(no pragmas) = %v, want a journal_mode refusal", err)
	}
	pragmas := strings.ReplaceAll(connectionPragmas, "&_pragma=temp_store(MEMORY)", "")
	withoutTempStore, err := sql.Open("sqlite", path+"?"+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := withoutTempStore.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	err = verifyPragmas(t.Context(), withoutTempStore, synchronousNormal)
	if err == nil || !strings.Contains(err.Error(), "temp_store") {
		t.Errorf("verifyPragmas(no temp_store) = %v, want a temp_store refusal", err)
	}

	// Production's options: these tests' own skip the disk's syncs.
	database, err := Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), OpenOptions{MinimumReaders: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := database.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	pools := map[string]*sql.DB{
		"writer": database.writer, "readers": database.readers, "maintenance": database.maintenance,
	}
	for name, pool := range pools {
		if err := verifyPragmas(t.Context(), pool, synchronousNormal); err != nil {
			t.Errorf("verifyPragmas(%s) = %v, want nil", name, err)
		}
	}
}

func TestRead_RefusesAWrite(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	_, err := database.readers.ExecContext(t.Context(), "INSERT INTO node (name, enrolled_at) VALUES ('x', 1)")
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Errorf("write through the reader pool: error = %v, want SQLite's read-only refusal", err)
	}
	write := "INSERT INTO node (name, enrolled_at) VALUES ('x', 1)"
	if _, err := database.writer.ExecContext(t.Context(), write); err != nil {
		t.Errorf("write through the writer: %v", err)
	}
}

func TestWrite_RollsBackWhenTheChangeFails(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	ctx := t.Context()
	failure := errors.New("the change failed")
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		upsertErr := queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
			Key: "shouldCheckForUpdates", ValueJSON: "false", SetAt: 1,
		})
		return errors.Join(upsertErr, failure)
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Write error = %v, want the change's error", err)
	}
	settings, err := database.Read().ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 0 {
		t.Errorf("after a failed change, settings = %v, want none", settings)
	}

	err = database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
			Key: "shouldCheckForUpdates", ValueJSON: "false", SetAt: 1,
		})
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	settings, err = database.Read().ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 || settings[0].Key != "shouldCheckForUpdates" {
		t.Errorf("after a committed change, settings = %v, want shouldCheckForUpdates", settings)
	}
}

func TestOrNotFound_TurnsNoRowsIntoErrNotFound(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	_, err := OrNotFound(database.Read().GetLiveUserByName(t.Context(), "nobody"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("OrNotFound(missing user) error = %v, want ErrNotFound", err)
	}
	role, err := OrNotFound(database.Read().GetRoleByName(t.Context(), "admin"))
	if err != nil || role.Name != "admin" {
		t.Errorf("OrNotFound(admin role) = %v, %v; want the built-in admin role", role, err)
	}
}

func TestOpen_RefusesOptionsNobodyChose(t *testing.T) {
	t.Parallel()
	_, err := Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), OpenOptions{})
	if err == nil || !strings.Contains(err.Error(), "reader pool minimum is 0") {
		t.Errorf("Open with no reader minimum = %v, want a refusal, not a value from code", err)
	}
	database := openMigrated(t)
	if err = database.requireSpaceToMigrate(t.Context(), t.TempDir(), 0); err == nil ||
		!strings.Contains(err.Error(), "database multiple is 0") {
		t.Errorf("a free-space database multiple of 0 = %v, want a refusal", err)
	}
	if err = database.requireSpaceToMigrate(t.Context(), t.TempDir(), 3); err != nil {
		t.Errorf("a free-space database multiple of 3 with room to spare = %v, want nil", err)
	}
}

// A connection without the page cache size or the log's size limit is refused (D303, DL16).
func TestVerifyPragmas_RefusesAConnectionWithoutItsCacheOrLogLimit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "plain.db")
	for _, pragma := range []string{"cache_size", "journal_size_limit"} {
		withoutIt := regexp.MustCompile(`&_pragma=`+pragma+`\([^)]*\)`).ReplaceAllString(connectionPragmas, "")
		pool, err := sql.Open("sqlite", path+"?"+withoutIt)
		if err != nil {
			t.Fatal(err)
		}
		err = verifyPragmas(t.Context(), pool, synchronousNormal)
		if closeErr := pool.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if err == nil || !strings.Contains(err.Error(), pragma) {
			t.Errorf("verifyPragmas(no %s) = %v, want a %s refusal", pragma, err, pragma)
		}
	}
}

// Every connection runs at NORMAL unless the options skip the disk's syncs, a test's choice; a
// connection at OFF is refused where NORMAL was asked for, so production's open, which sets
// nothing, can never run unsynced (test-weight.md §35).
func TestOpen_SyncsTheDiskUnlessToldToSkipIt(t *testing.T) {
	t.Parallel()
	for _, skips := range []bool{false, true} {
		options := OpenOptions{MinimumReaders: 1, SkipsDiskSyncs: skips}
		database, err := Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), options)
		if err != nil {
			t.Fatalf("Open(SkipsDiskSyncs %v): %v", skips, err)
		}
		want := map[bool]int{false: synchronousNormal, true: synchronousOff}[skips]
		pools := map[string]*sql.DB{
			"writer": database.writer, "readers": database.readers, "maintenance": database.maintenance,
		}
		for name, pool := range pools {
			var synchronous int
			if err := pool.QueryRowContext(t.Context(), "PRAGMA synchronous").Scan(&synchronous); err != nil {
				t.Fatal(err)
			}
			if synchronous != want {
				t.Errorf("SkipsDiskSyncs %v: the %s's synchronous is %d, want %d", skips, name, synchronous, want)
			}
		}
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	}
	unsynced, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "plain.db")+"?"+unsyncedPragmas)
	if err != nil {
		t.Fatal(err)
	}
	err = verifyPragmas(t.Context(), unsynced, synchronousNormal)
	if closeErr := unsynced.Close(); closeErr != nil {
		t.Error(closeErr)
	}
	if err == nil || !strings.Contains(err.Error(), "synchronous") {
		t.Errorf("verifyPragmas(synchronous OFF, want NORMAL) = %v, want a synchronous refusal", err)
	}
}
