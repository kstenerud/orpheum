// SPDX-License-Identifier: AGPL-3.0-or-later

// Package watch watches the library folders the core names, each a configured root or a folder inside
// one (D359), for changes with the kernel's inotify and notices, per folder, the folder holding every
// change once it has gone quiet, for the core to scan (D27, D272 §9). It is a sensor: it reads no file
// and decides nothing; the core decides what to watch and what to scan, and each folder's periodic
// scan stays beneath it, since a watcher can miss a change and never say so. Below, "a root" is a
// library folder it was given.
//
// # Invariants
//
//   - It watches exactly the folders the core names, and nothing else: a configured root's folders
//     no library uses are never walked or watched, a folder named is watched from then, and one no
//     longer named is stopped, its watches released and its changes not yet noticed dropped; each
//     start and stop is logged at info with its count of watches. Each folder's watch is in the
//     status, said not started until the kernel layer starts it (D359's amendment of 2026-10-27).
//     TestKernel_WatchesOnlyTheLibraryFoldersItIsGivenNeverTheRestOfTheirRoot,
//     TestKernel_AFolderGivenIsWatchedAndOneNoLongerGivenIsNot,
//     TestWatcher_SaysEachLibraryFoldersWatchAndOnlyTheFoldersItIsGiven.
//   - A folder no configured root holds is not watched, whatever the core names, and says why
//     (D107). TestKernel_AFolderNoConfiguredRootHoldsIsNotWatchedAndSaysWhy.
//   - A root's changes are noticed once, as the folder holding them all, when the root has gone
//     node.watch_quiet_seconds without one, or after maxHoldQuietSpells of them while changes keep
//     coming. TestWatcher_NoticesTheFolderHoldingARootsChangesOnceItGoesQuiet,
//     TestWatcher_NoticesAChangeThatNeverPausesAfterTheLongestHold.
//   - A folder made, moved in, written to, moved out or removed is noticed by the folder it is in,
//     and a folder made or moved in is watched with everything below it.
//     TestKernel_NoticesWhatChangesInEveryFolderAndInFoldersMadeSince.
//   - Lost events (the kernel's queue overflowed) notice every root whole.
//     TestKernel_AnOverflowNoticesEveryRootWhole.
//   - A root on a network filesystem is not watched, and says so (D27): changes made on another
//     machine never reach this one's kernel. A root that cannot be watched whole, as at the kernel's
//     watch limit, is not watched at all, and says why: partly watched, it would look watched.
//     TestKernel_ARootOnANetworkFilesystemIsNotWatchedAndSaysWhy,
//     TestKernel_ARootThatCannotBeWatchedWholeIsNotWatchedAndSaysWhy.
//   - A notice names a folder in UTF-8, the wire's text: one whose name is not is noticed by the
//     nearest folder above it. TestCleanFolder_IsRelativeCleanAndUTF8.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary.
//   - Lists folders only through the read-only confine.MediaRoot. Enforced by the forbidigo rule
//     tagged [media-root].
//
// # Read before changing
//
//   - D359's amendment of 2026-10-27: the node watches the library folders, never whole configured
//     roots.
//   - D27: change detection is a strategy that says which it is using; watchers fail silently.
//   - D272 §9: a watched change is a scan of the folder that changed, after a quiet spell.
//   - docs/contributors/design/plans/scanner.md, "Watching".
//
// # Where to change
//
//   - watcher.go: the quiet spell and the notices; inotify_linux.go: the kernel's watches.
package watch
