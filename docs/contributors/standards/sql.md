> **ABOUTME:** How SQL is written here: SQLite through sqlc and our own migration runner — table
> and column naming, where queries live, the one dynamic query, migrations, how the database is
> opened and written, tombstones, time, and query plans. Each rule names its enforcement.

# SQL Standard

The database is SQLite in WAL mode through a pure-Go driver
([D31](../decisions/working-notes.md), refined by D193 §6); queries are compiled to Go by sqlc
([D6](../decisions/working-notes.md)); the schema changes only through our runner
([D181](../decisions/working-notes.md)). The tables themselves are
[schema.md](../design/plans/schema.md), and the migration policy is
[migrations.md](../design/plans/migrations.md). The Go side is [go.md](go.md) §15.

"Enforced by" reads as in [go.md](go.md): a linter is `.golangci.yml`, a `TestArch_` test is in
`internal/archtest`, *(M1)* is a mechanism that lands with the store, and **review-only** has
none.

## 1. Naming

Names match [schema.md](../design/plans/schema.md), and sqlc turns them into Go names
(`payload_hash` becomes `PayloadHash`), so a poor column name is a poor field name everywhere.

| Thing | Rule | Example |
| --- | --- | --- |
| Table | singular noun, `snake_case`, the domain's word | `title`, `library_root`, `prepared_rendition` |
| Key | `id INTEGER PRIMARY KEY AUTOINCREMENT` on any table something references; never reissued, never a UUID (D173, D179) | `title.id` |
| Reference | `<table>_id`, or a role name when two point at one table | `library_id`, `source_file` |
| Time | integer milliseconds since the epoch, server clock, named `…_at` | `created_at`, `removed_at` |
| Client time | a client's monotonic clock, named `…_mono_ms` (D165) | `started_mono_ms` |
| Other quantities | the unit in the name | `runtime_ms`, `check_every_h`, `bytes` |
| Boolean | `INTEGER NOT NULL CHECK (x IN (0,1))`, named as a predicate in new columns | `is_original` |
| Closed vocabulary | `TEXT` with a `CHECK (… IN (…))`, mirrored by a Go type with constants | `job.state` |
| Index | `<table>_<purpose>` | `job_next`, `item_text_original` |
| View | `live_<table>` for the rows not tombstoned | `live_title` |
| Trigger | `<table>_<ai/ad/au>` (after insert, delete, update) | `item_text_ai` |

**Enforced by:** review against schema.md; `TestMigrate_FreshSchemaEqualsTheGolden` makes any rename a
visible, reviewed diff. A `CHECK` vocabulary is enforced by SQLite itself, and its Go type by
exhaustive ([go.md](go.md) §6).

## 2. Queries

- **All SQL lives in `internal/store/queries/*.sql`, one file per concern** (`library.sql`,
  `play_state.sql`, `job.sql`, `secret.sql`), and sqlc generates the Go in
  `internal/store/sqlcgen`. No other package holds SQL. **Enforced by:** depguard
  `sql-only-in-store` and `TestArch_OnlyStoreImportsDatabaseSQL` (nothing else can run it).
- **A query is named for what it does**, verb first, with its sqlc command:
  `-- name: ListLiveTitlesByLibrary :many`. The verbs are `Get` (one row, an error if absent),
  `List`, `Count`, `Insert`, `Update`, `Upsert`, `Mark` (sets a tombstone) and `Delete` (cache
  tables only, §6). **Review-only.**
- **Every column is named.** `SELECT *` and `RETURNING *` change shape when a migration adds a
  column. **Enforced by:** `TestArch_QueriesNameEveryColumn`, which reads the `.sql` files,
  because sqlc expands `*` before `sqlc vet` sees a query.
- **Values are parameters, always** (`?`, or `sqlc.arg(name)` where a bare `?` would give the
  Go parameter a poor name). No SQL text is built from strings in Go. **Enforced by:** sqlc's
  generated code (it has no other form); gosec G201 and G202 anywhere else.
- **No `UPDATE` or `DELETE` without a `WHERE`.** **Enforced by:** the `no-unbounded-write` rule
  in `sqlc.yaml`, run by `sqlc vet` in `make generate`.
- **Reads go through the `live_*` views** unless the query is about tombstoned rows (history,
  the scanner view of what went away). **Review-only.**
- **Queries on the `secret` table carry `SealedSecret` in their name**
  (`GetSealedSecret`, `UpsertSealedSecret`), and only `internal/core/secrets` calls them
  (D194 §3). **Enforced by:** forbidigo `[sealed-secret]`.

## 3. sqlc

[`sqlc.yaml`](../../../sqlc.yaml) at the repository root is the one configuration. What it
chooses, and why:

- **`sql_package: database/sql`**: the SQLite engine generates for it, and the pure-Go driver
  registers with it.
- **`emit_pointers_for_null_types: true`**: a nullable column is `*int64`, not
  `sql.NullInt64`, so absence reads as `nil`.
- **`emit_empty_slices: true`**: a `:many` query with no rows returns `[]T{}`, never `nil`.
- **`emit_json_tags: false`**: generated rows are store types and are never marshalled; the API
  maps them to its own camelCase structs, so a new column cannot leak into a response. musttag
  refuses to marshal an untagged struct, which makes the mistake a lint failure.
- **`query_parameter_limit: 1`**: a query with two or more parameters takes a `…Params` struct,
  so every call site names its values.
- **Generated code is committed and never edited.** `make generate` runs `sqlc generate` and
  then `sqlc vet`. **Enforced by:** `make check-generated` (regenerates and diffs); the pre-edit
  hook; golangci-lint skips `internal/store/sqlcgen`.
- **No `interface{}` in generated code.** SQLite's inferred types are sqlc's weak spot
  (audit §5); a column sqlc cannot type gets an explicit override in `sqlc.yaml`, and a test
  fails on `interface{}` or `any` in `sqlcgen` (`TestGeneratedQueries_HaveNoUntypedValues`).
- **SQLite features sqlc cannot parse** (FTS5 `MATCH` edge cases, 3.53's new `ALTER` forms) are
  not used in queries until sqlc parses them. Search, which needs FTS5, is the filter builder of
  §4.

## 4. The one dynamic query

Cut A's pages are fixed sqlc queries, plus **one small hand-written filter builder for search**
(D193 §4). It is the only place SQL text is assembled at run time, and there is no second
mechanism: no squirrel, no goqu, no `fmt.Sprintf` into SQL anywhere else. When the predicate
compiler arrives (D164), it **replaces** the builder; it never sits beside it.

- It lives in `internal/store`, in one file, and takes a typed filter value, never a string.
- **Identifiers come from a fixed map of constants** (the searchable fields to their columns);
  no identifier is ever taken from input. **Every value is a `?` parameter.**
- Its tests include a hostile value (`'; DROP TABLE title; --`) and assert it arrives as a
  parameter.

**Enforced by:** depguard `one-way-to-do-it` (squirrel, goqu); gosec G201/G202 everywhere, which
the builder answers with one explained `//nolint:gosec` at its concatenation; the rest is
**review-only**.

## 5. Migrations

D181 is the policy and [migrations.md](../design/plans/migrations.md) the mechanism. As rules:

- **A step is a file `internal/store/migrations/NNNN_what_it_does.sql`**, numbered from `0001`
  with no gaps. sqlc reads the same directory as the schema, so generated queries always match
  the migrated tables.
- **Append only, forward only.** A merged step is never edited, renamed or deleted; a mistake is
  a new step. There are no down migrations: the `VACUUM INTO` snapshot taken before migrating is
  the way back. **Enforced by:** the pre-edit hook blocks edits to existing steps; a test
  compares each step with a committed checksum list, so an edit fails even outside the agent
  (`TestSteps_MatchTheCommittedChecksums`).
- **A step holds DDL and data statements only.** The runner owns `PRAGMA user_version`,
  `application_id` and `foreign_keys`, the `BEGIN IMMEDIATE`, and the checks a SQL file cannot
  make of itself (`foreign_key_check`, `sqlite_sequence` never decreasing, every view
  compiling). **Enforced by:** the runner, and `TestMigrate_RefusesAStepThatBreaksWhatSQLCannotCheck`.
- **A step never calls generated query code or current Go constants**, never touches the
  filesystem or network, and never parses paths; a step that would need to instead invalidates
  derived data and schedules a rescan. Go code appears only for a data transform SQL cannot
  express, and such a step changes no schema. **Review-only.**
- **Rebuilds follow one pattern**: create new, copy with explicit `rowid` where FTS content
  depends on it, drop old, rename, recreate indexes, triggers and views. `CREATE TABLE … AS
  SELECT` is not used. **Review-only.**
- **Every released version has a golden fixture; every golden migrates to a schema equal to a
  fresh install's; killing the runner after any step and restarting ends identical; a step
  takes under 10 s at the reference scale.** **Enforced by:** the migration tests in
  `internal/store`; the 10 s budget is **review-only** until M2 builds the reference-scale
  database (audit 2026-09-24, §3.2).

## 6. Tombstones and deletes

- **A referenced row is never deleted.** It gets `removed_at`; references to it are
  `ON DELETE RESTRICT`; uniqueness is a partial index over live rows; reads use the `live_*`
  views (D173). **Enforced by:** SQLite, through `RESTRICT` and `foreign_keys(1)` (read back at
  startup, §7); review for the rest.
- **`DELETE` is for cache data**: `cache.db`'s tables, and class-C tables such as `sort_key` and
  the FTS indexes (schema.md). **Review-only.**

## 7. Opening the database

- **One `Open()` in `internal/store`** per database file (`library.db`, `cache.db`). Nothing
  else opens SQLite. **Enforced by:** depguard `sql-only-in-store`.
- **Pragmas are set in the DSN with the pure-Go `_pragma=` syntax**, so every connection the
  pool opens gets them (`foreign_keys` and `busy_timeout` are per connection; an `Exec` after
  opening would set them on one):
  `file:library.db?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(15000)`.
- **They are read back at startup, and the server refuses to run if any is wrong**:
  `journal_mode`, `synchronous`, `foreign_keys`, `busy_timeout`, and `application_id`. A file
  without our `application_id` is refused, not migrated; a database on a network filesystem is
  refused with a message (D31). **Enforced by:** `Open()`'s checks and their tests
  (`TestVerifyPragmas_RefusesAConnectionWithoutThem`, `TestOpen_RefusesAFileThatIsNotOurs`); the
  network-filesystem refusal is `datadir`'s (`TestCheckFilesystemType_RefusesNetworkFilesystems`).
- The choice between modernc and ncruces is made by benchmark on devnas and does not block M1
  (audit §5); both take this DSN form.

## 8. Connections: one writer, a pool of readers

**Each database has exactly one write connection and a separate read-only pool.**

- **The writer** is a `*sql.DB` with `SetMaxOpenConns(1)` and `_txlock=immediate`, so every
  write transaction takes SQLite's write lock at `BEGIN` and never has to upgrade a read.
  Writes queue in Go for that one connection instead of contending inside SQLite.
- **The readers** are a second `*sql.DB`, opened with `_pragma=query_only(1)` and sized
  `max(4, runtime.NumCPU())`. In WAL mode they never block the writer or each other, and
  SQLite itself refuses a write through them.
- **Why not one pool for both.** SQLite has one writer whatever the pool says. A pool of writers
  turns that into `SQLITE_BUSY` waits, and into `SQLITE_BUSY_SNAPSHOT` when a read transaction
  tries to upgrade — which `busy_timeout` cannot retry, since only a rollback and a fresh
  transaction recover it (D31, from Navidrome's production experience). One writer with
  `BEGIN IMMEDIATE` removes the upgrade, and with it the snapshot error. `busy_timeout` stays
  set on both for what remains: checkpoints, `VACUUM INTO`, and `orpheum migrate` or `backup`
  run beside a live server.
- **The API makes the split hard to miss**: the store hands out read queries from the pool and
  runs writes only through `store.Write(ctx, func(queries *sqlcgen.Queries) error)`, which owns
  `BEGIN`, `COMMIT` and `ROLLBACK`. **Enforced by:** `query_only` (SQLite refuses the write)
  and `TestRead_RefusesAWrite`, `TestWrite_RollsBackWhenTheChangeFails`; `database/sql` is unimportable elsewhere, so nothing else can
  open a transaction.

## 9. Transactions

- **A change that spans statements is one `store.Write` call.** Nothing else calls `Begin`.
  **Enforced by:** depguard `sql-only-in-store`.
- **A transaction waits on nothing but SQLite**: no network call, no node command, no FFmpeg,
  no user. Gather first, then write. **Review-only.**
- **Batch where volume is high**: the scanner's reports are ingested many records per
  transaction, not one each (D31's single-writer discipline). **Review-only**, and measured by
  M2's ingest test.
- **Rows are closed and their errors checked** in hand-written store code. **Enforced by:**
  sqlclosecheck, rowserrcheck; sqlc's generated code does both.

## 10. Values

- **Times are `INTEGER` milliseconds** from `clock.Clock` (`Now().UnixMilli()`), never SQLite's
  `CURRENT_TIMESTAMP` or `datetime('now')`, which a test cannot control. **Enforced by:**
  review; the `[clock]` rule stops the Go side reaching for `time.Now`.
- **JSON stored in a `TEXT` column** (`node.status`, `job.args`) is a Go struct with explicit
  camelCase tags, as on the wire. **Enforced by:** tagliatelle and musttag on that struct.
- **Nullable means optional**, and `NOT NULL` is the default for a new column unless absence is
  a real state. **Review-only.**

## 11. Query plans

Query shape matters more than caching: schema.md measured the poster grid's watched badge at
0.84 ms well-shaped and 7.2 ms when an `EXISTS` became a `SCAN` of `version`, against a 50 ms
page budget ([performance-targets.md](../design/plans/performance-targets.md)).

- **Every query a listing page runs has its `EXPLAIN QUERY PLAN` read** when it is written or
  changed, against the reference-scale database, and the plan is pasted into the review. A
  `SCAN` of a large table needs a reason.
- **A correlated check is a `LIMIT 1` scalar subquery driven by an index**, not an `EXISTS` the
  planner may turn into a scan.
- **Pages use keyset pagination** (`WHERE (sort_key, id) > (?, ?) … LIMIT ?`), never `OFFSET`.
- **Enforced by:** review now; a test that runs `EXPLAIN QUERY PLAN` over every `List…` query at
  the reference scale and fails on an unlisted `SCAN` *(M2)*.

## 12. Backups

- **A backup is `VACUUM INTO`**, run by the store for `internal/core/backup` (D77). It contains
  ciphertext only; `secret.key` is never written into the database or its backups (D194 §4).
  **Enforced by:** `TestCanary_ASecretAppearsNowhereButAsCiphertext` and the restore tests.
