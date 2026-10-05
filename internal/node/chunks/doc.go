// SPDX-License-Identifier: GPL-3.0-or-later

// Package chunks makes runs of chunks: one stream of a film, the copied picture, a quality or a
// sound, cut at the film's grid and written into a store in the node's prepared directory, chunk by
// chunk, each at its time in the film (D247 §10; plans/chunks.md §2). A run is one FFmpeg command
// through execx, whose segment muxer writes raw fragmented MP4 files; each, once FFmpeg lists it
// finished, is split into the store's init and the chunk, its decode times moved to the film's.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//   - Removes, renames and truncates nothing itself: the store, its files and the raw files are
//     the prepared directory's keeper's (prepared.Shelf). Enforced by the forbidigo rule tagged
//     [delete].
//   - A run reads its chunks and cuts at each bound after its first and at its end, a picture a
//     little early so its keyframe is not missed; an encode keyframes at its chunks' starts and
//     nowhere else; a sound past the film's first chunk starts a chunk early. Enforced by
//     TestArguments_ReadTheRunsChunksAndCutAtEachAndAtTheEnd,
//     TestArguments_EncodeAQualityWithKeyframesAtItsChunksStarts and
//     TestArguments_StartASoundAChunkEarlyAndCutItAtTheBounds.
//   - Each chunk is put in its store whole, as FFmpeg finishes it, at its time in the film plus
//     TimelineOffsetMs: a picture's shown at its bound, a sound's after the run's samples before
//     it less the encoder's priming; a lead-in and what FFmpeg wrote past the run are dropped.
//     Enforced by TestMake_PutsEachChunkAtItsTimeAsFFmpegFinishesIt and
//     TestMake_TimesASoundBySamplesAfterItsLeadIn; with the pinned FFmpeg, two runs' chunks join
//     into one stream at the film's times, by TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes
//     (integration tier).
//   - A run not of a run's shape is refused before anything is made. Enforced by
//     TestMake_RefusesARunNotOfARunsShape.
//   - What is not a raw file of the muxer's is refused, never read past its end. Enforced by
//     TestFragments_RefuseWhatIsNotARawFile.
//
// # Read before changing
//
//   - D247 §10: chunks made on request, cut at the source's keyframes.
//   - docs/contributors/design/research/slow-links.md §10: the recipe, measured.
//   - docs/contributors/design/plans/chunks.md: the build plan.
//   - D217 §5: the node owns its prepared directory and its budget.
//
// # Where to change
//
//   - arguments.go: the FFmpeg command line. maker.go: one run, its raw files and their times.
//     fmp4.go: reading and retiming fragmented MP4.
package chunks
