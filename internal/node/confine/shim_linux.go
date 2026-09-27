// SPDX-License-Identifier: GPL-3.0-or-later

package confine

import (
	"fmt"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// Probe says which mechanisms this machine can put in force, without putting any in force: the
// node's report at start (D112).
func Probe() []State {
	return []State{landlockState(), seccompState()}
}

// confineAndExec puts every mechanism the machine offers in force on this thread, then executes
// the tool from it, so the tool starts confined. A mechanism the machine does not offer is left
// out, as the node reported; one it offers that fails stops the run (ErrNotConfined).
func confineAndExec(spec Spec) error {
	// Landlock and a seccomp filter confine the thread that asks, and exec keeps only that thread.
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("%w: set no_new_privs: %w", ErrNotConfined, err)
	}
	if abi, err := landlockABI(); err == nil {
		if err := restrictLandlock(abi, spec); err != nil {
			return fmt.Errorf("%w: %w", ErrNotConfined, err)
		}
	}
	if seccompState().InForce {
		if err := installSeccomp(); err != nil {
			return fmt.Errorf("%w: %w", ErrNotConfined, err)
		}
	}
	arguments := append([]string{spec.Tool}, spec.Arguments...)
	//nolint:gosec // G204: the tool the node named, confined above, argv as a slice
	if err := syscall.Exec(spec.Tool, arguments, []string{}); err != nil {
		return fmt.Errorf("execute %s: %w", spec.Tool, err)
	}
	return nil
}
