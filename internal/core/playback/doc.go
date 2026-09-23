// SPDX-License-Identifier: GPL-3.0-or-later

// Package playback turns a decision into something playable and records what was watched: it
// issues single-playback media tokens, asks jobs for prepare work, and keeps play state (positions
// and watched spans, reported with the device's sequence) against the source file. Continue
// Watching, Next Up, hide and undo read from here.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Play state records against the source file, never a prepared rendition. Review-only until
//     M3's tests land.
//
// # Read before changing
//
//   - D157: a position belongs to the File.
//   - D165: the log records state changes and spans; an offline device must not rewind.
//   - D180: a prepared rendition's play state is its source's.
//   - D81: play state is an append-only event log plus a guarded position.
//   - D191: signed-in Watch Together viewing is also recorded.
//   - docs/contributors/design/plans/play-state.md, docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - Media tokens; the playback report endpoint's logic; Continue Watching and Next Up queries.
package playback
