// SPDX-License-Identifier: AGPL-3.0-or-later

// Package link is a core's link as a discovery's client reads it: the fragment after `#`,
// `<coreId>.<secret>.<fingerprint>`, which names the core, carries the secret only the core can spend
// and the identity key's fingerprint the client checks the core against at first contact; and the
// hash of the secret, `SHA-256("orpheum-locate" ‖ secret)`, by which the core registers the link
// with its discovery without the discovery ever learning the secret (the protocol's §3.1; D292 §4,
// A7). Public, so the closed multi-core service and the client read links one way (D275 §1).
//
// # Invariants
//
//   - A fragment written is read back as it was, and one that is not a link's is refused. Enforced
//     by TestFragment_IsReadBackAsWrittenAndAnythingElseIsRefused.
//   - A link's locate hash is SHA-256 over "orpheum-locate" and the secret, in base64url. Enforced
//     by TestLocateHash_IsTheSecretsHashUnderItsOwnLabel.
//   - An offer's key is SHA-256 over "orpheum-offer" and the secret, never the locate hash the
//     discovery holds. TestOfferKey_IsTheSecretsHashUnderALabelOfItsOwn.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - docs/contributors/design/discovery-protocol.md §3.1: the links and how a client finds a core.
//
// # Where to change
//
//   - link.go: the fragment, the locate hash and the offer key.
package link
