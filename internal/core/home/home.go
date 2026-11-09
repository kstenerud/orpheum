// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"net"
	"net/netip"
	"slices"
	"sync/atomic"
)

// Networks are what the rule is made of, as the config names them.
type Networks struct {
	// Listed are the networks home outright (server.home_networks).
	Listed []netip.Prefix
	// PrivateIsHome counts the private ranges, the core's loopback and its own public IPv6 /64 as
	// home (server.home_is_private, D452 §1, §2); false leaves the listed networks alone.
	PrivateIsHome bool
	// NotHome are the networks never home though private (server.not_home_networks): the tunnel's.
	NotHome []netip.Prefix
	// Interfaces are the networks of the core's own interfaces (InterfaceNetworks), excepted from the
	// private ranges but the one holding the address at home.
	Interfaces []netip.Prefix
}

// Rule is whether an address is the house's (the package's comment). Its zero value has no home; its
// methods take a nil *Rule as one with no home too.
type Rule struct {
	networks      atomic.Pointer[Networks]
	addressAtHome atomic.Pointer[netip.Addr]
	ownIPv6       atomic.Pointer[func() []netip.Prefix]
}

// NewRule returns the rule networks make.
func NewRule(networks Networks) *Rule {
	rule := &Rule{}
	rule.Set(networks)
	return rule
}

// Listed returns the rule whose home is networks alone.
func Listed(networks ...netip.Prefix) *Rule {
	return NewRule(Networks{Listed: networks})
}

// Set replaces the networks the rule is made of.
func (rule *Rule) Set(networks Networks) {
	held := Networks{
		Listed:        slices.Clone(networks.Listed),
		PrivateIsHome: networks.PrivateIsHome,
		NotHome:       slices.Clone(networks.NotHome),
		Interfaces:    slices.Clone(networks.Interfaces),
	}
	rule.networks.Store(&held)
}

// SetAddressAtHome names the server's address at home (D452 §3): an interface network holding it is
// the house's and stays home. An invalid address names none.
func (rule *Rule) SetAddressAtHome(address netip.Addr) {
	address = address.Unmap().WithZone("")
	rule.addressAtHome.Store(&address)
}

// CountOwnIPv6Networks has the rule ask own for the core's own public IPv6 /64s, as its own STUN
// servers last found them (rtcapi.Server.OwnIPv6Networks, D452 §2); nil asks nothing.
func (rule *Rule) CountOwnIPv6Networks(own func() []netip.Prefix) {
	rule.ownIPv6.Store(&own)
}

// Contains is whether address is the house's.
func (rule *Rule) Contains(address netip.Addr) bool {
	if rule == nil {
		return false
	}
	networks := rule.networks.Load()
	address = address.Unmap().WithZone("")
	if networks == nil || !address.IsValid() {
		return false
	}
	switch {
	case isInAny(address, networks.Listed):
		return true
	case !networks.PrivateIsHome:
		return false
	case address.IsLoopback():
		return true
	case isInAny(address, networks.NotHome):
		return false
	case isInAny(address, rule.ownIPv6Networks()):
		return true
	case !address.IsPrivate():
		return false
	default:
		return !rule.isInAForeignInterfaceNetwork(address, networks.Interfaces)
	}
}

// ContainsIP is Contains for a net.IP; one that is no address is not home.
func (rule *Rule) ContainsIP(ip net.IP) bool {
	address, isAddress := netip.AddrFromSlice(ip)
	return isAddress && rule.Contains(address)
}

// ownIPv6Networks are the core's own public IPv6 /64s, none until a source is named.
func (rule *Rule) ownIPv6Networks() []netip.Prefix {
	own := rule.ownIPv6.Load()
	if own == nil || *own == nil {
		return nil
	}
	return (*own)()
}

// isInAForeignInterfaceNetwork is whether address is in one of the core's interface networks that
// does not hold the address at home.
func (rule *Rule) isInAForeignInterfaceNetwork(address netip.Addr, interfaces []netip.Prefix) bool {
	atHome := netip.Addr{}
	if held := rule.addressAtHome.Load(); held != nil {
		atHome = *held
	}
	return slices.ContainsFunc(interfaces, func(network netip.Prefix) bool {
		return network.Contains(address) && !network.Contains(atHome) // an invalid atHome is in none
	})
}

// isInAny is whether address is in one of networks.
func isInAny(address netip.Addr, networks []netip.Prefix) bool {
	return slices.ContainsFunc(networks, func(network netip.Prefix) bool { return network.Contains(address) })
}

// InterfaceNetworks are the networks the core's interfaces are on, none when they cannot be read.
func InterfaceNetworks() []netip.Prefix {
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
