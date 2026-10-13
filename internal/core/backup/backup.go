// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// timestampLayout names backups so they sort by when they were taken.
const timestampLayout = "20060102T150405Z"

// archiveSuffix ends every backup's name: a gzipped tar of the persistent directory (D242 §5).
const archiveSuffix = ".tar.gz"

// persistentDatabaseName is the database's name in the persistent directory, which a database
// snapshot alone restores.
const persistentDatabaseName = "persistent.db"

// restoringName is where a restore writes its database before moving it into place.
const restoringName = "persistent.db.restoring"

// inspectingName is where a restore's plan puts the backup's database, in the run directory, to
// read its schema without changing anything the instance keeps.
const inspectingName = "restore-inspecting.db"

// ErrNoKeyToInclude is a backup asked to include secret.key on an instance that has none yet.
var ErrNoKeyToInclude = errors.New("there is no secret.key to include; the server writes one at its first start")

// Written is a backup made: where it is, and whether it holds the server's keys.
type Written struct {
	Path        string
	IncludesKey bool
}

// Write backs the persistent directory up into the backups directory: a tarball of its files, its
// database a VACUUM INTO snapshot, safe beside a live writer (D242 §5, D77). The server's keys are
// left out unless includeKey asks for them, for moving to another machine: secret.key, whoever holds
// which holds the secrets (D194 §4); and the identity key and the certificates' and ACME account's
// private keys, whoever holds which can pass for the server (D289 §2). A backup kept them all but
// secret.key in plain text, a scheduled one too (the second audit's T1, 2026-10-13).
func Write(
	ctx context.Context, database *store.Database, directories *instancedir.Directories, now time.Time, includeKey bool,
) (Written, error) {
	return write(ctx, database, directories, "backup-"+now.UTC().Format(timestampLayout), includeKey)
}

// write writes the backup stem.tar.gz, as Write says.
func write(
	ctx context.Context, database *store.Database, directories *instancedir.Directories, stem string, includeKey bool,
) (written Written, err error) {
	leftOut := serverKeys()
	if includeKey {
		if _, err = directories.ReadPersistentFile(secrets.KeyFileName); errors.Is(err, instancedir.ErrNotFound) {
			return Written{}, ErrNoKeyToInclude
		} else if err != nil {
			return Written{}, err
		}
		leftOut = nil
	}
	snapshotName := stem + ".snapshot.sqlite"
	snapshotPath, err := directories.BackupPath(snapshotName)
	if err != nil {
		return Written{}, err
	}
	if backupErr := database.BackupInto(ctx, snapshotPath); backupErr != nil {
		return Written{}, backupErr
	}
	defer func() { err = errors.Join(err, directories.RemoveBackupFile(snapshotName)) }()
	name := stem + archiveSuffix
	if archiveErr := directories.WriteBackupArchive(name, snapshotName, leftOut); archiveErr != nil {
		return Written{}, archiveErr
	}
	path, err := directories.BackupPath(name)
	return Written{Path: path, IncludesKey: includeKey}, err
}

// serverKeys are the files a backup leaves out unless asked for them: the key that seals the secrets,
// the core's identity key, and the certificates' and ACME account's private keys.
func serverKeys() []string {
	return append([]string{secrets.KeyFileName, identity.KeyFileName}, certificate.PrivateFileNames()...)
}

// ErrNewerBackup is returned for a backup a newer release made that this binary cannot use.
var ErrNewerBackup = errors.New("the backup was made by a newer release")

// Plan is what a restore would do, said before it does anything (backup-restore.md, Restore).
type Plan struct {
	SourcePath  string
	SourceState store.SchemaState
	// RestoredFiles are the files the backup holds, by their names in the persistent directory.
	RestoredFiles []string
	// IsDatabaseOnly is a source that is a database snapshot alone, as the migration runner keeps
	// before it migrates, not a backup: it replaces persistent.db and nothing else.
	IsDatabaseOnly           bool
	TargetDirectory          string
	ReplacesExistingDatabase bool   // a persistent.db is there and will be replaced
	SnapshotPath             string // where what is replaced is kept, if anything is
	InstallsKey              bool
	BinarySchemaVersion      int
}

// Describe says, in sentences, what the plan will do.
func (plan Plan) Describe() string {
	var text strings.Builder
	fmt.Fprintf(&text, "Restore %s (schema %d) into %s: %s.\n", plan.SourcePath, plan.SourceState.Version,
		plan.TargetDirectory, strings.Join(plan.RestoredFiles, ", "))
	if plan.ReplacesExistingDatabase {
		fmt.Fprintf(&text, "What is there now is replaced; a backup of it, key and all, is kept first as %s.\n",
			plan.SnapshotPath)
	} else {
		text.WriteString("There is no database there now; nothing is replaced.\n")
	}
	if plan.SourceState.Version < plan.BinarySchemaVersion {
		fmt.Fprintf(&text, "The backup is older than this release and is migrated from schema %d to %d on the "+
			"next start.\n", plan.SourceState.Version, plan.BinarySchemaVersion)
	}
	if plan.InstallsKey {
		text.WriteString("The backup holds secret.key, which is installed, so sealed secrets open as before.\n")
	} else {
		text.WriteString("The backup holds no secret.key: this machine's, or a new one, is used. Sealed secrets " +
			"from a backup made elsewhere show \"needs re-entering\"; nothing else is lost.\n")
	}
	return text.String()
}

// MakePlan checks a backup and says what restoring it would do, changing nothing the instance
// keeps: it reads the backup's database from a copy in the run directory, and removes it.
func MakePlan(
	ctx context.Context, directories *instancedir.Directories, sourcePath string, now time.Time,
) (Plan, error) {
	source, err := inspectSource(ctx, directories, sourcePath)
	if err != nil {
		return Plan{}, err
	}
	inspected := source.inspected
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
	databaseExists, err := directories.PersistentDatabaseExists()
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		SourcePath: sourcePath, SourceState: inspected.State, RestoredFiles: source.names,
		IsDatabaseOnly:  source.isDatabaseOnly,
		TargetDirectory: directories.PersistentDirectory(), ReplacesExistingDatabase: databaseExists,
		InstallsKey: slices.Contains(source.names, secrets.KeyFileName), BinarySchemaVersion: binaryVersion,
	}
	if databaseExists {
		plan.SnapshotPath, err = directories.BackupPath(preRestoreStem(now) + archiveSuffix)
	}
	return plan, err
}

// inspectedSource is what a restore's source holds: its files, by their names in the persistent
// directory, its database's schema, and whether it is a database snapshot alone.
type inspectedSource struct {
	names          []string
	inspected      store.BackupDescription
	isDatabaseOnly bool
}

// inspectSource reads what a restore's source holds: a backup, or a database snapshot alone, as the
// migration runner keeps before it migrates (pre-migrate-vN.sqlite). A backup's database is read
// from a copy in the run directory, removed after.
func inspectSource(
	ctx context.Context, directories *instancedir.Directories, sourcePath string,
) (source inspectedSource, err error) {
	names, archiveErr := instancedir.ListBackupArchive(sourcePath)
	if errors.Is(archiveErr, instancedir.ErrNotABackupArchive) {
		inspected, inspectErr := store.InspectBackup(ctx, sourcePath)
		if inspectErr != nil {
			return source, fmt.Errorf("%s is neither a backup nor a database snapshot: %w",
				sourcePath, errors.Join(archiveErr, inspectErr))
		}
		return inspectedSource{names: []string{persistentDatabaseName}, inspected: inspected, isDatabaseOnly: true},
			nil
	}
	if archiveErr != nil {
		return source, archiveErr
	}
	inspecting, err := directories.ExtractBackupDatabase(sourcePath, inspectingName)
	if err != nil {
		return source, err
	}
	defer func() { err = errors.Join(err, directories.RemoveRunFile(inspectingName)) }()
	source = inspectedSource{names: names}
	source.inspected, err = store.InspectBackup(ctx, inspecting)
	return source, err
}

// preRestoreStem names the backup a restore keeps of what it replaces.
func preRestoreStem(now time.Time) string {
	return "pre-restore-" + now.UTC().Format(timestampLayout)
}

// Restore carries out a plan, with the server stopped: it takes the instance's lock, keeps a
// backup of what it replaces, key and all, and writes the backup's files into the persistent
// directory. An older backup is migrated by the ordinary runner on the next start; one below the
// baseline, which the runner cannot migrate, MakePlan has already refused (D228).
func Restore(
	ctx context.Context, directories *instancedir.Directories, plan Plan, now time.Time, options store.OpenOptions,
) (err error) {
	lock, err := directories.LockInstance()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Release()) }()
	if plan.ReplacesExistingDatabase {
		if keepErr := keepWhatIsReplaced(ctx, directories, now, options); keepErr != nil {
			return keepErr
		}
	}
	if !plan.IsDatabaseOnly {
		return directories.RestorePersistentFromArchive(plan.SourcePath, restoringName)
	}
	return restoreDatabaseOnly(ctx, directories, plan.SourcePath)
}

// restoreDatabaseOnly replaces the database with a snapshot of one; the config and the key stay as
// they are.
func restoreDatabaseOnly(ctx context.Context, directories *instancedir.Directories, snapshotPath string) error {
	if err := directories.RemovePersistentFile(restoringName); err != nil {
		return err
	}
	restoringPath, err := directories.PersistentPath(restoringName)
	if err != nil {
		return err
	}
	if err := store.CopyBackup(ctx, snapshotPath, restoringPath); err != nil {
		return err
	}
	return directories.ReplacePersistentDatabase(restoringName)
}

// keepWhatIsReplaced backs the persistent directory up, key and all, before a restore replaces it.
func keepWhatIsReplaced(
	ctx context.Context, directories *instancedir.Directories, now time.Time, options store.OpenOptions,
) (err error) {
	database, err := store.Open(ctx, directories.PersistentDatabasePath(), options)
	if err != nil {
		return fmt.Errorf("open the database being replaced, to keep a copy: %w", err)
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	// The key goes in too, when there is one: a backup that holds a key replaces it, and the copy
	// kept is this machine's own.
	_, keyErr := directories.ReadPersistentFile(secrets.KeyFileName)
	if keyErr != nil && !errors.Is(keyErr, instancedir.ErrNotFound) {
		return keyErr
	}
	if _, err = write(ctx, database, directories, preRestoreStem(now), keyErr == nil); err != nil {
		return fmt.Errorf("keep what is being replaced: %w", err)
	}
	return nil
}

// Service makes backups of the running server's instance from the admin interface, one at a time
// (D242 §5), and lists them.
type Service struct {
	database    *store.Database
	directories *instancedir.Directories
	clock       clock.Clock

	// mutex makes one backup at a time: two in the same second would share a name.
	mutex sync.Mutex

	// queue and settings are where backups are asked for as jobs and their schedule read
	// (UseQueue); nil until the server's queue is made.
	queue    Queue
	settings Settings
}

// NewService returns the backup service of the server whose database and directories these are.
func NewService(database *store.Database, directories *instancedir.Directories, clockSource clock.Clock) *Service {
	return &Service{database: database, directories: directories, clock: clockSource}
}

// Make writes a backup now, as Write says.
func (service *Service) Make(ctx context.Context, includeKey bool) (Written, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return Write(ctx, service.database, service.directories, service.clock.Now(), includeKey)
}

// Directory is where the backups are.
func (service *Service) Directory() string {
	return service.directories.BackupDirectory()
}

// List lists the backups directory, newest first: the backups, and the copies kept before a
// restore or a migration.
func (service *Service) List() ([]instancedir.BackupFile, error) {
	return service.directories.ListBackups()
}
