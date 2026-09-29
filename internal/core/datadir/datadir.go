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

// The names of the files and directories this package owns under the state and cache
// directories, and the modes it makes them with.
const (
	libraryDatabaseName = "library.db"
	cacheDatabaseName   = "cache.db"
	backupDirectoryName = "backups"
	directoryMode       = 0o700
	fileMode            = 0o600
)

// ErrNotFound is returned when a state file or a stored artwork file does not exist.
var ErrNotFound = errors.New("state file not found")

// Directories are the server's state directory (library.db, secret.key, backups) and cache
// directory. Both are created on first use with mode 0700.
type Directories struct {
	stateDirectory string
	cacheDirectory string
}

// Prepare creates the state and cache directories if they are missing and refuses a state
// directory on a network filesystem, where SQLite's locking cannot be trusted (D31).
func Prepare(stateDirectory, cacheDirectory string) (*Directories, error) {
	for _, directory := range []string{stateDirectory, cacheDirectory} {
		if !filepath.IsAbs(directory) {
			return nil, fmt.Errorf("directory %q: want an absolute path", directory)
		}
		if err := os.MkdirAll(directory, directoryMode); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", directory, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(stateDirectory, backupDirectoryName), directoryMode); err != nil {
		return nil, fmt.Errorf("create the backup directory: %w", err)
	}
	if err := refuseNetworkFilesystem(stateDirectory); err != nil {
		return nil, err
	}
	return &Directories{stateDirectory: stateDirectory, cacheDirectory: cacheDirectory}, nil
}

// LibraryDatabasePath is the path of library.db.
func (directories *Directories) LibraryDatabasePath() string {
	return filepath.Join(directories.stateDirectory, libraryDatabaseName)
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
	return filepath.Join(directories.stateDirectory, backupDirectoryName)
}

// LibraryDatabaseExists reports whether library.db is there.
func (directories *Directories) LibraryDatabaseExists() (bool, error) {
	_, err := os.Stat(directories.LibraryDatabasePath())
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("look for %s: %w", directories.LibraryDatabasePath(), err)
}

// ReadStateFile reads a file this package keeps directly in the state directory.
func (directories *Directories) ReadStateFile(name string) (contents []byte, err error) {
	if nameErr := checkPlainFileName(name); nameErr != nil {
		return nil, nameErr
	}
	root, err := directories.openStateRoot()
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the state directory", root.Close())) }()
	contents, err = root.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", filepath.Join(directories.stateDirectory, name), ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Join(directories.stateDirectory, name), err)
	}
	return contents, nil
}

// WriteStateFile replaces a state file atomically, mode 0600: it writes a temporary file beside
// it, syncs it, and renames it into place, so a crash leaves the old file or the new one.
func (directories *Directories) WriteStateFile(name string, contents []byte) (err error) {
	if nameErr := checkPlainFileName(name); nameErr != nil {
		return nameErr
	}
	root, err := directories.openStateRoot()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the state directory", root.Close())) }()
	return writeAtomically(root, name, contents)
}

// RemoveStateFile removes a state file; a file already gone is not an error.
func (directories *Directories) RemoveStateFile(name string) (err error) {
	if nameErr := checkPlainFileName(name); nameErr != nil {
		return nameErr
	}
	root, err := directories.openStateRoot()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the state directory", root.Close())) }()
	err = root.Remove(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", filepath.Join(directories.stateDirectory, name), err)
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

// ReplaceLibraryDatabase moves a restored copy, already written into the state directory under
// the name restoredName, into place as library.db. The old database's WAL and shared-memory files
// are removed first: a stale WAL beside a different database is replayed into it. The caller
// has already snapshotted the old database.
func (directories *Directories) ReplaceLibraryDatabase(restoredName string) (err error) {
	if nameErr := checkPlainFileName(restoredName); nameErr != nil {
		return nameErr
	}
	root, err := directories.openStateRoot()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the state directory", root.Close())) }()
	for _, walFile := range []string{libraryDatabaseName + "-wal", libraryDatabaseName + "-shm"} {
		if err = ignoreMissing(root.Remove(walFile)); err != nil {
			return fmt.Errorf("remove %s: %w", walFile, err)
		}
	}
	if err = root.Rename(restoredName, libraryDatabaseName); err != nil {
		return fmt.Errorf("move %s into place: %w", restoredName, err)
	}
	return syncRoot(root)
}

// StatePath is the path of a state file, for a caller that must hand a path to SQLite (a
// restored copy written by VACUUM INTO). It refuses anything but a plain name.
func (directories *Directories) StatePath(name string) (string, error) {
	if err := checkPlainFileName(name); err != nil {
		return "", err
	}
	return filepath.Join(directories.stateDirectory, name), nil
}

// checkPlainFileName refuses a name that is not a plain file name. An os.Root already keeps every
// operation inside the state directory; this also keeps out hidden and temporary names.
func checkPlainFileName(name string) error {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("file %q: want a plain file name", name)
	}
	return nil
}

func (directories *Directories) openStateRoot() (*os.Root, error) {
	root, err := os.OpenRoot(directories.stateDirectory)
	if err != nil {
		return nil, fmt.Errorf("open the state directory: %w", err)
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
