// SPDX-License-Identifier: AGPL-3.0-or-later

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
		// No socket of any family, a Unix one included: a run reads files and pipes alone (execx), so
		// it talks to no network and, beside it, to none of the node's or the core's sockets, which a
		// decoder bug could otherwise reach past Landlock, which governs no connect(2) to a named
		// socket (the Plex-chain review's N1, 2026-10-20). A GPU run may create a Unix socket, which
		// its driver needs (onlyUnixSockets).
		unix.SYS_SOCKET, unix.SYS_SOCKETPAIR,
		// No session or process group of its own: a run that left its group would escape a kill sent
		// to the group, and hold its descriptors open past it (fdproxy's specification, Launching;
		// its third audit, M4).
		unix.SYS_SETSID, unix.SYS_SETPGID,
	}
}

// onlyUnixSockets lets socket(2) create a Unix socket and refuses every other family: CUDA asks
// NVIDIA's multi-process daemon over one and listens on one of its own as it starts (D331 §2). It
// ends the filter for socket(2), since it loads the call's first argument, the family.
func onlyUnixSockets(deny, allowAll unix.SockFilter) []unix.SockFilter {
	return []unix.SockFilter{
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_SOCKET, Jt: 0, Jf: 4},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: offsetFirstArgument},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.AF_UNIX, Jt: 0, Jf: 1},
		allowAll,
		deny,
	}
}

// startsNoProcess is the filter's end: no run starts a process, since FFmpeg starts none (its
// binaries import no fork, exec or spawn), and a process a run started would outlive its kill. A
// thread is a clone with CLONE_THREAD, and passes. The clone3 call keeps its flags in memory seccomp
// cannot read, so it answers ENOSYS, and glibc (2.34 and later) and Go fall back to clone, whose flags
// are its first argument. EPERM there would fail their threads instead (fdproxy's third audit, M3).
// Where an architecture has fork and vfork, they are refused with the denied calls above.
func startsNoProcess(deny, allowAll unix.SockFilter) []unix.SockFilter {
	notImplemented := unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.ENOSYS)}
	return []unix.SockFilter{
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_CLONE3, Jt: 0, Jf: 1},
		notImplemented,
		// clone with CLONE_THREAD in the low word of its flags is a thread: allowed. The kernel reads
		// only those 32 bits of the flags, and both architectures are little-endian.
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_CLONE, Jt: 0, Jf: 3},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: offsetFirstArgument},
		{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: unix.CLONE_THREAD, Jt: 1, Jf: 0},
		deny,
		allowAll,
	}
}

// Offsets into struct seccomp_data: the syscall's number, its architecture, and the low word of its
// first argument (after the 8-byte instruction pointer; little-endian on every architecture written).
const (
	offsetNumber        = 0
	offsetArch          = 4
	offsetFirstArgument = 16
)

// seccompState is seccomp's state on this machine: whether the kernel filters system calls.
func seccompState() State {
	state := State{Mechanism: MechanismSeccomp}
	if !isFilterWritten {
		state.CauseCode = CauseArchitectureUnwritten
		state.Reason = "the seccomp denylist is not written for this machine's architecture"
		state.Remedy = "run the node on amd64 or arm64, where it is written (D205)"
		return state
	}
	if _, err := unix.PrctlRetInt(unix.PR_GET_SECCOMP, 0, 0, 0, 0); err != nil {
		if errors.Is(err, unix.EINVAL) {
			state.CauseCode = CauseKernelWithout
			state.Reason = "the kernel was built without seccomp"
			state.Remedy = "run a kernel built with CONFIG_SECCOMP_FILTER"
			return state
		}
		state.CauseCode = CauseUnexplained
		state.Reason = fmt.Sprintf("seccomp answered %v", err)
		state.Remedy = "report it; the node cannot say what would fix it"
		return state
	}
	state.IsInForce = true
	return state
}

// denylist is the filter: another architecture's system calls are refused whole, each denied
// system call fails with EPERM, and no process is started (startsNoProcess); everything else is
// allowed. With allowsUnixSockets, socket(2) may create a Unix socket and nothing else
// (onlyUnixSockets).
func denylist(allowsUnixSockets bool) []unix.SockFilter {
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
	program = append(program, refuseForeignCalls()...)
	for _, number := range append(deniedSyscalls(), archDeniedSyscalls()...) {
		if allowsUnixSockets && number == unix.SYS_SOCKET {
			continue
		}
		program = append(program, equal(number), deny)
	}
	if allowsUnixSockets {
		program = append(program, onlyUnixSockets(deny, allowAll)...)
	}
	return append(program, startsNoProcess(deny, allowAll)...)
}

// installSeccomp puts the denylist on the calling thread, which the tool it executes inherits. It
// needs no_new_privs, which the shim has set.
func installSeccomp(allowsUnixSockets bool) error {
	program := denylist(allowsUnixSockets)
	filter := unix.SockFprog{Len: uint16(len(program)), Filter: &program[0]} //nolint:gosec // a few dozen instructions
	//nolint:gosec // G103: the filter, passed as prctl defines it
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER,
		uintptr(unsafe.Pointer(&filter)), 0, 0); err != nil {
		return fmt.Errorf("install the seccomp filter: %w", err)
	}
	return nil
}
