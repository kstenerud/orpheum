// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !linux

package execx

import "syscall"

// toolProcessAttributes start a tool in its own process group, which a stop kills whole. Only
// Linux can have the kernel kill it when the node dies.
func toolProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
