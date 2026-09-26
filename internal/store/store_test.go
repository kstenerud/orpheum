// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// testOpenOptions are the store options these tests use; in production they come from the
// config file.
func testOpenOptions() OpenOptions {
	return OpenOptions{MinimumReaders: 4}
}

// testMigrateOptions are the runner options these tests use, snapshotting into directory.
func testMigrateOptions(directory string) MigrateOptions {
	return MigrateOptions{SnapshotDirectory: directory, SpaceFactor: 3}
}

// openTemporary opens a new database in the test's temporary directory.
func openTemporary(t *testing.T) *Database {
	t.Helper()
	database, err := Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), testOpenOptions())
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
	err = empty.Migrate(t.Context(), MigrateOptions{SpaceFactor: 3})
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
	_, err := Open(t.Context(), filepath.Join(t.TempDir(), "library.db?mode=memory"), testOpenOptions())
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
	err = verifyPragmas(t.Context(), plain)
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
	err = verifyPragmas(t.Context(), withoutTempStore)
	if err == nil || !strings.Contains(err.Error(), "temp_store") {
		t.Errorf("verifyPragmas(no temp_store) = %v, want a temp_store refusal", err)
	}

	database := openTemporary(t)
	for name, pool := range map[string]*sql.DB{"writer": database.writer, "readers": database.readers} {
		if err := verifyPragmas(t.Context(), pool); err != nil {
			t.Errorf("verifyPragmas(%s) = %v, want nil", name, err)
		}
	}
}

func TestRead_RefusesAWrite(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	_, err := database.readers.ExecContext(t.Context(), "INSERT INTO tag (name) VALUES ('x')")
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Errorf("write through the reader pool: error = %v, want SQLite's read-only refusal", err)
	}
	if _, err := database.writer.ExecContext(t.Context(), "INSERT INTO tag (name) VALUES ('x')"); err != nil {
		t.Errorf("write through the writer: %v", err)
	}
}

func TestWrite_RollsBackWhenTheChangeFails(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	ctx := t.Context()
	failure := errors.New("the change failed")
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		upsertErr := queries.UpsertServerSetting(ctx, sqlcgen.UpsertServerSettingParams{
			Key: "check_for_updates", Value: "false", SetAt: 1,
		})
		return errors.Join(upsertErr, failure)
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Write error = %v, want the change's error", err)
	}
	settings, err := database.Read().ListServerSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 0 {
		t.Errorf("after a failed change, settings = %v, want none", settings)
	}

	err = database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertServerSetting(ctx, sqlcgen.UpsertServerSettingParams{
			Key: "check_for_updates", Value: "false", SetAt: 1,
		})
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	settings, err = database.Read().ListServerSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 || settings[0].Key != "check_for_updates" {
		t.Errorf("after a committed change, settings = %v, want check_for_updates", settings)
	}
}

func TestOne_TurnsNoRowsIntoErrNotFound(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	_, err := One(database.Read().GetLiveUserByName(t.Context(), "nobody"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("One(missing user) error = %v, want ErrNotFound", err)
	}
	role, err := One(database.Read().GetRoleByName(t.Context(), "admin"))
	if err != nil || role.Name != "admin" {
		t.Errorf("One(admin role) = %v, %v; want the built-in admin role", role, err)
	}
}

func TestOpen_RefusesOptionsNobodyChose(t *testing.T) {
	t.Parallel()
	_, err := Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), OpenOptions{})
	if err == nil || !strings.Contains(err.Error(), "reader pool minimum is 0") {
		t.Errorf("Open with no reader minimum = %v, want a refusal, not a value from code", err)
	}
	database := openMigrated(t)
	if err = database.requireSpaceToMigrate(t.Context(), t.TempDir(), 0); err == nil ||
		!strings.Contains(err.Error(), "space factor is 0") {
		t.Errorf("a space factor of 0 = %v, want a refusal", err)
	}
	if err = database.requireSpaceToMigrate(t.Context(), t.TempDir(), 3); err != nil {
		t.Errorf("a space factor of 3 with room to spare = %v, want nil", err)
	}
}
