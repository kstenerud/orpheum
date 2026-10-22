// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import "golang.org/x/sys/unix"

// isFilterWritten is true: the denylist is written for this architecture (D205).
const isFilterWritten = true

// auditArch is this architecture as seccomp names it.
const auditArch = unix.AUDIT_ARCH_AARCH64

// refuseForeignCalls is nothing: arm64 has one system-call table.
func refuseForeignCalls() []unix.SockFilter { return nil }

// archDeniedSyscalls is nothing: arm64 has no fork or vfork, and starts a process only by clone.
func archDeniedSyscalls() []uint32 { return nil }
