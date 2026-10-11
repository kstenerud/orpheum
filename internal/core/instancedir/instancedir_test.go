// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func prepareTemporaryDirectories(t *testing.T) *Directories {
	t.Helper()
	root := t.TempDir()
	directories, err := Open(PathsIn(root))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return directories
}

func TestOpen_CreatesPrivateDirectories(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	for _, directory := range []string{
		directories.persistentDirectory, directories.cacheDirectory, directories.backupsDirectory,
		directories.runDirectory,
	} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != directoryMode {
			t.Errorf("%s has mode %v, want %v", directory, info.Mode().Perm(), os.FileMode(directoryMode))
		}
	}
	elsewhere := t.TempDir()
	if _, err := Open(Paths{
		Persistent: "relative/persistent", Cache: elsewhere, Backups: elsewhere,
		Run: elsewhere,
	}); err == nil {
		t.Error("Open accepted a relative persistent directory")
	}
}

func TestWritePersistentFile_ReplacesAtomicallyWithMode0600(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	for _, contents := range []string{"first", "second"} {
		if err := directories.WritePersistentFile("secret.key", []byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := directories.ReadPersistentFile("secret.key")
	if err != nil || string(got) != "second" {
		t.Errorf("ReadPersistentFile = %q, %v; want the second write", got, err)
	}
	info, err := os.Stat(filepath.Join(directories.persistentDirectory, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fileMode {
		t.Errorf("secret.key has mode %v, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(directories.persistentDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "secret.key" {
			t.Errorf("the persistent directory holds %q; an atomic write leaves no temporary file", entry.Name())
		}
	}
}

func TestPersistentAndRunFiles_RefuseAnythingButAPlainName(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	for _, name := range []string{"", "../escape", "sub/file", ".hidden", "/etc/passwd"} {
		if err := directories.WritePersistentFile(name, []byte("x")); err == nil {
			t.Errorf("WritePersistentFile(%q) succeeded, want a refusal", name)
		}
		if _, err := directories.ReadPersistentFile(name); err == nil {
			t.Errorf("ReadPersistentFile(%q) succeeded, want a refusal", name)
		}
	}
	if _, err := directories.ReadPersistentFile("setup-code"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadPersistentFile(missing) error = %v, want ErrNotFound", err)
	}
	if err := directories.RemovePersistentFile("setup-code"); err != nil {
		t.Errorf("RemovePersistentFile(missing) = %v, want nil", err)
	}
	for _, name := range []string{"", "../escape", "sub/file", ".hidden"} {
		if err := directories.WriteRunFile(name, []byte("x")); err == nil {
			t.Errorf("WriteRunFile(%q) succeeded, want a refusal", name)
		}
	}
	if err := directories.WriteRunFile("setup-code", []byte("code")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directories.runDirectory, "setup-code")); err != nil {
		t.Errorf("a run file is not in the run directory: %v", err)
	}
}

func TestReplacePersistentDatabase_RemovesTheOldWAL(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	target := directories.PersistentDatabasePath()
	for _, path := range []string{target, target + "-wal", target + "-shm"} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	restoredPath := filepath.Join(directories.persistentDirectory, "persistent.db.restoring")
	if err := os.WriteFile(restoredPath, []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := directories.ReplacePersistentDatabase("persistent.db.restoring"); err != nil {
		t.Fatalf("ReplacePersistentDatabase: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "restored" {
		t.Errorf("persistent.db = %q, %v; want the restored copy", got, err)
	}
	for _, sidecar := range []string{target + "-wal", target + "-shm"} {
		if _, err := os.Stat(sidecar); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived; a stale WAL would be replayed into the restored database", sidecar)
		}
	}
	if err := directories.ReplacePersistentDatabase("../elsewhere.db"); err == nil {
		t.Error("ReplacePersistentDatabase accepted a copy outside the persistent directory")
	}
}

func TestLockInstance_RefusesASecondHolder(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	server, err := directories.LockInstance()
	if err != nil {
		t.Fatalf("the first lock: %v", err)
	}
	if _, err = directories.LockInstance(); !errors.Is(err, ErrInUse) {
		t.Errorf("a second lock while the first is held: error = %v, want ErrInUse", err)
	}
	if err = server.Release(); err != nil {
		t.Fatal(err)
	}
	restore, err := directories.LockInstance()
	if err != nil {
		t.Fatalf("a lock after release: %v", err)
	}
	if err = restore.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestListBackups_ListsNewestFirst(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	older := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	for index, name := range []string{"backup-old.sqlite", "backup-new.sqlite"} {
		path := filepath.Join(directories.BackupDirectory(), name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		moment := older.Add(time.Duration(index) * 24 * time.Hour)
		if err := os.Chtimes(path, moment, moment); err != nil {
			t.Fatal(err)
		}
	}
	files, err := directories.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != "backup-new.sqlite" {
		t.Errorf("ListBackups = %+v, want the newer first", files)
	}
}

func TestDiscardCacheDatabase_RemovesTheFileAndItsJournalsOnly(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	names := []string{"cache.db", "cache.db-wal", "cache.db-shm", "art"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(directories.cacheDirectory, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := directories.DiscardCacheDatabase(); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		_, err := os.Stat(filepath.Join(directories.cacheDirectory, name))
		if kept := err == nil; kept != (name == "art") {
			t.Errorf("%s kept = %v", name, kept)
		}
	}
	if err := directories.DiscardCacheDatabase(); err != nil {
		t.Errorf("discarding again, with nothing there: %v", err)
	}
}

func TestArt_StoresReadsAndRemovesOnlyItsOwnNames(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	sourceSha256Hex := strings.Repeat("ab", 32)
	if err := directories.WriteArt(sourceSha256Hex, "portrait-m.webp", []byte("RIFF")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directories.cacheDirectory, "art", sourceSha256Hex, "portrait-m.webp"))
	if err != nil || info.Mode().Perm() != fileMode {
		t.Fatalf("stored file: %v, %v; want mode 0600", info, err)
	}
	if got, err := directories.ReadArt(sourceSha256Hex, "portrait-m.webp"); err != nil || string(got) != "RIFF" {
		t.Errorf("ReadArt = %q, %v", got, err)
	}
	if _, err := directories.ReadArt(sourceSha256Hex, "portrait-s.webp"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a size never written: %v, want ErrNotFound", err)
	}

	for _, name := range []string{"../../state/persistent.db", "portrait-m.webp/../x", ".hidden", "poster.png"} {
		if err := directories.WriteArt(sourceSha256Hex, name, nil); !errors.Is(err, ErrBadArtName) {
			t.Errorf("WriteArt(%q) = %v, want ErrBadArtName", name, err)
		}
	}
	if err := directories.WriteArt("../"+sourceSha256Hex[3:], "portrait-m.webp", nil); !errors.Is(err, ErrBadArtName) {
		t.Errorf("a hash that is a path: %v, want ErrBadArtName", err)
	}
	if err := directories.RemoveArt(sourceSha256Hex, "portrait-m.webp"); err != nil {
		t.Fatal(err)
	}
	if _, err := directories.ReadArt(sourceSha256Hex, "portrait-m.webp"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after RemoveArt: %v", err)
	}
	if err := directories.RemoveArt(sourceSha256Hex, "portrait-m.webp"); err != nil {
		t.Errorf("removing again: %v", err)
	}
}

func TestHasArt_SaysWhetherASizeIsStoredWithoutReadingIt(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	sourceSha256Hex := strings.Repeat("ab", 32)
	if err := directories.WriteArt(sourceSha256Hex, "portrait-m.webp", []byte("RIFF")); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		sourceSha256Hex, name string
		want                  bool
	}{
		{sourceSha256Hex, "portrait-m.webp", true},
		{sourceSha256Hex, "portrait-l.none", false},
		{strings.Repeat("cd", 32), "portrait-m.webp", false},
	} {
		if found, err := directories.HasArt(check.sourceSha256Hex, check.name); found != check.want || err != nil {
			t.Errorf("HasArt(%s, %s) = %v, %v; want %v", check.sourceSha256Hex[:4], check.name, found, err, check.want)
		}
	}
	if _, err := directories.HasArt(sourceSha256Hex, "../x"); !errors.Is(err, ErrBadArtName) {
		t.Errorf("HasArt of a path: %v, want ErrBadArtName", err)
	}
}

func TestListArt_ListsEveryStoredFileWithItsSizeAndNothingElse(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	if files, err := directories.ListArt(); err != nil || len(files) != 0 {
		t.Fatalf("before any art: %v, %v", files, err)
	}
	first, second := strings.Repeat("ab", 32), strings.Repeat("cd", 32)
	stored := []ArtFile{{first, "portrait-s.webp", 3}, {first, "portrait-m.jpeg", 5}, {second, "landscape-l.none", 0}}
	for _, file := range stored {
		if err := directories.WriteArt(file.SourceSha256Hex, file.Name, make([]byte, file.SizeBytes)); err != nil {
			t.Fatal(err)
		}
	}
	stray := filepath.Join(directories.cacheDirectory, "art", first, "notes.txt")
	if err := os.WriteFile(stray, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := directories.ListArt()
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(files, func(a, b ArtFile) int {
		return strings.Compare(a.SourceSha256Hex+a.Name, b.SourceSha256Hex+b.Name)
	})
	want := []ArtFile{{first, "portrait-m.jpeg", 5}, {first, "portrait-s.webp", 3}, {second, "landscape-l.none", 0}}
	if !slices.Equal(files, want) {
		t.Errorf("ListArt = %+v, want %+v (the stray file skipped)", files, want)
	}
}
