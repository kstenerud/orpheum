// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"os"
	"testing"
)

// socketDirectoryPrefix starts the name of each directory SocketDirectory makes: short, so the path
// is the temporary directory's and a dozen bytes more.
const socketDirectoryPrefix = "o"

// SocketDirectory is a new directory for a test's Unix sockets, directly in the temporary directory
// and removed when the test ends. A socket's path is at most 108 bytes (103 on a Mac), and
// t.TempDir's carries the test's name and Go's own temporary directory, which leaves a socket a few
// bytes or none; this one's is short whatever the test is called.
func SocketDirectory(tb testing.TB) string {
	tb.Helper()
	//nolint:usetesting // t.TempDir's path is what this avoids: it leaves a socket a few bytes or none
	directory, err := os.MkdirTemp("", socketDirectoryPrefix)
	if err != nil {
		tb.Fatalf("make a socket directory: %v", err)
	}
	tb.Cleanup(func() {
		//nolint:forbidigo // the directory this test made for its own sockets, as t.TempDir's is removed
		if err := os.RemoveAll(directory); err != nil {
			tb.Errorf("remove the socket directory %s: %v", directory, err)
		}
	})
	return directory
}
