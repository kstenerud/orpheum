// SPDX-License-Identifier: AGPL-3.0-or-later

// Package release is the release record (security-updates.md Part A; D391 §6): one small JSON file,
// signed by a root key of the discovery's set, which the release workflow writes at each release
// (`go run ./cmd/release-record`), the owner signs (`go run ./cmd/discovery-sign record`), and every
// discovery serves at WellKnownPath, beside its address document. A core reads it there, checks its
// signature against the root key set it pinned, and takes what it says of its own version; the
// install script checks an image's digest against it (`orpheum verify-release`). A public building
// block, so the paid service serves the same file the same way (D275 §1).
//
// The record, the payload a root key signs, is
//
//	{"issuedAt": 1800000000000, "latest": "v0.3.2", "locations": ["registry.example/orpheum"],
//	 "releases": {"v0.3.2": {"digest": "sha256:…"},
//	              "v0.3.1": {"digest": "sha256:…", "security": true, "summary": "…", "fixedIn": "v0.3.2"}}}
//
// latest is the newest release; releases names each release the workflow has seen, with its image's
// digest and, for an older one, whether it has a security fix pending, what it fixes and the first
// version that has it; locations are the repositories the image may be pulled from, each a mirror,
// since the digest, not the place, is what the record signs.
//
// # Invariants
//
//   - A core is told of its own version and nothing else: the latest, its digest and where to pull
//     it only when the latest is newer than its version, and its version's own pending fix.
//     TestFor_TellsACoreOfItsOwnVersionAlone.
//   - A record is taken only when a root key of the set signed it, as a release record, never an
//     online key; an image is a release only by the digest the record names.
//     TestOpen_ARecordIsTakenOnlySignedByARootKey.
//   - A release with a security fix marks every older release that lacks one as needing it, fixed in
//     the release; an older release already pending keeps its first fix; a plain release marks none.
//     A release older than the latest is refused, as is a digest that is no image's.
//     TestReleased_ASecurityFixMarksEveryOlderReleaseThatLacksOne,
//     TestReleased_RefusesAnOlderReleaseOrADigestThatIsNone.
//   - Versions are compared as semantic versions, a pre-release before its release.
//     TestCompare_OrdersAsSemanticVersions.
//   - What is written is read back as it was. TestWrite_ReadsBackWhatItWrote.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - docs/contributors/design/plans/security-updates.md: the record, the banner, and why the
//     workflow writes it; D391 §6: why it is signed and where it is served.
//   - D273 §6: what a discovery is; D275 §1: why its building blocks are public.
//
// # Where to change
//
//   - release.go: the record, opening a signed one, answering a core and a release's change to it.
//   - version.go: comparing versions.
package release
