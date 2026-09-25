// SPDX-License-Identifier: GPL-3.0-or-later

package datadir

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func prepareTemporary(t *testing.T) *Directories {
	t.Helper()
	root := t.TempDir()
	directories, err := Prepare(filepath.Join(root, "state"), filepath.Join(root, "cache"))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return directories
}

func TestPrepare_CreatesPrivateDirectories(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	for _, directory := range []string{directories.state, directories.cache, directories.BackupDirectory()} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != directoryMode {
			t.Errorf("%s has mode %v, want %v", directory, info.Mode().Perm(), os.FileMode(directoryMode))
		}
	}
	if _, err := Prepare("relative/state", t.TempDir()); err == nil {
		t.Error("Prepare accepted a relative state directory")
	}
}

func TestWriteStateFile_ReplacesAtomicallyWithMode0600(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	for _, contents := range []string{"first", "second"} {
		if err := directories.WriteStateFile("secret.key", []byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := directories.ReadStateFile("secret.key")
	if err != nil || string(got) != "second" {
		t.Errorf("ReadStateFile = %q, %v; want the second write", got, err)
	}
	info, err := os.Stat(filepath.Join(directories.state, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fileMode {
		t.Errorf("secret.key has mode %v, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(directories.state)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "secret.key" && entry.Name() != backupDirectory {
			t.Errorf("the state directory holds %q; an atomic write leaves no temporary file", entry.Name())
		}
	}
}

func TestStateFiles_RefuseAnythingButAPlainName(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	for _, name := range []string{"", "../escape", "sub/file", ".hidden", "/etc/passwd"} {
		if err := directories.WriteStateFile(name, []byte("x")); err == nil {
			t.Errorf("WriteStateFile(%q) succeeded, want a refusal", name)
		}
		if _, err := directories.ReadStateFile(name); err == nil {
			t.Errorf("ReadStateFile(%q) succeeded, want a refusal", name)
		}
	}
	if _, err := directories.ReadStateFile("setup-code"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadStateFile(missing) error = %v, want ErrNotFound", err)
	}
	if err := directories.RemoveStateFile("setup-code"); err != nil {
		t.Errorf("RemoveStateFile(missing) = %v, want nil", err)
	}
}

func TestReplaceLibraryDatabase_RemovesTheOldWAL(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	target := directories.LibraryDatabase()
	for _, path := range []string{target, target + "-wal", target + "-shm"} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	restoredPath := filepath.Join(directories.state, "library.db.restoring")
	if err := os.WriteFile(restoredPath, []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := directories.ReplaceLibraryDatabase("library.db.restoring"); err != nil {
		t.Fatalf("ReplaceLibraryDatabase: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "restored" {
		t.Errorf("library.db = %q, %v; want the restored copy", got, err)
	}
	for _, sidecar := range []string{target + "-wal", target + "-shm"} {
		if _, err := os.Stat(sidecar); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived; a stale WAL would be replayed into the restored database", sidecar)
		}
	}
	if err := directories.ReplaceLibraryDatabase("../elsewhere.db"); err == nil {
		t.Error("ReplaceLibraryDatabase accepted a copy outside the state directory")
	}
}

func TestLockState_RefusesASecondHolder(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	server, err := directories.LockState()
	if err != nil {
		t.Fatalf("the first lock: %v", err)
	}
	if _, err = directories.LockState(); !errors.Is(err, ErrInUse) {
		t.Errorf("a second lock while the first is held: error = %v, want ErrInUse", err)
	}
	if err = server.Release(); err != nil {
		t.Fatal(err)
	}
	restore, err := directories.LockState()
	if err != nil {
		t.Fatalf("a lock after release: %v", err)
	}
	if err = restore.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteBackupFile_WritesIntoTheBackupDirectory(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	if err := directories.WriteBackupFile("backup.secret.key", []byte("key")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(directories.BackupDirectory(), "backup.secret.key"))
	if err != nil || string(got) != "key" {
		t.Errorf("the backup file = %q, %v", got, err)
	}
	if err := directories.WriteBackupFile("../secret.key", []byte("x")); err == nil {
		t.Error("WriteBackupFile wrote outside the backup directory")
	}
}

func TestListBackups_ListsNewestFirst(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
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
	directories := prepareTemporary(t)
	names := []string{"cache.db", "cache.db-wal", "cache.db-shm", "art"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(directories.cache, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := directories.DiscardCacheDatabase(); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		_, err := os.Stat(filepath.Join(directories.cache, name))
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
	directories := prepareTemporary(t)
	hash := strings.Repeat("ab", 32)
	if err := directories.WriteArt(hash, "portrait-m.webp", []byte("RIFF")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directories.cache, "art", hash, "portrait-m.webp"))
	if err != nil || info.Mode().Perm() != fileMode {
		t.Fatalf("stored file: %v, %v; want mode 0600", info, err)
	}
	if got, err := directories.ReadArt(hash, "portrait-m.webp"); err != nil || string(got) != "RIFF" {
		t.Errorf("ReadArt = %q, %v", got, err)
	}
	if _, err := directories.ReadArt(hash, "portrait-s.webp"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a size never written: %v, want ErrNotFound", err)
	}

	for _, name := range []string{"../../state/library.db", "portrait-m.webp/../x", ".hidden", "poster.png"} {
		if err := directories.WriteArt(hash, name, nil); !errors.Is(err, ErrBadArtName) {
			t.Errorf("WriteArt(%q) = %v, want ErrBadArtName", name, err)
		}
	}
	if err := directories.WriteArt("../"+hash[3:], "portrait-m.webp", nil); !errors.Is(err, ErrBadArtName) {
		t.Errorf("a hash that is a path: %v, want ErrBadArtName", err)
	}
	if err := directories.RemoveArt(hash, "portrait-m.webp"); err != nil {
		t.Fatal(err)
	}
	if _, err := directories.ReadArt(hash, "portrait-m.webp"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after RemoveArt: %v", err)
	}
	if err := directories.RemoveArt(hash, "portrait-m.webp"); err != nil {
		t.Errorf("removing again: %v", err)
	}
}

func TestHasArt_SaysWhetherASizeIsStoredWithoutReadingIt(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	hash := strings.Repeat("ab", 32)
	if err := directories.WriteArt(hash, "portrait-m.webp", []byte("RIFF")); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		hash, name string
		want       bool
	}{
		{hash, "portrait-m.webp", true},
		{hash, "portrait-l.none", false},
		{strings.Repeat("cd", 32), "portrait-m.webp", false},
	} {
		if found, err := directories.HasArt(check.hash, check.name); found != check.want || err != nil {
			t.Errorf("HasArt(%s, %s) = %v, %v; want %v", check.hash[:4], check.name, found, err, check.want)
		}
	}
	if _, err := directories.HasArt(hash, "../x"); !errors.Is(err, ErrBadArtName) {
		t.Errorf("HasArt of a path: %v, want ErrBadArtName", err)
	}
}

func TestListArt_ListsEveryStoredFileWithItsSizeAndNothingElse(t *testing.T) {
	t.Parallel()
	directories := prepareTemporary(t)
	if files, err := directories.ListArt(); err != nil || len(files) != 0 {
		t.Fatalf("before any art: %v, %v", files, err)
	}
	first, second := strings.Repeat("ab", 32), strings.Repeat("cd", 32)
	stored := []ArtFile{{first, "portrait-s.webp", 3}, {first, "portrait-m.jpeg", 5}, {second, "landscape-l.none", 0}}
	for _, file := range stored {
		if err := directories.WriteArt(file.Hash, file.Name, make([]byte, file.Size)); err != nil {
			t.Fatal(err)
		}
	}
	stray := filepath.Join(directories.cache, "art", first, "notes.txt")
	if err := os.WriteFile(stray, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := directories.ListArt()
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(files, func(a, b ArtFile) int { return strings.Compare(a.Hash+a.Name, b.Hash+b.Name) })
	want := []ArtFile{{first, "portrait-m.jpeg", 5}, {first, "portrait-s.webp", 3}, {second, "landscape-l.none", 0}}
	if !slices.Equal(files, want) {
		t.Errorf("ListArt = %+v, want %+v (the stray file skipped)", files, want)
	}
}
