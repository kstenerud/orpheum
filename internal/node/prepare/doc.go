// SPDX-License-Identifier: GPL-3.0-or-later

// Package prepare runs one FFmpeg command per prepare job, through execx, producing an fMP4 HLS
// rendition in the node's cache (an EVENT playlist that grows while the job runs). Text
// subtitles are internal/node/subtitle's. The output is a cache; a crash restarts the job from
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
//   - The node encodes with Quick Sync, else NVENC, else x264: the first that encodes a test
//     picture here, confined as a prepare is, with the GPU granted (cut-a §6).
//     Enforced by TestChooseEncoder_TakesTheFirstHardwareEncoderThatWorksConfinedOrX264 and
//     TestArguments_AnEncodeUsesTheNodesEncoderAndARemuxCopies.
//   - Each hardware encoder's trial is kept, with FFmpeg's words when it failed, and a prepare
//     says what made its video and FFmpeg's last speed, for the System page (D225 §3).
//     Enforced by TestChooseEncoder_EachTrialSaysWhyItFailedInFFmpegsWords and
//     TestPreparer_SaysWhatMadeTheVideoAndHowFastFFmpegSaidItRan.
//   - The boost raises the centre's share by 6 dB over the standard mix, with the pinned FFmpeg.
//     Enforced by TestPrepare_ARemuxIsHLSWithAStandardAndABoostedMixAndTheBoostRaisesTheCentre
//     (integration tier). The gain is provisional until the listening test sets it (cut-a §6).
//   - A rendition is written only under "<file_id>/<profile>/<run_id>" in the prepared directory.
//     Enforced by the renditionDir pattern, checked before anything is made.
//   - A prepare is watched for a stall (execx.RunWatched), with a deadline from its source's length
//     at the speed the last prepare of its profile ran here, or half of real time before one has.
//     Enforced by TestPreparer_WatchesARunWithADeadlineFromTheSourcesLengthAndTheSpeedLastMeasuredHere.
//   - A rendition's reported size is its regular files, measured by diskspace.TreeBytes as the
//     prepared budget measures it (D217 §5). Enforced by
//     TestPreparer_ARenditionsSizeIsItsRegularFilesAsThePreparedBudgetCountsThem.
//
// # Read before changing
//
//   - D193 §2: prepare, the profiles and scheduling.
//   - D4: the third playback path, background-prepared.
//   - D169: the prepared rendition is a cache.
//   - D180: its play state is its source's.
//   - D217 §3 and §5: every audio track, and the node owns its prepared directory.
//   - D225: the System page shows what the encoder trials found and what each prepare used.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - arguments.go: the FFmpeg command line and the mixes. encoder.go: choosing the video
//     encoder and its arguments. prepare.go: one run, its progress and
//     its size.
package prepare
