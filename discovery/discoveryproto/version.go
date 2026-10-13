// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import (
	"slices"
	"strconv"
	"strings"
)

// ParseVersion reads ProtocolHeader's "major.minor", and says whether it is one: a bare major, a
// sign, a space or a third part is not.
func ParseVersion(header string) (major, minor int, isVersion bool) {
	majorText, minorText, hasDot := strings.Cut(header, ".")
	major, majorErr := strconv.Atoi(majorText)
	minor, minorErr := strconv.Atoi(minorText)
	if !hasDot || majorErr != nil || minorErr != nil || major < 0 || minor < 0 ||
		strconv.Itoa(major) != majorText || strconv.Itoa(minor) != minorText {
		return 0, 0, false
	}
	return major, minor, true
}

// Serves reports whether a discovery serving majors answers a call that says header: any minor of a
// major it serves (the protocol's §5).
func Serves(majors []int, header string) bool {
	major, _, isVersion := ParseVersion(header)
	if !isVersion {
		return false
	}
	return slices.Contains(majors, major)
}
