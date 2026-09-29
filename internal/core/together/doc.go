// SPDX-License-Identifier: GPL-3.0-or-later

// Package together will be Watch Together, built in M4 (cut-a §8); for now it holds only this
// doc. It is to hold the session, its link and members, one WebSocket per device carrying the
// messages of cut-a §5.1, the group state machine (Idle, Waiting, Paused, Playing), and voice
// signalling. The session never depends on an account; a signed-in member's viewing is also
// recorded. Sessions live in memory and end with the process.
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
//   - Every switch over a message type or group state names every value. Enforced by exhaustive.
//   - Every goroutine has an owner and ends when its context ends. Review-only (standards/go.md,
//     Concurrency); the race detector runs in make test-all.
//
// # Read before changing
//
//   - D62: Watch Together is first-class; the client surface is six methods.
//   - D66: no clock synchronisation.
//   - D117: the session link is the grant; revocation is the control.
//   - D119: a join link admits; membership persists; rotation, ejection, a cap.
//   - D191: the session behaves as if nobody has an account.
//   - docs/contributors/design/plans/watch-together.md, docs/contributors/design/plans/cut-a.md
//     §5.1.
//
// # Where to change
//
//   - The session and membership; the message types; the group state machine; conflict resolution
//     by arrival order.
package together
