// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fingerprint finds the sound a season's episodes share: their intro, in their opening
// minutes, and their credits, in their closing ones (D101). Each video's opening and closing are
// fingerprinted with FFmpeg's chromaprint muxer, mixed to stereo first, and the fingerprints of a set
// are compared pair by pair, as intro-skipper does for Jellyfin: the bits of each point against the
// other's at every offset, the longest run of points the same taken as the stretch the two share,
// and of those a video shares with the others, the one most of them overlap. The core names the set
// and decides what a stretch is and how sure it is; this package measures (D101).
//
// # Invariants
//
//   - The sound is mixed to stereo before it is fingerprinted, since chromaprint fails on 5.1
//     (D115), and a season of generated episodes in 5.1, the same intro and credits at a different
//     offset in each amid sound of their own, has each found to within a second of where it is.
//     Enforced by TestMeasure_FindsASeasonsIntroAndCreditsToWithinASecond (integration tier).
//
//   - Two fingerprints share the longest stretch whose points differ by no more than
//     maxPointBitDifference bits, at whatever offset it is in each, and none shorter than minRepeat.
//     Enforced by TestShared_FindsTheStretchTwoFingerprintsShareAtItsOffsetInEach.
//
//   - A stretch stays one across a gap shorter than maxGap and is two across a longer one.
//     Enforced by TestShared_KeepsAStretchWholeAcrossAGapShorterThanMaxGap.
//
//   - A video's repeat is the stretch most of the others share with it, heard in how many, so a
//     longer stretch two share, a recap, is not taken for the intro; a video that shares nothing
//     has none. Enforced by TestRepeats_FindsEachFingerprintsStretchTheOthersShareAndHowMany.
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx and TestArch_OnlyExecxImportsOsExec.
//
// # Read before changing
//
//   - D101: the core names the set, the node measures, and the core decides.
//   - D115: chromaprint and 5.1.
//   - D102, D114: the passes are deferrable, and how fast this one is.
//
// # Where to change
//
//   - measure.go: the FFmpeg run of one stretch of sound and the reading of its raw fingerprint.
//     match.go: the comparison and its thresholds. The agent (internal/node/agent, repeats.go)
//     checks each video before calling here and says each video's repeats in its own time.
package fingerprint
