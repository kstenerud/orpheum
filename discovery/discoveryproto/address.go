// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import (
	"encoding/binary"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// notPublic are the ranges no client on the internet reaches, though Go's netip calls many of them
// global unicast: shared carrier NAT, DS-Lite's and the IETF's own block, documentation, benchmarking,
// 6to4, NAT64, Teredo and the discard prefix (the protocol's §2.4; C7, C9, G12). Private ranges,
// loopback, link-local and multicast are not global unicast, and IsPublic refuses them on that.
//
//nolint:gochecknoglobals // a table of constants, parsed once
var notPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier NAT (RFC 6598)
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments, DS-Lite's 192.0.0.0/29 among them
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation (RFC 5737)
	netip.MustParsePrefix("192.88.99.0/24"),  // 6to4 relays, deprecated (RFC 7526)
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking (RFC 2544)
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64's well-known prefix (RFC 6052)
	netip.MustParsePrefix("64:ff9b:1::/48"),  // NAT64 for local use (RFC 8215)
	netip.MustParsePrefix("100::/64"),        // discard (RFC 6666)
	netip.MustParsePrefix("2001::/23"),       // IETF protocol assignments, Teredo's 2001::/32 among them
	netip.MustParsePrefix("2001:db8::/32"),   // documentation (RFC 3849)
	netip.MustParsePrefix("2002::/16"),       // 6to4, deprecated
	netip.MustParsePrefix("3fff::/20"),       // documentation (RFC 9637)
	netip.MustParsePrefix("5f00::/16"),       // segment routing (RFC 9602)
}

// globalUnicastIPv6 is the only IPv6 space assigned for global unicast (RFC 4291 §2.4).
var globalUnicastIPv6 = netip.MustParsePrefix("2000::/3") //nolint:gochecknoglobals // a constant, parsed once

// IsPublic reports whether a client on the internet could reach address: a global unicast address
// in none of the ranges that are not. The one classing of addresses, for what a core publishes and
// what a probe may connect to alike (the protocol's §2.4, §2.6; G12). An IPv4 address written as
// IPv6 (::ffff:a.b.c.d) is classed as the IPv4 address it is.
func IsPublic(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.Zone() != "" {
		return false
	}
	if address.Is6() && !globalUnicastIPv6.Contains(address) {
		return false
	}
	for _, prefix := range notPublic {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// AddressLabel is an address as one label of a name, `<label>.<coreId>.<zone>` (the protocol's
// §2.7; C15): an IPv4 address dashed (192-168-1-20), an IPv6 address as its eight groups dashed,
// lowercase, without zero padding or compression (2a0f-5000-0-0-0-0-0-1), so one address has one
// label and no label holds "--". It is false for an address no label is made for: one with a zone,
// or none at all.
func AddressLabel(address netip.Addr) (string, bool) {
	address = address.Unmap()
	if !address.IsValid() || address.Zone() != "" {
		return "", false
	}
	if address.Is4() {
		return strings.ReplaceAll(address.String(), ".", "-"), true
	}
	bytes := address.As16()
	groups := make([]string, 0, len(bytes)/2)
	for group := range slices.Chunk(bytes[:], 2) {
		groups = append(groups, strconv.FormatUint(uint64(binary.BigEndian.Uint16(group)), 16))
	}
	return strings.Join(groups, "-"), true
}
