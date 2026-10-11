// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !linux

package confine

import (
	"fmt"
	"syscall"
)

// unconfinedHere is why no mechanism is in force on this platform.
const unconfinedHere = "Landlock and seccomp are Linux's; this platform has neither"

// MechanismStates says no mechanism is in force: the shim is a no-op off Linux (cut-a §7).
func MechanismStates() []State {
	return []State{
		{Mechanism: MechanismLandlock, CauseCode: CauseNotLinux, Reason: unconfinedHere, Remedy: "run the node on Linux"},
		{Mechanism: MechanismSeccomp, CauseCode: CauseNotLinux, Reason: unconfinedHere, Remedy: "run the node on Linux"},
	}
}

// confineAndExec executes the tool unconfined.
func confineAndExec(spec Spec) error {
	arguments := append([]string{spec.Tool}, spec.Arguments...)
	if err := syscall.Exec(spec.Tool, arguments, []string{}); err != nil {
		return fmt.Errorf("execute %s: %w", spec.Tool, err)
	}
	return nil
}
