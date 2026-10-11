// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"fmt"
	"syscall"
)

// networkFilesystemNameByMagic maps the statfs magic numbers of network filesystems to their names.
func networkFilesystemNameByMagic() map[int64]string {
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
	return refuseNetworkFilesystemType(directory, stat.Type)
}

func refuseNetworkFilesystemType(directory string, magic int64) error {
	if name, isNetwork := networkFilesystemNameByMagic()[magic]; isNetwork {
		return fmt.Errorf("%s is on %s, a network filesystem; SQLite's locking is not reliable there, so the "+
			"persistent directory must be on a local disk (D31)", directory, name)
	}
	return nil
}
