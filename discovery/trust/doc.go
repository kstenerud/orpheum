// SPDX-License-Identifier: AGPL-3.0-or-later

// Package trust is a discovery's identity, a set of root keys, and every check of what it signs
// (D391 §2): the root key set's statements, an online key's delegation, an address document's and a
// release record's signatures. A signature is an SSH signature (`ssh-keygen -Y sign -n <kind>`), so
// the owner signs with a YubiKey's resident key, and any one root key of the set signs anything. A
// public building block, so the paid service signs and checks with the same code (D275 §1). Every
// signing and every check is here, behind OpenDocument, OpenRecord, Advance and Sign, so a change to
// the scheme lands in one place.
//
// # Invariants
//
//   - A signature holds only in its kind's namespace, by a key of the set, over the payload's exact
//     bytes; ssh-keygen's own signature holds. TestVerify_ASignatureSSHKeygenMadeHoldsAndNothingElse.
//   - A key is a hardware key's Ed25519 or ECDSA P-256, or a plain Ed25519, and no other type.
//     TestSign_AHardwareKeysSignatureHoldsAndAKeyOfAnotherTypeIsRefused.
//   - An empty set is refused, never trusted. TestAnchor_AnEmptySetIsRefusedNeverTrusted.
//   - A set first used is signed by one of its own keys, and later sets are taken in order of version,
//     each signed by a key of the one before. TestFirstUse_TakesASelfSignedSetAndCatchesUpThroughEachLaterOne,
//     TestAdvance_ASetSignedByAKeyNoLongerInTheSetIsRefused.
//   - An online key signs address documents only, for the issuer its delegation names and while it
//     lasts; a release record is a root key's alone.
//     TestOpenDocument_AnOnlineKeySignsOnlyForItsIssuerAndWhileItsDelegationLasts.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - D391 §2, §3, §6: the key set, the address document, the release record.
//   - docs/users/discovery-signing.md: how the owner signs, with a YubiKey.
//
// # Where to change
//
//   - trust.go: the kinds, the sets, the delegation and the checks; sshsig.go: the SSH signature's
//     format.
package trust
