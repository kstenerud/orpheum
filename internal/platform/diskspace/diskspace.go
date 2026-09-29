// SPDX-License-Identifier: GPL-3.0-or-later

package diskspace

import (
	"fmt"
	"syscall"
)

// Space is a filesystem's size and what is free on it, in bytes, as a user who is not root sees
// them.
type Space struct {
	TotalBytes     int64
	AvailableBytes int64
}

// UsedPercent is how full the filesystem is, as a whole percent.
func (space Space) UsedPercent() int {
	if space.TotalBytes <= 0 {
		return 0
	}
	return int((space.TotalBytes - space.AvailableBytes) * 100 / space.TotalBytes)
}

// Measure is the size and free space of the filesystem that holds path.
func Measure(path string) (Space, error) {
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(path, &filesystem); err != nil {
		return Space{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	blockSize := int64(filesystem.Bsize) //nolint:unconvert // int64 on Linux, uint32 on macOS
	if blockSize < 0 {
		return Space{}, fmt.Errorf("statfs %s: negative block size %d", path, blockSize)
	}
	return Space{
		TotalBytes:     int64(filesystem.Blocks) * blockSize, //nolint:gosec // a disk's blocks fit
		AvailableBytes: int64(filesystem.Bavail) * blockSize, //nolint:gosec // as above
	}, nil
}
