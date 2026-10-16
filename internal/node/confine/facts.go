// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Facts are what the filesystem says about a media root at the start of a scan (D42): facts the
// node owns, which the core compares against the last complete scan (D208). An unplugged disk
// leaves the empty directory under its mount point, which lists cleanly: its device id is then
// its parent's, and it is no longer a mount point.
type Facts struct {
	FilesystemDevice uint64
	IsMountPoint     bool
	IsReadOnly       bool
}

// Facts reads the root's device id, whether it is a mount point — its device differs from its
// parent's — and whether its filesystem is mounted read-only.
func (media *MediaRoot) Facts() (Facts, error) {
	device, err := deviceOfPath(media.path)
	if err != nil {
		return Facts{}, err
	}
	parent, err := deviceOfPath(filepath.Dir(media.path))
	if err != nil {
		return Facts{}, err
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(media.path, &filesystem); err != nil {
		return Facts{}, fmt.Errorf("statfs %s: %w", media.path, err)
	}
	const readOnly = 1 // ST_RDONLY on Linux, MNT_RDONLY on macOS
	return Facts{
		FilesystemDevice: device,
		IsMountPoint:     device != parent || media.path == "/",
		IsReadOnly:       uint64(filesystem.Flags)&readOnly != 0, //nolint:gosec // int64 on Linux, uint32 on macOS
	}, nil
}

func deviceOfPath(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	device, ok := DeviceOf(info)
	if !ok {
		return 0, fmt.Errorf("stat %s: no device id on this platform", path)
	}
	return device, nil
}

// InodeOf is a file's inode on its filesystem, and whether the platform gives one.
func InodeOf(info fs.FileInfo) (uint64, bool) {
	system, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return system.Ino, true
}

// DeviceOf is the device id of the filesystem an entry is on, when the platform reports one.
func DeviceOf(info fs.FileInfo) (uint64, bool) {
	system, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(system.Dev), true //nolint:unconvert // uint64 on Linux, int32 on macOS
}
