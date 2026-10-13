// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto_test

import (
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

func TestParseVersion_ReadsMajorDotMinorOnly(t *testing.T) {
	t.Parallel()
	major, minor, isVersion := discoveryproto.ParseVersion(discoveryproto.ProtocolVersion)
	if !isVersion || major != discoveryproto.ProtocolMajor || minor != discoveryproto.ProtocolMinor {
		t.Errorf("ProtocolVersion %q reads as %d.%d (%t); want ProtocolMajor.ProtocolMinor",
			discoveryproto.ProtocolVersion, major, minor, isVersion)
	}
	for _, header := range []string{
		"", "1", "1.", ".1", "1.0.0", "v1.0", "+1.0", "-1.0", "01.0", "1.00", " 1.0", "a.b",
	} {
		if _, _, isVersion := discoveryproto.ParseVersion(header); isVersion {
			t.Errorf("ParseVersion(%q) is a version; want refused", header)
		}
	}
}

func TestServes_AnyMinorOfAServedMajorAndNothingElse(t *testing.T) {
	t.Parallel()
	served := []int{1}
	for header, want := range map[string]bool{
		"1.0": true, "1.7": true, "2.0": false, "0.9": false, "1": false, "": false,
	} {
		if got := discoveryproto.Serves(served, header); got != want {
			t.Errorf("Serves(%v, %q) = %t, want %t", served, header, got, want)
		}
	}
}
