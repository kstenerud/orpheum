// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLandlockState_SaysWhyItIsMissingAndWhatWouldPutItThere(t *testing.T) {
	t.Parallel()
	for _, want := range []struct {
		errno  error
		cause  string
		remedy string
	}{
		{unix.EOPNOTSUPP, CauseDisabledAtBoot, "lsm="},
		{unix.ENOSYS, CauseKernelWithout, "5.13"},
		{unix.EPERM, CauseBlockedByRuntime, "container runtime"},
		{unix.EIO, CauseUnexplained, "report it"},
	} {
		got := landlockStateOf(want.errno)
		if got.IsInForce || got.CauseCode != want.cause || !strings.Contains(got.Remedy, want.remedy) ||
			got.Reason == "" {
			t.Errorf("Landlock answering %v: %+v, want %s with a remedy naming %q (D112)",
				want.errno, got, want.cause, want.remedy)
		}
	}
	if got := landlockStateOf(nil); !got.IsInForce || got.CauseCode != "" {
		t.Errorf("Landlock answering an ABI: %+v, want in force", got)
	}
}

// A hardware encode is granted every NVIDIA GPU that detection reports, not only the first two.
func TestGPUDevices_AreGrantedEveryNVIDIAGPUDetectionFinds(t *testing.T) {
	t.Parallel()
	devices := t.TempDir()
	for _, name := range []string{"nvidia0", "nvidia1", "nvidia2", "nvidia10", "nvidiactl", "nvidia-uvm"} {
		if err := os.WriteFile(filepath.Join(devices, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	detected := NVIDIADevices(devices)
	want := []string{
		filepath.Join(devices, "nvidia0"), filepath.Join(devices, "nvidia1"), filepath.Join(devices, "nvidia10"),
		filepath.Join(devices, "nvidia2"),
	}
	if !slices.Equal(detected, want) {
		t.Errorf("detected %v, want each nvidiaN and no control device", detected)
	}
	granted := gpuDevicesUnder(devices)
	for _, device := range detected {
		if !slices.Contains(granted, device) {
			t.Errorf("%s is detected but not granted; granted %v", device, granted)
		}
	}
}

func TestGrantsFor_OnlyARunThatUsesTheGPUIsGrantedItsDevices(t *testing.T) {
	t.Parallel()
	has := func(spec Spec, path string) bool {
		for _, rule := range grantsFor(spec) {
			if rule.path == path {
				return true
			}
		}
		return false
	}
	plain := Spec{Tool: "/opt/ffmpeg/ffmpeg"}
	gpu := Spec{Tool: "/opt/ffmpeg/ffmpeg", Access: Access{UsesGPU: true}}
	if has(plain, "/dev/dri") || has(plain, "/sys/devices") || has(plain, "/proc/devices") ||
		has(plain, "/proc/self/task") {
		t.Error("a run that does not use the GPU is granted its devices, or what its driver reads")
	}
	if !has(gpu, "/dev/dri") || !has(gpu, "/sys/class/drm") || !has(gpu, "/dev/nvidiactl") {
		t.Error("a hardware encode is not granted the GPU's devices and the files that describe them")
	}
	// CUDA reads these as it starts, and names its threads; without /proc/devices it starts
	// nvidia-modprobe, which no run may (D331 §3).
	for _, path := range []string{
		"/proc/devices", "/proc/cpuinfo", "/proc/sys/vm/mmap_min_addr",
		"/proc/self/maps", "/proc/self/status", "/proc/self/task",
	} {
		if !has(gpu, path) {
			t.Errorf("a hardware encode is not granted %s, which its driver needs to start", path)
		}
	}
}
