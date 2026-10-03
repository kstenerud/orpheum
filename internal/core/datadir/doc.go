// SPDX-License-Identifier: GPL-3.0-or-later

// Package datadir is the only core package that touches the filesystem: a core instance's
// directories, as its top-level config names them (D242 §3). The persistent directory holds what a
// backup keeps (persistent.db, secret.key, the full config); the cache directory what can be
// rebuilt (cache.db, art/); the backups directory the backups and pre-migration snapshots; and the
// run directory what only a running server needs (its lock and the setup code). It never opens a
// path under a media root; media bytes come from the node.
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
//   - Every persistent and run file is reached through os.Root, by a plain name, written
//     atomically with mode 0600. Enforced by TestPersistentAndRunFiles_RefuseAnythingButAPlainName
//     and TestWritePersistentFile_ReplacesAtomicallyWithMode0600.
//   - A running server holds the instance's lock, so restore cannot replace its database.
//     Enforced by TestLockInstance_RefusesASecondHolder.
//   - A persistent directory on a network filesystem is refused (D31). Enforced by
//     TestRefuseNetworkFilesystemType_RefusesNetworkFilesystems.
//
// # Read before changing
//
//   - D193 §1: the core never opens a media file.
//   - D194 §3 and D198: secret.key lives in the persistent directory, mode 0600, never in the
//     database.
//   - D242: an instance is one directory; what each of its directories holds.
//   - D31: refuse to run with the database on a network filesystem.
//   - principles §2: the user's files are not ours to touch.
//   - docs/contributors/design/plans/cut-a.md §2.
//
// # Where to change
//
//   - datadir.go: the layout (persistent.db, cache.db, the backups, persistent and run files) and
//     atomic writes.
//   - art.go: the artwork store under the cache directory. lock_unix.go: the instance's lock. The
//     sockets are internal/platform/unixsocket's, since the node listens on one too.
//     netfs_*.go: the network-filesystem refusal.
package datadir
