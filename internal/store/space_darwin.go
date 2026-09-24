// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"syscall"
)

// availableBytes is the space an unprivileged process may still write under directory.
func availableBytes(directory string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", directory, err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
