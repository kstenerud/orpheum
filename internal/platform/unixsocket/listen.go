// SPDX-License-Identifier: GPL-3.0-or-later

package unixsocket

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

// The socket directory and the socket are private to the user the process runs as: their
// permissions are the node channel's authentication (D107).
const (
	socketDirectoryMode = 0o700
	socketMode          = 0o600
)

// ErrPathTooLong is a socket path longer than a Unix socket's may be.
var ErrPathTooLong = errors.New("a Unix socket's path is too long")

// CheckPath refuses a socket path, name in directory, longer than a Unix socket's may be: the one
// check Listen makes, for a caller that must know before it listens (D242: default-config).
func CheckPath(directory, name string) error {
	path := filepath.Join(directory, name)
	if len(path) > maxSocketPathBytes {
		return fmt.Errorf("socket path %s is %d bytes, and may have at most %d: %w", path, len(path),
			maxSocketPathBytes, ErrPathTooLong)
	}
	return nil
}

// Listen listens on the Unix socket name in directory, which it creates with mode 0700 if it
// is missing. The socket is mode 0600: its permissions are the node channel's authentication
// (cut-a §2, D107). A socket a crash left behind is removed first; anything else at that path is
// refused, never removed.
func Listen(directory, name string) (*net.UnixListener, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("socket directory %q: want an absolute path", directory)
	}
	if err := CheckPath(directory, name); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, name)
	if err := os.MkdirAll(directory, socketDirectoryMode); err != nil {
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
	if chmodErr := os.Chmod(path, socketMode); chmodErr != nil {
		return nil, errors.Join(fmt.Errorf("make %s mode 0600: %w", path, chmodErr), listener.Close())
	}
	return listener, nil
}
