// SPDX-License-Identifier: AGPL-3.0-or-later

// Package systemvideo makes the server's own videos (D435 §4): short clips FFmpeg draws from nothing,
// by a recipe this build knows, into the node's system folder, which the node lists as one of its
// roots so the core scans and plays them as any video. The device lab's sound timing clip is the
// first.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx and TestArch_OnlyExecxImportsOsExec.
//
//   - FFmpeg reads nothing: its inputs are lavfi's generated sources, and it writes only the one file
//     in the system folder, granted that folder alone. A video not of SystemVideos is refused
//     without a run. Enforced by TestMake_FFmpegDrawsTheClipIntoTheSystemFolderAlone and
//     TestMake_AVideoNotKnownIsRefusedWithoutARun.
//
//   - A video whose file is there is kept, unless a rebuild asks for it again (D435 §7). Enforced by
//     TestMake_AVideoThereIsKeptAndARebuildMakesItAgain.
//
// # Read before changing
//
//   - D435: the server's own videos live in one hidden system library.
//   - D26: every FFmpeg run is confined.
package systemvideo
