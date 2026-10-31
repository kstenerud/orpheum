// SPDX-License-Identifier: AGPL-3.0-or-later

// Command release-record writes a release into the unsigned release record (security-updates.md
// Part A; discovery/release), as the release workflow runs it at each release:
//
//	go run ./cmd/release-record --record release --version v0.3.2 --digest sha256:… \
//	  [--security "what the release fixes, in a sentence for an admin"] [--pull registry.example/orpheum] \
//	  [--id op1-… --root-version <n>]   (for a record made new, or after a rotation)
//
// and says how to sign it with an everyday key of the publisher root: `ssh-keygen -Y sign -n
// orpheum-release-v1`, then `go run ./cmd/discovery-sign check --kind release`, which publishes it. It
// is a program of its own, not a subcommand of `orpheum`, since no household runs it; it is Go, not a
// script, so the record is written by the very type a core reads it with.
//
// # Invariants
//
//   - A release is written into the record by release.Record.Released, dated, its sequence one more;
//     a release older than the latest, or a version given another digest, writes nothing and exits
//     non-zero; a written record says how to sign it.
//     TestRun_WritesEachReleaseIntoTheRecordAndRefusesAnOlderOne.
//
// # Read before changing
//
//   - docs/contributors/design/plans/security-updates.md; D393 §2.
//
// # Where to change
//
//   - main.go: the flags; discovery/release: the record and a release's change to it.
package main
