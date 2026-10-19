// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import (
	"encoding/json"
	"strings"
	"testing"
)

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

// A realm sign-in's answer says when its core was claimed under "claimedAt", the key the SDK's
// realm.ts reads to date the warning that the realm's server changed, and says nothing when the
// discovery does not know, which the page reads as no date (D312 §2).
func TestClientRealmResponse_SaysWhenItsCoreWasClaimedUnderTheKeyThePageReads(t *testing.T) {
	t.Parallel()
	dated, err := json.Marshal(ClientRealmResponse{ClaimedAt: 1760000000000})
	if err != nil || !strings.Contains(string(dated), `"claimedAt":1760000000000`) {
		t.Errorf("a claimed core's answer = %s, %v; want claimedAt", dated, err)
	}
	undated, err := json.Marshal(ClientRealmResponse{})
	if err != nil || strings.Contains(string(undated), "claimedAt") {
		t.Errorf("an undated answer = %s, %v; want no claimedAt", undated, err)
	}
}
