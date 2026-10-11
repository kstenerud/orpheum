// SPDX-License-Identifier: AGPL-3.0-or-later

package unixsocket

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListen_MakesAPrivateSocketAndReplacesAStaleOne(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "run")
	listener, err := Listen(directory, "core.sock")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "core.sock")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the socket is mode %v, want 0600: its permissions are the authentication", info.Mode().Perm())
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil || directoryInfo.Mode().Perm() != 0o700 {
		t.Errorf("the socket directory is %v, %v; want mode 0700", directoryInfo.Mode().Perm(), err)
	}
	var dialer net.Dialer
	connection, err := dialer.DialContext(t.Context(), "unix", path)
	if err != nil {
		t.Fatalf("dial the socket: %v", err)
	}
	if closeErr := connection.Close(); closeErr != nil {
		t.Error(closeErr)
	}
	// A crash leaves the socket file behind; the next start replaces it.
	listener.SetUnlinkOnClose(false)
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	again, err := Listen(directory, "core.sock")
	if err != nil {
		t.Fatalf("listen over a stale socket: %v", err)
	}
	if err := again.Close(); err != nil {
		t.Error(err)
	}
}

func TestListen_RefusesToRemoveWhatIsNotASocket(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "core.sock")
	if err := os.WriteFile(path, []byte("somebody's file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(directory, "core.sock"); err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Errorf("error = %v, want a refusal", err)
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "somebody's file" {
		t.Errorf("the file became %q, %v; it must be left alone", contents, err)
	}
	long := filepath.Join(directory, strings.Repeat("d", 100))
	if _, err := Listen(long, "core.sock"); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Errorf("an over-long path: error = %v, want a refusal naming the limit", err)
	}
}
