// SPDX-License-Identifier: AGPL-3.0-or-later

// Command discovery-watch tells the owner of every signature served that the owner's signing tool did
// not make (the root key audit's F4; D393 §8): it fetches both root chains, every version, the address
// document and the release record from every place named, and reports each signature whose payload,
// key and counter the ledger's expected file (written by cmd/discovery-sign) does not hold, and each
// place that did not answer. Run on the owner's machine every 15 minutes (a timer), its non-zero exit
// alerting the owner's phone:
//
//	go run ./cmd/discovery-watch --expected <ledger>/expected --place https://discovery.example --place ...
//
// # Invariants
//
//   - A signature the ledger holds is quiet; one it does not, by another key or counter, or a place
//     that does not answer, is reported and the exit is non-zero.
//     TestRun_ReportsEverySignatureTheLedgerDoesNotHold.
//
// # Read before changing
//
//   - docs/contributors/design/research/root-key-audit-2026-10-31.md F4, §3.7; D393 §8.
//
// # Where to change
//
//   - main.go: the places, the fetches and the report; discovery/trust/ledger.go: the expected file.
package main
