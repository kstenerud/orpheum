// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import "testing"

// A realm is a master realm of 5 to 32 lowercase letters, digits and inner hyphens, alone or with a
// dot and a subrealm of 1 to 32 of the same; no part holds a dot (D297 §1).
func TestIsRealm_AMasterOfFiveToThirtyTwoAndASubrealmAfterOneDot(t *testing.T) {
	t.Parallel()
	for realm, want := range map[string]bool{
		"karlnas":                           true,
		"karlnas.test":                      true,
		"karlnas.t":                         true,
		"the-smiths-2.living-room":          true,
		"abcdefghijklmnopqrstuvwxyz012345":  true,
		"abcdefghijklmnopqrstuvwxyz0123456": false,
		"nas":                               false,
		"nas.test":                          false,
		"karlnas.":                          false,
		"karlnas.a.b":                       false,
		"Karlnas":                           false,
		"-karlnas":                          false,
		"karlnas.-a":                        false,
		"karl nas":                          false,
		"kärlnas":                           false,
		"":                                  false,
	} {
		if got := IsRealm(realm); got != want {
			t.Errorf("IsRealm(%q) = %t; want %t", realm, got, want)
		}
	}
}
