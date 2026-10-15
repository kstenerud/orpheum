> **ABOUTME:** How SQL is written here: SQLite through sqlc and our own migration runner — table
> and column naming, where queries live, search's hand-written statements, migrations, how the database is
> opened and written, tombstones, time, query plans, and the data-layer principles of D303. Each
> rule names its enforcement.

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
| Client time | a client's monotonic clock, named `…_mono_ms` (D165) | `happened_mono_ms` |
| Other quantities | the unit in the name: `_ms` for milliseconds, any other unit spelled out, a setting key's too (D227 §6; since 2026-09-30 for setting keys) | `runtime_ms`, `check_every_hours`, `segment_length_seconds`, `size_bytes` |
| Boolean | `INTEGER NOT NULL CHECK (x IN (0,1))`, named as a predicate in new columns | `is_original` |
| Closed vocabulary | `TEXT` with a `CHECK (… IN (…))`, mirrored by a Go type with constants | `job.state` |
| Index | `<table>_<purpose>` | `job_next`, `item_text_original` |
| View | `live_<table>` for the rows not tombstoned | `live_file` |
| Trigger | `<table>_<ai/ad/au>` (after insert, delete, update) | `item_text_ai` |

**Enforced by:** review against schema.md; `TestMigrate_FreshSchemaEqualsTheGolden` makes any rename a
visible, reviewed diff. A `CHECK` vocabulary is enforced by SQLite itself, and its Go type by
exhaustive ([go.md](go.md) §6).

## 2. Queries

- **All SQL lives in `internal/store/queries/*.sql` for persistent.db and
  `internal/store/cache/queries/*.sql` for cache.db, one file per concern** (`library.sql`,
  `play.sql`, `secret.sql`; cache.db's `job.sql`), and sqlc generates the Go in
  `internal/store/sqlcgen` and `internal/store/cachegen`. Search's three statements
  (`internal/store/search.go`, §4) and the invariants (`internal/store/invariants.sql`, §13), which
  a runner reads one by one, are the exceptions. No other package holds SQL. **Enforced by:** depguard
  `sql-only-in-store` and `TestArch_OnlyStoreImportsDatabaseSQL` (nothing else can run it).
- **A query is named for what it does**, verb first, with its sqlc command:
  `-- name: ListFilmsByName :many`. The verbs are `Get` (one row, an error if absent),
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
- **No upsert that writes when nothing changed** (§13, DL9). **Enforced by:** the
  `upsert-only-if-changed` rule in `sqlc.yaml`.
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
  not used in queries until sqlc parses them. Search, which needs FTS5's `MATCH`, is written by
  hand (§4).
- **A list argument is a JSON array**, read with `IN (SELECT value FROM json_each(CAST(sqlc.arg(ids)
  AS TEXT)))`: `sqlc.slice` generates `[]interface{}`, which the rule above refuses, and the
  `CAST` makes sqlc type the argument as a string. The caller marshals its ids.
  (`ListShownArtwork`; audit-2026-09-25 §5.2.)
- **Query files are ASCII**, comments included: sqlc miscounts a multibyte character and fails
  with an unrelated syntax error (plan-deviations P13; it recurred in M2). Write "section 3", not
  "§3". **Enforced by:** `scripts/check_source.py`.

## 4. Search's hand-written statements

Cut A's pages are fixed sqlc queries. D193 §4 allowed one small hand-written filter builder for
search; what exists is less: **three static statements in `internal/store/search.go`**, one per
route (D48: the two FTS5 indexes and a substring match), written by hand only because sqlc
cannot parse `MATCH`. No
SQL text is assembled at run time anywhere: no builder, no squirrel, no goqu, no `fmt.Sprintf`
into SQL (plan-deviations P36). When the predicate compiler arrives (D164), it is the one
mechanism for dynamic queries; nothing sits beside it.

- The statements are `const`s; **every value, the search text included, is a `?` parameter.**
- They list only what browse lists: live items with a live file (`browse.sql`).

**Enforced by:** depguard `one-way-to-do-it` (squirrel, goqu); gosec G201/G202 everywhere; the
rest is **review-only**.

## 5. Migrations

D181 is the policy and [migrations.md](../design/plans/migrations.md) the mechanism. As rules:

- **A step is a file `internal/store/migrations/NNNN_what_it_does.sql`**, numbered with no gaps
  from `0001` or from a baseline. sqlc reads the same directory as the schema, so generated
  queries always match the migrated tables.
- **Until migrations are live, a baseline and one step** ([D228](../decisions/working-notes.md#d228--until-migrations-are-live-librarydbs-schema-is-a-baseline-and-one-step-and-a-schema-change-folds-the-last-step-into-the-baseline)): `NNNN_baseline.sql` builds a
  new database at its version, and a schema change folds the last step into it and adds the
  change as the one step after it. **Once production runs, a sliding window**
  ([D272](../decisions/working-notes.md#d272--production-runs-on-naslan-from-an-image-it-builds-and-migrations-keep-every-step-since-the-version-it-last-ran)
  §5, §6): `internal/store/migrations/CHECKPOINT` holds the version production last ran, the
  baseline stays at it, frozen, and a schema change appends a step; when the owner says
  production has migrated, they move the checkpoint and the steps up to it fold into the
  baseline. **Enforced by:** `TestSteps_KeepTheMigrationWindow`; the pre-edit hook; the
  checkpoint is a gate file. The owner commits `internal/store/migrations/LIVE` to end both.
- **Append only, forward only.** A merged step is never edited, renamed or deleted; a mistake is
  a new step. Until migrations are live, the baseline is the exception, rewritten by each fold.
  There are no down migrations: the `VACUUM INTO` snapshot taken before migrating is the way
  back. **Enforced by:** the pre-edit hook blocks edits to existing steps; a test compares each
  step with a committed checksum list, so an edit fails even outside the agent
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
- **Once migrations are live, every released version has a golden fixture; every golden
  migrates to a schema equal to a fresh install's; killing the runner after any step and
  restarting ends identical; a step takes under 10 s at the reference scale.** Until then, the
  only goldens are the baseline's and the current version's (D228). **Enforced by:** the migration tests in
  `internal/store`; the 10 s budget by `TestReferenceScale` in its integration tier, which fills
  a database at the baseline's schema with the reference library (about 46 MB) and times every
  later step and a rebuild of `file`.

## 6. Tombstones and deletes

- **A referenced row is never deleted.** It gets `removed_at`; references to it are
  `ON DELETE RESTRICT`; uniqueness is a partial index over live rows; reads use the `live_*`
  views (D173). **Enforced by:** SQLite, through `RESTRICT` and `foreign_keys(1)` (read back at
  startup, §7); review for the rest.
- **`DELETE` is for cache data**: `cache.db`'s tables, and class-C tables such as `sort_key` and
  the FTS indexes (schema.md). **Review-only.**

## 7. Opening the database

- **One `Open()` in `internal/store`** per database file (`persistent.db`, `cache.db`). Nothing
  else opens SQLite. **Enforced by:** depguard `sql-only-in-store`.
- **Pragmas are set in the DSN with the pure-Go `_pragma=` syntax**, so every connection the
  pool opens gets them (`foreign_keys` and `busy_timeout` are per connection; an `Exec` after
  opening would set them on one):
  `file:persistent.db?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(15000)&_pragma=temp_store(MEMORY)&_pragma=cache_size(-16384)&_pragma=journal_size_limit(67108864)`
  (the last two since D303: a 16 MiB page cache per connection, and the WAL file trimmed to
  64 MiB after a checkpoint).
- **They are read back at startup, and the server refuses to run if any is wrong**:
  `journal_mode`, `synchronous`, `foreign_keys`, `busy_timeout`, `temp_store`, `cache_size`,
  `journal_size_limit`, and `application_id`. A file
  without our `application_id` is refused, not migrated; a database on a network filesystem is
  refused with a message (D31). **Enforced by:** `Open()`'s checks and their tests
  (`TestVerifyPragmas_RefusesAConnectionWithoutThem`, `TestOpen_RefusesAFileThatIsNotOurs`); the
  network-filesystem refusal is `instancedir`'s (`TestRefuseNetworkFilesystemType_RefusesNetworkFilesystems`).
- The choice between modernc and ncruces is made by benchmark on devnas and does not block M1
  (audit §5); both take this DSN form.

## 8. Connections: one writer, a pool of readers

**Each database has exactly one write connection and a separate read-only pool.**

- **The writer** is a `*sql.DB` with `SetMaxOpenConns(1)` and `_txlock=immediate`, so every
  write transaction takes SQLite's write lock at `BEGIN` and never has to upgrade a read.
  Writes queue in Go for that one connection instead of contending inside SQLite.
- **The readers** are a second `*sql.DB`, opened with `_pragma=query_only(1)` and sized
  `max(read_connections_minimum, runtime.NumCPU())`, the minimum from the config file. In WAL mode they never block the writer or each other, and
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
- **JSON stored in a `TEXT` column** (`node.last_status_json`, `job.args_json`) is a Go struct with explicit
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
- **Enforced by:** `TestReferenceScale` (the integration tier): it reads every `List…` query's
  plan at the reference scale and fails on a full scan not listed, with its reason, in
  `fullScansAllowed`; and it times each browse page against the 50 ms budget. Its first run found
  every list's live-file check reading all files for each title — seconds a page.

## 12. Backups

- **A backup is `VACUUM INTO`**, run by the store for `internal/core/backup` (D77). It contains
  ciphertext only; `secret.key` is never written into the database or its backups (D194 §4).
  **Enforced by:** `TestCanary_ASecretAppearsNowhereButAsCiphertext` and the restore tests.

## 13. Data-layer principles (D303)

Adopted after nas.example's requests waited 3–47 s behind a 25–30 s write transaction. The evidence
and the reasoning are the [access audit](../design/research/database-access-audit-2026-10-15.md),
the [practice research](../design/research/data-layer-practice-2026-10-15.md) (DL1–DL17) and the
[independent DBA's audit](../design/research/data-layer-independent-audit-2026-10-15.md) (M1–M13,
and its amendments to DL3, DL8, DL11, DL15 and DL16). Each rule carries its source IDs. An
enforcement marked *(D303 A/B/C/D)* is built in that phase of D303 §9 and is review-only until
then; [audit-checklist](../audit-checklist.md) item 23 asks the questions at every commit.

### Correctness

- **Decide outside, verify inside** (M1). A write whose content depends on something read
  outside its transaction re-checks it inside: a compare-and-set predicate (`WHERE revision = ?`,
  `WHERE input_fingerprint IS ?`, or a domain guard like `MovePlayPosition`'s), or a re-read. A
  zero-row result is a conflict to retry or drop, never success. Rows people edit carry a
  `revision` raised on every write. **Enforced by:** a test that a person's edit between a job's
  read and its write wins *(D303 C)*.
- **A read built from several queries uses one read transaction** (M2): `ReadTx`, which pins one
  connection and one snapshot. Inside it, read only through its queries: a `Read()` inside waits
  for another reader, and once every reader is lent to a `ReadTx` that waits the same way, none
  returns. **Enforced by:** review; `ReadTx`'s snapshot test *(D303 A)*.
- **Identity is declared** (M3): whatever must not be duplicated has a `UNIQUE` constraint on its
  natural key beside its surrogate key. **Enforced by:** the invariants *(D303 B)*.
- **A write transaction covers one consistency unit** (M4, DL8): a title with its versions, text
  and sort key; a series with its episodes and positions. It reads nothing it did not need, waits
  on nothing but SQLite (§9), and fits a budget of milliseconds. A job of many units commits unit
  by unit, records what is done, and resumes after a crash. **Enforced by:** the writer-hold
  budget on the pass and a kill-and-resume test *(D303 C)*; the slow-write log in production.
- **Integrity is the engine's** (M5): every table is `STRICT`; a rule a row obeys alone is a
  `CHECK`; a subtype row (`title`, `collection`) is tied to its supertype's kind by a composite
  foreign key; `NOT NULL` wherever absence is not a state. **Enforced by:** a schema test that
  every table is `STRICT` *(D303 B)*.
- **Invariants are executable** (M7): data invariants are SQL queries that must return no rows,
  in `internal/store/invariants.sql`, run after store and identify tests, after migrations, and
  nightly in production, with `foreign_key_check` and FTS5's `integrity-check`. **Enforced by:**
  `storetest` runs `CheckInvariants` after every test that opens a database through it
  (`TestCheckInvariants_NamesEachThatDoesNotHold`); the nightly run *(D303 D)*.
- **`persistent.db` and `cache.db` agree by protocol** (M11): no transaction spans both, so a
  change writes `persistent.db` first, as the fact, then `cache.db`; the second write is
  idempotent, and the invariants find orphans either way. **Enforced by:** review; the invariants.

### Access paths

- **Each query's access path and the index serving it are known** (DL1): every query's plan,
  both databases' and search's, is read, and a full scan of a table or of an index, or an
  automatic index, fails unless `wholeReadsAllowed` lists it with its reason. Without statistics
  SQLite plans the same whatever the rows, so the check runs on empty databases, in the unit tier.
  **Enforced by:** `TestQueryPlans_EveryQueryReadsByAnIndexOrSaysWhy`.
- **An index is ordered by how queries bind it** (DL2): equality columns first, then one range or
  the ORDER BY. A column with a handful of values (a locale, a kind, a state) leads only when
  every query also binds the next column. `SEARCH … (locale=?)` reads the whole locale. **Enforced
  by:** the weak-prefix check from statistics *(D303 B)*.
- **A foreign key whose parent can be deleted, or whose action cascades or sets null, has a child
  index** (DL3, amended); any other child column is indexed when a query reads by it. **Enforced
  by:** a schema test over the pragmas *(D303 B)*.
- **A partial index serves only queries that repeat its condition literally** (DL4), never a
  parameter for it; a query on the tombstoned side has its own index. **Enforced by:** the plan
  check naming the index each such query must use *(D303 B)*.
- **No speculative indexes** (DL5): each serves a named path; columns updated often stay out of
  indexes. **Review-only.**
- **Planner statistics are committed** (DL15): a `sqlite_stat1` generated from the reference
  library is loaded into every database; nothing runs `ANALYZE` or `PRAGMA optimize`; plans are
  tested over several library shapes. **Enforced by:** a staleness test and a check that no code
  runs `ANALYZE` *(D303 B)*.

### Requests and writes

- **Nothing a request does grows with the library** (DL6): lists page by keyset with a `LIMIT`
  (§11); a count is `COUNT` in SQL, never rows fetched and counted; a request's number of
  statements is constant as the library grows. **Enforced by:** the store counts every statement
  into the request's `querytally.Tally`, and a slow request's log line lists them
  (`TestTally_CountsEveryStatementByItsNameAndEveryWrite`,
  `TestLoggingSlow_NamesTheStatementsASlowRequestRan`); the check that each list route's count
  is the same at two library sizes comes with the generated library *(D303 C)*.
- **Reading never writes** (DL10): nothing on a read request's path takes the writer. Bookkeeping
  such as a session's last use is kept in memory and written in batches, never older over newer.
  **Enforced by:** the api marks a GET or a HEAD as only reading (`internal/platform/waiting`);
  the store reports a write made under that mark, production logs it, and every database
  `storetest` opens fails the test that made it unless `readerWritesAllowed` lists it with why
  (`TestWrite_ReportsAWriteAReadingRequestMakes`, `TestHandler_MarksAGetOrAHeadAsReading`).
- **Bulk work is set-based or batched** (DL7): a constant number of statements per unit, never
  one statement per item across the library. **Enforced by:** statement counts at two library
  sizes *(D303 C)*.
- **A write that changes nothing writes nothing** (DL9): every `DO UPDATE` carries `WHERE col IS
  NOT excluded.col …`; a derivation `UPDATE` carries the same guard; a trigger that maintains
  derived data is `AFTER UPDATE OF <its columns> … WHEN old.x IS NOT new.x`; the writer's own
  stamps are left out of every comparison, and nothing is stamped that can be derived. **Enforced
  by:** `sqlc vet`'s `upsert-only-if-changed` rule refuses a `DO UPDATE` with no `WHERE`, and
  `TestUpserts_EachWritesOnlyWhenItChangesSomething` reads the generated statements for the same; a
  query that must touch the row regardless (one that returns it) opts out with
  `-- @sqlc-vet-disable upsert-only-if-changed` and says why. A second pass over an unchanged
  library writes zero rows *(D303 C)*.
- **People's writes go first** (DL11): every write passes through the writer gate, which hands
  the one writer to the oldest interactive write before background work, and to background work
  that has waited past its bound before either, so none starves. Group commit (D303 §1) joins
  it when the writer's measured queue shows small writes waiting on each other's commits.
  **Enforced by:** the writer gate's order test *(D303 A)*.

### Derived data

- **Grouping and resolution are separate derivations** (DL12): grouping files into titles and
  series needs the library; resolving one title's values needs that title. A change re-derives
  what it touched; the whole-library run is a backstop that compares before it writes.
  **Enforced by:** a test that changing one item writes O(1) rows, and one that incremental
  equals rebuilt *(D303 C)*.
- **A derivation knows its inputs are unchanged without recomputing** (DL13): a fingerprint of
  the inputs' values and a rules version, bumped when the derivation's output on a fixed corpus
  changes. **Enforced by:** the corpus-hash test *(D303 C)*.
- **Requests to re-derive coalesce** (DL14): a burst marks work and wakes one worker; the mark
  survives a restart. **Enforced by:** a `clocktest` test counting runs *(D303 C)*.

### Physical design and operations

- **Data is split by volatility and width** (M8): telemetry and bookkeeping stay out of
  `persistent.db`'s write path; wide, rarely read payloads (probe output, NFO text, provider
  bodies) live in side tables so hot rows stay narrow. **Review-only.**
- **Connections are tuned by measurement and read back** (DL16, §7): `cache_size`,
  `journal_size_limit`; a read transaction never stays open across a slow consumer.
  **Enforced by:** `verifyPragmas` *(D303 A)*.
- **Slow work names itself** (DL17): a write that waited names the write that held the writer; a
  slow request lists its statements; a slow statement its name, time and pages touched.
  **Enforced by:** tests of each log line *(D303 A, D)*.
- **The database is checked and recoverable** (M6): an hourly backup through its own
  connection, skipped when nothing changed, thinned to 24 hourly, 7 daily and 4 weekly, each copy
  passing `integrity_check`; a restore drill; a runbook. **Enforced by:** *(D303 D)*.
- **Every growing table has a retention line** in schema.md (M10), and **deleting a person
  pseudonymises** (M12: their name and personal fields cleared, their history kept). Edits keep
  no history (M13). **Review-only.**
- **Tests treat the data as precious** (M9): random operation sequences checked against a full
  rebuild and the invariants; crash injection mid-job; fixtures with skewed shapes. *(D303 C)*.
