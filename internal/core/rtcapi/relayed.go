// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// MaxRelayedCandidates is the most remote candidates an offer a discovery passed on may list: a
// browser offers a few, and each is a pair the core's ICE checks (the protocol's §2.5; D291 §6,
// S16; the security audit's A2).
const MaxRelayedCandidates = 10

// ErrOfferRefused is an offer a discovery passed on that the core's own check refused, its reason
// wrapped with it.
var ErrOfferRefused = errors.New("the core refused the offer")

// sharedAddressSpace is carrier-grade NAT's range (RFC 6598), 100.64.0.0/10: never a client
// reachable from outside.
func sharedAddressSpace() netip.Prefix {
	return netip.PrefixFrom(netip.AddrFrom4([4]byte{100, 64, 0, 0}), 10)
}

// AnswerRelayed answers an offer a discovery passed on, which the core checks itself first: one
// sha-256 fingerprint, no more than MaxRelayedCandidates candidates, and no candidate that names a
// network the core's own LAN path serves. Its connection checks no remote address but one reachable
// from outside, so an offer cannot make the core send its ICE checks into its own network (the
// protocol's §2.5; the security audit's A2). Once it connects, onProvenAtHome, unless nil, is told
// when it proved itself from home, which a discovery's word cannot make it (D300 §2, D356 §1).
func (server *Server) AnswerRelayed(
	ctx context.Context, offer Description, onProvenAtHome func(ProvenAtHome),
) (Description, error) {
	if offer.Type != "offer" {
		return Description{}, fmt.Errorf("%w: want an offer, not %q", ErrOfferRefused, offer.Type)
	}
	checked, err := checkRelayedOffer(offer.SDP, server.takesNamedRelayed)
	if err != nil {
		return Description{}, fmt.Errorf("%w: %w", ErrOfferRefused, err)
	}
	return server.answerWith(ctx, server.relayedAPI, checked, onProvenAtHome)
}

// OfferedIPv6Addresses are the public IPv6 addresses an offer's SDP names among its candidates: a
// device's own, as its browser found them, which may be absent, hidden behind an mDNS name or left
// out when it equals the host's (house-ipv6-2026-10-27.md §3 (d)). What the core compares with the
// network the discovery saw the offer from, for its log alone (D356, Q5).
func OfferedIPv6Addresses(sdp string) []netip.Addr {
	var addresses []netip.Addr
	for line := range strings.SplitSeq(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "a=candidate:") {
			continue
		}
		address, err := netip.ParseAddr(fields[4])
		if err != nil {
			continue
		}
		if address = address.Unmap().WithZone(""); address.Is6() && address.IsGlobalUnicast() &&
			!address.IsPrivate() && !slices.Contains(addresses, address) {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

// checkRelayedOffer is an offer's SDP as the core takes it from a discovery, or why it is refused.
// Its host candidates named by mDNS (.local) are dropped: they resolve only on the client's own LAN,
// and resolving them would send queries into the core's. So is every candidate whose address
// isTaken refuses: one in the core's own networks, which the connection may answer a check from
// but never send one to first (D300 §2; the security audit's A2).
func checkRelayedOffer(sdp string, isTaken func(net.IP) bool) (string, error) {
	lines := strings.Split(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	fingerprints, candidates := 0, 0
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "a=fingerprint:"):
			fingerprints++
			algorithm, _, _ := strings.Cut(strings.TrimPrefix(line, "a=fingerprint:"), " ")
			if !strings.EqualFold(algorithm, "sha-256") {
				return "", fmt.Errorf("a fingerprint by %q; only sha-256 is taken", algorithm)
			}
		case strings.HasPrefix(line, "a=candidate:"):
			if fields := strings.Fields(line); len(fields) > 4 &&
				(strings.HasSuffix(fields[4], ".local") || !isTaken(net.ParseIP(fields[4]))) {
				continue
			}
			candidates++
		}
		kept = append(kept, line)
	}
	switch {
	case fingerprints != 1:
		// pion checks the certificate against one fingerprint, a session-level line winning over a
		// media-level one: a second would let a discovery end DTLS in the middle (A9).
		return "", fmt.Errorf("%d fingerprints; exactly one is taken", fingerprints)
	case candidates > MaxRelayedCandidates:
		return "", fmt.Errorf("%d candidates; at most %d are taken", candidates, MaxRelayedCandidates)
	}
	return strings.Join(kept, "\r\n"), nil
}

// FingerprintOf is an SDP's a=fingerprint value ("sha-256 AB:..."), the first it holds, or empty:
// what an answer's signature names the offer by (the protocol's §2.5 item 4).
func FingerprintOf(sdp string) string {
	for line := range strings.SplitSeq(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n") {
		if value, isFingerprint := strings.CutPrefix(line, "a=fingerprint:"); isFingerprint {
			return value
		}
	}
	return ""
}

// isFromOutside is whether a remote candidate's address is one a client outside the core's network
// has: not loopback, link-local, private, carrier-grade NAT's, multicast or unspecified, and in
// none of the core's own prefixes, which ownPrefixes names.
func isFromOutside(ip net.IP, ownPrefixes func() []netip.Prefix) bool {
	address, isAddress := netip.AddrFromSlice(ip)
	if !isAddress {
		return false
	}
	address = address.Unmap()
	if address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsPrivate() || address.IsMulticast() || address.IsUnspecified() ||
		sharedAddressSpace().Contains(address) {
		return false
	}
	for _, prefix := range ownPrefixes() {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// interfacePrefixes are the networks the core's interfaces are on: a client in one of them is on
// the core's LAN, which its own path serves.
func interfacePrefixes() []netip.Prefix {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	prefixes := make([]netip.Prefix, 0, len(addresses))
	for _, each := range addresses {
		network, isNetwork := each.(*net.IPNet)
		if !isNetwork {
			continue
		}
		address, isAddress := netip.AddrFromSlice(network.IP)
		ones, bits := network.Mask.Size()
		if address = address.Unmap(); address.Is4() && bits == 128 {
			ones -= 96
		}
		if isAddress {
			prefixes = append(prefixes, netip.PrefixFrom(address, ones).Masked())
		}
	}
	return prefixes
}
