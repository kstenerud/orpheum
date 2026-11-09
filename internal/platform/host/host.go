// SPDX-License-Identifier: AGPL-3.0-or-later

package host

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// System is a machine's operating system: what `lsb_release -a` shows, the kernel and the
// platform. A field the machine does not say is empty.
type System struct {
	// Distribution is lsb_release's "Distributor": "Debian GNU/Linux".
	Distribution string `json:"distribution,omitempty"`
	// Description is its "Description": "Debian GNU/Linux 13 (trixie)".
	Description string `json:"description,omitempty"`
	// Release is its "Release": "13".
	Release string `json:"release,omitempty"`
	// Codename is its "Codename": "trixie".
	Codename string `json:"codename,omitempty"`
	// Kernel is the kernel's release, as uname -r says it.
	Kernel string `json:"kernel,omitempty"`
	// Platform is the operating system and architecture, as Go names them: "linux/amd64".
	Platform string `json:"platform"`
}

// Describe is this machine. On a system without os-release, only the kernel and platform are
// known.
func Describe() System {
	info := System{}
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if contents, err := os.ReadFile(path); err == nil { //nolint:gosec // a fixed system path
			info = ParseOSRelease(string(contents))
			break
		}
	}
	var name unix.Utsname
	if err := unix.Uname(&name); err == nil {
		info.Kernel = unix.ByteSliceToString(name.Release[:])
	}
	info.Platform = runtime.GOOS + "/" + runtime.GOARCH
	return info
}

// ParseOSRelease reads os-release's lines of KEY=value, the value quoted or not.
func ParseOSRelease(osRelease string) System {
	values := map[string]string{}
	lines := bufio.NewScanner(strings.NewReader(osRelease))
	for lines.Scan() {
		key, value, found := strings.Cut(strings.TrimSpace(lines.Text()), "=")
		if !found || strings.HasPrefix(key, "#") {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else {
			value = strings.Trim(value, `'"`)
		}
		values[key] = value
	}
	return System{
		Distribution: values["NAME"], Description: values["PRETTY_NAME"], Release: values["VERSION_ID"],
		Codename: values["VERSION_CODENAME"],
	}
}

// MultipathTCPSetting is the kernel's MPTCP switch: 1 where MPTCP is on. A core on the host's network
// reads the host's own (D454 §4, §5).
const MultipathTCPSetting = "/proc/sys/net/mptcp/enabled"

// IsMultipathTCPOn is whether the kernel switch at path says MPTCP is on; false where the kernel has
// no MPTCP, or the file cannot be read. Our programs open no MPTCP socket, so it only says the host's
// other programs may (D454 §4).
func IsMultipathTCPOn(path string) bool {
	setting, err := os.ReadFile(path) //nolint:gosec // G304: the kernel's switch, or a test's
	return err == nil && strings.TrimSpace(string(setting)) == "1"
}
