// SPDX-License-Identifier: GPL-3.0-or-later

// Package container reads the structure of a video's container without reading its media: the
// elements of a Matroska file and the boxes of an MP4, through an io.ReaderAt, so the caller
// decides how the file is opened. The file's identity (internal/node/hash) is anchored by them.
// Every read is bounded: a hostile file cannot make the node read or walk without end.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//   - The file's identity reads only what anchoring it needs (internal/node/hash's invariants and
//     tests, which read through these readers).
//
// # Read before changing
//
//   - D179: identity is a hash of the media payload.
//
// # Where to change
//
//   - read.go: bounded reads; ebml.go: Matroska elements; mp4.go: MP4 boxes.
package container
