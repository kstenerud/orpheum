// SPDX-License-Identifier: GPL-3.0-or-later

// Package store is the database: the one Open() for library.db and cache.db with its pragmas read
// back at startup, the single writer connection and the reader pool, our migration runner, the
// sqlc-generated queries in sqlcgen, and the one hand-written filter builder for search. It knows
// tables, not features.
//
// # Invariants
//
//   - It is the only package that imports database/sql or the SQLite driver. Enforced by the
//     depguard rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - It imports nothing from internal/core or internal/node. Enforced by the depguard rule store-
//     is-below-core.
//   - SQL is never built by string concatenation. Enforced by gosec (G201, G202).
//   - Every query names its columns; no SELECT * or RETURNING *. Enforced by
//     TestArch_QueriesNameEveryColumn.
//   - Rows are closed and their errors checked. Enforced by sqlclosecheck and rowserrcheck.
//   - Generated code in sqlcgen is never edited by hand. Enforced by make check-generated and the
//     pre-edit hook.
//   - Generated code has no untyped value (interface{} or any) outside DBTX. Enforced by
//     TestGeneratedQueries_HaveNoUntypedValues.
//   - Every connection's pragmas read back as set, on the writer and the readers, or Open refuses.
//     Enforced by TestVerifyPragmas_RefusesAConnectionWithoutThem.
//   - A non-empty file without our application_id is refused, never migrated. Enforced by
//     TestOpen_RefusesAFileThatIsNotOurs.
//   - Writes go through Write on the one writer; the reader pool is query_only. Enforced by
//     TestRead_RefusesAWrite and TestWrite_RollsBackWhenTheChangeFails.
//   - A fresh schema equals testdata/golden/schema-vNNNN.sql for the current version. Enforced by
//     TestMigrate_FreshSchemaEqualsTheGolden.
//   - Each step commits alone, so an interrupted migration resumes and ends identical; a step
//     that fails rolls back whole. Enforced by TestMigrate_ResumesAfterAnInterruption and
//     TestMigrate_RollsBackAStepThatFailsHalfway.
//   - The runner refuses a step that leaves a dangling foreign key, reissues an id or breaks a
//     view. Enforced by TestMigrate_RefusesAStepThatBreaksWhatSQLCannotCheck.
//   - A newer database is refused unless it declared itself compatible, and the refusal names
//     the snapshot. Enforced by TestMigrate_RefusesANewerDatabaseAndNamesTheSnapshot.
//   - The pre-migration snapshot survives success. Enforced by
//     TestMigrate_KeepsTheSnapshotAfterSuccess.
//   - A committed step is never edited, renamed or deleted. Enforced by the pre-edit hook and
//     TestSteps_MatchTheCommittedChecksums.
//   - Each applied step's checksum is recorded, and a database where an applied step has since
//     changed is refused (plan-deviations P17).
//     TestMigrate_RecordsEveryStepsChecksumAndRefusesOneThatChanged.
//   - cache.db is built from one schema file, cache/schema.sql, and versioned by discard: a file
//     from another version is reported as outdated for the caller to delete, never migrated.
//     Enforced by TestOpenCache_ReportsAnotherVersionAsOutdated.
//   - cache.db carries its own application_id, so library.db is never taken for a cache and
//     discarded. Enforced by TestOpenCache_RefusesLibraryDB.
//   - Cache queries are generated into cachegen, apart from sqlcgen, so a library query cannot
//     name a cache table (M2-1). Enforced by sqlc.yaml's two entries.
//   - Every data-transforming step has a test on its rows: step 0002 in
//     TestMigrate_Step0002MarksTheSettingsAPersonSetAsModified, step 0007 in
//     TestMigrate_Step0007KeepsEveryStoredResponseAsItsItemsOwn, step 0011 in
//     TestMigrate_Step0011KeysEveryFileByItsReportedBytes.
//   - Open and the runner take their choices (the reader pool minimum, the free-space factor) from
//     the caller and refuse zero, never a value from here (D201). Enforced by
//     TestOpen_RefusesOptionsNobodyChose.
//
// # Read before changing
//
//   - D6: sqlc, no ORM.
//   - D31: SQLite, pure-Go driver, WAL, pragmas read back (as refined by D193).
//   - D181: the migration policy and our small runner.
//   - D185: the schema as SQL that runs.
//   - D173: tombstones, not deletes; live_* views.
//   - docs/contributors/design/plans/schema.md, docs/contributors/design/plans/migrations.md.
//   - docs/contributors/standards/sql.md.
//
// # Where to change
//
//   - migrations/: append-only numbered SQL files. A new step adds its line to
//     migrations/SHA256SUMS (sha256sum's format) and its golden, generated with
//     go test ./internal/store -run Golden -update and committed on its own.
//   - queries/: one .sql file per concern; run make generate.
//   - cache/schema.sql and cache/queries/: cache.db. Raise the version line on any schema change.
//   - cache.go: OpenCache, and cache.db's Read and Write.
//   - search.go: search's three statements, hand-written because sqlc cannot parse FTS5's MATCH;
//     static SQL with parameters only.
//   - open.go: Open, the pragmas and their read-back, Read, Write and One.
//   - steps.go and migrate.go: the embedded steps and the runner (D181); step_record.go: the
//     applied steps' checksums.
//   - backup.go: VACUUM INTO, and reading a backup without changing it.
//   - space_*.go: free space for the pre-migration preflight, per platform.
package store
