// SPDX-License-Identifier: AGPL-3.0-or-later

package ffstreamer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// A file source serves the film it opened, and fails a run whose film changes under it: what it
// made would be of two films (READ).
func TestFileSource_ServesTheFilmItOpenedAndFailsOneThatChanges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "film.mkv")
	if err := os.WriteFile(path, []byte("the film's bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // the test's own file
	facts, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	source := NewFileSource(file, facts, diskwatch.NewGate(clock.System{}), root, path, time.Minute)
	buffer := make([]byte, 5)
	if err := source.ReadAt(t.Context(), buffer, 4); err != nil || !bytes.Equal(buffer, []byte("film'")) {
		t.Fatalf("read %q, %v; want the film's bytes at 4", buffer, err)
	}
	if source.Size() != 16 {
		t.Errorf("the film's size: %d; want 16", source.Size())
	}
	if err := os.WriteFile(path, []byte("the film's bytes, and more"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := source.ReadAt(t.Context(), buffer, 0); !errors.Is(err, ErrFilmChanged) {
		t.Errorf("a read of a film that grew: %v; want ErrFilmChanged", err)
	}
}
