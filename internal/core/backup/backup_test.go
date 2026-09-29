// SPDX-License-Identifier: GPL-3.0-or-later

package backup

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

const canary = "CANARY-backup-9b1d"

var testTime = time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

// server is a data directory with a migrated database and a secret.key, as `orpheum run` leaves it.
type server struct {
	directories *datadir.Directories
	database    *store.Database
	key         secrets.Key
}

func newServer(t *testing.T) server {
	t.Helper()
	root := t.TempDir()
	directories, err := datadir.Prepare(filepath.Join(root, "state"), filepath.Join(root, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = directories.WriteStateFile(secrets.KeyFileName, key.EncodeKeyFile()); err != nil {
		t.Fatal(err)
	}
	return server{directories: directories, database: openAt(t, directories), key: key}
}

func openAt(t *testing.T, directories *datadir.Directories) *store.Database {
	t.Helper()
	database, err := store.Open(t.Context(), directories.LibraryDatabasePath(), storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := database.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	err = database.Migrate(t.Context(), storetest.MigrateOptions(directories.BackupDirectory()))
	if err != nil {
		t.Fatal(err)
	}
	return database
}

// populate gives the server a user, a changed setting and a sealed secret.
func (server server) populate(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	storetest.InsertUser(t, server.database, "owner", "admin")
	now := clocktest.NewFake(testTime)
	seeded := settingstest.Seeded(t, server.database)
	err := seeded.Set(ctx, nil, map[string]json.RawMessage{"should_check_for_updates": json.RawMessage("false")}, nil,
		true)
	if err != nil {
		t.Fatal(err)
	}
	err = secrets.NewStore(server.database, server.key, now, rand.Reader, seeded).
		Set(ctx, secrets.TMDBAPIKey, secrets.NewValue(canary), nil)
	if err != nil {
		t.Fatal(err)
	}
}

// emptyDataDirectory is a state directory with nothing in it yet.
func emptyDataDirectory(t *testing.T) *datadir.Directories {
	t.Helper()
	root := t.TempDir()
	directories, err := datadir.Prepare(filepath.Join(root, "state"), filepath.Join(root, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return directories
}

func restore(t *testing.T, directories *datadir.Directories, source string, key []byte) Plan {
	t.Helper()
	plan, err := MakePlan(t.Context(), directories, source, key, testTime)
	if err != nil {
		t.Fatalf("MakePlan: %v", err)
	}
	if err = Restore(t.Context(), directories, plan, key, storetest.OpenOptions()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	return plan
}

func TestRestore_WithoutTheKeyMarksSecretsNeedingReentryAndLosesNothingElse(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	original.populate(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	if written.KeyPath != "" {
		t.Errorf("a backup without --include-key wrote a key: %s", written.KeyPath)
	}
	contents, err := os.ReadFile(written.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), canary) {
		t.Error("the backup holds the secret in plain text; it must carry ciphertext only")
	}

	target := emptyDataDirectory(t)
	restore(t, target, written.DatabasePath, nil)
	restored := openAt(t, target)
	if _, err = store.OrNotFound(restored.Read().GetLiveUserByName(t.Context(), "owner")); err != nil {
		t.Errorf("the restored database lost the owner: %v", err)
	}
	restoredSettings := settings.NewService(restored, clocktest.NewFake(testTime),
		slog.New(slog.NewTextHandler(t.Output(), nil)))
	entries, err := restoredSettings.List(t.Context(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Definition.Key == "should_check_for_updates" && string(entry.Value) != "false" {
			t.Errorf("the restored database lost a setting: %+v", entry)
		}
	}
	newKey, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	needing, err := secrets.NewStore(restored, newKey, clocktest.NewFake(testTime), rand.Reader, restoredSettings).
		NeedingReentry(t.Context())
	if err != nil || len(needing) != 1 || needing[0].Name != secrets.TMDBAPIKey {
		t.Errorf("after a restore without the key, needing re-entry = %+v, %v; want the TMDB key", needing, err)
	}
}

func TestRestore_WithTheIncludedKeyOpensSecrets(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	original.populate(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, true)
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(written.KeyPath)
	if err != nil {
		t.Fatalf("the included key: %v", err)
	}
	target := emptyDataDirectory(t)
	if plan := restore(t, target, written.DatabasePath, key); !plan.InstallsKey {
		t.Error("the plan does not say it installs the key")
	}
	installed, err := target.ReadStateFile(secrets.KeyFileName)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := secrets.ParseKeyFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	// Open authenticates the ciphertext against the key and the row, so opening is the proof.
	restoredDatabase := openAt(t, target)
	_, err = secrets.NewStore(restoredDatabase, parsed, clocktest.NewFake(testTime), rand.Reader,
		settings.NewService(restoredDatabase, clocktest.NewFake(testTime),
			slog.New(slog.NewTextHandler(t.Output(), nil)))).
		Open(t.Context(), secrets.TMDBAPIKey)
	if err != nil {
		t.Errorf("with the key restored, the secret does not open: %v", err)
	}
}

func TestRestore_KeepsTheDatabaseItReplaces(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	original.populate(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	target := emptyDataDirectory(t)
	existing := openAt(t, target)
	storetest.InsertUser(t, existing, "someone-else", "user")
	if err = existing.Close(); err != nil {
		t.Fatal(err)
	}

	plan := restore(t, target, written.DatabasePath, nil)
	if !plan.ReplacesExistingDatabase || !strings.Contains(plan.Describe(), plan.SnapshotPath) {
		t.Errorf("the plan did not say what it replaces and where it keeps it:\n%s", plan.Describe())
	}
	kept, err := store.Open(t.Context(), plan.SnapshotPath, storetest.OpenOptions())
	if err != nil {
		t.Fatalf("the kept copy: %v", err)
	}
	defer func() {
		if closeErr := kept.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	if _, err = store.OrNotFound(kept.Read().GetLiveUserByName(t.Context(), "someone-else")); err != nil {
		t.Errorf("the kept copy lost the replaced database's user: %v", err)
	}
}

func TestRestore_RefusesWhileTheServerRuns(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	target := emptyDataDirectory(t)
	running, err := target.LockState()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := MakePlan(t.Context(), target, written.DatabasePath, nil, testTime)
	if err != nil {
		t.Fatal(err)
	}
	if err = Restore(t.Context(), target, plan, nil, storetest.OpenOptions()); !errors.Is(err, datadir.ErrInUse) {
		t.Errorf("a restore while the server runs: %v, want ErrInUse", err)
	}
	if exists, existsErr := target.LibraryDatabaseExists(); existsErr != nil || exists {
		t.Errorf("the refused restore wrote library.db anyway (%v)", existsErr)
	}
	if err = running.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestMakePlan_RefusesAForeignOrNewerBackup(t *testing.T) {
	t.Parallel()
	target := emptyDataDirectory(t)
	foreign := filepath.Join(t.TempDir(), "foreign.sqlite")
	storetest.ExecRaw(t, foreign, "CREATE TABLE someone_elses (id INTEGER)")
	if _, err := MakePlan(t.Context(), target, foreign, nil, testTime); !errors.Is(err, store.ErrNotOurs) {
		t.Errorf("a foreign file: %v, want ErrNotOurs", err)
	}

	original := newServer(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	version, err := store.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = MakePlan(t.Context(), target, written.DatabasePath, nil, testTime); err != nil {
		t.Fatalf("a backup from this release: %v", err)
	}
	if _, err = MakePlan(t.Context(), target, written.DatabasePath, []byte("not a key"), testTime); err == nil {
		t.Error("MakePlan accepted a key file that is not one")
	}
	storetest.ExecRaw(t, written.DatabasePath,
		"PRAGMA user_version = "+strconv.Itoa(version+1),
		"UPDATE schema_meta SET compatible_version = "+strconv.Itoa(version+1)+" WHERE id = 1")
	_, err = MakePlan(t.Context(), target, written.DatabasePath, nil, testTime)
	if !errors.Is(err, ErrNewerBackup) {
		t.Errorf("a backup from a newer, incompatible release: %v, want ErrNewerBackup", err)
	}
}

func TestMakePlan_RefusesABackupOlderThanTheBaseline(t *testing.T) {
	t.Parallel()
	target := emptyDataDirectory(t)
	original := newServer(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := store.Steps()
	if err != nil {
		t.Fatal(err)
	}
	if !steps[0].IsBaseline {
		t.Fatal("the first step is not a baseline; this test needs one (D228)")
	}
	storetest.ExecRaw(t, written.DatabasePath, "PRAGMA user_version = "+strconv.Itoa(steps[0].Version-1))
	_, err = MakePlan(t.Context(), target, written.DatabasePath, nil, testTime)
	if !errors.Is(err, store.ErrBeforeBaseline) {
		t.Errorf("a backup below the baseline, which the server would refuse at start: %v, want ErrBeforeBaseline", err)
	}
}
