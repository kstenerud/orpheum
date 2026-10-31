// SPDX-License-Identifier: AGPL-3.0-or-later

// Package release is the release record (security-updates.md Part A; D393 §2; the root key audit's
// §3.1): signed by a key of Orpheum's publisher root, a chain of its own compiled into every binary,
// never a discovery's. The release workflow writes it (`go run ./cmd/release-record`), the owner signs
// it (`ssh-keygen -Y sign`, then `go run ./cmd/discovery-sign check`), and every discovery and the
// publisher's own places serve it at RecordPath beside the publisher's chain. A core follows that
// chain, opens the record, and takes what it says of its own version; the install's bootstrap checks
// it with ssh-keygen before it runs any image. A public building block (D275 §1).
//
// The record, in the strict line format:
//
//	orpheum-signed release 1
//	id op1-…
//	root-version 3
//	sequence 57
//	issued 2026-11-02T09:00:00Z
//	expires 2026-12-02T09:00:00Z
//	latest v0.4.0
//	image v0.4.0 sha256:…        (each release's multi-architecture image digest, newest first)
//	pending v0.3.2 v0.4.0 <summary>   (a fix a version lacks: version, fixed in, what it fixes)
//	pull ghcr.io/kstenerud/orpheum    (where an image may be pulled: mirrors, the digest is the release)
//	location https://…                (where the record is served)
//
// # Invariants
//
//   - A core is told of its own version and nothing else: the latest, its digest and where to pull
//     it only when the latest is newer, and its version's own pending fix.
//     TestFor_TellsACoreOfItsOwnVersionAlone.
//   - A record is taken only signed by the publisher's chain in the release namespace, naming its id
//     and a root version it has reached, never a discovery's chain; it reads back as written.
//     TestOpen_ARecordIsTakenOnlySignedByThePublishersKey, TestWrite_ReadsBackWhatItWrote.
//   - A released version's digest never changes. TestReleased_AVersionsDigestNeverChanges.
//   - A security release marks every older release lacking a fix; an older release, a tag that is
//     none and a digest that is none are refused; each release dates the record and adds one to its
//     sequence. TestReleased_ASecurityFixMarksEveryOlderReleaseThatLacksOne.
//   - Versions are compared as semantic versions, a pre-release before its release.
//     TestCompare_OrdersAsSemanticVersions.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - docs/contributors/design/plans/security-updates.md; D393 §2, §6, §8; the audit's F2, F4, F11.
//
// # Where to change
//
//   - release.go: the record, its lines, opening a signed one, telling a core, a release's change.
//   - version.go: comparing versions.
package release
