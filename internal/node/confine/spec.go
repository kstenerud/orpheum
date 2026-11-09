// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// The mechanisms, as the node's report names them (cut-a §7).
const (
	MechanismLandlock = "landlock"
	MechanismSeccomp  = "seccomp"
)

// ShimArgument is the first argument that makes orpheum the shim: it confines itself with a Spec
// and becomes the tool (D193 §4). It is not a subcommand anyone types, so the usage leaves it out.
const ShimArgument = "__confined-exec"

// Access is what one FFmpeg run may touch beyond the tool itself and the system's libraries: the
// files or directories it reads, the directories it writes, and whether it uses the GPU, for a
// hardware encode (cut-a §6, §7): its device nodes, and the /sys and /proc files that describe
// them. Anything else under Landlock is refused.
type Access struct {
	Read    []string `json:"read,omitempty"`
	Write   []string `json:"write,omitempty"`
	UsesGPU bool     `json:"usesGpu,omitempty"`
}

// Spec is one confined run: the tool, its arguments, and its access.
type Spec struct {
	Tool      string   `json:"tool"`
	Arguments []string `json:"arguments"`
	Access    Access   `json:"access"`
}

// State is one mechanism on this machine: in force, or not, why, and what would put it there.
type State struct {
	Mechanism string
	IsInForce bool
	// CauseCode names why it is not in force, from a closed set a client puts in its viewer's words.
	CauseCode string
	Reason    string
	Remedy    string
}

// The causes a mechanism is not in force (D112).
const (
	CauseDisabledAtBoot   = "disabled_at_boot"
	CauseKernelWithout    = "kernel_without"
	CauseBlockedByRuntime = "blocked_by_runtime"
	CauseNotLinux         = "not_linux"
	// CauseArchitectureUnwritten is a mechanism not yet written for the machine's architecture.
	CauseArchitectureUnwritten = "architecture_unwritten"
	CauseUnexplained           = "unexplained"
)

// IsMissingFromKernel is whether the mechanism is not in force because the machine's kernel lacks
// it: too old, built without it, or not switched on at boot (Landlock's LSM not listed). That is old
// hardware, not a fault, and the node runs without it (D454 §3). A mechanism the kernel has that
// cannot be put in force (a runtime blocking it, an answer the node cannot explain) is not missing
// from the kernel, nor is one not written for the machine's architecture.
func (state State) IsMissingFromKernel() bool {
	return !state.IsInForce && (state.CauseCode == CauseKernelWithout || state.CauseCode == CauseDisabledAtBoot)
}

// IsConfined is whether every mechanism is in force.
func IsConfined(states []State) bool {
	for _, state := range states {
		if !state.IsInForce {
			return false
		}
	}
	return len(states) > 0
}

// EncodeSpec is a spec as the shim's argument carries it.
func EncodeSpec(spec Spec) (string, error) {
	for _, path := range append(append([]string{spec.Tool}, spec.Access.Read...), spec.Access.Write...) {
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("a confined run names absolute paths, not %q", path)
		}
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("encode a confined run: %w", err)
	}
	return string(encoded), nil
}

// ErrNotConfined is a mechanism the kernel offers that could not be put in force: the shim then
// refuses to run the tool rather than run it with less than the node reported.
var ErrNotConfined = errors.New("could not confine the run")

// Shim confines this process with the encoded spec and becomes its tool; it returns only when it
// could not. The caller is orpheum's main, before anything else runs.
func Shim(encoded string) error {
	var spec Spec
	if err := json.Unmarshal([]byte(encoded), &spec); err != nil {
		return fmt.Errorf("read a confined run: %w", err)
	}
	return confineAndExec(spec)
}
