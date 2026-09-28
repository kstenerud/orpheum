// SPDX-License-Identifier: GPL-3.0-or-later

package hardware

import (
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/sys/unix"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// GPUs are the GPU devices on this machine an encoder opens: each render node under /dev/dri,
// which Quick Sync and VA-API use, and each /dev/nvidiaN, which NVENC uses.
func GPUs() []nodeproto.GPU { return gpusUnder("/dev", "/sys/class/drm") }

// gpusUnder is GPUs with the device and DRM class directories named, so a test can build them.
func gpusUnder(devices, drm string) []nodeproto.GPU {
	var found []nodeproto.GPU
	renderNodes, _ := filepath.Glob(filepath.Join(devices, "dri", "renderD*")) //nolint:errcheck // a fixed pattern
	for _, device := range renderNodes {
		driver := driverOf(filepath.Join(drm, filepath.Base(device), "device", "driver"))
		found = append(found, nodeproto.GPU{Device: device, Driver: driver, NoAccess: noAccess(device)})
	}
	nvidia, _ := filepath.Glob(filepath.Join(devices, "nvidia[0-9]*")) //nolint:errcheck // a fixed pattern
	for _, device := range nvidia {
		found = append(found, nodeproto.GPU{Device: device, Driver: "nvidia", NoAccess: noAccess(device)})
	}
	slices.SortFunc(found, func(one, other nodeproto.GPU) int {
		if one.Device < other.Device {
			return -1
		}
		if one.Device > other.Device {
			return 1
		}
		return 0
	})
	return found
}

// driverOf is the name of the driver a device's driver link points to, or empty without one.
func driverOf(link string) string {
	target, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// noAccess is why the node's user may not open device for reading and writing, or empty when it
// may: most often that the user is not in the group that owns it (render, video).
func noAccess(device string) string {
	if err := unix.Access(device, unix.R_OK|unix.W_OK); err != nil {
		return err.Error()
	}
	return ""
}
