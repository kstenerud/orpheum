// SPDX-License-Identifier: AGPL-3.0-or-later

package hardware

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func TestGPUs_ReportsEachDeviceItsDriverAndWhyItCannotBeOpened(t *testing.T) {
	t.Parallel()
	machine := t.TempDir()
	devices, drm := filepath.Join(machine, "dev"), filepath.Join(machine, "sys", "class", "drm")
	for _, directory := range []string{
		filepath.Join(devices, "dri"), filepath.Join(drm, "renderD128", "device"),
		filepath.Join(machine, "sys", "bus", "pci", "drivers", "i915"),
	} {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	driver := filepath.Join(machine, "sys", "bus", "pci", "drivers", "i915")
	if err := os.Symlink(driver, filepath.Join(drm, "renderD128", "device", "driver")); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{
		filepath.Join(devices, "dri", "renderD128"): 0o600, // the node's user may open it
		filepath.Join(devices, "dri", "card0"):      0o600, // not a render node: never reported
		filepath.Join(devices, "nvidia0"):           0o000, // not in the owning group
		filepath.Join(devices, "nvidiactl"):         0o600, // the control device, not a GPU
	} {
		if err := os.WriteFile(path, nil, mode); err != nil {
			t.Fatal(err)
		}
	}
	found := gpusUnder(devices, drm)
	if len(found) != 2 {
		t.Fatalf("found %+v; want the render node and nvidia0", found)
	}
	if want := (nodeproto.GPU{
		DevicePath: filepath.Join(devices, "dri", "renderD128"),
		Driver:     "i915",
	}); found[0] != want {
		t.Errorf("the render node is %+v, want %+v", found[0], want)
	}
	if nvidia := found[1]; nvidia.Driver != "nvidia" || (os.Geteuid() != 0 && nvidia.NoAccessReason == "") {
		t.Errorf("nvidia0 is %+v; want the nvidia driver, and why the node cannot open it", nvidia)
	}
}
