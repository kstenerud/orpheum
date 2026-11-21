// SPDX-License-Identifier: AGPL-3.0-or-later

// Package systemvideos keeps the server's own videos (D435): the few clips the server plays for
// itself, such as the sound timing clip, held in one hidden system library on a connected node's
// system folder. A use finds a video by its purpose and codec; the first use makes the library and
// queues a make_system_videos job, whose handler asks the node to make them and scans the folder
// once made. It reads and changes the library through internal/core/library, which alone writes the
// library tables; the API's system video routes call it.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//
//   - Opens no file and walks no directory; the videos are made by the node. Enforced by the
//     forbidigo rule tagged [core-fs].
//
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//
//   - It writes no table: the system library is made through library.CreateSystemLibrary, given a
//     folder through library.Admin.AddRoot and moved through library.Admin.Repoint (D429).
//     TestArch_EachTableHasOneWritingPackage.
//
//   - The first use of a system video makes the system library over a connected node's system
//     folder and asks that node to make the videos; what it made is scanned, and the next use finds
//     the file. A video a scan finds gone is made again on its next use. With no connected node that
//     has a system folder the videos are unavailable and no library is made; when the library's node
//     is gone, the next use moves it to another node's system folder and makes them there (D435
//     §4–§6, D510). TestSystemVideos_TheFirstUseMakesTheLibraryAndTheNodeMakesTheVideos,
//     TestSystemVideos_AVideoRemovedFromItsFolderIsMadeAgain,
//     TestSystemVideos_NoNodeToHoldThemIsUnavailable, TestSystemVideos_ANodeGoneMovesTheLibraryToAnother.
//
//   - Any signed-in person may play a system video, and its playback names no library (D435 §1, §6).
//     TestReachFile_ASystemVideoIsReachedByAnyoneAndNothingElseIs,
//     TestWorkOfFile_ASystemVideoNamesNoLibrary.
//
// # Read before changing
//
//   - D435: the server's own videos live in one hidden system library, found by its kind, made on
//     first use.
//   - D510: a system video removed from its folder is made again.
//   - D429: a library folder moves only by a repoint.
//
// # Where to change
//
//   - systemvideos.go: the list of videos, finding one, the library's placement on a node, and the
//     make job's handler and assignee. A new video adds its recipe to internal/node/systemvideo too.
package systemvideos
