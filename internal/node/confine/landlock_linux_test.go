// SPDX-License-Identifier: GPL-3.0-or-later

package confine

import (
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
	if has(plain, "/dev/dri") || has(plain, "/sys/devices") {
		t.Error("a run that does not use the GPU is granted its devices")
	}
	if !has(gpu, "/dev/dri") || !has(gpu, "/sys/class/drm") || !has(gpu, "/dev/nvidiactl") {
		t.Error("a hardware encode is not granted the GPU's devices and the files that describe them")
	}
}
