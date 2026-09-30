// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package namedfile

import (
	"io/fs"
	"time"
)

// inodeAndChangedAt reports nothing where the platform has no inode or ctime: the file's size and
// modification time alone then say whether it changed.
func inodeAndChangedAt(fs.FileInfo) (uint64, time.Time) {
	return 0, time.Time{}
}
