// SPDX-License-Identifier: AGPL-3.0-or-later

package diskspace

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall"
)

// BytesPerGiB is a gibibyte: the unit of every size a person sets or is shown in GiB.
const BytesPerGiB = 1 << 30

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

// TreeBytes is the size of every regular file under root: what the files written there take. A
// symbolic link, device or pipe is not counted, so a link to a large file elsewhere adds nothing. A
// file or directory under root that is gone by the time it is read, removed as it was walked, is
// not counted: a run of chunks starting failed measuring its store while a cancelled run's
// directory was removed beside it (the owner's jumps, 2026-10-12). A root that is gone fails.
func TreeBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return goneUnder(root, path, err)
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return goneUnder(root, path, infoErr)
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure %s: %w", root, err)
	}
	return total, nil
}

// goneUnder is err, unless it says a path under root was gone when read: nothing, so the walk goes
// on without it.
func goneUnder(root, path string, err error) error {
	if path != root && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
