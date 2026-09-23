// SPDX-License-Identifier: GPL-3.0-or-later

// Package datadir is the only core package that touches the filesystem: the state directory
// (/var/lib/orpheum: library.db, secret.key, TLS and ACME keys) and the cache directory
// (/var/cache/orpheum: cache.db, art/). It never opens a path under a media root; media bytes come
// from the node.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - It is the only core package allowed to open, create or remove files. Enforced by the
//     forbidigo rules tagged [core-fs] and [delete], whose path exceptions name this package.
//   - Errors from the filesystem are wrapped where they enter. Enforced by wrapcheck, which is
//     scoped to this package.
//
// # Read before changing
//
//   - D193 §1: the core never opens a media file.
//   - D194 §3: secret.key lives beside the configuration, mode 0600, never in the database.
//   - D31: refuse to run with the database on a network filesystem.
//   - principles §2: the user's files are not ours to touch.
//   - docs/contributors/design/plans/cut-a.md §2.
//
// # Where to change
//
//   - Directory layout and permissions; the network-filesystem refusal; key files.
package datadir
