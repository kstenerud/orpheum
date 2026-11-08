// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"net/netip"
	"slices"
	"testing"

	"github.com/pion/webrtc/v4"
)

// The core's own IPv6 networks on a connection are the /64s of its public IPv6 candidates, its host
// ones and those STUN found: never a private, link-local or IPv4 address, a relay's, or the client's.
func TestOwnIPv6Networks_AreThePublicIPv6CandidatesOfTheCoresOwnEnd(t *testing.T) {
	t.Parallel()
	local := func(id string, kind webrtc.ICECandidateType, ip string) webrtc.ICECandidateStats {
		return webrtc.ICECandidateStats{ID: id, Type: webrtc.StatsTypeLocalCandidate, CandidateType: kind, IP: ip}
	}
	report := webrtc.StatsReport{
		// the production NAS's, 2026-10-19: Docker's own addresses, and the public one STUN found.
		"a": local("a", webrtc.ICECandidateTypeHost, "172.18.0.2"),
		"b": local("b", webrtc.ICECandidateTypeHost, "fd77:5e::2"),
		"c": local("c", webrtc.ICECandidateTypeHost, "fe80::b465:19ff:fe0e:47f1"),
		"d": local("d", webrtc.ICECandidateTypeSrflx, "2a0f:1000:4753:8afc:0211:22ff:fe33:4455"),
		"e": local("e", webrtc.ICECandidateTypeSrflx, "2a0f:1000:4753:8afc:1::9"),
		// A machine with its own public address, not translated.
		"f": local("f", webrtc.ICECandidateTypeHost, "2a0f:5000::20"),
		"g": local("g", webrtc.ICECandidateTypeRelay, "2a0f:2000::3"),
		"h": local("h", webrtc.ICECandidateTypeSrflx, "11.22.33.144"),
		"i": webrtc.ICECandidateStats{
			ID: "i", Type: webrtc.StatsTypeRemoteCandidate, CandidateType: webrtc.ICECandidateTypeSrflx,
			IP: "2a0f:1000:9999::1",
		},
	}
	want := []netip.Prefix{netip.MustParsePrefix("2a0f:1000:4753:8afc::/64"), netip.MustParsePrefix("2a0f:5000::/64")}
	if got := ownIPv6Networks(report); !slices.Equal(got, want) {
		t.Errorf("ownIPv6Networks = %v; want %v", got, want)
	}
}
