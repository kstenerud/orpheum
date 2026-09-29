// SPDX-License-Identifier: GPL-3.0-or-later

package backup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/store"
)

// timestampLayout names backups so they sort by when they were taken.
const timestampLayout = "20060102T150405Z"

// restoringName is where a restore writes its copy before moving it into place.
const restoringName = "library.db.restoring"

// WrittenBackup is what a backup produced.
type WrittenBackup struct {
	DatabasePath string
	KeyPath      string // empty unless the key was included
}

// Write backs library.db up into the backup directory with VACUUM INTO. The file carries sealed
// ciphertext only; with includeKey, secret.key is written beside it, for moving to a new machine,
// and whoever holds both holds the secrets (D194 §4).
func Write(
	ctx context.Context, database *store.Database, directories *datadir.Directories, now time.Time, includeKey bool,
) (WrittenBackup, error) {
	stem := "backup-" + now.UTC().Format(timestampLayout)
	written := WrittenBackup{DatabasePath: filepath.Join(directories.BackupDirectory(), stem+".sqlite")}
	if err := database.BackupInto(ctx, written.DatabasePath); err != nil {
		return WrittenBackup{}, err
	}
	if !includeKey {
		return written, nil
	}
	keyFileContents, err := directories.ReadStateFile(secrets.KeyFileName)
	if err != nil {
		return written, fmt.Errorf("include the key: %w", err)
	}
	if err = directories.WriteBackupFile(stem+"."+secrets.KeyFileName, keyFileContents); err != nil {
		return written, fmt.Errorf("include the key: %w", err)
	}
	written.KeyPath = filepath.Join(directories.BackupDirectory(), stem+"."+secrets.KeyFileName)
	return written, nil
}

// ErrNewerBackup is returned for a backup a newer release made that this binary cannot use.
var ErrNewerBackup = errors.New("the backup was made by a newer release")

// Plan is what a restore would do, said before it does anything (backup-restore.md, Restore).
type Plan struct {
	SourcePath               string
	SourceState              store.SchemaState
	TargetPath               string
	ReplacesExistingDatabase bool   // a library.db is there and will be replaced
	SnapshotPath             string // where the replaced database is kept, if there is one
	InstallsKey              bool
	BinarySchemaVersion      int
}

// Describe says, in sentences, what the plan will do.
func (plan Plan) Describe() string {
	var text strings.Builder
	fmt.Fprintf(&text, "Restore %s (schema %d) into %s.\n", plan.SourcePath, plan.SourceState.Version, plan.TargetPath)
	if plan.ReplacesExistingDatabase {
		fmt.Fprintf(&text, "The database there now is replaced; a copy of it is kept first as %s.\n", plan.SnapshotPath)
	} else {
		text.WriteString("There is no database there now; nothing is replaced.\n")
	}
	if plan.SourceState.Version < plan.BinarySchemaVersion {
		fmt.Fprintf(&text, "The backup is older than this release and is migrated from schema %d to %d on the "+
			"next start.\n", plan.SourceState.Version, plan.BinarySchemaVersion)
	}
	if plan.InstallsKey {
		text.WriteString("The given secret.key is installed, so sealed secrets open as before.\n")
	} else {
		text.WriteString("No key is given: this machine's secret.key, or a new one, is used. Sealed secrets from a " +
			"backup made elsewhere show \"needs re-entering\"; nothing else is lost.\n")
	}
	return text.String()
}

// MakePlan checks a backup and says what restoring it would do, changing nothing. The key file's
// contents are those of a secret.key to install with it, or nil.
func MakePlan(
	ctx context.Context, directories *datadir.Directories, sourcePath string, keyFileContents []byte, now time.Time,
) (Plan, error) {
	inspected, err := store.InspectBackup(ctx, sourcePath)
	if err != nil {
		return Plan{}, err
	}
	binaryVersion, err := store.SchemaVersion()
	if err != nil {
		return Plan{}, err
	}
	if !inspected.State.UsableBy(binaryVersion) {
		return Plan{}, fmt.Errorf("%s is at schema %d and needs schema %d or newer; this release understands %d: %w",
			sourcePath, inspected.State.Version, inspected.State.CompatibleVersion, binaryVersion, ErrNewerBackup)
	}
	if err = store.RefuseBeforeBaseline(inspected.State); err != nil {
		return Plan{}, fmt.Errorf("%s cannot be restored by this release: %w", sourcePath, err)
	}
	if keyFileContents != nil {
		if _, err = secrets.ParseKeyFile(keyFileContents); err != nil {
			return Plan{}, err
		}
	}
	databaseExists, err := directories.LibraryDatabaseExists()
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		SourcePath: sourcePath, SourceState: inspected.State, TargetPath: directories.LibraryDatabasePath(),
		ReplacesExistingDatabase: databaseExists,
		InstallsKey:              keyFileContents != nil, BinarySchemaVersion: binaryVersion,
	}
	if databaseExists {
		plan.SnapshotPath = filepath.Join(directories.BackupDirectory(),
			"pre-restore-"+now.UTC().Format(timestampLayout)+".sqlite")
	}
	return plan, nil
}

// Restore carries out a plan, with the server stopped: it takes the state directory's lock, keeps
// the database it replaces, writes a fresh copy of the backup, moves it into place and installs
// the key if one was given. An older backup is migrated by the ordinary runner on the next start;
// one below the baseline, which the runner cannot migrate, MakePlan has already refused (D228).
func Restore(
	ctx context.Context, directories *datadir.Directories, plan Plan, keyFileContents []byte, options store.OpenOptions,
) (err error) {
	lock, err := directories.LockState()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Release()) }()
	if plan.ReplacesExistingDatabase {
		err = snapshotCurrentDatabase(ctx, directories.LibraryDatabasePath(), plan.SnapshotPath, options)
		if err != nil {
			return err
		}
	}
	err = directories.RemoveStateFile(restoringName)
	if err != nil {
		return err
	}
	restoringPath, err := directories.StatePath(restoringName)
	if err != nil {
		return err
	}
	err = store.CopyBackup(ctx, plan.SourcePath, restoringPath)
	if err != nil {
		return err
	}
	err = directories.ReplaceLibraryDatabase(restoringName)
	if err != nil {
		return err
	}
	if plan.InstallsKey {
		err = directories.WriteStateFile(secrets.KeyFileName, keyFileContents)
		if err != nil {
			return fmt.Errorf("install the key: %w", err)
		}
	}
	return nil
}

// snapshotCurrentDatabase copies the database about to be replaced into snapshot.
func snapshotCurrentDatabase(ctx context.Context, current, snapshot string, options store.OpenOptions) (err error) {
	database, err := store.Open(ctx, current, options)
	if err != nil {
		return fmt.Errorf("open the database being replaced, to keep a copy: %w", err)
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	if err = database.BackupInto(ctx, snapshot); err != nil {
		return fmt.Errorf("keep the database being replaced: %w", err)
	}
	return nil
}
