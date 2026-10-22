// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build linux && !amd64 && !arm64

package confine

import "golang.org/x/sys/unix"

// isFilterWritten is false: the seccomp denylist is written for amd64 and arm64 only (D205), so
// on this architecture seccomp is reported not in force rather than guessed at.
const isFilterWritten = false

// auditArch is unused here.
const auditArch = 0

// refuseForeignCalls is unused here.
func refuseForeignCalls() []unix.SockFilter { return nil }

// archDeniedSyscalls is unused here.
func archDeniedSyscalls() []uint32 { return nil }
