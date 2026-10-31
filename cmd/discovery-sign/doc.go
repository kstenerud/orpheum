// SPDX-License-Identifier: AGPL-3.0-or-later

// Command discovery-sign is the owner's signing tool (the root key audit's §3.7; D393): it builds a
// root statement, an address document or a drill from a typed edit (keys of either tier added or
// removed, versions skipped, thresholds and locations changed), shows the payload and what changed,
// and says the `ssh-keygen -Y sign -n <namespace> -f <stub>` command that signs it with one touch and
// one PIN; then `check` checks the signature by the very code a core uses, keeps payload and
// signature in the owner's ledger, its expected file the watcher's (cmd/discovery-watch), and writes
// the files every place serves. It does not run ssh-keygen itself, since a process is started by
// internal/node/execx alone (forbidigo's exec rule), and no root key is ever put in an agent (the
// audit's F16). `bootstrap` writes the install's bootstrap script from the publisher's chain. No
// household runs it.
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
//   - The install's bootstrap checks the record's signature by the publisher's keys before it pulls
//     or runs anything, then pulls the latest's image by its digest (the audit's §3.9).
//     TestBootstrap_VerifiesTheRecordBeforeItRunsAnything.
//
// # Read before changing
//
//   - D393; docs/contributors/design/research/root-key-audit-2026-10-31.md §3.6, §3.7, §3.9, §4.
//
// # Where to change
//
//   - commands.go: each command; ledger.go: the ledger and the published files; bootstrap.sh: the
//     install's bootstrap.
package main
