// SPDX-License-Identifier: GPL-3.0-or-later

package execx

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"strings"
)

// sampleGroup is what /proc says of the process group pgid now: whether a process in it waits on a
// device, the processor time they have used, and the bytes they have had read from and written to
// storage. A process that ends between the listing and the reading is left out.
func sampleGroup(pgid int) groupSample {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return groupSample{}
	}
	var sample groupSample
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err != nil {
			continue
		}
		stat, isParsed := parseStat(string(raw))
		if !isParsed || stat.groupID != pgid {
			continue
		}
		sample.processCount++
		sample.isWaitingOnDevice = sample.isWaitingOnDevice || stat.state == "D"
		sample.cpuTicks += stat.cpuTicks
		sample.storageBytes += storageBytesOf(pid)
	}
	return sample
}

// procStat is what a process's /proc/<pid>/stat says that a stall is judged by.
type procStat struct {
	// state is its state letter: R running, S sleeping, D waiting on a device, T stopped, Z ended.
	state string
	// groupID is its process group.
	groupID int
	// cpuTicks is the user and system time it has used, in clock ticks.
	cpuTicks int64
}

// parseStat reads /proc/<pid>/stat: the fields after the parenthesised command name, which may
// itself hold spaces and parentheses, are the state, the parent, the process group, and so on, the
// user and system times the 12th and 13th after the state (proc(5)).
func parseStat(stat string) (procStat, bool) {
	closing := strings.LastIndex(stat, ")")
	if closing < 0 {
		return procStat{}, false
	}
	fields := strings.Fields(stat[closing+1:])
	const userTimeField, systemTimeField = 11, 12
	if len(fields) <= systemTimeField {
		return procStat{}, false
	}
	groupID, groupErr := strconv.Atoi(fields[2])
	userTicks, userErr := strconv.ParseInt(fields[userTimeField], 10, 64)
	systemTicks, systemErr := strconv.ParseInt(fields[systemTimeField], 10, 64)
	if groupErr != nil || userErr != nil || systemErr != nil {
		return procStat{}, false
	}
	return procStat{state: fields[0], groupID: groupID, cpuTicks: userTicks + systemTicks}, true
}

// storageBytesOf is the bytes process pid has had read from and written to storage, as
// /proc/<pid>/io says, or zero when it cannot be read.
func storageBytesOf(pid int) int64 {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/io")
	if err != nil {
		return 0
	}
	var total int64
	lines := bufio.NewScanner(bytes.NewReader(raw))
	for lines.Scan() {
		key, value, _ := strings.Cut(lines.Text(), ":")
		if key != "read_bytes" && key != "write_bytes" {
			continue
		}
		if counted, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			total += counted
		}
	}
	return total
}
