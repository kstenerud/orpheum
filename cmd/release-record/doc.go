// SPDX-License-Identifier: AGPL-3.0-or-later

// Command release-record writes a release into a discovery's release record (security-updates.md
// Part A; discovery/release), as the release workflow runs it at each release:
//
//	go run ./cmd/release-record --record release.json --version v0.3.2 --digest sha256:… \
//	  [--security "what the release fixes, in a sentence for an admin"]
//
// The record's file is then put where the discovery reads it: its deployment's release record
// (single.Parts.ReleaseRecord; the paid service's own). It is a program of its own, not a
// subcommand of `orpheum`, since no household runs it; it is Go, not a script, so the record is
// written by the very type the discovery reads it with.
//
// # Invariants
//
//   - A release is written into the record by release.Record.Released, and the file is written
//     whole; a release it refuses writes nothing and exits non-zero saying why.
//     TestRun_WritesEachReleaseIntoTheRecordAndRefusesAnOlderOne.
//
// # Read before changing
//
//   - docs/contributors/design/plans/security-updates.md: the record and who writes it (the owner,
//     2026-10-31: the release workflow).
//
// # Where to change
//
//   - main.go: the flags; discovery/release: the record and a release's change to it.
package main
