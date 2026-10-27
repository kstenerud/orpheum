// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

// A connection's path is told by the core's end of its pair: its own address is direct, one its
// NAT opened is punched, and a relay at either end is relayed; the family is the core's end's.
func TestPathOf_IsToldByTheCoresEndOfThePair(t *testing.T) {
	t.Parallel()
	candidate := func(kind webrtc.ICECandidateType, address string) *webrtc.ICECandidate {
		return &webrtc.ICECandidate{Typ: kind, Address: address}
	}
	host, srflx, prflx, relay := webrtc.ICECandidateTypeHost, webrtc.ICECandidateTypeSrflx,
		webrtc.ICECandidateTypePrflx, webrtc.ICECandidateTypeRelay
	for _, each := range []struct {
		local, remote *webrtc.ICECandidate
		want          Path
	}{
		{candidate(host, "2a0f:1000::20"), candidate(srflx, "2a01::1"), PathDirectIPv6},
		{candidate(host, "192.168.1.20"), candidate(prflx, "192.168.1.30"), PathDirectIPv4},
		{candidate(srflx, "2a0f:1000::20"), candidate(srflx, "2a01::1"), PathPunchedIPv6},
		{candidate(prflx, "203.0.113.9"), candidate(srflx, "198.51.100.4"), PathPunchedIPv4},
		// The core's public IPv4 address is the client's: out to the NAT both share and back (D354 §1).
		{candidate(srflx, "11.22.33.144"), candidate(srflx, "11.22.33.144"), PathHairpinnedIPv4},
		{candidate(prflx, "11.22.33.144"), candidate(prflx, "::ffff:11.22.33.144"), PathHairpinnedIPv4},
		{candidate(host, "192.168.1.20"), candidate(prflx, "192.168.1.20"), PathDirectIPv4},
		{candidate(srflx, "2a0f:1000::20"), candidate(srflx, "2a0f:1000::20"), PathPunchedIPv6},
		{candidate(host, "203.0.113.9"), candidate(relay, "198.51.100.4"), PathRelayed},
		{candidate(relay, "198.51.100.4"), candidate(host, "203.0.113.9"), PathRelayed},
		{nil, candidate(host, "203.0.113.9"), PathFailed},
	} {
		if got := pathOf(each.local, each.remote); got != each.want {
			t.Errorf("pathOf(%v, %v) = %s; want %s", each.local, each.remote, got, each.want)
		}
	}
}
