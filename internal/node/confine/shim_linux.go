// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// MechanismStates says which mechanisms this machine can put in force, without putting any in force: the
// node's report at start (D112).
func MechanismStates() []State {
	return []State{landlockState(), seccompState()}
}

// oomScoreAdjPath is where a process says how willing it is to be killed when the machine runs out
// of memory.
const oomScoreAdjPath = "/proc/self/oom_score_adj"

// beKilledFirst makes the tool the first the kernel kills when the machine runs out of memory, by
// raising its oom_score_adj to the highest, which any process may do: a film that drove FFmpeg to
// use memory without end fails as FFmpeg's failure on it, and the node, and every other job, goes
// on (the owner, 2026-10-05, D248). Best effort: a machine with no /proc runs the tool as it is.
func beKilledFirst(path string) {
	_ = os.WriteFile(path, []byte("1000"), 0o600) //nolint:errcheck // best effort, as above
}

// confineAndExec puts every mechanism the machine offers in force on this thread, then executes
// the tool from it, so the tool starts confined. A mechanism the machine does not offer is left
// out, as the node reported; one it offers that fails stops the run (ErrNotConfined).
func confineAndExec(spec Spec) error {
	// Landlock and a seccomp filter confine the thread that asks, and exec keeps only that thread.
	runtime.LockOSThread()
	beKilledFirst(oomScoreAdjPath)
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("%w: set no_new_privs: %w", ErrNotConfined, err)
	}
	isLandlocked := false
	if abi, err := landlockABI(); err == nil {
		if err := restrictLandlock(abi, spec); err != nil {
			return fmt.Errorf("%w: %w", ErrNotConfined, err)
		}
		isLandlocked = true
	}
	if seccompState().IsInForce {
		// A GPU run may create a Unix socket, which its driver needs, only where Landlock keeps it from
		// the keys that admit a process to the core's and the node's sockets (D331 §1, §2).
		if err := installSeccomp(spec.Access.UsesGPU && isLandlocked); err != nil {
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
