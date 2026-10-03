// SPDX-License-Identifier: GPL-3.0-or-later

package datadir

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/errs"
)

// The names of the files this package owns in the instance's directories, and the modes it makes
// them with.
const (
	persistentDatabaseName = "persistent.db"
	cacheDatabaseName      = "cache.db"
	directoryMode          = 0o700
	fileMode               = 0o600
)

// ErrNotFound is returned when a persistent or run file, or a stored artwork file, does not exist.
var ErrNotFound = errors.New("file not found")

// Paths are where a core instance's directories are, as its top-level config says (D242 §3).
type Paths struct {
	// Persistent holds what a backup keeps: persistent.db, secret.key and the full config.
	Persistent string
	// Cache holds what can be rebuilt: cache.db and the artwork.
	Cache string
	// Backups is where backups and pre-migration snapshots go, outside Persistent.
	Backups string
	// Run holds what only a running server needs: its lock and the setup code.
	Run string
}

// PathsIn are the directories an instance keeps inside its own directory, as `orpheum
// default-config` makes them: persistent, cache, backups and run (D242 §3).
func PathsIn(instanceDirectory string) Paths {
	return Paths{
		Persistent: filepath.Join(instanceDirectory, "persistent"), Cache: filepath.Join(instanceDirectory, "cache"),
		Backups: filepath.Join(instanceDirectory, "backups"), Run: filepath.Join(instanceDirectory, "run"),
	}
}

// Directories are a core instance's directories (D242 §3), each created on first use with mode
// 0700.
type Directories struct {
	persistentDirectory string
	cacheDirectory      string
	backupsDirectory    string
	runDirectory        string
}

// Prepare creates the instance's directories if they are missing and refuses a persistent
// directory on a network filesystem, where SQLite's locking cannot be trusted (D31).
func Prepare(paths Paths) (*Directories, error) {
	for _, directory := range []string{paths.Persistent, paths.Cache, paths.Backups, paths.Run} {
		if !filepath.IsAbs(directory) {
			return nil, fmt.Errorf("directory %q: want an absolute path", directory)
		}
		if err := os.MkdirAll(directory, directoryMode); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", directory, err)
		}
	}
	if err := refuseNetworkFilesystem(paths.Persistent); err != nil {
		return nil, err
	}
	return &Directories{
		persistentDirectory: paths.Persistent, cacheDirectory: paths.Cache, backupsDirectory: paths.Backups,
		runDirectory: paths.Run,
	}, nil
}

// PersistentDirectory is the directory a backup keeps.
func (directories *Directories) PersistentDirectory() string {
	return directories.persistentDirectory
}

// CacheDirectory is the directory of what can be rebuilt.
func (directories *Directories) CacheDirectory() string {
	return directories.cacheDirectory
}

// PersistentDatabasePath is the path of persistent.db.
func (directories *Directories) PersistentDatabasePath() string {
	return filepath.Join(directories.persistentDirectory, persistentDatabaseName)
}

// CacheDatabasePath is the path of cache.db.
func (directories *Directories) CacheDatabasePath() string {
	return filepath.Join(directories.cacheDirectory, cacheDatabaseName)
}

// DiscardCacheDatabase removes cache.db and its WAL and shared-memory files, which is how a
// cache from another schema version is replaced (migrations.md, versioned by discard). Files
// already gone are not an error.
func (directories *Directories) DiscardCacheDatabase() (err error) {
	root, err := os.OpenRoot(directories.cacheDirectory)
	if err != nil {
		return fmt.Errorf("open the cache directory: %w", err)
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the cache directory", root.Close())) }()
	for _, name := range []string{cacheDatabaseName + "-wal", cacheDatabaseName + "-shm", cacheDatabaseName} {
		if removeErr := root.Remove(name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", filepath.Join(directories.cacheDirectory, name), removeErr)
		}
	}
	return nil
}

// BackupDirectory is where backups and pre-migration snapshots are written.
func (directories *Directories) BackupDirectory() string {
	return directories.backupsDirectory
}

// PersistentDatabaseExists reports whether persistent.db is there.
func (directories *Directories) PersistentDatabaseExists() (bool, error) {
	_, err := os.Stat(directories.PersistentDatabasePath())
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("look for %s: %w", directories.PersistentDatabasePath(), err)
}

// ReadPersistentFile reads a file this package keeps directly in the persistent directory.
func (directories *Directories) ReadPersistentFile(name string) ([]byte, error) {
	return readFileIn(directories.persistentDirectory, name)
}

// WritePersistentFile replaces a persistent file atomically, mode 0600: it writes a temporary file
// beside it, syncs it, and renames it into place, so a crash leaves the old file or the new one.
func (directories *Directories) WritePersistentFile(name string, contents []byte) error {
	return writeFileIn(directories.persistentDirectory, name, contents)
}

// RemovePersistentFile removes a persistent file; a file already gone is not an error.
func (directories *Directories) RemovePersistentFile(name string) error {
	return removeFileIn(directories.persistentDirectory, name)
}

// ReadRunFile reads a file this package keeps directly in the run directory: the setup code.
func (directories *Directories) ReadRunFile(name string) ([]byte, error) {
	return readFileIn(directories.runDirectory, name)
}

// WriteRunFile replaces a run file atomically, mode 0600, as WritePersistentFile does.
func (directories *Directories) WriteRunFile(name string, contents []byte) error {
	return writeFileIn(directories.runDirectory, name, contents)
}

// RemoveRunFile removes a run file; a file already gone is not an error.
func (directories *Directories) RemoveRunFile(name string) error {
	return removeFileIn(directories.runDirectory, name)
}

// readFileIn reads the plain name in directory.
func readFileIn(directory, name string) (contents []byte, err error) {
	if nameErr := checkPlainFileName(name); nameErr != nil {
		return nil, nameErr
	}
	root, err := openRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close "+directory, root.Close())) }()
	contents, err = root.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", filepath.Join(directory, name), ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Join(directory, name), err)
	}
	return contents, nil
}

// writeFileIn replaces the plain name in directory atomically, mode 0600.
func writeFileIn(directory, name string, contents []byte) (err error) {
	if nameErr := checkPlainFileName(name); nameErr != nil {
		return nameErr
	}
	root, err := openRoot(directory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close "+directory, root.Close())) }()
	return writeAtomically(root, name, contents)
}

// removeFileIn removes the plain name in directory; a file already gone is not an error.
func removeFileIn(directory, name string) (err error) {
	if nameErr := checkPlainFileName(name); nameErr != nil {
		return nameErr
	}
	root, err := openRoot(directory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close "+directory, root.Close())) }()
	err = root.Remove(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", filepath.Join(directory, name), err)
	}
	return nil
}

// WriteBackupFile writes a file into the backup directory atomically, mode 0600: the key a
// `backup --include-key` puts beside its database (D194 §4).
func (directories *Directories) WriteBackupFile(name string, contents []byte) (err error) {
	if nameErr := checkPlainFileName(name); nameErr != nil {
		return nameErr
	}
	root, err := os.OpenRoot(directories.BackupDirectory())
	if err != nil {
		return fmt.Errorf("open the backup directory: %w", err)
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the backup directory", root.Close())) }()
	return writeAtomically(root, name, contents)
}

// BackupFile is one file in the backup directory.
type BackupFile struct {
	Name       string
	ModifiedAt time.Time
}

// ListBackups lists the backup directory, newest first.
func (directories *Directories) ListBackups() ([]BackupFile, error) {
	entries, err := os.ReadDir(directories.BackupDirectory())
	if err != nil {
		return nil, fmt.Errorf("list the backup directory: %w", err)
	}
	files := []BackupFile{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), infoErr)
		}
		files = append(files, BackupFile{Name: entry.Name(), ModifiedAt: info.ModTime()})
	}
	slices.SortFunc(files, func(first, second BackupFile) int { return second.ModifiedAt.Compare(first.ModifiedAt) })
	return files, nil
}

// ReplacePersistentDatabase moves a restored copy, already written into the persistent directory under
// the name restoredName, into place as persistent.db. The old database's WAL and shared-memory files
// are removed first: a stale WAL beside a different database is replayed into it. The caller
// has already snapshotted the old database.
func (directories *Directories) ReplacePersistentDatabase(restoredName string) (err error) {
	if nameErr := checkPlainFileName(restoredName); nameErr != nil {
		return nameErr
	}
	root, err := openRoot(directories.persistentDirectory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the persistent directory", root.Close())) }()
	for _, walFile := range []string{persistentDatabaseName + "-wal", persistentDatabaseName + "-shm"} {
		if err = ignoreMissing(root.Remove(walFile)); err != nil {
			return fmt.Errorf("remove %s: %w", walFile, err)
		}
	}
	if err = root.Rename(restoredName, persistentDatabaseName); err != nil {
		return fmt.Errorf("move %s into place: %w", restoredName, err)
	}
	return syncRoot(root)
}

// PersistentPath is the path of a persistent file, for a caller that must hand a path to SQLite (a
// restored copy written by VACUUM INTO). It refuses anything but a plain name.
func (directories *Directories) PersistentPath(name string) (string, error) {
	if err := checkPlainFileName(name); err != nil {
		return "", err
	}
	return filepath.Join(directories.persistentDirectory, name), nil
}

// checkPlainFileName refuses a name that is not a plain file name. An os.Root already keeps every
// operation inside its directory; this also keeps out hidden and temporary names.
func checkPlainFileName(name string) error {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("file %q: want a plain file name", name)
	}
	return nil
}

// openRoot opens directory as an os.Root.
func openRoot(directory string) (*os.Root, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", directory, err)
	}
	return root, nil
}

// writeAtomically writes name under root through a temporary file, synced, then renamed.
func writeAtomically(root *os.Root, name string, contents []byte) (err error) {
	temporary := "." + name + "." + rand.Text()
	handle, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return fmt.Errorf("create a temporary file for %s: %w", name, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, ignoreMissing(root.Remove(temporary)))
		}
	}()
	if _, err = handle.Write(contents); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", temporary, err), handle.Close())
	}
	if err = handle.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync %s: %w", temporary, err), handle.Close())
	}
	if err = handle.Close(); err != nil {
		return fmt.Errorf("close %s: %w", temporary, err)
	}
	if err = root.Rename(temporary, name); err != nil {
		return fmt.Errorf("move %s into place: %w", name, err)
	}
	return syncRoot(root)
}

// syncRoot makes a rename in root's directory durable.
func syncRoot(root *os.Root) error {
	handle, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open %s to sync it: %w", root.Name(), err)
	}
	return errors.Join(
		errs.WrapIf("sync "+root.Name(), handle.Sync()), errs.WrapIf("close "+root.Name(), handle.Close()),
	)
}

func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
