// SPDX-License-Identifier: AGPL-3.0-or-later

package discoveryproto

import (
	"net/netip"
	"slices"
)

// HouseNetworkBits is how much of an IPv6 address names a household's network: a home router hands
// its LAN one /64, which a provider gives one household alone (D356 §5).
const HouseNetworkBits = 64

// MaxHouseNetworks is the most house networks a heartbeat carries and a discovery counts: a fifth
// counts for nothing (D356 §3, §5).
const MaxHouseNetworks = 4

// How a house network was had (D356 §3, §6).
const (
	// HouseNetworkLearnt is a network a device the core proved at home asked the discovery from.
	HouseNetworkLearnt = "learnt"
	// HouseNetworkConfigured is a network the admin named (discovery.house_ipv6_networks).
	HouseNetworkConfigured = "configured"
)

// HouseNetwork is one of the house's public IPv6 /64s, "2a0f:5000:1:2::/64", as a core's heartbeat
// tells its discovery, and how the core had it: HouseNetworkLearnt or HouseNetworkConfigured. A
// discovery judges a device at home when it asks from one (D356 §5).
type HouseNetwork struct {
	Network string `json:"network"`
	Source  string `json:"source"`
}

// HouseNetworkOf is the house network a device asking from source is in: its /64, when source is a
// public IPv6 address, and false for any other, an IPv4 address above all, which a carrier shares
// among households and so says nothing of one (D356 §2). What a discovery passes on with an offer as
// its clientNetwork.
func HouseNetworkOf(source netip.Addr) (netip.Prefix, bool) {
	source = source.Unmap().WithZone("")
	if !source.Is6() || !IsPublic(source) {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(source, HouseNetworkBits).Masked(), true
}

// IsHouseNetwork is whether network can be a house's: a public IPv6 /64, as HouseNetworkOf makes one.
// A shorter or longer prefix, a private, IPv4 or unmasked one, is not (D356 §5).
func IsHouseNetwork(network netip.Prefix) bool {
	return network.IsValid() && network.Bits() == HouseNetworkBits && network == network.Masked() &&
		network.Addr().Is6() && !network.Addr().Is4In6() && IsPublic(network.Addr())
}

// IsInHouseNetworks is whether a device asking from client is in one of a heartbeat's house networks:
// the first MaxHouseNetworks of them, each counted only when IsHouseNetwork holds, so a heartbeat
// naming a wider or a private network, or many, names no household by it (D356 §5). The one match,
// for both discoveries and Orpheum's tests.
func IsInHouseNetworks(client netip.Addr, networks []HouseNetwork) bool {
	client = client.Unmap().WithZone("")
	if !client.Is6() {
		return false
	}
	return slices.ContainsFunc(networks[:min(len(networks), MaxHouseNetworks)], func(named HouseNetwork) bool {
		network, err := netip.ParsePrefix(named.Network)
		return err == nil && IsHouseNetwork(network) && network.Contains(client)
	})
}
