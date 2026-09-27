// SPDX-License-Identifier: GPL-3.0-or-later

package confine

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// The mechanisms, as the node's report names them (cut-a §7).
const (
	Landlock = "landlock"
	Seccomp  = "seccomp"
)

// ShimArgument is the first argument that makes orpheum the shim: it confines itself with a Spec
// and becomes the tool (D193 §4). It is not a subcommand anyone types, so the usage leaves it out.
const ShimArgument = "__confined-exec"

// Access is what one FFmpeg run may touch beyond the tool itself and the system's libraries: the
// files or directories it reads, and the directories it writes (cut-a §7). Anything else under
// Landlock is refused. Every encode is libx264 in M3; a hardware encoder's run will need its
// GPU's device nodes, and the /sys files that describe them, granted here when it arrives.
type Access struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
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
	InForce   bool
	Reason    string
	Remedy    string
}

// IsConfined is whether every mechanism is in force.
func IsConfined(states []State) bool {
	for _, state := range states {
		if !state.InForce {
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
