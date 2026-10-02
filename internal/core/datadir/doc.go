// SPDX-License-Identifier: GPL-3.0-or-later

// Package datadir is the only core package that touches the filesystem: the state directory
// (/var/lib/orpheum: persistent.db, secret.key, TLS and ACME keys) and the cache directory
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
//   - Every state file is reached through os.Root, by a plain name, written atomically with mode
//     0600. Enforced by TestStateFiles_RefuseAnythingButAPlainName and
//     TestWriteStateFile_ReplacesAtomicallyWithMode0600.
//   - A running server holds the state lock, so restore cannot replace its database. Enforced by
//     TestLockState_RefusesASecondHolder.
//   - A state directory on a network filesystem is refused (D31). Enforced by
//     TestRefuseNetworkFilesystemType_RefusesNetworkFilesystems.
//
// # Read before changing
//
//   - D193 §1: the core never opens a media file.
//   - D194 §3 and D198: secret.key lives in the state directory, mode 0600, never in the database.
//   - D31: refuse to run with the database on a network filesystem.
//   - principles §2: the user's files are not ours to touch.
//   - docs/contributors/design/plans/cut-a.md §2.
//
// # Where to change
//
//   - datadir.go: the layout (persistent.db, cache.db, backups/, state files) and atomic writes.
//   - art.go: the artwork store under the cache directory. lock_unix.go: the state lock. The
//     sockets are internal/platform/unixsocket's, since the node listens on one too.
//     netfs_*.go: the network-filesystem refusal.
package datadir
