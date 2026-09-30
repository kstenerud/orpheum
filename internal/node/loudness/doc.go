// SPDX-License-Identifier: GPL-3.0-or-later

// Package loudness measures how loud each audio track of a video is, by EBU R128 with FFmpeg's
// ebur128 filter, so the core can set a gain that brings films to one level and the player can
// apply it (D226). A track with more than two channels is measured as it is, which a receiver
// plays, and as the standard stereo mix a prepare makes of it, which a stereo device hears: that
// mix renormalises its gains and plays 7 to 8 dB quieter. One run reads the file once for every
// track.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx and TestArch_OnlyExecxImportsOsExec.
//
//   - The video is read only as a local file, with every other protocol refused, and each sound
//     is measured through its own named filter, the stereo mix being prepare's own. Enforced by
//     TestMeasure_ReadsOnlyTheFileAndMeasuresEachSoundByName.
//
//   - A measurement matches what FFmpeg measures of a sound of known level, as it is and as the
//     stereo mix. Enforced by TestMeasure_AToneOfKnownLevelMeasuresAsItIs (integration tier).
//
//   - A measurement is watched for a stall, with the deadline of reading the whole file. Enforced
//     by TestMeasure_IsWatchedForAPositionThatStandsStillAndSaysWhenItStalled.
//
// # Read before changing
//
//   - D226: what is measured and why the stereo mix is measured too.
//   - D83: loudness is measured once and applied as gain.
//   - D100: a command names its target by path and payload hash.
//
// # Where to change
//
//   - measure.go: FFmpeg's arguments and the reading of its summaries. The agent
//     (internal/node/agent) checks the source before calling here.
package loudness
