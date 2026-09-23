// SPDX-License-Identifier: GPL-3.0-or-later

// Package backup writes and restores backups of library.db with VACUUM INTO, the pre-migration
// snapshots, and orpheum restore (a command-line operation with the server stopped, never an
// endpoint). A database backup never contains secret.key; restoring without the key marks sealed
// secrets as needing re-entry and loses nothing else.
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
//   - A restore refuses a file without the application_id and one whose compatible_version is
//     newer. Review-only until M1's tests land.
//
// # Read before changing
//
//   - D77: back up with VACUUM INTO and test the restore.
//   - D181: the migration policy and its snapshots.
//   - D185: the database is backed up whole.
//   - D194 §4: backups and the key.
//   - docs/contributors/design/plans/backup-restore.md.
//
// # Where to change
//
//   - Backup scheduling and pruning; the restore command's checks.
package backup
