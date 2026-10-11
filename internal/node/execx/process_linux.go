// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import "syscall"

// toolProcessAttributes start a tool in its own process group, which a stop kills whole, and have
// the kernel kill it when the node dies, so no FFmpeg outlives the node however the node ends.
// The kernel sends the signal when the thread that started the tool ends; the node never ends a
// thread of its own, so that is when the node does.
func toolProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
