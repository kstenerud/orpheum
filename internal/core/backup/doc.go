// SPDX-License-Identifier: GPL-3.0-or-later

// Package backup writes and restores backups of an instance's persistent directory (D242 §5): a
// tarball in the backups directory of its files, its database a VACUUM INTO snapshot. A backup is
// made by orpheum backup, or by an admin from the admin interface (Service); orpheum restore is a
// command-line operation with the server stopped, never an endpoint, and first keeps a backup of
// what it replaces. The pre-migration snapshots are the store's migration runner's, not this
// package's. A backup holds secret.key only when asked for; restoring without the key marks sealed
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
//     newer. Enforced by TestMakePlan_RefusesAForeignOrNewerBackup.
//   - A restore refuses a backup below the baseline schema, which the server would refuse at
//     start (D228). Enforced by TestMakePlan_RefusesABackupOlderThanTheBaseline.
//   - A restore says what it will overwrite, keeps it, and runs only with the server stopped; its
//     plan changes nothing. It takes a backup, or the database snapshot the migration runner keeps
//     before it migrates, which replaces the database alone (D181).
//     TestRestore_TakesTheDatabaseSnapshotKeptBeforeAMigration, Enforced by TestRestore_KeepsTheDatabaseItReplaces,
//     TestRestore_RefusesWhileTheServerRuns and TestMakePlan_LeavesNothingBehind.
//   - A backup holds ciphertext only, and secret.key only when asked for; restoring without the
//     key marks secrets as needing re-entry and loses nothing else. Enforced by
//     TestWrite_RefusesToIncludeAKeyThereIsNot,
//     TestRestore_WithoutTheKeyMarksSecretsNeedingReentryAndLosesNothingElse and
//     TestRestore_WithTheIncludedKeyOpensSecrets; every table's copy, by the store's
//     TestBackup_ARestoredCopyReproducesEveryTable.
//
// # Read before changing
//
//   - D77: back up with VACUUM INTO and test the restore.
//   - D181: the migration policy and its snapshots.
//   - D185: the database is backed up whole.
//   - D228: a schema below the baseline cannot be migrated.
//   - D194 §4: backups and the key.
//   - D242 §5: a backup is a tarball of the persistent directory.
//   - docs/contributors/design/plans/backup-restore.md.
//
// # Where to change
//
//   - backup.go: Write (orpheum backup, --include-key, and Service.Make for the admin interface),
//     MakePlan and Restore (orpheum restore). The archive itself is internal/core/datadir's
//     (archive.go), the only core package that touches files.
//     Scheduled backups and pruning are backup jobs, which come in M4 (D223 §6).
package backup
