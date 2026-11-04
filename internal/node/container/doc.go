// SPDX-License-Identifier: AGPL-3.0-or-later

// Package container reads the structure of a video's container without reading its media: the
// elements of a Matroska file and the boxes of an MP4, through an io.ReaderAt, so the caller
// decides how the file is opened. The file's identity (internal/node/hash) is anchored by them, and
// a film's keyframes are read from their indexes (KeyframesMs, D259 §2). A Matroska file's cues may
// list fewer keyframes than its picture has, a muxer's choice: every one listed is a keyframe.
// Every read is bounded: a hostile file cannot make the node read or walk without end.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//   - The file's identity reads only what anchoring it needs (internal/node/hash's invariants and
//     tests, which read through these readers).
//   - A film's keyframes from a Matroska file's cues on its picture's track, or from an MP4's sync
//     samples with their composition offsets and edit, are times FFmpeg shows keyframes at; a file
//     with no index says so (ErrNoIndex), and no input panics (D259 §2). A sync sample before the
//     edit's start counts at the start when no other is at or before it, as the read-through counts
//     a keyframe flagged discarded (StartingKeyframes; the 11-06 batch's item 1).
//     TestKeyframesMs_AnMP4sSyncSampleBeforeItsEditsStartIsAtTheStart,
//     TestKeyframesMs_MatroskaCuesAreThePicturesKeyframes,
//     TestKeyframesMs_MP4SyncSamplesAreThePicturesKeyframes, TestKeyframesMs_AFileWithNoIndexSaysSo,
//     FuzzKeyframesMs, and against ffprobe on files the pinned tools make,
//     TestKeyframesMs_AreTheKeyframesFFprobeShowsInRealFiles.
//   - A file is read from a pipe only when its container reads in one pass from its start: Matroska,
//     an MPEG transport stream, or an MP4 whose index comes before its media; anything else, or a
//     file it cannot read, is read as a file (D329 §7).
//     TestReadsStraightThrough_OnlyContainersReadInOnePassFromTheirStart.
//
// # Read before changing
//
//   - D179: identity is a hash of the media payload.
//   - D259: a film's cut points from its file's own index.
//
// # Where to change
//
//   - straight.go: whether a file reads in one pass from its start.
//   - read.go: bounded reads; ebml.go: Matroska elements; mp4.go: MP4 boxes; keyframes.go: a
//     picture's keyframes from either's index.
package container
