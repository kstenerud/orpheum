// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSocketDirectory_IsShortWhateverTheTestIsCalledAndGoneWhenItEnds(t *testing.T) {
	t.Parallel()
	// The temporary directory itself, which the socket directory is made in: t.TempDir's is what it
	// avoids.
	temporary := filepath.Clean(os.TempDir()) //nolint:usetesting // the directory t.TempDir's path is too long under
	var directory string
	// A parent's cleanup runs once its subtests, parallel ones too, have ended and run their own.
	t.Cleanup(func() {
		if _, err := os.Stat(directory); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the socket directory %s after its test ended: %v; want it removed", directory, err)
		}
	})
	t.Run("a test whose name is long enough that t.TempDir would leave its socket no room at all", func(t *testing.T) {
		t.Parallel()
		directory = SocketDirectory(t)
		if parent := filepath.Dir(directory); parent != temporary {
			t.Errorf("the socket directory %s is in %s; want it directly in the temporary directory %s",
				directory, parent, temporary)
		}
		if most := len(temporary) + 16; len(directory) > most {
			t.Errorf("the socket directory %s is %d bytes; want at most %d", directory, len(directory), most)
		}
		listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", filepath.Join(directory, "core.sock"))
		if err != nil {
			t.Fatalf("listen on a socket in it: %v", err)
		}
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
}
