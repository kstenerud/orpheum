// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import (
	"encoding/json"
	"reflect"
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

// A dead end's reason round-trips whole (D398 §2).
func TestDeadEndRequest_TheReasonRoundTrips(t *testing.T) {
	t.Parallel()
	sent := DeadEndRequest{
		PathsRequest: PathsRequest{Locator: "a-locator"},
		IPv6:         HasIPv6No, Cause: DeadEndCauseNoIPv6,
		Tried: []DeadEndTry{{Kind: PathKindIPv6, Ended: "unreachable"}, {Kind: PathKindWebRTC, Ended: "failed"}},
	}
	encoded, err := json.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"locator":"a-locator","ipv6":"no",` +
		`"tried":[{"kind":"ipv6","ended":"unreachable"},{"kind":"webrtc","ended":"failed"}],"cause":"noIpv6"}`
	if string(encoded) != want {
		t.Errorf("the reason encodes as %s; want %s", encoded, want)
	}
	var read DeadEndRequest
	if err = json.Unmarshal(encoded, &read); err != nil || !reflect.DeepEqual(read, sent) {
		t.Errorf("the reason reads back as %+v (%v); want %+v", read, err, sent)
	}
}

// A body from before D398, the locator alone, reads as a dead end with no reason, and one with no
// reason is sent as the locator alone, as before (D398 §2).
func TestDeadEndRequest_TheReasonIsOptional(t *testing.T) {
	t.Parallel()
	var old DeadEndRequest
	want := DeadEndRequest{PathsRequest: PathsRequest{Locator: "a-locator"}}
	if err := json.Unmarshal([]byte(`{"locator":"a-locator"}`), &old); err != nil || !reflect.DeepEqual(old, want) {
		t.Errorf("an old body reads as %+v (%v); want the locator and no reason", old, err)
	}
	bare, err := json.Marshal(want)
	if err != nil || string(bare) != `{"locator":"a-locator"}` {
		t.Errorf("a dead end with no reason encodes as %s (%v); want the locator alone", bare, err)
	}
}

// The heartbeat's answer counts its dead ends by cause under "deadEndCausesLastWeek", the key Connect
// sends, and a discovery that keeps no causes sends none, which the System page reads as the count
// alone (D398 §6).
func TestAddressesResponse_CountsDeadEndsByCauseUnderTheKeyConnectSends(t *testing.T) {
	t.Parallel()
	var read AddressesResponse
	err := json.Unmarshal([]byte(`{"deadEndsLastWeek":3,"deadEndCausesLastWeek":{"noIpv6":2,"none":1}}`), &read)
	if err != nil || !reflect.DeepEqual(read.DeadEndCausesLastWeek, map[string]int{"noIpv6": 2, "none": 1}) {
		t.Errorf("the causes read as %v (%v); want noIpv6 2 and none 1", read.DeadEndCausesLastWeek, err)
	}
	bare, err := json.Marshal(AddressesResponse{DeadEndsLastWeek: 3})
	if err != nil || strings.Contains(string(bare), "deadEndCausesLastWeek") {
		t.Errorf("an answer with no causes = %s (%v); want no causes", bare, err)
	}
}
