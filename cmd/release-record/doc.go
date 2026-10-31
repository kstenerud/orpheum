// SPDX-License-Identifier: AGPL-3.0-or-later

// Command release-record writes a release into the unsigned release record (security-updates.md
// Part A; discovery/release), as the release workflow runs it at each release:
//
//	go run ./cmd/release-record --record release.json --version v0.3.2 --digest sha256:… \
//	  [--security "what the release fixes, in a sentence for an admin"] [--location registry.example/orpheum]
//
// and says how to sign it with a root key (D391 §6): `go run ./cmd/discovery-sign record`, or
// `ssh-keygen -Y sign` by hand and `discovery-sign seal`. The signed record is then published beside
// the release and put where each discovery serves it (single.Parts.ReleaseRecord; the paid service's
// own). It is a program of its own, not a subcommand of `orpheum`, since no household runs it; it is
// Go, not a script, so the record is written by the very type a core reads it with.
//
// # Invariants
//
//   - A release is written into the record by release.Record.Released, dated, and the file is
//     written whole; a release it refuses writes nothing and exits non-zero saying why; a written
//     record says how to sign it. TestRun_WritesEachReleaseIntoTheRecordAndRefusesAnOlderOne.
//
// # Read before changing
//
//   - docs/contributors/design/plans/security-updates.md: the record and who writes it (the owner,
//     2026-10-31: the release workflow); D391 §6: the record signed.
//
// # Where to change
//
//   - main.go: the flags; discovery/release: the record and a release's change to it.
package main
