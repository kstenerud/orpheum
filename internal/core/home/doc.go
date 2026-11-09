// SPDX-License-Identifier: AGPL-3.0-or-later

// Package home decides whether an address a request or a WebRTC connection comes from is the house's
// (D452 §1, §2): the one rule internal/core/api judges a direct request by and internal/core/rtcapi
// judges a connection's selected pair and the checks a relayed connection answers by, so the two
// never disagree.
//
// An address is home, in this order:
//
//  1. Loopback is home (D334 §2), though the core's own interfaces hold it.
//  2. One in a network the config excepts (server.not_home_networks: the tunnel's 10.77.0.0/24 and
//     fd77::/64, through which the internet arrives) is not.
//  3. One in the core's own public IPv6 /64, as its own STUN servers last found it (rtcapi's own
//     networks), is (§2); never a device's address, never a network a discovery reports (D356 §7).
//  4. One neither in 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 or fc00::/7 nor in a listed network
//     (server.home_networks, a VPN's say), carrier-grade NAT's 100.64.0.0/10 and every public
//     address, is not.
//  5. Any other is, unless it is in a network of the core's own interfaces (Docker's bridge, whose
//     gateway is where docker-proxy hands the core the internet's requests: the Plex-chain review's
//     L1) that does not hold the address at home, so a core on the host's network keeps the house's.
//
// No config makes a listed network win outright any more (the owner, 2026-11-09); Listed, which does,
// is for tests alone.
//
// # Invariants
//
//   - A private source is home; one in a network of the core's own interfaces is not, its gateway
//     neither, unless that network holds the address at home. TestRule_APrivateSourceIsHome,
//     TestRule_DockersBridgeNetworkIsNotHomeNorItsGateway,
//     TestRule_AnInterfaceNetworkHoldingTheAddressAtHomeStaysHome.
//   - The tunnel's networks, carrier-grade NAT's and a public address are not home.
//     TestRule_TheTunnelsNetworksAreNotHome, TestRule_CarrierGradeNATIsNotHome,
//     TestRule_APublicAddressIsNotHome.
//   - The core's own public IPv6 /64 is home; another public /64 is not.
//     TestRule_TheCoresOwnIPv6NetworkIsHome.
//   - Loopback stays home, though an interface holds it. TestRule_LoopbackStaysHome.
//   - A listed network is home, under the same exceptions as a private one.
//     TestRule_AListedNetworkIsHomeButTheExceptionsStillApply.
//   - The rule, the address at home and the own networks' source are each replaced whole while
//     requests read them; a nil rule is no home. TestRule_ANilRuleHasNoHome.
//
// # Read before changing
//
// D452 §1–2; D297 §6 and D300 §3, which it amends; D334 §2, §4; D356 §7.
//
// # Where to change
//
//   - home.go: Rule.Contains, the rule's order; InterfaceNetworks, the core's own networks.
package home
