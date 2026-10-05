// SPDX-License-Identifier: GPL-3.0-or-later

// Package chunks cuts a film into chunks made when a player first asks for one, so a slow link
// gets lower qualities hls.js switches between by itself (D247 §10, chunks.md). It holds, for now,
// the grid: a film's chunk boundaries, its main picture's keyframes, which a read-through keeps in
// cache.db (internal/core/readthrough), merged so each chunk is at least hls_segment_length_seconds
// long. The node's making of chunks, the playlists and the player's move to them come after
// (chunks.md §5).
//
// # Invariants
//
//   - A grid starts at the picture's first keyframe, and each next start is the first keyframe at
//     least the segment length past the last; a last chunk shorter than that is merged into the one
//     before, and a picture with no keyframe before its end cannot be cut (ErrCannotBeCut).
//     Enforced by TestGridOf_MergesKeyframesGreedilyIntoChunksOfAtLeastTheLength and
//     TestGridOf_APictureWithNoKeyframeBeforeItsEndCannotBeCut.
//
//   - A film's grid is its main picture's timeline, a cover's never, by its bytes' payload hash;
//     while there is none it says so without an error, and a film FFmpeg could not read through,
//     or with no picture, cannot be cut. Enforced by
//     TestGrid_IsTheMainPicturesKeptTimelineMergedToTheSegmentLength and
//     TestGrid_SaysThereIsNoneYetOrThatTheFilmCannotBeCut.
//
//   - It reads the timelines and never writes them: internal/core/readthrough writes
//     video_timeline. Enforced by TestArch_EachTableHasOneWritingPackage.
//
//   - Imports nothing from internal/node, never os/exec, no database/sql: the depguard rules
//     core-boundary and sql-only-in-store, TestArch_CoreNeverImportsNode and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - D247 §10: chunks cut at the source's keyframes, made on request.
//   - docs/contributors/design/plans/chunks.md §1 (the words) and §3 (the core).
//   - docs/contributors/design/research/slow-links.md §10: the measured recipe.
//
// # Where to change
//
//   - grid.go: GridOf, the merging of keyframes, and Service.Grid, a film's grid from its timeline.
package chunks
