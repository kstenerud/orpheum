// SPDX-License-Identifier: GPL-3.0-or-later

package confine_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
)

func TestMediaRoot_ReadFileRefusesAFilePastItsBound(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "film.nfo"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	media, err := confine.OpenMediaRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = media.Close() }) //nolint:errcheck // a test's root
	contents, err := media.ReadFile("film.nfo", 5)
	if err != nil || string(contents) != "12345" {
		t.Errorf("a file at its bound: %q, %v", contents, err)
	}
	if _, err := media.ReadFile("film.nfo", 4); !errors.Is(err, bounded.ErrTooLarge) {
		t.Errorf("a file past its bound: %v, want bounded.ErrTooLarge", err)
	}
	if _, err := media.ReadFile("gone.nfo", 5); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: %v, want os.ErrNotExist", err)
	}
}

func TestMediaRoot_OpenNeverWaitsForAPipesWriter(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(directory, "film.mkv"), 0o600); err != nil {
		t.Fatal(err)
	}
	media, err := confine.OpenMediaRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = media.Close() }) //nolint:errcheck // a test's root
	opened := make(chan error, 1)
	go func() {
		file, err := media.Open("film.mkv")
		if err == nil {
			_ = file.Close() //nolint:errcheck // read-only
		}
		opened <- err
	}()
	select {
	case err := <-opened:
		if err != nil {
			t.Errorf("open a pipe: %v, want it open, for the caller to see it is no regular file", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("opening a pipe waited for a writer; a file named in its place would hang its reader")
	}
}
