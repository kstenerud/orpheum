// SPDX-License-Identifier: GPL-3.0-or-later

package confine

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// deniedSyscalls are refused to every FFmpeg run (cut-a §7): nothing a demuxer, decoder or
// encoder needs, and each a step out of the sandbox — debugging another process, changing what is
// mounted or which namespace it is in, loading kernel code, reaching the kernel's keys.
func deniedSyscalls() []uint32 {
	return []uint32{
		unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV,
		unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_PIVOT_ROOT, unix.SYS_CHROOT,
		unix.SYS_UNSHARE, unix.SYS_SETNS,
		unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN, unix.SYS_USERFAULTFD,
		unix.SYS_KEYCTL, unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY,
		unix.SYS_INIT_MODULE, unix.SYS_FINIT_MODULE, unix.SYS_DELETE_MODULE, unix.SYS_KEXEC_LOAD,
		unix.SYS_OPEN_BY_HANDLE_AT, unix.SYS_REBOOT, unix.SYS_SWAPON, unix.SYS_SWAPOFF, unix.SYS_ACCT,
	}
}

// deniedSocketFamilies are the address families a run may not open a socket in: no FFmpeg run
// talks to a network (cut-a §7).
func deniedSocketFamilies() []uint32 {
	return []uint32{unix.AF_INET, unix.AF_INET6, unix.AF_PACKET}
}

// Offsets into struct seccomp_data: the syscall's number, its architecture, and its first
// argument's low 32 bits on a little-endian machine.
const (
	offsetNumber   = 0
	offsetArch     = 4
	offsetArgument = 16
)

// seccompState is seccomp's state on this machine: whether the kernel filters system calls.
func seccompState() State {
	state := State{Mechanism: Seccomp}
	if !isFilterWritten {
		state.Cause = CauseArchitectureUnwritten
		state.Reason = "the seccomp denylist is not written for this machine's architecture"
		state.Remedy = "run the node on amd64 or arm64, where it is written (D205)"
		return state
	}
	if _, err := unix.PrctlRetInt(unix.PR_GET_SECCOMP, 0, 0, 0, 0); err != nil {
		if errors.Is(err, unix.EINVAL) {
			state.Cause = CauseKernelWithout
			state.Reason = "the kernel was built without seccomp"
			state.Remedy = "run a kernel built with CONFIG_SECCOMP_FILTER"
			return state
		}
		state.Cause = CauseUnexplained
		state.Reason = fmt.Sprintf("seccomp answered %v", err)
		state.Remedy = "report it; the node cannot say what would fix it"
		return state
	}
	state.InForce = true
	return state
}

// denylist is the filter: another architecture's system calls are refused whole, and each denied
// system call, and a socket in a denied family, fails with EPERM; everything else is allowed.
func denylist() []unix.SockFilter {
	deny := unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)}
	allowAll := unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW}
	load := func(offset uint32) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: offset}
	}
	// equal skips the next instruction, the denial, unless the value is k.
	equal := func(k uint32) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: k, Jt: 0, Jf: 1}
	}
	program := []unix.SockFilter{
		load(offsetArch),
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: auditArch, Jt: 1, Jf: 0},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_KILL_PROCESS},
		load(offsetNumber),
	}
	program = append(program, foreignCalls()...)
	for _, number := range deniedSyscalls() {
		program = append(program, equal(number), deny)
	}
	// A socket(2) call goes on to its family; every other call is allowed.
	program = append(program,
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_SOCKET, Jt: 1, Jf: 0},
		allowAll,
		load(offsetArgument),
	)
	for _, family := range deniedSocketFamilies() {
		program = append(program, equal(family), deny)
	}
	return append(program, allowAll)
}

// installSeccomp puts the denylist on the calling thread, which the tool it executes inherits. It
// needs no_new_privs, which the shim has set.
func installSeccomp() error {
	program := denylist()
	filter := unix.SockFprog{Len: uint16(len(program)), Filter: &program[0]} //nolint:gosec // a few dozen instructions
	//nolint:gosec // G103: the filter, passed as prctl defines it
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER,
		uintptr(unsafe.Pointer(&filter)), 0, 0); err != nil {
		return fmt.Errorf("install the seccomp filter: %w", err)
	}
	return nil
}
