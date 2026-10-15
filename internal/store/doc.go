// SPDX-License-Identifier: AGPL-3.0-or-later

// Package store is the database: the one Open() for persistent.db and cache.db with its pragmas read
// back at startup, the single writer connection and the reader pool, our migration runner, the
// sqlc-generated queries in sqlcgen, and search's three hand-written static statements. It knows
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
//   - The store knows tables, not who changes them; each table has one writing package, named in
//     schema.md's "Who writes each table", and any other package asks it (architecture map F5).
//     Enforced by TestArch_EachTableHasOneWritingPackage.
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
//   - ReadTx's queries, on either database, all see one commit, whatever commits meanwhile (D303,
//     M2). Enforced by TestReadTx_SeesOneCommitWhateverCommitsMeanwhile and
//     TestCacheReadTx_SeesOneCommitWhateverCommitsMeanwhile.
//   - Every write takes the one writer through the writer gate: a write whose context a person
//     waits on (internal/platform/waiting) before background work, and background work once
//     passed over backgroundPassedOverAtMost times (D303, DL11). Enforced by
//     TestWriterGate_HandsTheWriterToAPersonBeforeBackgroundWork and
//     TestWriterGate_BackgroundWorkGoesNextOncePassedOverEnough.
//   - A slow write's report names the write it waited behind (D303, DL17). Enforced by
//     TestWrite_ASlowWaitNamesTheWriteItWaitedBehind.
//   - A backup reads a snapshot on the maintenance connection, never the writer, so writes go on
//     while it copies (D303 §7). Enforced by TestBackupInto_CopiesWhileAWriteHoldsTheWriter.
//   - Every query of both databases, and each of search's statements, reads by an index, or is
//     listed with why it may read a whole table; an entry naming D303 is a defect the redesign
//     removes (D303, DL1). Enforced by TestQueryPlans_EveryQueryReadsByAnIndexOrSaysWhy.
//   - A write made under a request that only reads (waiting.ForAReader) is reported to
//     OnReaderWrite, and storetest's databases fail the test that made it unless
//     readerWritesAllowed lists it with why (D303, DL10). Enforced by
//     TestWrite_ReportsAWriteAReadingRequestMakes.
//   - CheckInvariants names every invariant of invariants.sql that does not hold, with a dangling
//     foreign key and a full-text index that disagrees with its text, and storetest runs it after
//     every test (D303, M7). Enforced by TestCheckInvariants_NamesEachThatDoesNotHold.
//   - Every statement either database runs, search's included, counts by its query's name into the
//     querytally.Tally its context carries, and every write transaction counts once (D303, DL6).
//     Enforced by TestTally_CountsEveryStatementByItsNameAndEveryWrite.
//   - A fresh schema equals testdata/golden/schema-vNNNN.sql for the current version, and the
//     baseline alone the golden for its version. Enforced by TestMigrate_FreshSchemaEqualsTheGolden.
//   - Each step commits alone, so an interrupted migration resumes and ends identical; a step
//     that fails rolls back whole. Enforced by TestMigrate_ResumesAfterAnInterruption and
//     TestMigrate_RollsBackAStepThatFailsHalfway.
//   - The runner refuses a step that leaves a dangling foreign key, reissues an id or breaks a
//     view. Enforced by TestMigrate_RefusesAStepThatBreaksWhatSQLCannotCheck.
//   - A newer database is refused unless it declared itself compatible, and the refusal names
//     the snapshot. Enforced by TestMigrate_RefusesANewerDatabaseAndNamesTheSnapshot.
//   - The pre-migration snapshot survives success. Enforced by
//     TestMigrate_KeepsTheSnapshotAfterSuccess.
//   - A committed step is never edited, renamed or deleted; until migrations are live, the
//     baseline is the exception, rewritten by each fold (D228). Enforced by the pre-edit hook and
//     TestSteps_MatchTheCommittedChecksums.
//   - Until migrations are live (no migrations/LIVE), the steps are a baseline and at most one
//     step after it (D228); once production runs (migrations/CHECKPOINT), the baseline is the
//     version it holds, with every step since kept (D272 §5). Enforced by
//     TestSteps_KeepTheMigrationWindow and TestWindowProblem_HoldsTheBaselineAtTheCheckpointAndOneStepBeforeIt.
//   - A baseline builds only a new file. A database below its version is refused, and one that
//     reached its version through the step it folded carries on. Enforced by
//     TestMigrate_ADatabaseMigratedThroughTheFoldedStepsCarriesOn and
//     TestMigrate_ABaselineBuildsANewDatabaseAtItsVersion.
//   - Each applied step's checksum is recorded in the step's own transaction, and a database
//     where an applied step has since changed is refused (plan-deviations P17). Enforced by
//     TestMigrate_RecordsEveryStepsChecksumAndRefusesOneThatChanged.
//   - cache.db is built from one schema file, cache/schema.sql, and versioned by discard: a file
//     from another version is reported as outdated for the caller to delete, never migrated.
//     Enforced by TestOpenCache_ReportsAnotherVersionAsOutdated.
//   - cache.db carries its own application_id, so persistent.db is never taken for a cache and
//     discarded. Enforced by TestOpenCache_RefusesPersistentDB.
//   - Cache queries are generated into cachegen, apart from sqlcgen, so a library query cannot
//     name a cache table (M2-1). Enforced by sqlc.yaml's two entries.
//   - JobStorage keeps internal/platform/jobqueue's storage promises on cache.db's job table: the
//     claim order, the counted attempt, a merge only into a queued job, a cancelled job that
//     stays cancelled, and a target held by its own column (D227 §2, D231 §2).
//     TestJobStorage_ClaimsInOrderAndCountsTheAttempt,
//     TestJobStorage_ShowsThePlanTheOpenJobsAndMergesOnlyIntoAQueuedOne,
//     TestJobStorage_RecordsAWaitWithItsFlagsAndNeverUndoesACancel,
//     TestJobStorage_RequeuesWithTheAttemptCountedAndKnowsATargetByItsColumn.
//   - Every step has a test on its rows, until a fold takes it into the baseline (D228): step 0037
//     in TestMigrate_Step0037KeepsEachGrantAndAddsWhatADiscoveryFindsTheCoreBy, step 0043 in
//     TestMigrate_Step0043KeepsEachSortKeyFoundByItsItem, step 0044 in
//     TestMigrate_Step0044KeepsTheTextAndRewritesAnIndexOnlyOnChange, step 0045 in
//     TestMigrate_Step0045IndexesServeTheirPaths, step 0046 in
//     TestMigrate_Step0046CountsWhatWaitsByItsIndex.
//     The runner's handling of such a step, and of a rebuild, is tested on synthetic steps in
//     TestMigrate_ADataTransformingStepKeepsTheRowsAndRaisesTheCompatibleVersion and
//     TestMigrate_ARebuildKeepsTheIDsAndTheIDSequence.
//   - Open and the runner take their choices (the reader pool minimum, the free-space database
//     multiple) from the caller and refuse zero, never a value from here (D201). Enforced by
//     TestOpen_RefusesOptionsNobodyChose.
//   - A migration the free space cannot hold, the snapshot plus a rebuild's worst case, is refused
//     before anything is written (D228 §4).
//     TestMigrate_RefusesWhenTheFreeSpaceCannotHoldItAndChangesNothing.
//   - Every query for a title's live files takes the live files of its live versions, so they
//     agree once a version can be removed. Enforced by
//     TestTitleFiles_EveryQueryLeavesOutARemovedVersion.
//
// # Read before changing
//
//   - D6: sqlc, no ORM.
//   - D31: SQLite, pure-Go driver, WAL, pragmas read back (as refined by D193).
//   - D181: the migration policy and our small runner.
//   - D228: until migrations are live, a baseline and one step, and a change folds the last step.
//   - D185: the schema as SQL that runs.
//   - D303: the data-layer principles (standards/sql.md §13).
//   - D173: tombstones, not deletes; live_* views.
//   - docs/contributors/design/plans/schema.md, docs/contributors/design/plans/migrations.md.
//   - docs/contributors/standards/sql.md.
//
// # Where to change
//
//   - open.go is a twin of orpheum-connect's (look-alike-watch-list.md, Twins): a change to it is
//     copied there in the same batch, and checked to work there; ours.go is Orpheum's own.
//   - migrations/: numbered SQL files, each with its line in migrations/SHA256SUMS (sha256sum's
//     format). Until migrations are live (D228), a schema change folds the last step into the
//     baseline (NNNN_baseline.sql at that step's version, its "orpheum:baseline folds" line naming
//     the folded step's checksum) and adds the change as the next step; from then on steps are
//     append-only. The goldens are regenerated with go test ./internal/store -run Golden -update.
//   - queries/: one .sql file per concern; run make generate.
//   - cache/schema.sql and cache/queries/: cache.db. Raise the version line on any schema change.
//   - cache.go: OpenCache, and cache.db's Read and Write. jobs.go: the job queue's storage on
//     cache.db (JobStorage), which internal/platform/jobqueue drives.
//   - search.go: search's three statements, hand-written because sqlc cannot parse FTS5's MATCH;
//     static SQL with parameters only.
//   - open.go: Open, the pragmas and their read-back, Read, ReadTx, Write and OrNotFound;
//     writer_gate.go: the order in which writes take the one writer; tally.go: counting each
//     statement into its context's tally.
//   - steps.go and migrate.go: the embedded steps and the runner (D181); migrate_checks.go: the
//     checks a step cannot make of itself (dangling foreign keys, reissued ids, views that no
//     longer compile); step_record.go: the applied steps' checksums, and the refusal of a file
//     below the baseline (D228).
//   - backup.go: VACUUM INTO, and reading a backup without changing it.
//   - invariants.sql and invariants.go: what must never be true of persistent.db, as queries that
//     return where it is (D303, M7); add one with each rule a row or a set of rows must keep.
//   - migrate.go's requireSpaceToMigrate: the pre-migration free-space preflight, which measures
//     through internal/platform/diskspace.
//   - failure.go: IsStorageFailure, a database's disk failing rather than what was written, which
//     parks the work that writes (D231 §5).
package store
