// SPDX-License-Identifier: GPL-3.0-or-later

package confine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
