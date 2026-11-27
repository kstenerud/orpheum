// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"fmt"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// toolProcessAttributes start a tool in its own process group, which a stop kills whole, and have
// the kernel kill it when the node dies, so no FFmpeg outlives the node however the node ends.
// The kernel sends the signal when the thread that started the tool ends; the node never ends a
// thread of its own but an idle run's (startIdle), which lives as long as its tool, so that is when
// the node does.
func toolProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

// startIdle starts a tool by start on a thread of its own in the idle scheduling class, which the
// tool inherits, and every thread and process it starts after it: Go's SysProcAttr cannot set a
// child's class. The thread keeps that class, which an unprivileged process cannot leave again, so
// it is never given back to Go: it lives until exited closes, because the kernel's parent-death
// signal fires when the thread that started the tool ends, and then ends with its goroutine.
func startIdle(start func() error, exited <-chan struct{}) error {
	started := make(chan error, 1)
	go func() {
		runtime.LockOSThread() // never unlocked: the thread ends with this goroutine
		// Thread 0 is the calling thread, this one alone.
		if err := unix.SchedSetAttr(0, &unix.SchedAttr{Policy: unix.SCHED_IDLE}, 0); err != nil {
			started <- fmt.Errorf("enter the idle scheduling class: %w", err)
			return
		}
		err := start()
		started <- err
		if err == nil {
			<-exited
		}
	}()
	return <-started
}
