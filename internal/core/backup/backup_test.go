// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

const canary = "CANARY-backup-9b1d"

var testTime = time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

// server is an instance with a migrated database and a secret.key, as a first `orpheum core` leaves
// it.
type server struct {
	directories *instancedir.Directories
	database    *store.Database
	key         secrets.Key
}

func newServer(t *testing.T) server {
	t.Helper()
	root := t.TempDir()
	directories, err := instancedir.Open(instancedir.PathsIn(root))
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = directories.WritePersistentFile(secrets.KeyFileName, key.EncodeKeyFile()); err != nil {
		t.Fatal(err)
	}
	return server{directories: directories, database: openAt(t, directories), key: key}
}

func openAt(t *testing.T, directories *instancedir.Directories) *store.Database {
	t.Helper()
	database, err := store.Open(t.Context(), directories.PersistentDatabasePath(), storetest.OpenOptions())
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
	err := seeded.Set(ctx, map[string]json.RawMessage{"should_check_for_updates": json.RawMessage("false")}, nil,
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

// emptyInstance is an instance with nothing in it yet.
func emptyInstance(t *testing.T) *instancedir.Directories {
	t.Helper()
	root := t.TempDir()
	directories, err := instancedir.Open(instancedir.PathsIn(root))
	if err != nil {
		t.Fatal(err)
	}
	return directories
}

func restore(t *testing.T, directories *instancedir.Directories, source string) Plan {
	t.Helper()
	plan, err := MakePlan(t.Context(), directories, source, testTime)
	if err != nil {
		t.Fatalf("MakePlan: %v", err)
	}
	if err = Restore(t.Context(), directories, plan, testTime, storetest.OpenOptions()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	return plan
}

// archiveOf is a backup archive whose database is the SQLite file at databasePath, as Write makes
// one: for a test that needs a backup holding a database no server would write.
func archiveOf(t *testing.T, databasePath string) string {
	t.Helper()
	directories := emptyInstance(t)
	contents, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := directories.BackupPath("snapshot.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(snapshot, contents, 0o600); err != nil { //nolint:gosec // G703: under t.TempDir()
		t.Fatal(err)
	}
	if err = directories.WriteBackupArchive("backup.tar.gz", "snapshot.sqlite", nil); err != nil {
		t.Fatal(err)
	}
	path, err := directories.BackupPath("backup.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// databaseOf is the database a backup archive holds, written out where a test can change it.
func databaseOf(t *testing.T, archivePath string) string {
	t.Helper()
	path, err := emptyInstance(t).ExtractBackupDatabase(archivePath, "persistent.db")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRestore_WithoutTheKeyMarksSecretsNeedingReentryAndLosesNothingElse(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	original.populate(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	checkHoldsNoKeyAndCiphertextOnly(t, written)

	target := emptyInstance(t)
	restore(t, target, written.Path)
	restored := openAt(t, target)
	if _, err = store.OrNotFound(restored.Read().GetLiveUserByName(t.Context(), "owner")); err != nil {
		t.Errorf("the restored database lost the owner: %v", err)
	}
	restoredSettings := settings.NewService(restored, clocktest.NewFake(testTime),
		slog.New(slog.NewTextHandler(t.Output(), nil)))
	entries, err := restoredSettings.List(t.Context(), false)
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
	if err != nil || !written.IncludesKey {
		t.Fatalf("a backup with the key: %+v, %v", written, err)
	}
	target := emptyInstance(t)
	if plan := restore(t, target, written.Path); !plan.InstallsKey {
		t.Error("the plan does not say it installs the key")
	}
	installed, err := target.ReadPersistentFile(secrets.KeyFileName)
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
	target := emptyInstance(t)
	existing := openAt(t, target)
	storetest.InsertUser(t, existing, "someone-else", "user")
	if err = existing.Close(); err != nil {
		t.Fatal(err)
	}

	plan := restore(t, target, written.Path)
	if !plan.ReplacesExistingDatabase || !strings.Contains(plan.Describe(), plan.SnapshotPath) {
		t.Errorf("the plan did not say what it replaces and where it keeps it:\n%s", plan.Describe())
	}
	kept, err := store.Open(t.Context(), databaseOf(t, plan.SnapshotPath), storetest.OpenOptions())
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
	target := emptyInstance(t)
	running, err := target.LockInstance()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := MakePlan(t.Context(), target, written.Path, testTime)
	if err != nil {
		t.Fatal(err)
	}
	err = Restore(t.Context(), target, plan, testTime, storetest.OpenOptions())
	if !errors.Is(err, instancedir.ErrInUse) {
		t.Errorf("a restore while the server runs: %v, want ErrInUse", err)
	}
	if exists, existsErr := target.PersistentDatabaseExists(); existsErr != nil || exists {
		t.Errorf("the refused restore wrote persistent.db anyway (%v)", existsErr)
	}
	if err = running.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestMakePlan_RefusesAForeignOrNewerBackup(t *testing.T) {
	t.Parallel()
	target := emptyInstance(t)
	foreign := filepath.Join(t.TempDir(), "foreign.sqlite")
	storetest.ExecRaw(t, foreign, "CREATE TABLE someone_elses (id INTEGER)")
	if _, err := MakePlan(t.Context(), target, archiveOf(t, foreign), testTime); !errors.Is(err, store.ErrNotOurs) {
		t.Errorf("a foreign database: %v, want ErrNotOurs", err)
	}
	if _, err := MakePlan(t.Context(), target, foreign, testTime); !errors.Is(err, store.ErrNotOurs) {
		t.Errorf("a foreign database snapshot: %v, want ErrNotOurs", err)
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
	if _, err = MakePlan(t.Context(), target, written.Path, testTime); err != nil {
		t.Fatalf("a backup from this release: %v", err)
	}
	newer := databaseOf(t, written.Path)
	storetest.ExecRaw(t, newer,
		"PRAGMA user_version = "+strconv.Itoa(version+1),
		"UPDATE schema_meta SET compatible_version = "+strconv.Itoa(version+1)+" WHERE id = 1")
	_, err = MakePlan(t.Context(), target, archiveOf(t, newer), testTime)
	if !errors.Is(err, ErrNewerBackup) {
		t.Errorf("a backup from a newer, incompatible release: %v, want ErrNewerBackup", err)
	}
}

func TestMakePlan_RefusesABackupOlderThanTheBaseline(t *testing.T) {
	t.Parallel()
	target := emptyInstance(t)
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
	older := databaseOf(t, written.Path)
	storetest.ExecRaw(t, older, "PRAGMA user_version = "+strconv.Itoa(steps[0].Version-1))
	_, err = MakePlan(t.Context(), target, archiveOf(t, older), testTime)
	if !errors.Is(err, store.ErrBeforeBaseline) {
		t.Errorf("a backup below the baseline, which the server would refuse at start: %v, want ErrBeforeBaseline", err)
	}
}

// MakePlan reads the backup's database from a copy in the run directory and removes it: a plan
// changes nothing the instance keeps.
func TestMakePlan_LeavesNothingBehind(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	written, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	target := emptyInstance(t)
	if _, err = MakePlan(t.Context(), target, written.Path, testTime); err != nil {
		t.Fatal(err)
	}
	if _, err = target.ReadRunFile(inspectingName); !errors.Is(err, instancedir.ErrNotFound) {
		t.Errorf("the plan left its copy of the database in the run directory: %v", err)
	}
	if exists, existsErr := target.PersistentDatabaseExists(); existsErr != nil || exists {
		t.Errorf("the plan wrote persistent.db (%v)", existsErr)
	}
}

// A backup with the key is refused on an instance that has none yet, rather than made without it.
// A backup made without asking for the keys holds none of the server's private keys, the identity key
// and the certificates' and ACME account's among them, though the certificates themselves are kept;
// one asked for them holds them all (D289 §2: every backup, a scheduled one too, held them in plain
// text, the second audit's T1, 2026-10-13).
func TestWrite_LeavesEveryPrivateKeyOutUnlessAskedForTheKeys(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	keys := append([]string{identity.KeyFileName}, certificate.PrivateFileNames()...)
	for _, name := range append(keys, "tls-certificate.pem", identity.InstallationFileName) {
		if err := original.directories.WritePersistentFile(name, []byte("PRIVATE "+name)); err != nil {
			t.Fatal(err)
		}
	}
	without, err := Write(t.Context(), original.database, original.directories, testTime, false)
	if err != nil {
		t.Fatal(err)
	}
	names, err := instancedir.ListBackupArchive(without.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range append(keys, secrets.KeyFileName) {
		if slices.Contains(names, key) {
			t.Errorf("a backup made without the keys holds %s: %v", key, names)
		}
	}
	if !slices.Contains(names, "tls-certificate.pem") {
		t.Errorf("a backup made without the keys lost the certificate itself: %v", names)
	}
	withoutNames := names
	with, err := Write(t.Context(), original.database, original.directories, testTime.Add(time.Hour), true)
	if err != nil {
		t.Fatal(err)
	}
	if names, err = instancedir.ListBackupArchive(with.Path); err != nil {
		t.Fatal(err)
	}
	for _, key := range append(keys, secrets.KeyFileName) {
		if !slices.Contains(names, key) {
			t.Errorf("a backup asked for the keys lacks %s: %v", key, names)
		}
	}
	for _, kept := range [][]string{names, withoutNames} {
		if slices.Contains(kept, identity.InstallationFileName) {
			t.Errorf("a backup holds %s, so a copy restored elsewhere is the same installation (D437 §5): %v",
				identity.InstallationFileName, kept)
		}
	}
}

func TestWrite_RefusesToIncludeAKeyThereIsNot(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	if err := original.directories.RemovePersistentFile(secrets.KeyFileName); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(t.Context(), original.database, original.directories, testTime, true); !errors.Is(err,
		ErrNoKeyToInclude) {
		t.Errorf("a backup with a key there is not: %v, want ErrNoKeyToInclude", err)
	}
}

// checkHoldsNoKeyAndCiphertextOnly fails unless a backup made without the key holds no key, and its
// database the secrets as ciphertext only.
func checkHoldsNoKeyAndCiphertextOnly(t *testing.T, written Written) {
	t.Helper()
	names, err := instancedir.ListBackupArchive(written.Path)
	if err != nil || written.IncludesKey || slices.Contains(names, secrets.KeyFileName) {
		t.Errorf("a backup without --include-key holds %v (%v); want no key", names, err)
	}
	contents, err := os.ReadFile(databaseOf(t, written.Path))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), canary) {
		t.Error("the backup holds the secret in plain text; it must carry ciphertext only")
	}
}

// The snapshot the migration runner keeps before it migrates is a database alone, and the way
// back from a newer release's database that the server's refusal names (D181): restore takes it,
// replacing the database and keeping the config and the key.
func TestRestore_TakesTheDatabaseSnapshotKeptBeforeAMigration(t *testing.T) {
	t.Parallel()
	original := newServer(t)
	original.populate(t)
	snapshot := filepath.Join(t.TempDir(), "pre-migrate-v1.sqlite")
	if err := original.database.BackupInto(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	target := newServer(t)
	if err := target.directories.WritePersistentFile("config.toml", []byte("listen = 1")); err != nil {
		t.Fatal(err)
	}
	if err := target.database.Close(); err != nil {
		t.Fatal(err)
	}
	plan := restore(t, target.directories, snapshot)
	if !plan.IsDatabaseOnly || plan.InstallsKey || !slices.Equal(plan.RestoredFiles, []string{"persistent.db"}) {
		t.Errorf("the plan for a snapshot = %+v; want the database alone", plan)
	}
	restored := openAt(t, target.directories)
	if _, err := store.OrNotFound(restored.Read().GetLiveUserByName(t.Context(), "owner")); err != nil {
		t.Errorf("the restored snapshot lost the owner: %v", err)
	}
	for name, want := range map[string]string{
		"config.toml": "listen = 1", secrets.KeyFileName: string(target.key.EncodeKeyFile()),
	} {
		if got, err := target.directories.ReadPersistentFile(name); err != nil || string(got) != want {
			t.Errorf("%s after restoring a snapshot = %q, %v; want it kept", name, got, err)
		}
	}
}

// A backup whose database copy fails its integrity check is refused and leaves nothing behind in
// the backup directory (D303 §7, M6).
func TestWrite_RefusesADamagedCopyAndLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	instance := newServer(t)
	storetest.ExecRaw(t, instance.database.Path(), "PRAGMA ignore_check_constraints = 1; "+
		"INSERT INTO user (name, role_id, created_at, admin_away) VALUES ('mallory', 1, 1, 'sometimes')")
	_, err := Write(t.Context(), instance.database, instance.directories, testTime, false)
	if !errors.Is(err, store.ErrDamagedCopy) {
		t.Errorf("a backup of a row its CHECK forbids: %v, want store.ErrDamagedCopy", err)
	}
	files, err := instance.directories.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name, "backup-") {
			t.Errorf("the refused backup left %s behind", file.Name)
		}
	}
}

// cutShortCopier copies the database whole, then fails as a stop that cancels the copy's check does:
// the snapshot is there, and the error is not a damaged copy's.
type cutShortCopier struct{ database *store.Database }

func (copier cutShortCopier) BackupInto(ctx context.Context, path string) error {
	if err := copier.database.BackupInto(ctx, path); err != nil {
		return err
	}
	return fmt.Errorf("check the copy %q: %w", path, context.Canceled)
}

// A backup whose copy is cut short, its check cancelled as the server stops, leaves no snapshot
// behind, and a backup in the same second then takes its name: the snapshot left made `orpheum
// backup` fail with "output file already exists" after a stop that cut the start's scheduled backup
// short (cmd/orpheum's canary, 2026-10-26).
func TestWrite_ACopyCutShortLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	instance := newServer(t)
	stem := "backup-" + testTime.Format(timestampLayout)
	_, err := write(t.Context(), cutShortCopier{instance.database}, instance.directories, stem, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a backup whose check was cancelled: %v, want context.Canceled", err)
	}
	files, err := instance.directories.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name, "backup-") {
			t.Errorf("the backup cut short left %s behind", file.Name)
		}
	}
	if _, err = Write(t.Context(), instance.database, instance.directories, testTime, false); err != nil {
		t.Errorf("a backup in the same second as one cut short: %v", err)
	}
}

// A backup whose snapshot's name another backup of the same second holds is refused, and leaves that
// one's snapshot alone: it is the other's, still being made or archived.
func TestWrite_LeavesTheSnapshotOfAnotherBackupInTheSameSecondAlone(t *testing.T) {
	t.Parallel()
	instance := newServer(t)
	snapshotName := "backup-" + testTime.Format(timestampLayout) + ".snapshot.sqlite"
	othersPath, err := instance.directories.BackupPath(snapshotName)
	if err != nil {
		t.Fatal(err)
	}
	if err = instance.database.BackupInto(t.Context(), othersPath); err != nil {
		t.Fatal(err)
	}
	_, err = Write(t.Context(), instance.database, instance.directories, testTime, false)
	if !errors.Is(err, store.ErrCopyExists) {
		t.Errorf("a backup whose snapshot's name is taken: %v, want store.ErrCopyExists", err)
	}
	files, err := instance.directories.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(files, func(file instancedir.BackupFile) bool { return file.Name == snapshotName }) {
		t.Errorf("the refused backup removed the other's snapshot %s: %+v", snapshotName, files)
	}
}
