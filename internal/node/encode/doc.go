// SPDX-License-Identifier: AGPL-3.0-or-later

// Package encode is how the node encodes a picture and mixes a sound: the H.264 encoder it chose by
// trying each hardware one at startup, its arguments for a chunk's quality, and the stereo mixes of
// a track that a chunk of its sound is made with and its loudness measured as. It runs nothing but
// the encoders' trials; internal/node/chunks runs the encodes and internal/node/readthrough the
// measurements.
//
// # Invariants
//
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and,
//     transitively, TestArch_NodeNeverImportsCore.
//
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//
//   - The node encodes with Quick Sync, else NVENC, else VAAPI, else x264: the first that encodes a
//     test picture here, confined as a run of chunks is, with the GPU granted (cut-a §6, D332), each
//     trial kept with FFmpeg's words when it failed, for the System page (D225 §3). Enforced by
//     TestChooseEncoder_TakesTheFirstHardwareEncoderThatWorksConfinedOrX264,
//     TestChooseEncoder_EachTrialSaysWhyItFailedInFFmpegsWords and
//     TestChooseEncoder_AnAMDCardEncodesThroughVAAPIAfterQuickSyncAndNVENC.
//
//   - The node's encode speed is FFmpeg's own speed for the chosen encoder making the lowest quality,
//     400 kb/s at 360 lines, from a generated 1080p picture, in percent of real time: the last it said,
//     though the trial was cut short, and none when it said none (D457). Enforced by
//     TestMeasureSpeed_IsFFmpegsLastSpeedEncodingTheLowestQuality and
//     TestMeasureSpeed_ATrialCutShortGivesTheSpeedItReachedAndOneThatSaidNoneFails.
//
//   - A quality is 8-bit H.264 with keyframes where its chunks start and nowhere else, IDR pictures
//     on every encoder, its times counted from its first picture (startAtZero). Enforced by
//     TestQualityArguments_OnVAAPIScaleThenUploadWithTheChunksKeyframes and
//     TestQualityArguments_ForcedKeyframesAreIDRsOnQuickSyncAndNVENC.
//
//   - On VAAPI a quality of a codec FFmpeg decodes through VAAPI may be decoded and scaled on the GPU,
//     the picture kept on the card (the owner, 2026-11-27): decoded by VAAPI on the run's device and
//     scaled by scale_vaapi to the same -2 width, as NV12, with no upload, its rates, keyframes and
//     everything else the same as decoded on the CPU; decoded on the CPU, every encoder's quality is
//     as before, and Quick Sync, NVENC and x264 never decode on the GPU. Enforced by
//     TestQualityArguments_OnVAAPIDecodedOnTheGPUScaleOnTheCard.
//
//   - A track with a centre has a standard and a boosted stereo mix, one without its one mix, each
//     carrying the track's levelling gain (D217 §3, D226 §4). Enforced by
//     TestMixFilter_ATrackWithACentreHasAStandardAndABoostedMix and
//     TestMixFilter_EachMixOfAMeasuredTrackCarriesItsLevellingGain. The boost's gain is provisional
//     until the listening test sets it (cut-a §6).
//
// # Read before changing
//
//   - cut-a §6 and D332: the encoders and their order.
//   - D225 §3: the System page shows what the encoder trials found.
//   - D217 §3 and D226 §4: the mixes, and the gain each carries.
//   - D344: these were the whole-film prepare's, which is retired; chunks use them now.
//
// # Where to change
//
//   - encoder.go: the encoders, their trial and a quality's arguments. mix.go: a track's mixes.
package encode
