// SPDX-License-Identifier: AGPL-3.0-or-later

package selfuse

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
)

// Scope is what a Sample measured: the process's control group, the processes it started among it,
// or the process alone.
type Scope string

// The scopes.
const (
	// ScopeGroup is the process's cgroup (v2): a container's, or a service's, so a node's FFmpeg
	// runs count as its own.
	ScopeGroup Scope = "cgroup"
	// ScopeProcess is the process alone, from /proc/self: where no cgroup of its own could be read.
	ScopeProcess Scope = "process"
)

// Sample is what a process has used, read at one moment.
type Sample struct {
	Scope Scope
	// CPUMicros is the CPU time used since the scope began, user and system, in microseconds: the
	// difference between two samples is the time used between them, whatever was missed.
	CPUMicros int64
	// HeldBytes is the memory held that the kernel cannot take back without the process: anonymous
	// memory, its heap and stacks.
	HeldBytes int64
	// CacheBytes is the files' pages kept in memory for it, which the kernel takes back when it
	// needs the room.
	CacheBytes int64
	// PeakBytes is the most memory it has held at once since the scope began; zero when the kernel
	// does not say (memory.peak came with Linux 5.19).
	PeakBytes int64
}

// userHz is the clock tick /proc counts CPU time in: USER_HZ, fixed at 100 by the kernel's ABI on
// every architecture Linux runs userspace on, whatever the kernel's own tick.
const userHz = 100

// Read reads what this process uses now, from the root of the machine's filesystem.
func Read() (Sample, error) {
	return ReadFrom(os.DirFS("/"))
}

// ReadFrom reads what the process uses from root, a filesystem holding proc/ and sys/: its cgroup's
// counters when it is in a cgroup of its own, else the process's own.
func ReadFrom(root fs.FS) (Sample, error) {
	if sample, err := readGroup(root); err == nil {
		return sample, nil
	}
	return readProcess(root)
}

// errNoGroup is a process in no cgroup of its own that can be read: the machine's root group, which
// counts every process on it, or cgroup v1.
var errNoGroup = errors.New("no cgroup v2 of its own")

// readGroup reads the process's cgroup v2 counters. The machine's root group has no memory.current,
// so a process in it, which would count the whole machine, is not read as its own.
func readGroup(root fs.FS) (Sample, error) {
	membership, err := fs.ReadFile(root, "proc/self/cgroup")
	if err != nil {
		return Sample{}, fmt.Errorf("read its cgroup: %w", err)
	}
	relative, found := groupPath(membership)
	if !found {
		return Sample{}, errNoGroup
	}
	directory := path.Join("sys/fs/cgroup", relative)
	if _, statErr := fs.Stat(root, path.Join(directory, "memory.current")); statErr != nil {
		return Sample{}, errNoGroup
	}
	cpu, err := readKeyed(root, path.Join(directory, "cpu.stat"), "usage_usec")
	if err != nil {
		return Sample{}, err
	}
	memory, err := readKeys(root, path.Join(directory, "memory.stat"))
	if err != nil {
		return Sample{}, err
	}
	sample := Sample{Scope: ScopeGroup, CPUMicros: cpu, HeldBytes: memory["anon"], CacheBytes: memory["file"]}
	if peak, err := readNumber(root, path.Join(directory, "memory.peak")); err == nil {
		sample.PeakBytes = peak
	}
	return sample, nil
}

// groupPath is the cgroup v2 path in /proc/self/cgroup's "0::<path>" line.
func groupPath(membership []byte) (string, bool) {
	for line := range strings.SplitSeq(string(membership), "\n") {
		if relative, found := strings.CutPrefix(line, "0::"); found {
			return strings.TrimPrefix(relative, "/"), true
		}
	}
	return "", false
}

// readProcess reads the process's own CPU time from /proc/self/stat and its memory from
// /proc/self/status: its resident anonymous and file pages, and its high-water mark.
func readProcess(root fs.FS) (Sample, error) {
	stat, err := fs.ReadFile(root, "proc/self/stat")
	if err != nil {
		return Sample{}, fmt.Errorf("read the process's CPU time: %w", err)
	}
	ticks, err := cpuTicks(stat)
	if err != nil {
		return Sample{}, err
	}
	status, err := fs.ReadFile(root, "proc/self/status")
	if err != nil {
		return Sample{}, fmt.Errorf("read the process's memory: %w", err)
	}
	memory := statusKibibytes(status)
	return Sample{
		Scope: ScopeProcess, CPUMicros: ticks * (1_000_000 / userHz),
		HeldBytes: memory["RssAnon"] * 1024, CacheBytes: memory["RssFile"] * 1024, PeakBytes: memory["VmHWM"] * 1024,
	}, nil
}

// cpuTicks is utime plus stime, /proc/self/stat's fields 14 and 15. The command's name, field 2, is
// in parentheses and may hold spaces, so the fields are counted from the last ')'.
func cpuTicks(stat []byte) (int64, error) {
	closing := bytes.LastIndexByte(stat, ')')
	if closing < 0 {
		return 0, errors.New("/proc/self/stat has no command name")
	}
	fields := strings.Fields(string(stat[closing+1:]))
	const utimeAfterName, stimeAfterName = 11, 12 // fields 14 and 15, the name being field 2
	if len(fields) <= stimeAfterName {
		return 0, fmt.Errorf("/proc/self/stat has %d fields after the name", len(fields))
	}
	user, userErr := strconv.ParseInt(fields[utimeAfterName], 10, 64)
	system, systemErr := strconv.ParseInt(fields[stimeAfterName], 10, 64)
	if err := errors.Join(userErr, systemErr); err != nil {
		return 0, fmt.Errorf("/proc/self/stat's CPU time: %w", err)
	}
	return user + system, nil
}

// statusKibibytes are /proc/self/status's "Name:   123 kB" lines, by name.
func statusKibibytes(status []byte) map[string]int64 {
	values := map[string]int64{}
	scanner := bufio.NewScanner(bytes.NewReader(status))
	for scanner.Scan() {
		name, rest, found := strings.Cut(scanner.Text(), ":")
		number, isKibibytes := strings.CutSuffix(strings.TrimSpace(rest), " kB")
		if !found || !isKibibytes {
			continue
		}
		if value, err := strconv.ParseInt(number, 10, 64); err == nil {
			values[name] = value
		}
	}
	return values
}

// readKeys reads a cgroup file of "key value" lines.
func readKeys(root fs.FS, name string) (map[string]int64, error) {
	content, err := fs.ReadFile(root, name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	values := map[string]int64{}
	for line := range strings.SplitSeq(string(content), "\n") {
		key, number, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		if value, err := strconv.ParseInt(number, 10, 64); err == nil {
			values[key] = value
		}
	}
	return values, nil
}

// readKeyed reads one key of a cgroup file of "key value" lines; an error when it is not there.
func readKeyed(root fs.FS, name, key string) (int64, error) {
	values, err := readKeys(root, name)
	if err != nil {
		return 0, err
	}
	value, found := values[key]
	if !found {
		return 0, fmt.Errorf("%s has no %s", name, key)
	}
	return value, nil
}

// readNumber reads a cgroup file holding one number.
func readNumber(root fs.FS, name string) (int64, error) {
	content, err := fs.ReadFile(root, name)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", name, err)
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return value, nil
}
