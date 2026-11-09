// SPDX-License-Identifier: AGPL-3.0-or-later

package home_test

import (
	"net/netip"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/home"
)

// dockersBridge is Docker's default bridge, a network of the core's own interfaces in a container.
var dockersBridge = netip.MustParsePrefix("172.17.0.0/16")

// deployed is the rule a deployment with a tunnel writes (internal/deployment's core.toml), the core in
// a container on Docker's bridge.
func deployed() *home.Rule {
	return home.NewRule(home.Networks{
		Listed:        []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")},
		PrivateIsHome: true,
		NotHome:       []netip.Prefix{netip.MustParsePrefix("10.77.0.0/24"), netip.MustParsePrefix("fd77::/64")},
		Interfaces: []netip.Prefix{
			dockersBridge, netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"),
			netip.MustParsePrefix("10.77.0.0/24"),
		},
	})
}

// expectHome fails t unless rule judges each address as want.
func expectHome(t *testing.T, rule *home.Rule, want bool, addresses ...string) {
	t.Helper()
	for _, each := range addresses {
		if got := rule.Contains(netip.MustParseAddr(each)); got != want {
			t.Errorf("%s is home: %v; want %v", each, got, want)
		}
	}
}

// A router that hands the house a new network locked admin out while home was the typed address's /24;
// every private range is the house's now (D452 §1).
func TestRule_APrivateSourceIsHome(t *testing.T) {
	t.Parallel()
	expectHome(t, deployed(), true, "192.168.1.20", "192.168.7.3", "10.0.0.5", "172.20.1.9", "fd12:3456::9",
		"::ffff:192.168.1.20")
}

// docker-proxy hands the core the internet's requests from the bridge's gateway, so Docker's bridge
// is never home, its gateway neither (the Plex-chain review's L1, D452 §1).
func TestRule_DockersBridgeNetworkIsNotHomeNorItsGateway(t *testing.T) {
	t.Parallel()
	expectHome(t, deployed(), false, "172.17.0.1", "172.17.0.3")
}

// The internet arrives through the tunnel, whose networks are private (D452 §1).
func TestRule_TheTunnelsNetworksAreNotHome(t *testing.T) {
	t.Parallel()
	expectHome(t, deployed(), false, "10.77.0.1", "10.77.0.200", "fd77::1")
}

// Carrier-grade NAT's 100.64.0.0/10 is not private (D452 §1).
func TestRule_CarrierGradeNATIsNotHome(t *testing.T) {
	t.Parallel()
	expectHome(t, deployed(), false, "100.64.0.1", "100.127.255.254")
}

// A public address is the internet's, IPv4 or IPv6.
func TestRule_APublicAddressIsNotHome(t *testing.T) {
	t.Parallel()
	expectHome(t, deployed(), false, "203.0.113.9", "8.8.8.8", "2a0f:5000:1:2::30")
}

// The house's public IPv6 /64 is home as the core itself sees it: its own address's /64 (D452 §2);
// another /64 is not, and none is before the core has found its own.
func TestRule_TheCoresOwnIPv6NetworkIsHome(t *testing.T) {
	t.Parallel()
	rule := deployed()
	expectHome(t, rule, false, "2a0f:5000:1:2::30")
	rule.CountOwnIPv6Networks(func() []netip.Prefix {
		return []netip.Prefix{netip.MustParsePrefix("2a0f:5000:1:2::/64")}
	})
	expectHome(t, rule, true, "2a0f:5000:1:2::30", "2a0f:5000:1:2:aaaa:bbbb:cccc:dddd")
	expectHome(t, rule, false, "2a0f:5000:1:3::30", "2a0f:6000:1:2::30")
}

// A core on the host's network has the house's network among its interfaces; the one holding the
// address at home is the house's and stays home (D452 §1, §3).
func TestRule_AnInterfaceNetworkHoldingTheAddressAtHomeStaysHome(t *testing.T) {
	t.Parallel()
	rule := home.NewRule(home.Networks{
		PrivateIsHome: true,
		Interfaces:    []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24"), dockersBridge},
	})
	expectHome(t, rule, false, "192.168.1.20") // the address at home not yet known
	rule.SetAddressAtHome(netip.MustParseAddr("192.168.1.250"))
	expectHome(t, rule, true, "192.168.1.20")
	expectHome(t, rule, false, "172.17.0.1")
}

// The core's loopback, which a command in its container alone reaches, stays home, though an interface
// holds it (D334 §2), listed or not.
func TestRule_LoopbackStaysHome(t *testing.T) {
	t.Parallel()
	expectHome(t, deployed(), true, "127.0.0.1", "::1")
	unlisted := home.NewRule(home.Networks{
		PrivateIsHome: true, Interfaces: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
	})
	expectHome(t, unlisted, true, "127.0.0.1", "::1")
}

// A config naming neither server.home_is_private nor server.not_home_networks keeps the rule it had:
// its listed networks, and nothing else, not even a private address or the core's own /64.
func TestRule_WithoutPrivateItIsTheListedNetworksAlone(t *testing.T) {
	t.Parallel()
	rule := home.NewRule(home.Networks{
		Listed:     []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24"), netip.MustParsePrefix("127.0.0.0/8")},
		Interfaces: []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")},
	})
	rule.CountOwnIPv6Networks(func() []netip.Prefix {
		return []netip.Prefix{netip.MustParsePrefix("2a0f:5000:1:2::/64")}
	})
	expectHome(t, rule, true, "192.168.1.20", "127.0.0.1")
	expectHome(t, rule, false, "192.168.2.20", "10.0.0.5", "::1", "2a0f:5000:1:2::30")
}

// A nil rule, or one never set, has no home.
func TestRule_ANilRuleHasNoHome(t *testing.T) {
	t.Parallel()
	var none *home.Rule
	expectHome(t, none, false, "127.0.0.1", "192.168.1.20")
	expectHome(t, &home.Rule{}, false, "127.0.0.1", "192.168.1.20")
}
