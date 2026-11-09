// SPDX-License-Identifier: AGPL-3.0-or-later

// Package home decides whether an address a request or a WebRTC connection comes from is the house's
// (D452 §1, §2): the one rule internal/core/api judges a direct request by and internal/core/rtcapi
// judges a connection's selected pair and the checks a relayed connection answers by, so the two
// never disagree. It also keeps the address at home, the server's address in the house, learned from
// the requests that name it or set by an admin (D452 §3, §4).
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
// The address at home is, first, the configured one (discovery.lan_addresses); else the last a
// direct request from home taught by naming the server by a private IPv4 address on the plain or
// HTTPS port, or by a name that resolves to one, or an admin set; kept in the database across a
// restart. Each change is told to the rule and to every reaction (rtcapi's host candidates, the
// discovery's addresses) and logged as homeAddressLearned.
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
//   - A private IPv4 address a request names teaches the address at home, kept across a restart with
//     the device and when; nothing else teaches, and only a change writes.
//     TestHomeAddress_ARequestFromHomeTeachesItAndARestartKeepsIt, TestHomeAddress_LearnsOnlyOnChange,
//     TestHomeAddress_OnlyAPrivateIPv4AddressTeaches.
//   - A configured address wins over a learned or set one.
//     TestHomeAddress_AConfiguredAddressWinsAndNothingIsLearned.
//   - A name is resolved, once, only while no address is known.
//     TestHomeAddress_ANameIsLookedUpForItsPrivateIPv4Answer.
//   - An admin's set is kept naming them; their removal forgets the name, not the address.
//     TestHomeAddress_AnAdminsSetIsKeptWithTheirName.
//   - The setup page's guess is a private IPv4 address of no container's interface.
//     TestHomeAddress_TheGuessIsAPrivateIPv4AddressOfNoContainerInterface.
//   - It is kept as two settings, so the settings page's change or reset reaches the rule at once.
//     TestHomeAddress_TheSettingsPageChangesItLive.
//   - A core sharing the host's network takes its default route's interface's address until one is
//     learned or set; a core in a container's network guesses nothing, as its default route is a
//     bridge. TestHomeAddress_ACoreOnTheHostsNetworkGuessesTheDefaultRoutesAddressUntilOneIsLearned,
//     TestHomeAddress_ACoreInAContainersNetworkGuessesNothing,
//     TestDefaultRouteAddress_IsTheDefaultRoutesInterfacesPrivateIPv4AddressButNoContainers.
//   - A core sharing the host's network learns only an address one of its interfaces holds, since a
//     request's Host is the device's to write; an admin's set is not held to it.
//     TestHomeAddress_ACoreOnTheHostsNetworkLearnsOnlyAnAddressItHolds.
//
// # Read before changing
//
// D452 §1–4; D297 §6 and D300 §3, which it amends; D334 §2, §4; D356 §7.
//
// # Where to change
//
//   - home.go: Rule.Contains, the rule's order; InterfaceNetworks, the core's own networks.
//   - address.go: AddressAtHome, the address at home learned, set and kept.
//   - default_route.go: DefaultRouteAddress, the guess of a core on the host's network.
package home
