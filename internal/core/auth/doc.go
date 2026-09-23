// SPDX-License-Identifier: GPL-3.0-or-later

// Package auth is who may do what: local accounts, argon2id password hashes, sessions, invites,
// and the single grant table from which every credential (session, invite, join, cast) is an
// opaque, revocable, hashed row. There are two roles, admin and user; a Watch Together guest is
// not a principal.
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
//   - A password is stored only as an argon2id PHC string. Review-only until M1's tests land; the
//     canary test (D194) then checks nothing stores or logs it.
//
// # Read before changing
//
//   - D125: access is role-based with two roles; a Watch Together guest is not a principal.
//   - D154: one grant table, opaque and revocable.
//   - D168: entitlement is resolved at the point of use, never pushed.
//   - D194 §2: argon2id with RFC 9106's second parameters; re-hash on login; rate-limit failures.
//   - docs/contributors/design/plans/auth.md.
//
// # Where to change
//
//   - Password hashing and its parameters; the grant table's issue, verify and revoke; the invite
//     flow of cut-a §5 (Setup and auth).
package auth
