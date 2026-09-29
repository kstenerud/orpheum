// SPDX-License-Identifier: GPL-3.0-or-later

package host

import (
	"runtime"
	"testing"
)

func TestParse_ReadsWhatLSBReleaseShowsFromOSRelease(t *testing.T) {
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
