// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto_test

import (
	"net/netip"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// learnt is a learnt house network.
func learnt(network string) discoveryproto.HouseNetwork {
	return discoveryproto.HouseNetwork{Network: network, Source: discoveryproto.HouseNetworkLearnt}
}

// A device asking from inside one of the heartbeat's house networks, a public IPv6 /64, is at home;
// one outside them, or asking over IPv4, is not (D356 §5).
func TestInHouseNetworks_ADeviceInAPublicSlash64IsAtHome(t *testing.T) {
	t.Parallel()
	networks := []discoveryproto.HouseNetwork{
		learnt("2a0f:1000:1:2::/64"),
		{Network: "2a0f:3000:9:8::/64", Source: discoveryproto.HouseNetworkConfigured},
	}
	for client, want := range map[string]bool{
		"2a0f:1000:1:2:aaaa:bbbb:cccc:dddd": true, "2a0f:3000:9:8::1": true,
		"2a0f:1000:1:3::1": false, "2a0f:1000:1:2::1%eth0": true, "11.22.33.144": false,
		"::ffff:11.22.33.144": false,
	} {
		if got := discoveryproto.IsInHouseNetworks(netip.MustParseAddr(client), networks); got != want {
			t.Errorf("IsInHouseNetworks(%s) = %t; want %t", client, got, want)
		}
	}
}

// A shorter or longer prefix, a private, documentation or IPv4 network, one written unmasked, and a
// fifth network count for nothing: a heartbeat cannot name a wide range, or many, as one household's
// (D356 §5).
func TestInHouseNetworks_AShorterPrivateOrIPv4NetworkCountsForNothing(t *testing.T) {
	t.Parallel()
	for network, client := range map[string]string{
		"2a0f:1000::/32":         "2a0f:1000:1:2::1",
		"2a0f:1000:1:2::/96":     "2a0f:1000:1:2::1",
		"fd77:1:2:3::/64":        "fd77:1:2:3::1",
		"2001:db8:1:2::/64":      "2001:db8:1:2::1",
		"11.22.33.0/24":         "11.22.33.144",
		"2a0f:1000:1:2::9/64":    "2a0f:1000:1:2::1",
		"not a network":          "2a0f:1000:1:2::1",
		"::ffff:11.22.33.0/120": "::ffff:11.22.33.144",
	} {
		if discoveryproto.IsInHouseNetworks(netip.MustParseAddr(client),
			[]discoveryproto.HouseNetwork{learnt(network)}) {
			t.Errorf("a device at %s is at home by the network %q", client, network)
		}
	}
	fifth := []discoveryproto.HouseNetwork{
		learnt("2a0f:1000:1:1::/64"), learnt("2a0f:1000:1:2::/64"), learnt("2a0f:1000:1:3::/64"),
		learnt("2a0f:1000:1:4::/64"), learnt("2a0f:1000:1:5::/64"),
	}
	if discoveryproto.IsInHouseNetworks(netip.MustParseAddr("2a0f:1000:1:5::1"), fifth) {
		t.Error("a device in the fifth house network is at home; want only the first four counted")
	}
	if !discoveryproto.IsInHouseNetworks(netip.MustParseAddr("2a0f:1000:1:4::1"), fifth) {
		t.Error("a device in the fourth house network is not at home")
	}
}

// The network a discovery passes on with an offer is its source's /64 when that is a public IPv6
// address, and none for an IPv4, private or documentation one (D356 §2).
func TestHouseNetworkOf_IsAPublicIPv6SourcesSlash64AndNothingElse(t *testing.T) {
	t.Parallel()
	for source, want := range map[string]string{
		"2a0f:1000:1:2:aaaa:bbbb:cccc:dddd": "2a0f:1000:1:2::/64", "11.22.33.144": "",
		"::ffff:11.22.33.144": "", "fd77::1": "", "2001:db8::1": "", "fe80::1%eth0": "", "::1": "",
	} {
		network, isNetwork := discoveryproto.HouseNetworkOf(netip.MustParseAddr(source))
		if got := map[bool]string{true: network.String(), false: ""}[isNetwork]; got != want {
			t.Errorf("HouseNetworkOf(%s) = %q; want %q", source, got, want)
		}
		if isNetwork && !discoveryproto.IsHouseNetwork(network) {
			t.Errorf("HouseNetworkOf(%s) = %s, which is no house network", source, network)
		}
	}
}
