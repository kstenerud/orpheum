// SPDX-License-Identifier: AGPL-3.0-or-later

// Package backup writes and restores backups of an instance's persistent directory (D242 §5): a
// tarball in the backups directory of its files, its database a VACUUM INTO snapshot. A backup is
// made by orpheum backup, or by an admin from the admin interface (Service); orpheum restore is a
// command-line operation with the server stopped, never an endpoint, and first keeps a backup of
// what it replaces. The pre-migration snapshots are the store's migration runner's, not this
// package's. A backup holds the server's keys only when asked for: secret.key, the identity key, and
// the certificates' and ACME account's private keys (D289 §2); restoring without them marks sealed
// secrets as needing re-entry, and the core is issued a new certificate.
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
//   - A restore refuses a file without the application_id and one whose compatible_version is
//     newer. Enforced by TestMakePlan_RefusesAForeignOrNewerBackup.
//   - A restore refuses a backup below the baseline schema, which the server would refuse at
//     start (D228). Enforced by TestMakePlan_RefusesABackupOlderThanTheBaseline.
//   - A restore says what it will overwrite, keeps it, and runs only with the server stopped; its
//     plan changes nothing. It takes a backup, or the database snapshot the migration runner keeps
//     before it migrates, which replaces the database alone (D181).
//     TestRestore_TakesTheDatabaseSnapshotKeptBeforeAMigration, Enforced by TestRestore_KeepsTheDatabaseItReplaces,
//     TestRestore_RefusesWhileTheServerRuns and TestMakePlan_LeavesNothingBehind.
//   - A backup holds ciphertext only, and the server's keys (secret.key, the identity key, the
//     certificates' and ACME account's private keys) only when asked for; restoring without them
//     marks secrets as needing re-entry and loses nothing else. Enforced by
//     TestWrite_LeavesEveryPrivateKeyOutUnlessAskedForTheKeys, TestWrite_RefusesToIncludeAKeyThereIsNot,
//     TestRestore_WithoutTheKeyMarksSecretsNeedingReentryAndLosesNothingElse and
//     TestRestore_WithTheIncludedKeyOpensSecrets; every table's copy, by the store's
//     TestBackup_ARestoredCopyReproducesEveryTable.
//   - A backup whose database copy fails its integrity check is refused and leaves nothing behind
//     (D303 §7, M6). Enforced by TestWrite_RefusesADamagedCopyAndLeavesNothingBehind.
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
//   - A backup is a job, a person's or the schedule's: the schedule's is asked for at
//     backup_hour_local_tz in the server's time zone once the newest backup is backup_every_days old, at
//     once when it is a day overdue, and never holds secret.key; past backups_kept, the oldest
//     backups are pruned, and the copies kept before a restore or a migration never are (D243 §11,
//     D246 §7). TestQueueScheduled_AsksAtTheQuietHourWhenDueAndAtOnceWhenOverdue,
//     TestQueueScheduled_TheHourIsTheServersAndTheGapIsInDays,
//     TestPrune_KeepsTheNewestBackupsAndEveryCopyKeptBeforeARestoreOrMigration,
//     TestJobArgsMerge_HoldsTheKeyWhenEitherAskedAndARunningOneCoversWhatItHolds.
//
// # Where to change
//
//   - backup.go: Write (orpheum backup, --include-key, and Service.Make for the admin interface),
//     MakePlan and Restore (orpheum restore). The archive itself is internal/core/instancedir's
//     (archive.go), the only core package that touches files.
//   - jobs.go: a backup as a job (D243 §11): a person's request, the scheduled one the sweep asks
//     for, the handler, which prunes past backups_kept, and how two requests merge.
package backup
