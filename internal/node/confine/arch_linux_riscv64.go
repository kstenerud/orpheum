// SPDX-License-Identifier: GPL-3.0-or-later

package confine

import "golang.org/x/sys/unix"

// auditArch is this architecture as seccomp names it.
const auditArch = unix.AUDIT_ARCH_RISCV64

// foreignCalls is nothing: riscv64 has one system-call table.
func foreignCalls() []unix.SockFilter { return nil }
