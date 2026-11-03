// SPDX-License-Identifier: AGPL-3.0-or-later

// Command discovery-sign is the owner's signing tool (the root key audit's §3.7; D393, D394). Its
// guided commands ask plain questions and run ssh-keygen themselves, so the owner types no flag:
// `ceremony` is day one for both chains (keys made on their devices or, with --software, as plain files; each chain's
// genesis and the address document, each signature checked by the code a core uses before the next step),
// `sign-release` shows an unsigned release record and signs it once the owner types the digest's start, and
// `suspend`, `resume`, `add` and `remove` change a key's standing in both chains (D394 §4), offering only the keys
// allowed to sign each change. Beneath them the flag commands build a root statement, an address document or a drill
// from a typed edit (keys added, removed, suspended or resumed, versions skipped, thresholds and locations changed) and
// `check` checks a signature by the very code a core uses, keeps payload and signature in the owner's ledger, its
// expected file the watcher's (cmd/discovery-watch), and writes the files every place serves. keygen.go alone starts a
// process (ssh-keygen, on the owner's terminal and environment, since a hardware key asks for its touch and PIN there;
// the lint names that one file), and a test puts a fake in its place. No root key is ever put in an agent (the audit's
// F16). A Mac is no key and the tool has no kind for one (D395 §3). `bootstrap` writes the install's bootstrap
// script from the publisher's chain. No household runs it.
//
// docs/users/discovery-signing.md is the owner's guide: the day-one ceremony and each rotation.
//
// # Invariants
//
//   - The owner's day one and a rotation, through the tool, publish what a core follows: a leaving
//     signer whose new set does not sign is refused, and the ledger holds each signature made.
//     TestSign_TheOwnersDayOneAndARotationArePublishedAsACoreFollowsThem.
//   - A record changing a released version's digest is refused before it is kept; a drill names the
//     keys that signed it. TestCheck_ARecordChangingADigestIsRefusedAndADrillNamesItsKeys.
//   - A ceremony, scripted from its first question to its last, makes both chains, signs and checks each,
//     and publishes what a core follows; its private software keys are outside the records folder, and
//     a keys directory inside it is refused.
//     TestCeremony_ASoftwareCeremonyPublishesWhatACoreFollows,
//     TestCeremony_AKeysDirectoryInsideTheFolderIsRefused.
//   - A hardware ceremony makes one key per chain per device, its own application name each, keeps the
//     stubs apart in stubs/ and says the text to paste into suggested.go.
//     TestCeremony_AHardwareCeremonyMakesKeysPerChainAndSaysWhatToPaste.
//   - An interrupted ceremony run again carries on from the folder's notes and the ledger, making no
//     key twice. TestCeremony_AnInterruptedCeremonyResumesWithoutMakingKeysAgain.
//   - A signature by another key than the one asked for, or by a hardware key without its PIN, is
//     refused at its step, before any later key is asked or anything is kept.
//     TestCeremony_AWrongSignatureIsRefusedBeforeTheNextStep,
//     TestCeremony_ASignatureWithoutThePINIsRefused.
//   - sign-release signs only once the owner types the start of the record's digest; a mistyped or
//     empty answer signs nothing. TestSignRelease_NeedsTheOwnersConfirmationOfTheDigest.
//   - The install's bootstrap checks the record's signature by the publisher's keys before it pulls
//     or runs anything, then pulls the latest's image by its digest (the audit's §3.9).
//     TestBootstrap_VerifiesTheRecordBeforeItRunsAnything.
//   - A key is suspended and resumed through the tool, the change shown and who signs it said: a
//     suspension signed by the key alone is refused, and a resume only a recovery key's (D394 §4).
//     TestRoot_AKeyIsSuspendedAndOnlyARecoveryKeyResumesIt.
//   - The install's bootstrap trusts no suspended key. TestBootstrap_TrustsNoSuspendedKey.
//
// # Read before changing
//
//   - D393; D394 §4; docs/contributors/design/research/root-key-audit-2026-10-31.md §3.6, §3.7, §3.9, §4.
//
// # Where to change
//
//   - ceremony.go: the guided commands and their shared steps (the place for the rotation, suspend,
//     resume, add and remove flows: a method of guided in commands(), using signWith and accept);
//     keygen.go: ssh-keygen, behind the keygen interface; prompt.go: the questions.
//   - commands.go: each flag command; ledger.go: the ledger and the published files; bootstrap.sh: the
//     install's bootstrap.
package main
