// SPDX-License-Identifier: GPL-3.0-or-later

// Package turn will run the TURN server embedded in the binary (pion/turn) that carries Watch
// Together voice, and issue short-lived TURN credentials derived from the sealed shared secret. It
// is built in M4 (cut-a §8); for now it holds only this doc.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Derives credentials with secrets.Value.Reveal and never logs or returns the shared secret.
//     The first half is permitted by the forbidigo rule tagged [reveal]; the second is checked by
//     the canary test (D194).
//
// # Read before changing
//
//   - D68: the media server is already the relay; TURN is embedded.
//   - D63: voice is WebRTC.
//   - D193 §4: browser WebRTC through embedded TURN; the ladder is deleted.
//   - D194: the TURN shared secret is sealed; rotating it is free.
//   - docs/contributors/design/plans/watch-together.md.
//
// # Where to change
//
//   - The TURN listener's lifecycle; credential derivation for GET /together/{id}/ice.
package turn
