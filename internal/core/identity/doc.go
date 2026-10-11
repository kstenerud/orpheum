// SPDX-License-Identifier: AGPL-3.0-or-later

// Package identity is the core's identity key (D273): an Ed25519 key made on first start and kept in
// identity.key in the persistent directory, beside secret.key, by which the core proves to its
// discovery that it is the core that enrolled.
//
// # Invariants
//
//   - The key is made once and read back on every later start; a file that is not a key is refused,
//     never replaced. TestLoadOrMake_MakesTheKeyOnceAndKeepsIt, TestParseKeyFile_RefusesWhatIsNotAKey.
//   - A signature covers "orpheum-discovery-v1:<purpose>:<nonce>", so one made to enrol cannot open a
//     session, nor the reverse. TestSign_NamesItsPurposeSoOneCannotBeUsedForAnother.
//
// # Read before changing
//
//   - D273: the discovery, and why a key rather than a shared secret.
//   - docs/contributors/design/plans/discovery-core.md: the wire.
//
// # Where to change
//
//   - identity.go: the key, its file and its signatures.
package identity
