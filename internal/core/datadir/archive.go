// SPDX-License-Identifier: GPL-3.0-or-later

package datadir

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/errs"
)

// archiveFolder is the folder a backup archive keeps the persistent directory's files under, so an
// archive unpacked by hand makes a persistent directory (D242 §5).
const archiveFolder = "persistent/"

// ErrNotABackupArchive is an archive that is not a backup of a persistent directory: one that holds
// anything but plain files under persistent/, or no persistent.db.
var ErrNotABackupArchive = errors.New("not a backup of a persistent directory")

// BackupPath is the path of a file in the backups directory, for a caller that must hand a path to
// SQLite (a database snapshot written by VACUUM INTO). It refuses anything but a plain name.
func (directories *Directories) BackupPath(name string) (string, error) {
	if err := checkPlainFileName(name); err != nil {
		return "", err
	}
	return filepath.Join(directories.backupsDirectory, name), nil
}

// RemoveBackupFile removes a file from the backups directory; a file already gone is not an error.
func (directories *Directories) RemoveBackupFile(name string) error {
	return removeFileIn(directories.backupsDirectory, name)
}

// WriteBackupArchive writes the archive name into the backups directory, atomically, mode 0600: a
// gzipped tar of the persistent directory's files under persistent/ (D242 §5). The live database
// and its WAL are not read; the snapshot snapshotName, a VACUUM INTO copy already in the backups
// directory, goes in as persistent.db instead. The files leftOut names are left out. Only the
// persistent directory's own files are kept: it holds no folders.
func (directories *Directories) WriteBackupArchive(name, snapshotName string, leftOut []string) (err error) {
	for _, plain := range []string{name, snapshotName} {
		if nameErr := checkPlainFileName(plain); nameErr != nil {
			return nameErr
		}
	}
	persistentRoot, err := openRoot(directories.persistentDirectory)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, errs.WrapIf("close the persistent directory", persistentRoot.Close()))
	}()
	backupsRoot, err := openRoot(directories.backupsDirectory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the backups directory", backupsRoot.Close())) }()
	names, err := persistentFileNames(directories.persistentDirectory, leftOut)
	if err != nil {
		return err
	}
	return writeStreamAtomically(backupsRoot, name, func(output io.Writer) error {
		return writeArchive(output, func(add func(archived string, source *os.Root, sourceName string) error) error {
			if addErr := add(persistentDatabaseName, backupsRoot, snapshotName); addErr != nil {
				return addErr
			}
			for _, fileName := range names {
				if addErr := add(fileName, persistentRoot, fileName); addErr != nil {
					return addErr
				}
			}
			return nil
		})
	})
}

// persistentFileNames are the persistent directory's own regular files a backup keeps: all but the
// live database, its WAL and shared memory, the hidden files of an atomic write, and leftOut.
func persistentFileNames(directory string, leftOut []string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list the persistent directory: %w", err)
	}
	skipped := append([]string{
		persistentDatabaseName, persistentDatabaseName + "-wal", persistentDatabaseName + "-shm",
	}, leftOut...)
	var names []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") || slices.Contains(skipped, entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// writeArchive writes a gzipped tar to output: addAll calls add once for each file, which copies
// sourceName under source into the archive as persistent/archived, mode 0600.
func writeArchive(
	output io.Writer, addAll func(add func(archived string, source *os.Root, sourceName string) error) error,
) error {
	zipped := gzip.NewWriter(output)
	archive := tar.NewWriter(zipped)
	add := func(archived string, source *os.Root, sourceName string) (err error) {
		file, err := source.Open(sourceName)
		if err != nil {
			return fmt.Errorf("open %s to back it up: %w", sourceName, err)
		}
		defer func() { err = errors.Join(err, errs.WrapIf("close "+sourceName, file.Close())) }()
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("read %s: %w", sourceName, err)
		}
		header := &tar.Header{
			Typeflag: tar.TypeReg, Name: archiveFolder + archived, Mode: fileMode, Size: info.Size(),
			ModTime: info.ModTime(),
		}
		if err = archive.WriteHeader(header); err != nil {
			return fmt.Errorf("archive %s: %w", archived, err)
		}
		if _, err = io.Copy(archive, file); err != nil {
			return fmt.Errorf("archive %s: %w", archived, err)
		}
		return nil
	}
	if err := addAll(add); err != nil {
		return err
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("finish the archive: %w", err)
	}
	if err := zipped.Close(); err != nil {
		return fmt.Errorf("finish the archive: %w", err)
	}
	return nil
}

// writeStreamAtomically writes name under root as write produces it, through a temporary file,
// synced, then renamed, mode 0600, so a crash leaves no half-written file under the name.
func writeStreamAtomically(root *os.Root, name string, write func(io.Writer) error) (err error) {
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
	if err = write(handle); err != nil {
		return errors.Join(err, handle.Close())
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

// ListBackupArchive lists the files a backup archive holds, by their names in the persistent
// directory, refusing an archive that is not a backup of one (ErrNotABackupArchive).
func ListBackupArchive(archivePath string) ([]string, error) {
	var names []string
	err := readArchive(archivePath, func(name string, _ io.Reader) error {
		names = append(names, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !slices.Contains(names, persistentDatabaseName) {
		return nil, fmt.Errorf("%s holds no %s: %w", archivePath, persistentDatabaseName, ErrNotABackupArchive)
	}
	return names, nil
}

// ExtractBackupDatabase writes the persistent.db a backup archive holds into the run directory as
// target, for a restore to inspect before it changes anything, and returns its path.
func (directories *Directories) ExtractBackupDatabase(archivePath, target string) (string, error) {
	if err := checkPlainFileName(target); err != nil {
		return "", err
	}
	if err := removeFileIn(directories.runDirectory, target); err != nil {
		return "", err
	}
	err := directories.extractInto(archivePath, directories.runDirectory, func(name string) (string, bool) {
		return target, name == persistentDatabaseName
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(directories.runDirectory, target), nil
}

// RestorePersistentFromArchive writes every file a backup archive holds into the persistent
// directory, each atomically, and its persistent.db last, through restoredName and
// ReplacePersistentDatabase. A file the persistent directory has and the archive does not, as
// secret.key when the backup left it out, is kept.
func (directories *Directories) RestorePersistentFromArchive(archivePath, restoredName string) error {
	if err := checkPlainFileName(restoredName); err != nil {
		return err
	}
	if err := removeFileIn(directories.persistentDirectory, restoredName); err != nil {
		return err
	}
	err := directories.extractInto(archivePath, directories.persistentDirectory, func(name string) (string, bool) {
		if name == persistentDatabaseName {
			return restoredName, true
		}
		return name, true
	})
	if err != nil {
		return err
	}
	return directories.ReplacePersistentDatabase(restoredName)
}

// extractInto writes each file of the archive that keep keeps into directory, under the name keep
// gives it, atomically.
func (directories *Directories) extractInto(
	archivePath, directory string, keep func(name string) (target string, isKept bool),
) (err error) {
	root, err := openRoot(directory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close "+directory, root.Close())) }()
	return readArchive(archivePath, func(name string, contents io.Reader) error {
		target, isKept := keep(name)
		if !isKept {
			return nil
		}
		return writeStreamAtomically(root, target, func(output io.Writer) error {
			if _, copyErr := io.Copy(output, contents); copyErr != nil {
				return fmt.Errorf("extract %s: %w", name, copyErr)
			}
			return nil
		})
	})
}

// readArchive reads a gzipped tar, calling each with every file's name in the persistent directory
// and its contents. It refuses, as ErrNotABackupArchive, an entry that is not a regular file, or
// whose name is not a plain name under persistent/.
func readArchive(archivePath string, each func(name string, contents io.Reader) error) (err error) {
	file, err := os.Open(archivePath) //nolint:gosec // G304: the operator names the archive to restore
	if err != nil {
		return fmt.Errorf("open the backup %s: %w", archivePath, err)
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close "+archivePath, file.Close())) }()
	zipped, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%s: %w: %w", archivePath, ErrNotABackupArchive, err)
	}
	archive := tar.NewReader(zipped)
	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			return nil
		}
		if nextErr != nil {
			return fmt.Errorf("%s: %w: %w", archivePath, ErrNotABackupArchive, nextErr)
		}
		name, isUnderFolder := strings.CutPrefix(header.Name, archiveFolder)
		if header.Typeflag != tar.TypeReg || !isUnderFolder || checkPlainFileName(name) != nil ||
			path.Clean(header.Name) != header.Name {
			return fmt.Errorf("%s holds %q: %w", archivePath, header.Name, ErrNotABackupArchive)
		}
		if eachErr := each(name, archive); eachErr != nil {
			return eachErr
		}
	}
}
