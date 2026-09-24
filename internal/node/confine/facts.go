// SPDX-License-Identifier: GPL-3.0-or-later

package confine

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Facts are what the filesystem says about a media root at the start of a scan (D42): facts the
// node owns, which the core compares against the last complete scan (D208). An unplugged disk
// leaves the empty directory under its mount point, which lists cleanly: its device id is then
// its parent's, and it is no longer a mount point.
type Facts struct {
	Device     uint64
	MountPoint bool
	ReadOnly   bool
}

// Facts reads the root's device id, whether it is a mount point — its device differs from its
// parent's — and whether its filesystem is mounted read-only.
func (media *MediaRoot) Facts() (Facts, error) {
	device, err := deviceOf(media.path)
	if err != nil {
		return Facts{}, err
	}
	parent, err := deviceOf(filepath.Dir(media.path))
	if err != nil {
		return Facts{}, err
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(media.path, &filesystem); err != nil {
		return Facts{}, fmt.Errorf("statfs %s: %w", media.path, err)
	}
	const readOnly = 1 // ST_RDONLY on Linux, MNT_RDONLY on macOS
	return Facts{
		Device:     device,
		MountPoint: device != parent || media.path == "/",
		ReadOnly:   uint64(filesystem.Flags)&readOnly != 0, //nolint:gosec // int64 on Linux, uint32 on macOS
	}, nil
}

func deviceOf(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	system, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat %s: no device id on this platform", path)
	}
	return uint64(system.Dev), nil //nolint:unconvert // uint64 on Linux, int32 on macOS
}
