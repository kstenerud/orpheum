// SPDX-License-Identifier: AGPL-3.0-or-later

// Command discovery-sign is how a discovery's owner signs with a root key (D391 §2): the root key
// sets' chain, an online key's delegation, the address document and the release record, each with
// whichever one of the set's keys is at hand. It signs through the ssh-agent, so a YubiKey's resident
// key loaded with `ssh-add -K` signs, asking for a touch, and no key leaves its device; or with a
// --key-file, a plain Ed25519 key's, for an operator without a hardware key. It writes the signed
// files a discovery serves (discovery/single, the paid service) and checks each in Go before it
// writes it. It is a program of its own, not a subcommand of `orpheum`, since no household runs it:
//
//	go run ./cmd/discovery-sign root-set --sets root-sets.json --add yubikey-1.pub --add yubikey-2.pub
//	go run ./cmd/discovery-sign document --sets root-sets.json --issuer discovery.example ... --out doc.json
//
// docs/users/discovery-signing.md is the owner's guide, the YubiKeys' set-up included.
//
// # Invariants
//
//   - A set is signed by a key of the set before it, the first by one of its own, and a key removed
//     signs no later set; the chain written is the one a core catches up through.
//     TestRootSet_AnyOneKeyMakesTheNextSetAndARemovedKeySignsNoMore.
//   - What it signs, a document, a delegation and a record, is what a core opens, each by a root
//     key. TestDocument_WhatTheHelperSignsACoreOpens.
//   - A signature made by hand is sealed only when it holds under the chain.
//     TestSeal_TakesAHandMadeSignatureOnlyWhenItHolds.
//
// # Read before changing
//
//   - D391 §2, §3, §6; discovery/trust, where every signature is made and checked.
//
// # Where to change
//
//   - main.go: the ssh-agent and the usage; sign.go: each thing it signs.
package main
