// SPDX-License-Identifier: AGPL-3.0-or-later

package host

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseOSRelease_ReadsWhatLSBReleaseShowsFromOSRelease(t *testing.T) {
	t.Parallel()
	got := ParseOSRelease(`PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME='Debian GNU/Linux'
# a comment
VERSION_ID="13"
VERSION_CODENAME=trixie
ID=debian
`)
	want := System{
		Distribution: "Debian GNU/Linux", Description: "Debian GNU/Linux 13 (trixie)", Release: "13",
		Codename: "trixie",
	}
	if got != want {
		t.Errorf("Parse = %+v, want %+v", got, want)
	}
}

func TestDescribe_SaysThisMachinesPlatformAndKernel(t *testing.T) {
	t.Parallel()
	got := Describe()
	if got.Platform != runtime.GOOS+"/"+runtime.GOARCH || got.Kernel == "" {
		t.Errorf("Describe = %+v; want this machine's platform and kernel", got)
	}
}

// MPTCP is on where the kernel's switch says 1, and off where it says 0 or the kernel has none
// (D454 §4).
func TestIsMultipathTCPOn_IsTheKernelsSwitch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for setting, want := range map[string]bool{"1\n": true, "0\n": false} {
		path := filepath.Join(dir, "enabled")
		if err := os.WriteFile(path, []byte(setting), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := IsMultipathTCPOn(path); got != want {
			t.Errorf("the switch saying %q: on %v; want %v", setting, got, want)
		}
	}
	if IsMultipathTCPOn(filepath.Join(dir, "absent")) {
		t.Error("a kernel with no MPTCP switch: on; want off")
	}
}
