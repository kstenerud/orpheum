// SPDX-License-Identifier: AGPL-3.0-or-later

// Package proof is what a core's signature to its discovery covers, and its check: Ed25519 over
// "orpheum-discovery-v1:<purpose>:<nonce>", so a signature made to enrol cannot open a session, nor
// the reverse (D273). The core signs through it, and a discovery, Orpheum's own or the closed
// multi-core service, checks through it (D275 §1).
//
// # Invariants
//
//   - A signature names the protocol and its purpose; one for one purpose fails the other's check.
//     Enforced by TestVerify_ASignatureServesOnlyThePurposeItWasMadeFor.
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
