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
//     in the system folder, granted that folder alone. A video with no recipe, or a file name that is
//     a path rather than a bare name, is refused without a run. Enforced by
//     TestMake_FFmpegDrawsTheClipIntoTheSystemFolderAlone, TestMake_AVideoNotKnownIsRefusedWithoutARun
//     and TestMake_AFileNameThatIsAPathIsRefusedWithoutARun.
//
//   - A video whose file is there is kept, unless a rebuild asks for it again (D435 §7). Enforced by
//     TestMake_AVideoThereIsKeptAndARebuildMakesItAgain.
//
// # Read before changing
//
//   - D435: the server's own videos live in one hidden system library.
//   - D26: every FFmpeg run is confined.
//
// # Where to change
//
//   - systemvideo.go: recipes holds each video's FFmpeg arguments; a new video adds its purpose or
//     codec to internal/nodeproto, and to the core's list (internal/core/library, systemvideos.go),
//     which names its file. The agent (internal/node/agent) runs Make for
//     the makeSystemVideos command.
package systemvideo
