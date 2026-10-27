// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
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
	own := []netip.Prefix{netip.MustParsePrefix("2a0f:1000:1:2::/64")}
	overIPv4 := told(server, pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
		webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"), nil)
	if len(overIPv4) != 1 || overIPv4[0].Path != PathDirectIPv4 || overIPv4[0].Device.String() != "192.168.1.30" {
		t.Errorf("a direct pair from 192.168.1.30, in the home network, told %+v; want it proven from there",
			overIPv4)
	}
	overIPv6 := told(server, pairOf(webrtc.ICECandidateTypeHost, "[2a0f:1000:1:2::5]:8443",
		webrtc.ICECandidateTypeHost, "[2a0f:1000:1:2::30]:5000"), own)
	if len(overIPv6) != 1 || overIPv6[0].Path != PathDirectIPv6 || overIPv6[0].Device.String() != "2a0f:1000:1:2::30" {
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
	own := []netip.Prefix{netip.MustParsePrefix("2a0f:1000:1:2::/64")}
	for name, pair := range map[string]*webrtc.ICECandidatePair{
		"relayed": pairOf(webrtc.ICECandidateTypeRelay, "198.51.100.4:3478",
			webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"),
		"relayed by the client": pairOf(webrtc.ICECandidateTypeHost, "192.168.1.199:8443",
			webrtc.ICECandidateTypeRelay, "192.168.1.30:5000"),
		"punched": pairOf(webrtc.ICECandidateTypeSrflx, "11.22.33.144:40001",
			webrtc.ICECandidateTypePrflx, "192.168.1.30:5000"),
		"punched over IPv6": pairOf(webrtc.ICECandidateTypeSrflx, "[2a0f:1000:1:2::5]:40001",
			webrtc.ICECandidateTypePrflx, "[2a0f:1000:1:2::30]:5000"),
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
