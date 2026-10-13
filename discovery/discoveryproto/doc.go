// SPDX-License-Identifier: AGPL-3.0-or-later

// Package discoveryproto is the wire between a core and its discovery (D273): the paid service at
// app.orpheum-app.com, or the owner's own. Types only, as nodeproto is, in a public package the
// closed multi-core service imports beside Orpheum's own discovery (D275 §1).
//
// # Invariants
//
//   - Types and constants, and only the few functions both sides must compute alike: the version
//     signal's parsing (the protocol's §5). Anything else is the caller's. Review.
//   - Every field names its JSON key in camelCase. Enforced by tagliatelle.
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
//     version signal.
package discoveryproto
