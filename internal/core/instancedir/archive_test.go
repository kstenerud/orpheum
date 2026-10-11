// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeFiles writes each file, by name, into directory.
func writeFiles(t *testing.T, directory string, files map[string]string) {
	t.Helper()
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// A backup archive holds the persistent directory's files, the database as its snapshot rather
// than the live file and its WAL, and leaves out what it is told to; a restore writes them back,
// keeping a file the archive does not hold (D242 §5).
func TestBackupArchive_HoldsThePersistentFilesAndRestoresThem(t *testing.T) {
	t.Parallel()
	source := prepareTemporaryDirectories(t)
	writeFiles(t, source.persistentDirectory, map[string]string{
		"persistent.db": "live", "persistent.db-wal": "wal", "config.toml": "listen = 1", "secret.key": "key",
	})
	writeFiles(t, source.backupsDirectory, map[string]string{"snapshot.sqlite": "snapshot"})
	if err := source.WriteBackupArchive("backup.tar.gz", "snapshot.sqlite", []string{"secret.key"}); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(source.backupsDirectory, "backup.tar.gz")
	info, err := os.Stat(archivePath)
	if err != nil || info.Mode().Perm() != fileMode {
		t.Fatalf("the archive: %v, %v; want it written with mode 0600", info, err)
	}
	names, err := ListBackupArchive(archivePath)
	slices.Sort(names)
	if err != nil || !slices.Equal(names, []string{"config.toml", "persistent.db"}) {
		t.Errorf("the archive holds %v, %v; want the config and the database, not the WAL or the key", names, err)
	}
	extracted, err := source.ExtractBackupDatabase(archivePath, "restoring.db")
	if got, readErr := os.ReadFile(extracted); err != nil || readErr != nil || string(got) != "snapshot" {
		t.Errorf("the extracted database is %q, %v, %v; want the snapshot", got, err, readErr)
	}
	target := prepareTemporaryDirectories(t)
	writeFiles(t, target.persistentDirectory, map[string]string{
		"persistent.db": "old", "persistent.db-wal": "old wal", "secret.key": "this machine's key",
	})
	if err = target.RestorePersistentFromArchive(archivePath, "persistent.db.restoring"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"persistent.db": "snapshot", "config.toml": "listen = 1", "secret.key": "this machine's key",
	} {
		if got, readErr := os.ReadFile(filepath.Join(target.persistentDirectory, name)); readErr != nil ||
			string(got) != want {
			t.Errorf("restored %s = %q, %v; want %q", name, got, readErr, want)
		}
	}
	if _, err = os.Stat(filepath.Join(target.persistentDirectory, "persistent.db-wal")); !os.IsNotExist(err) {
		t.Errorf("the old WAL survived the restore: %v", err)
	}
}

// writeTar writes a gzipped tar holding one entry.
func writeTar(t *testing.T, path string, header *tar.Header, contents string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zipped := gzip.NewWriter(file)
	archive := tar.NewWriter(zipped)
	header.Size = int64(len(contents))
	if header.Typeflag != tar.TypeReg {
		header.Size = 0
		contents = ""
	}
	if err = archive.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err = archive.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(archive.Close(), zipped.Close(), file.Close()); err != nil {
		t.Fatal(err)
	}
}

// An archive is restored only when it holds plain files under persistent/ and a database: a name
// that climbs out, a folder, a link or a stray file is refused before anything is written.
func TestListBackupArchive_RefusesAnythingButPlainFilesUnderPersistent(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	for name, header := range map[string]*tar.Header{
		"climbing":    {Typeflag: tar.TypeReg, Name: "persistent/../secret.key", Mode: 0o600},
		"nested":      {Typeflag: tar.TypeReg, Name: "persistent/sub/persistent.db", Mode: 0o600},
		"outside":     {Typeflag: tar.TypeReg, Name: "persistent.db", Mode: 0o600},
		"link":        {Typeflag: tar.TypeSymlink, Name: "persistent/persistent.db", Linkname: "/etc/passwd"},
		"hidden":      {Typeflag: tar.TypeReg, Name: "persistent/.persistent.db", Mode: 0o600},
		"no database": {Typeflag: tar.TypeReg, Name: "persistent/config.toml", Mode: 0o600},
	} {
		path := filepath.Join(directory, name+".tar.gz")
		writeTar(t, path, header, "x")
		if _, err := ListBackupArchive(path); !errors.Is(err, ErrNotABackupArchive) {
			t.Errorf("%s: %v, want ErrNotABackupArchive", name, err)
		}
	}
	notAnArchive := filepath.Join(directory, "plain")
	writeFiles(t, directory, map[string]string{"plain": "not gzip"})
	if _, err := ListBackupArchive(notAnArchive); !errors.Is(err, ErrNotABackupArchive) {
		t.Errorf("a file that is no archive: %v, want ErrNotABackupArchive", err)
	}
}
