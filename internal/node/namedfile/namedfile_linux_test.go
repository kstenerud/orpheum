// SPDX-License-Identifier: GPL-3.0-or-later

package namedfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/namedfile"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
)

func TestOpen_RefusesASameSizeReplacementThatKeptItsModificationTime(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	full := filepath.Join(directory, "film.mkv")
	keptAt := time.Unix(1_700_000_000, 0)
	writeKeepingModificationTime(t, full, "a film", keptAt)
	root, hashes := nodetest.OpenMediaRoot(t, directory), namedfile.NewHashes()
	named := nodetest.PayloadHash(t, "a film")
	file, _, err := hashes.Open(root, "film.mkv", named)
	if err != nil {
		t.Fatalf("the file named: %v", err)
	}
	_ = file.Close() //nolint:errcheck // read-only
	// What cp -p or rsync -t leaves: the same size and the old mtime, different bytes. The
	// filesystem's clock is coarse, so it is written until its ctime has moved on.
	remembered := changedAt(t, full)
	for changedAt(t, full).Equal(remembered) {
		writeKeepingModificationTime(t, full, "b film", keptAt)
	}
	if _, _, err := hashes.Open(root, "film.mkv", named); !errors.Is(err, namedfile.ErrChanged) {
		t.Errorf("a same-size replacement with the old mtime: %v, want ErrChanged", err)
	}
}

func writeKeepingModificationTime(t *testing.T, full, content string, modifiedAt time.Time) {
	t.Helper()
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(full, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}
}

func changedAt(t *testing.T, full string) time.Time {
	t.Helper()
	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	system, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("no stat on Linux")
	}
	return time.Unix(system.Ctim.Sec, system.Ctim.Nsec)
}
