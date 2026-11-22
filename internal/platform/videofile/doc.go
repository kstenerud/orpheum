// SPDX-License-Identifier: AGPL-3.0-or-later

// Package videofile says which files a scan reads as videos and which folders it skips, by their
// names: the one test of it, which the core's scan files a listing by and the node's folder listing
// counts a folder's videos by (D539 §7, AGENTS.md rule 11). What a scan does with a file, and the
// rule it names for one it skips, stay the core's (internal/core/library); this package only names.
//
// # Invariants
//
//   - A video is a file whose extension, in any case, is one of the video extensions or a disc's
//     image, and which is neither an AppleDouble file nor a sample.
//     TestIsVideo_IsAVideoExtensionOrADiscsImageButNoSampleOrAppleDouble.
//   - A scan skips the root's .trash, a sample folder, a folder a NAS or an operating system keeps for
//     itself, and a disc's folder, each at any depth but .trash.
//     TestIsSkippedFolder_IsTheTrashASampleASystemOrADiscsFolder.
//   - The core's scan reads as a video what this package calls one, and skips the folders it does:
//     TestKindOf_ReadsAsAVideoWhatVideofileCallsOne (internal/core/library).
//   - This package imports nothing from the module. Enforced by the depguard rule platform-is-a-leaf.
//
// # Read before changing
//
//   - D360: the extensions a scan reads are the code's, not settings.
//   - D162, D306 §5, D350: samples, a disc's folder and a disc's image.
//   - D539 §7: a machine's folder listing counts each folder's videos.
//
// # Where to change
//
//   - videofile.go.
package videofile
