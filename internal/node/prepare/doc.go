// SPDX-License-Identifier: GPL-3.0-or-later

// Package prepare runs one FFmpeg command per prepare job, through execx, producing an fMP4 HLS
// rendition in the node's cache (an EVENT playlist that grows while the job runs), plus
// extract_subtitle jobs for text tracks. The output is a cache; a crash restarts the job from
// zero.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//   - A rendition is fMP4 HLS with an EVENT playlist, every source audio track carried, and a
//     boosted mix beside the standard one for a track with a centre (D217 §3). Enforced by
//     TestArguments_CarryEveryTrackWithABoostedMixWhereThereIsACentre.
//   - Copied HEVC is tagged hvc1; h264-8m is 8-bit H.264 with a keyframe every two seconds.
//     Enforced by TestArguments_TagCopiedHEVCForSafariAndReEncodeTo8BitH264.
//   - The boost raises the centre's share by 6 dB over the standard mix, with the pinned FFmpeg.
//     Enforced by TestPrepare_ARemuxIsHLSWithAStandardAndABoostedMixAndTheBoostRaisesTheCentre
//     (integration tier). The gain is provisional until the listening test sets it (cut-a §6).
//   - A rendition is written only under "<file_id>/<profile>" in the prepared directory.
//     Enforced by the renditionDir pattern, checked before anything is made.
//
// # Read before changing
//
//   - D193 §2: prepare, the profiles and scheduling.
//   - D4: the third playback path, background-prepared.
//   - D169: the prepared rendition is a cache.
//   - D180: its play state is its source's.
//   - D217 §3 and §5: every audio track, and the node owns its prepared directory.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - arguments.go: the FFmpeg command line and the mixes. prepare.go: one run, its progress and
//     its size. Subtitle extraction comes with the subtitle slice.
package prepare
