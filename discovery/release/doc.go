// SPDX-License-Identifier: AGPL-3.0-or-later

// Package release is a discovery's release record (security-updates.md Part A): one small JSON file in
// the discovery's deployment, which the release workflow writes at each release (`orpheum
// release-record`) and a discovery answers each core from, about the core's own version alone. A
// public building block, so the closed multi-core service answers from the same file the same way
// (D275 §1).
//
// The file is
//
//	{"latest": "v0.3.2", "digest": "sha256:…",
//	 "versions": {"v0.3.1": {"security": true, "summary": "…", "fixedIn": "v0.3.2"}}}
//
// latest is the newest release and digest its image's; versions names each older release the
// workflow has seen, and whether it has a security fix pending, what it fixes and the first version
// that has it.
//
// # Invariants
//
//   - A core is told of its own version and nothing else: the latest and its digest only when the
//     latest is newer than its version, and its version's own pending fix. A file absent, unreadable
//     or not a record is nothing known, never an error to the core.
//     TestAnswer_TellsACoreOfItsOwnVersionAlone, TestAnswer_AFileAbsentOrBrokenIsNothingKnown.
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
//     workflow writes it.
//   - D273 §6: what a discovery is; D275 §1: why its building blocks are public.
//
// # Where to change
//
//   - release.go: the record, reading it, answering a core and a release's change to it.
//   - version.go: comparing versions.
package release
