// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import "golang.org/x/sys/unix"

// isFilterWritten is true: the denylist is written for this architecture (D205).
const isFilterWritten = true

// auditArch is this architecture as seccomp names it.
const auditArch = unix.AUDIT_ARCH_X86_64

// x32Bit marks a system call made through the x32 ABI, which shares the x86-64 architecture
// value and would otherwise pass under other numbers.
const x32Bit = 0x40000000

// refuseForeignCalls refuses every x32 system call.
func refuseForeignCalls() []unix.SockFilter {
	return []unix.SockFilter{
		{Code: unix.BPF_JMP | unix.BPF_JGE | unix.BPF_K, K: x32Bit, Jt: 0, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
	}
}
