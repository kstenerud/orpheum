// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import "path/filepath"

// NVIDIADevices are the NVIDIA GPUs' device nodes under devDirectory, one per GPU: nvidia0,
// nvidia1 and on. GPU detection reports these and a hardware encode is granted these, so what
// the node says it has and what a confined encode may open are one list.
func NVIDIADevices(devDirectory string) []string {
	found, _ := filepath.Glob(filepath.Join(devDirectory, "nvidia[0-9]*")) //nolint:errcheck // a fixed pattern
	return found
}
