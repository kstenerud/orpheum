// SPDX-License-Identifier: AGPL-3.0-or-later

// Package proof is what a core's signatures cover, and their checks: Ed25519 over
// "orpheum-discovery-v1:<purpose>:<discovery base URL>:<core>:<nonce>" to its discovery, so a
// signature made to enrol cannot open a session, nor serve another discovery or core (D273; the
// protocol's §2.1); over a WebRTC answer's exact SDP, for its client (§2.5); and the identity key's
// fingerprint every link carries (§3.1). The core signs through it, and a discovery, Orpheum's own
// or the closed multi-core service, and the client check through it (D275 §1).
//
// # Invariants
//
//   - A signature names the protocol and its purpose; one for one purpose fails the other's check.
//     Enforced by TestVerify_ASignatureServesOnlyThePurposeItWasMadeFor.
//   - A signature to a discovery serves only that discovery and that core. Enforced by
//     TestVerify_ASignatureServesOnlyTheDiscoveryAndCoreItWasMadeFor.
//   - An answer's signature covers its exact SDP, its offer and that offer's fingerprint. Enforced by
//     TestVerifyAnswer_CoversTheExactSDPTheOfferAndItsFingerprint.
//   - An offer's proof covers its DTLS fingerprint under the client's session's or link's key, which
//     the discovery holds neither of. TestOffer_ProvesOnlyItsFingerprintUnderItsKey.
//   - A person reads the fingerprint in safe32 groups, the same digest links carry in base64url.
//     TestFingerprintText_IsTheSameDigestInSafe32Groups.
//   - The fingerprint is the public key's SHA-256 in base64url. Enforced by
//     TestFingerprint_IsTheKeysSHA256InBase64URL.
//   - Imports nothing from internal/, which a module outside Orpheum cannot import. Enforced by
//     TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - D273: why a key rather than a shared secret; D275 §1: why this package is public.
//   - docs/contributors/design/plans/discovery-core.md: the wire.
//
// # Where to change
//
//   - proof.go: what is signed, and the check.
package proof
