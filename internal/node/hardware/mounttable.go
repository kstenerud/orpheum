// SPDX-License-Identifier: AGPL-3.0-or-later

package hardware

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// MountTable is the system's mount table, read once and again only when the kernel says it
// changed: an open /proc/self/mountinfo raises POLLPRI after a mount or unmount (proc(5)). The disk
// gate asks it for the mount point of every operation, each 1 MiB fdproxy read among them, where
// reading and parsing the table each time cost 1.5-1.8 ms (audit 2026-10-25 N2).
type MountTable struct {
	path string
	// hasChanged is whether the table open in file changed since it was last read: whether it
	// raises POLLPRI; a test's says so when told.
	hasChanged func(file *os.File) bool

	mutex  sync.Mutex
	file   *os.File // guarded by mutex: the table, open; nil until it opens
	mounts []mount  // guarded by mutex: as last read
}

// NewMountTable is the system's mount table, read at its first use.
func NewMountTable() *MountTable {
	return &MountTable{path: "/proc/self/mountinfo", hasChanged: raisesPriority}
}

// MountPointOf is the mount point of the filesystem path is on, from the mount table, which reads
// nothing on the filesystem: what the disk gate keys a disk mounted inside a root by (D328 §1).
// False when the table cannot be read or holds none.
func (table *MountTable) MountPointOf(path string) (string, bool) {
	table.mutex.Lock()
	defer table.mutex.Unlock()
	if err := table.readIfChanged(); err != nil {
		return "", false
	}
	holding, found := mountHolding(table.mounts, path)
	return holding.mountPoint, found
}

// readIfChanged reads the table when it is not open yet or the kernel says it changed. A table that
// cannot be opened is tried again at the next call; one that cannot be read is closed, to be opened
// and read afresh then. Called with mutex held.
func (table *MountTable) readIfChanged() error {
	if table.file != nil && !table.hasChanged(table.file) {
		return nil
	}
	if table.file == nil {
		file, err := os.Open(table.path)
		if err != nil {
			return fmt.Errorf("open the mount table: %w", err)
		}
		table.file = file
	}
	// The poll above took the change as seen, so the table read now is at least as new as it.
	content, err := readFromStart(table.file)
	if err != nil {
		readonly.Close(table.file)
		table.file, table.mounts = nil, nil
		return err
	}
	table.mounts = parseMounts(content)
	return nil
}

// readFromStart is file's whole content, read from its start.
func readFromStart(file *os.File) ([]byte, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("read the mount table: %w", err)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read the mount table: %w", err)
	}
	return content, nil
}

// raisesPriority is whether file, an open mountinfo, raises POLLPRI or POLLERR: a mount or unmount
// since it was last polled. A poll that fails says changed, so the table is read again rather than
// kept stale.
func raisesPriority(file *os.File) bool {
	polled := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLPRI}} //nolint:gosec // a descriptor fits in int32
	ready, err := unix.Poll(polled, 0)
	if err != nil {
		return true
	}
	return ready > 0 && polled[0].Revents&(unix.POLLPRI|unix.POLLERR) != 0
}
