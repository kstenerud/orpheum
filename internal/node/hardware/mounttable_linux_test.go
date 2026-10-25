// SPDX-License-Identifier: AGPL-3.0-or-later

package hardware

import "testing"

// The system's own table, once read, says it has not changed until a mount or an unmount: the
// kernel's POLLPRI, which the node's table waits for before reading it again. A mount made by
// another process in the moment between is a change, so the read is tried a few times.
func TestMountTable_TheSystemsTableSaysUnchangedOnceRead(t *testing.T) {
	t.Parallel()
	table := NewMountTable()
	if mountPoint, found := table.MountPointOf("/proc/self"); !found || mountPoint != "/proc" {
		t.Fatalf("/proc/self is on %q, %t; want /proc", mountPoint, found)
	}
	for range 5 {
		table.mutex.Lock()
		changed := raisesPriority(table.file)
		table.mutex.Unlock()
		if !changed {
			return
		}
		table.MountPointOf("/") // read it again, the change taken as seen
	}
	t.Error("the system's mount table, just read, says it changed at every poll")
}
