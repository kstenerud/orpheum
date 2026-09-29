// SPDX-License-Identifier: GPL-3.0-or-later

package datadir

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
)

// maxSocketPathBytes is the longest socket path both Linux (108 bytes) and macOS (104) accept,
// counting the terminating NUL.
const maxSocketPathBytes = 103

// ListenUnix listens on the Unix socket name in directory, which it creates with mode 0700 if it
// is missing. The socket is mode 0600: its permissions are the node channel's authentication
// (cut-a §2, D107). A socket a crash left behind is removed first; anything else at that path is
// refused, never removed.
func ListenUnix(directory, name string) (*net.UnixListener, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("socket directory %q: want an absolute path", directory)
	}
	path := filepath.Join(directory, name)
	if len(path) > maxSocketPathBytes {
		return nil, fmt.Errorf("socket path %s is %d bytes; a Unix socket's path may have at most %d",
			path, len(path), maxSocketPathBytes)
	}
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return nil, fmt.Errorf("create the socket directory %s: %w", directory, err)
	}
	info, err := os.Lstat(path)
	switch {
	case err == nil && info.Mode()&fs.ModeSocket == 0:
		return nil, fmt.Errorf("%s exists and is not a socket; it is left alone", path)
	case err == nil:
		if removeErr := os.Remove(path); removeErr != nil {
			return nil, fmt.Errorf("remove the stale socket %s: %w", path, removeErr)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if chmodErr := os.Chmod(path, fileMode); chmodErr != nil {
		return nil, errors.Join(fmt.Errorf("make %s mode 0600: %w", path, chmodErr), listener.Close())
	}
	return listener, nil
}
