// SPDX-License-Identifier: AGPL-3.0-or-later

// Package trust is the root key design of the root key audit's §3 (D393): the signed objects' strict
// line format, the check of one signature, a root chain and the rules a core follows it by, and the
// files a discovery publishes. Every signing and every check is here, so a change to the scheme
// lands in one place; discovery/addressdoc and discovery/release read their objects' lines through
// it. A public building block, so the paid service signs and checks with the same code (D275 §1).
//
// The design, in short: an identity is the hash of its chain's first statement ("od1-…" a
// discovery's, "op1-…" the publisher's). Each statement names two tiers of single keys: everyday keys
// sign documents, records and changes of the everyday tier; recovery keys sign anything, alone change
// the recovery tier, thresholds and roles, and may skip versions past a thief's statements. Keys of
// both tiers are added and removed by a signed statement; a key removed stays named removed for good,
// and a key leaving the set has the new set sign too. Every signature is an SSH signature in a
// namespace of its object's type, by a key byte for byte expected, a key marked uv carrying the
// user-verified flag, which neither x/crypto nor ssh-keygen checks. A "delegate" line is reserved for
// an online key's freshness statements, not built (D393 §9).
//
// # Invariants
//
//   - A signature holds only in the namespace expected, over the exact bytes, by a key expected; one
//     ssh-keygen made holds. TestVerify_ASignatureSSHKeygenMadeHoldsAndNothingElse.
//   - A key marked uv signs only with the user-verified flag; a key of a type the design does not take
//     signs nothing. TestVerify_AKeyMarkedUVSignsOnlyWithItsPIN.
//   - The format is strict text, its lines in order, an unknown critical line or format refused.
//     TestParse_TheFormatIsStrictAndAnUnknownCriticalLineRefuses.
//   - An identity is its genesis' hash, a genesis signed by one of its own keys, and a kept chain is
//     checked again from it. TestGenesis_TheIdIsTheGenesisHashAndAStateIsCheckedAgainFromIt.
//   - Each row of the audit's §3.6 rotation table: TestRotation_E1LostIsReplacedByE2,
//     TestRotation_E1StolenWithoutItsPINIsRemovedAtOnce,
//     TestRotation_E1StolenWithItsPINIsRecoveredBySkippingPastTheThief,
//     TestRotation_BothEverydayKeysLostAreReplacedByARecoveryKey,
//     TestRotation_R1LostIsReplacedByR2AndNeverByAnEverydayKey,
//     TestRotation_R1StolenWithItsPINIsResolvedByR2sSkip, TestRotation_AllKeysLostIsANewIdentity.
//   - The tiers' rules: an everyday key never changes the recovery tier nor skips; two statements of
//     one version are refused unless one is a recovery key's; a leaving signer has the new set sign
//     too; removed keys stay named. TestFollow_TheTiersRulesHold.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - D393, and docs/contributors/design/research/root-key-audit-2026-10-31.md §3: the design.
//   - docs/users/discovery-signing.md: how the owner signs.
//
// # Where to change
//
//   - format.go: the line format; sshsig.go: an SSH signature and its check; root.go: a root
//     statement; chain.go: following a chain; publish.go: the published files; ledger.go: the
//     owner's ledger of expected signatures, which the watcher reads.
package trust
