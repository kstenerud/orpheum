// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// told is what onProvenAtHome was told of each connection.
func told(server *Server, pair *webrtc.ICECandidatePair, own []netip.Prefix) []ProvenAtHome {
	var proven []ProvenAtHome
	server.tellIfProvenAtHome(pair, own, func(each ProvenAtHome) { proven = append(proven, each) })
	return proven
}

// A connection whose pair reaches an address of the core's own from a device in the house, in a home
// network or in the core's own public IPv6 network, proves itself from home, and says from which
// device (D300 §3, D356 §1).
func TestProvenAtHome_ADirectConnectionFromTheHouseProvesItself(t *testing.T) {
	t.Parallel()
	server := watchedServer(t, &testutil.Log{}, clocktest.NewFake(time.Unix(1_800_000_000, 0)))
	own := []netip.Prefix{netip.MustParsePrefix("2a0f:5000:1:2::/64")}
	overIPv4 := told(server, pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
		webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"), nil)
	if len(overIPv4) != 1 || overIPv4[0].Path != PathDirectIPv4 || overIPv4[0].Device.String() != "192.168.1.30" {
		t.Errorf("a direct pair from 192.168.1.30, in the home network, told %+v; want it proven from there",
			overIPv4)
	}
	overIPv6 := told(server, pairOf(webrtc.ICECandidateTypeHost, "[2a0f:5000:1:2::5]:8443",
		webrtc.ICECandidateTypeHost, "[2a0f:5000:1:2::30]:5000"), own)
	if len(overIPv6) != 1 || overIPv6[0].Path != PathDirectIPv6 || overIPv6[0].Device.String() != "2a0f:5000:1:2::30" {
		t.Errorf("a direct pair from the core's own IPv6 network told %+v; want it proven from there", overIPv6)
	}
	server.tellIfProvenAtHome(pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
		webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"), nil, nil) // telling no one does nothing
}

// A relayed, punched or hairpinned connection, a direct one from away, and none at all prove nothing:
// only a device that reached the core across the house's network teaches (D354 §4, D356 §1, §8).
func TestProvenAtHome_ARelayedPunchedOrHairpinnedConnectionProvesNothing(t *testing.T) {
	t.Parallel()
	server := watchedServer(t, &testutil.Log{}, clocktest.NewFake(time.Unix(1_800_000_000, 0)))
	own := []netip.Prefix{netip.MustParsePrefix("2a0f:5000:1:2::/64")}
	for name, pair := range map[string]*webrtc.ICECandidatePair{
		"relayed": pairOf(webrtc.ICECandidateTypeRelay, "198.51.100.4:3478",
			webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"),
		"relayed by the client": pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
			webrtc.ICECandidateTypeRelay, "192.168.1.30:5000"),
		"punched": pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
			webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"),
		"punched over IPv6": pairOf(webrtc.ICECandidateTypeSrflx, "[2a0f:5000:1:2::5]:40001",
			webrtc.ICECandidateTypePrflx, "[2a0f:5000:1:2::30]:5000"),
		"hairpinned": pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
			webrtc.ICECandidateTypeSrflx, "11.22.33.144:51234"),
		"direct from away": pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
			webrtc.ICECandidateTypeSrflx, "198.51.100.4:5000"),
		"direct from another private network": pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
			webrtc.ICECandidateTypePrflx, "10.0.0.7:5000"),
		"none": nil,
	} {
		if proven := told(server, pair, own); len(proven) != 0 {
			t.Errorf("a %s connection told %+v; want nothing", name, proven)
		}
	}
}

// A request over WebRTC is from home only over a direct pair from a home network or the core's own
// IPv6 network: a punched pair is not, even from a home address or the house's /64, nor is a relayed
// or hairpinned one, and none at all is not (D439 §1). The home networks are read as each request
// finds them, so an admin's change counts from the next (D439 §2).
func TestIsDirectlyFromHome_OnlyADirectPairFromTheHouseIs(t *testing.T) {
	t.Parallel()
	server := watchedServer(t, &testutil.Log{}, clocktest.NewFake(time.Unix(1_800_000_000, 0)))
	own := []netip.Prefix{netip.MustParsePrefix("2a0f:5000:1:2::/64")}
	direct := pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
		webrtc.ICECandidateTypePrflx, "192.168.1.30:5000")
	for name, each := range map[string]struct {
		pair *webrtc.ICECandidatePair
		want bool
	}{
		"direct from a home network": {direct, true},
		"direct from the house's IPv6 network": {pairOf(webrtc.ICECandidateTypeHost, "[2a0f:5000:1:2::5]:8443",
			webrtc.ICECandidateTypeHost, "[2a0f:5000:1:2::30]:5000"), true},
		"punched from a home address": {pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
			webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"), false},
		"punched from the house's IPv6 network": {pairOf(webrtc.ICECandidateTypeSrflx, "[2a0f:5000:1:2::5]:40001",
			webrtc.ICECandidateTypePrflx, "[2a0f:5000:1:2::30]:5000"), false},
		"relayed from a home address": {pairOf(webrtc.ICECandidateTypeRelay, "198.51.100.4:3478",
			webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"), false},
		"hairpinned": {pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
			webrtc.ICECandidateTypeSrflx, "11.22.33.144:51234"), false},
		"none": {nil, false},
	} {
		if got := server.isDirectlyFromHome(each.pair, own); got != each.want {
			t.Errorf("a request over a pair %s is from home: %t; want %t", name, got, each.want)
		}
	}
	server.SetHomeNetworks([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	if server.isDirectlyFromHome(direct, own) {
		t.Error("after the home networks changed, a direct pair from the old one is from home; want away")
	}
	if IsDirectlyFromHome(t.Context()) {
		t.Error("a request not over WebRTC is directly from home; want not")
	}
	if !IsDirectlyFromHome(context.WithValue(t.Context(), fromHomeKey{}, true)) {
		t.Error("a request marked from home is not; want it")
	}
}
