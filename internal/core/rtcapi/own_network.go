// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"net/netip"
	"slices"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
)

// ownIPv6Bits is how much of an IPv6 address names the network it is on: one link's /64, which a
// provider gives one household alone.
const ownIPv6Bits = 64

// ownIPv6Networks are the /64s of the core's own public IPv6 addresses among a connection's local
// candidates: its host candidates, and those STUN found, the address Docker's translation gives it
// outside, through the core's own STUN servers alone (cmd/orpheum's iceServersFor; the Plex-chain
// review's L3). The house's IPv6 network, which the house's IPv4 network alone named in
// server.home_networks leaves out, and which a provider's new prefix moves; read from each
// connection, it is always the current one (nas.example, 2026-10-19: the owner's Mac reached the core
// over IPv6 from the same /64 and was refused admin).
func ownIPv6Networks(report webrtc.StatsReport) []netip.Prefix {
	var networks []netip.Prefix
	for _, stats := range report {
		candidate, isCandidate := stats.(webrtc.ICECandidateStats)
		if !isCandidate || !isOwnCandidate(candidate) {
			continue
		}
		address, err := netip.ParseAddr(candidate.IP)
		if err != nil {
			continue
		}
		// A public address: never a private (fc00::/7), link-local or IPv4 one.
		address = address.Unmap().WithZone("")
		if !address.Is6() || !address.IsGlobalUnicast() || address.IsPrivate() {
			continue
		}
		network := netip.PrefixFrom(address, ownIPv6Bits).Masked()
		if !slices.Contains(networks, network) {
			networks = append(networks, network)
		}
	}
	slices.SortFunc(networks, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
	return networks
}

// isOwnCandidate is whether candidate is the core's own address: a local host candidate, or one STUN
// found, never a relay's.
func isOwnCandidate(candidate webrtc.ICECandidateStats) bool {
	isOwnType := candidate.CandidateType == webrtc.ICECandidateTypeHost ||
		candidate.CandidateType == webrtc.ICECandidateTypeSrflx
	return candidate.Type == webrtc.StatsTypeLocalCandidate && isOwnType
}

// heldNetworks are a connection's own IPv6 networks, kept once it connects, when its candidates are
// all gathered, and read by each request it carries after.
type heldNetworks struct {
	networks atomic.Pointer[[]netip.Prefix]
}

// keep keeps the own IPv6 networks a connection's stats name.
func (held *heldNetworks) keep(report webrtc.StatsReport) {
	networks := ownIPv6Networks(report)
	held.networks.Store(&networks)
}

// held are the networks kept, none before the connection connected.
func (held *heldNetworks) held() []netip.Prefix {
	if networks := held.networks.Load(); networks != nil {
		return *networks
	}
	return nil
}
