// SPDX-License-Identifier: AGPL-3.0-or-later

// Package clientnet names the network a client's address is on: an IPv6 address's /64, the prefix
// one network gives its devices, or an IPv4 address whole, the public address of a household or a
// phone's carrier. A link speed is kept by it (D257 §1), and a stranger's asking for device codes is
// counted by it (D288 §6), so a phone's changing IPv6 address is one network, and one network's
// billions of IPv6 addresses are one too.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - An IPv6 address is its /64, an IPv4 address (mapped into IPv6 or not) itself, and an address
//     it cannot read "". Enforced by TestOf_IsAnIPv6AddressesSlash64OrAnIPv4AddressWhole.
//   - An address's site is an IPv6 address's /48 and an IPv4 address itself. Enforced by
//     TestSiteOf_IsAnIPv6AddressesSlash48OrAnIPv4AddressWhole.
//
// # Read before changing
//
//   - D257 §1: link speeds kept by network; D288 §6: device codes counted by network.
//
// # Where to change
//
//   - clientnet.go: Of, and SiteOf.
package clientnet
