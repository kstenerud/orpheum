// SPDX-License-Identifier: AGPL-3.0-or-later

// Package discoveryproto is the wire between a core and its discovery (D273): the paid service, or
// the owner's own. Types only, as nodeproto is, in a public package the
// closed multi-core service imports beside Orpheum's own discovery (D275 §1).
//
// # Invariants
//
//   - Types and constants, and only the few functions both sides must compute alike: the version
//     signal's parsing (the protocol's §5), the address classes and an address's name label (§2.4,
//     §2.7), and the house networks' match (§3.2; D356 §5). Anything else is the caller's. Review.
//   - A house network is a public IPv6 /64 and nothing else, and a device is in the house's networks
//     only by the first four of them; an IPv4 source names none. TestInHouseNetworks_ADeviceInAPublicSlash64IsAtHome,
//     TestInHouseNetworks_AShorterPrivateOrIPv4NetworkCountsForNothing,
//     TestHouseNetworkOf_IsAPublicIPv6SourcesSlash64AndNothingElse.
//   - One classing of addresses: a carrier-NAT, a DS-Lite and a documentation address are never
//     public, whatever Go's netip calls them. TestIsPublic_CarrierNATDSLiteAndDocumentationAreNeverPublic.
//   - One address has one name label, and no label holds "--" (C15).
//     TestAddressLabel_OneAddressOneLabelAndNoDoubleDash.
//   - Every field names its JSON key in camelCase. Enforced by tagliatelle.
//   - A realm sign-in's answer dates its core's claim under the key the SDK reads, and omits it
//     undated. TestClientRealmResponse_SaysWhenItsCoreWasClaimedUnderTheKeyThePageReads.
//   - Imports nothing from internal/, which a module outside Orpheum cannot import. Enforced by
//     TestArch_PublicPackagesImportNothingInternal.
//   - The protocol is additive wherever it can be; a breaking change raises ProtocolMajor, and the
//     discovery answers the previous two majors for at least six months each (D273 §6, §7). The
//     header alone says the version, "major.minor", and a discovery answers any minor of a major it
//     serves. TestServes_AnyMinorOfAServedMajorAndNothingElse. Review, for the rest.
//
// # Read before changing
//
//   - D273 §6–§9: what the discovery is, and the protocol's stability; D275 §1: why it is public.
//   - docs/contributors/design/plans/discovery-core.md: the wire as the core speaks it.
//
// # Where to change
//
//   - discoveryproto.go: the calls' bodies, the states and the signalling messages. version.go: the
//     version signal. address.go: the address classes and labels. house.go: the house networks (D356).
package discoveryproto
