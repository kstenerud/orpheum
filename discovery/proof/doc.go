// SPDX-License-Identifier: AGPL-3.0-or-later

// Package proof is what a core's signatures cover, and their checks: Ed25519 over "orpheum-
// discovery-v1:<purpose>:<discovery's issuer>:<core>:<nonce>" to its discovery, so a signature made
// to enrol cannot open a session, nor serve another discovery or core (D273; the protocol's §2.1);
// over a WebRTC answer's exact SDP, for its client (§2.5); and the identity key's fingerprint every
// link carries (§3.1); and a realm sign-in's key, which a person's password makes and which proves
// the page to the core and the core to the page (D297); and the nonces a core signs, which a
// discovery issues and spends once each (D343). The core signs through it, and a discovery,
// Orpheum's own or the closed multi-core service, and the client check through it (D275 §1).
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
//   - A realm key is PBKDF2-SHA256 of the password, salted by the realm and the name in lower case,
//     and any part wrong proves nothing either way: an ask holds for its person, a tag for its core
//     and identity key (D297 §3). Enforced by TestRealmKey_IsTheVectorsWhateverTheCaseAndSpacesTyped
//     and TestRealmKey_AnyPartWrongProvesNothing.
//   - A device code's outcome opens only with the key of the device that asked, and the check code
//     both screens show is made of that key, the nonce the device committed to and the core's, so a
//     key swapped in shows another, and no one knows it before the code is entered (D293 §2, D321).
//     Enforced by TestSealDeviceOutcome_OpensOnlyWithTheDevicesKeyAndAsTheBrowserOpensIt,
//     TestDeviceCheckCode_IsTheKeysAndBothNoncesAndTheBrowsersAlike and
//     TestDeviceCommitmentAndKeyHash_AreTheBrowsersAlike.
//   - A nonce carries its expiry and a MAC under its issuer's key, so issuing keeps nothing; it is
//     spent once, by a valid signature for its purpose, discovery and core, before it expires (the
//     security audit's S13, D343). Enforced by TestNonces_ANonceWorksOnceForItsPurposeUntilItExpires
//     and TestNonces_IssuingKeepsNothing.
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
//   - realm.go: a realm sign-in's key, and what it proves.
//   - devicecode.go: a device code through the paid service: its check code, and its sealed outcome.
//   - nonces.go: the nonces a core signs, issued and spent once each.
package proof
