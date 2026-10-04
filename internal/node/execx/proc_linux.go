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
		if stat.state == "D" {
			sample.isWaitingOnDevice = true
			if sample.waitingOnPath == "" {
				sample.waitingOnPath = blockedPathOf(entry.Name())
			}
		}
		sample.cpuTicks += stat.cpuTicks
		sample.storageBytes += storageBytesOf(pid)
	}
	return sample
}

// blockedPathOf is the file a process, by its /proc entry's name, is blocked on: the file its
// blocked call's first argument, a file descriptor, names (proc(5): /proc/<pid>/syscall, then
// /proc/<pid>/fd/<fd>); empty when it is not a file's path or /proc does not say.
func blockedPathOf(pid string) string {
	raw, err := os.ReadFile("/proc/" + pid + "/syscall") //nolint:gosec // G304: a process's own /proc entry
	if err != nil {
		return ""
	}
	return blockedPathFrom(string(raw), func(fd int64) (string, error) {
		return os.Readlink("/proc/" + pid + "/fd/" + strconv.FormatInt(fd, 10))
	})
}

// blockedPathFrom reads /proc/<pid>/syscall's text, its call's number and arguments, and is the
// absolute path its first argument names as a file descriptor, by linkOf; empty otherwise.
func blockedPathFrom(syscall string, linkOf func(fd int64) (string, error)) string {
	fields := strings.Fields(syscall)
	if len(fields) < 2 || fields[0] == "running" || strings.HasPrefix(fields[0], "-") {
		return ""
	}
	fd, err := strconv.ParseInt(strings.TrimPrefix(fields[1], "0x"), 16, 64)
	if err != nil || fd < 0 {
		return ""
	}
	path, err := linkOf(fd)
	if err != nil || !strings.HasPrefix(path, "/") {
		return ""
	}
	return path
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
