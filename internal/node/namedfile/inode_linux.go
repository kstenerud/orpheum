// SPDX-License-Identifier: AGPL-3.0-or-later

package namedfile

import (
	"io/fs"
	"syscall"
	"time"
)

// inodeAndChangedAt is the file's inode and the time its inode last changed (ctime), which a copy
// that keeps the modification time (cp -p, rsync -t) cannot set back.
func inodeAndChangedAt(info fs.FileInfo) (uint64, time.Time) {
	system, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, time.Time{}
	}
	return system.Ino, time.Unix(system.Ctim.Sec, system.Ctim.Nsec)
}
