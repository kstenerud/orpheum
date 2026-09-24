// SPDX-License-Identifier: GPL-3.0-or-later

package datadir

import (
	"fmt"
	"syscall"
)

// networkFilesystems maps the statfs magic numbers of network filesystems to their names.
func networkFilesystems() map[int64]string {
	return map[int64]string{
		0x6969:     "NFS",
		0x517B:     "SMB",
		0xFF534D42: "CIFS",
		0xFE534D42: "SMB2",
		0x564C:     "NCP",
		0x7461636F: "OCFS2",
		0x47504653: "GPFS",
		0x00C36400: "CEPH",
		0x5346414F: "AFS",
		0x6B414653: "AFS (kAFS)",
	}
}

// refuseNetworkFilesystem refuses a directory on a network filesystem, whose locking SQLite's
// WAL mode cannot rely on (D31).
func refuseNetworkFilesystem(directory string) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return fmt.Errorf("statfs %s: %w", directory, err)
	}
	return checkFilesystemType(directory, stat.Type)
}

func checkFilesystemType(directory string, magic int64) error {
	if name, isNetwork := networkFilesystems()[magic]; isNetwork {
		return fmt.Errorf("%s is on %s, a network filesystem; SQLite's locking is not reliable there, so the "+
			"state directory must be on a local disk (D31)", directory, name)
	}
	return nil
}
