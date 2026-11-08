> **ABOUTME:** How to design and use a SQLite data layer that stays fast as a library grows: what
> SQLite's planner, writer, triggers, FTS5 and WAL actually do, what Navidrome, Jellyfin, Plex,
> Emby, PocketBase and Rails learned, a proposed set of data-layer principles with what would
> enforce each, and the gates to build. Written after the database access audit of the same day.

# A data layer that stays fast: research, 2026-10-15

The [database access audit](database-access-audit-2026-10-15.md) found the production NAS's requests waiting
3–47 s behind an identification pass that held SQLite's one writer for 25–30 s. Its eight
findings are each an instance of a general mistake, and the project had no written principles
for data-layer design that would have caught them: [sql.md](../../standards/sql.md) covers
naming, sqlc, migrations, the single writer and keyset pages, and its plan check reads only
`List…` queries for `SCAN`. This round asked what experienced database engineers, SQLite's own
documentation and comparable projects say, so that the fixes are built to principles and the
principles are enforced.

Four threads ran in parallel: SQLite internals (the official documentation, read and reproduced
on SQLite 3.46.1); Go and SQLite in production (driver source, Go's `database/sql` source, and
the source and issue trackers of Navidrome, Jellyfin, PocketBase and Tailscale's driver);
general data-layer design (Winand's *Use The Index, Luke*, Kleppmann's *Designing
Data-Intensive Applications*, *Database Reliability Engineering*, DeBrie's access-pattern
method); and mechanical gates (what modernc exposes, run against our schema). Claims below that
were measured or reproduced say so.

**Our engine.** modernc.org/sqlite v1.59.0 bundles **SQLite 3.53.4**, compiled with STAT4,
FTS5, the `dbstat` table and the pre-update hook, and without statement scan-status or the
bytecode table. 3.53.4 contains the fix for the WAL-reset corruption race that existed from
3.7.0 to 3.51.2; any `sqlite3` command-line tool pointed at a live database must be 3.51.3 or
later too.

## 1. What SQLite actually does

### 1.1 An index serves only a prefix of equalities, and `SEARCH` does not mean "few rows"

An index (or a `WITHOUT ROWID` table's primary key) narrows a query only through a left-to-right
prefix of its columns, each bound by `=`, `IN` or `IS`, with at most one range on the last
column used. Every other condition is checked row by row over whatever the prefix selected
([optoverview §2](https://sqlite.org/optoverview.html), [queryplanner](https://sqlite.org/queryplanner.html)).
So `EXPLAIN QUERY PLAN`'s `SEARCH sort_key USING PRIMARY KEY (locale=?)` for `WHERE item_id = ?
AND locale = ?` means "read every row of the locale and test `item_id` on each" (reproduced).
**Only the terms inside the parentheses narrow; the question is how many rows match them.**

Without statistics the planner assumes about **ten rows per value** of an index's leading
column, so it believes that plan is cheap, and it never uses skip-scan (which pays at about 18
duplicates). The planner's own checklist calls an index whose leftmost column has more than
10–20 rows per value "low-quality" and says to avoid low-cardinality leading columns (booleans,
enums, a locale) unless every query binds the next ones
([queryplanner-ng §6](https://sqlite.org/queryplanner-ng.html)).

A `WITHOUT ROWID` table is one B-tree on its primary key: efficient only for a prefix of that
key. Its secondary indexes carry the whole primary key, so `sort_key(item_id, locale)` alone
covers the delete and the join that scanned
([withoutrowid §4](https://sqlite.org/withoutrowid.html)).

`AUTOMATIC INDEX` in a plan means SQLite builds a throwaway index on every execution because a
real one is missing; `USE TEMP B-TREE FOR ORDER BY` means the result is sorted after reading it;
`CORRELATED SCALAR SUBQUERY` runs once per outer row ([eqp](https://sqlite.org/eqp.html)). The
plan format is for people and has changed between versions; matching tokens in a test is fine,
parsing it exactly is not.

### 1.2 A partial index is used only when the query repeats its condition word for word

SQLite uses a partial index with condition X for a query with WHERE W only when it can prove W
implies X, and it tries just two rules: a term of W appears verbatim among X's terms, or X says
`z IS NOT NULL` and W compares `z` with an operator other than `IS`. No algebra; a bound
parameter never matches a literal in the index's condition
([partialindex §3](https://sqlite.org/partialindex.html)). So `file_payload … WHERE removed_at
IS NULL` cannot serve `removed_at IS NOT NULL` (reproduced: `SCAN`). The other side of a
tombstone needs its own index.

### 1.3 A write that changes nothing still costs, and still fires every trigger

SQLite never skips a trigger or an index update because the values are equal (measured by
counting WAL pages on 5,000-row tables, and read in `src/update.c`):

| Case | Pages written | Triggers |
| --- | --- | --- |
| Row rewritten with the same bytes, column not indexed | 0 (the cell is compared and left) | fire |
| An indexed column named in `SET`, same value | 23 (index entry deleted and reinserted) | fire |
| A guarded statement (`… WHERE col IS NOT ?`) | 0 | do not fire |

An `AFTER UPDATE OF col` trigger fires whenever `col` is *named* in `SET`. Our `item_text_au`
trigger therefore deletes and re-inserts every item in both FTS5 indexes on every pass. On
20,000 rows with two trigger-maintained FTS5 indexes (one trigram), the plain upsert pass took
**1.22 s and wrote 1,736 WAL pages; the same upsert with `DO UPDATE … WHERE col IS NOT
excluded.col` took 0.03 s and wrote none** (measured). The documented fixes are the guard in the
statement ([lang_upsert §2.1](https://sqlite.org/lang_upsert.html)) and a narrowed trigger,
`AFTER UPDATE OF name, … WHEN old.name IS NOT new.name OR …`
([lang_createtrigger](https://sqlite.org/lang_createtrigger.html)). `IS NOT` is null-safe; `<>`
is not.

### 1.4 One writer; the length of its transactions is the whole cost

Measured on modernc in this sandbox: a one-row `BEGIN IMMEDIATE; UPDATE; COMMIT` takes about
**35 µs**; an update batched 1,000 to a transaction about **4.2 µs** a row; a compare-and-skip
update that changes nothing about **5.4 µs** a row. A request's write is microseconds of work; a
30 s transaction ahead of it is a million times that.

- `BEGIN IMMEDIATE` is right and we use it: a deferred transaction that upgrades to a write
  fails with `SQLITE_BUSY` at once, without consulting the busy handler
  ([lang_transaction §2.2](https://sqlite.org/lang_transaction.html);
  [Hubert](https://berthub.eu/articles/posts/a-brief-post-on-sqlite3-database-locked-despite-timeout/)).
- **Go's pool hands the one writer to a random waiter.** With `SetMaxOpenConns(1)`, a released
  connection goes to `db.connRequests.TakeRandom()` (go1.26.2 `database/sql/sql.go`): no first
  come first served and no priority. A request's write competes at 1/N odds with every queued
  background write at each hand-off. SQLite's own busy handler is unfair too (older waiters sleep
  longest), which Rails replaced ([Margheim](https://fractaledmind.github.io/2024/04/15/sqlite-on-rails-the-how-and-why-of-optimal-performance/)).
- A long write transaction also grows the WAL (it cannot be reset mid-transaction), spills
  dirty pages into it once they pass `cache_size`, and keeps FTS5's pending terms in memory.
- `BEGIN CONCURRENT` and `wal2` exist only on SQLite side branches, are not in modernc, and a
  `wal2` database is unreadable by mainline SQLite. Not an option.

### 1.5 Statistics: the planner guesses without them, and gathering them is a write

The planner chooses between indexes from `sqlite_stat1` (and `sqlite_stat4`, which our build
has). The documentation's current advice (3.46 onwards) is `PRAGMA optimize=0x10002` as each
connection opens and `PRAGMA optimize` periodically and after schema changes
([pragma optimize](https://sqlite.org/pragma.html#pragma_optimize),
[lang_analyze](https://sqlite.org/lang_analyze.html)). Statistics reach a connection only when it
loads the schema, so the reader pool must run it too or be recycled.

Navidrome's experience argues for caution: `PRAGMA optimize`'s budgeted analysis wrote
statistics that misled the planner into an index that matched the whole table, making a song
listing about **90× slower** (5.5 s, 96K tracks; [PR #5740](https://github.com/navidrome/navidrome/pull/5740)).
They moved to a full `ANALYZE` after migrations and after full scans (1.9–9.5 s on 96K–920K
tracks), then found one full `ANALYZE` held the write lock for **26 min 41 s** on a slow NAS and
changed it to `ANALYZE <index>` one index at a time with pauses
([PR #6201](https://github.com/navidrome/navidrome/pull/6201)). SQLite also allows a fixed,
committed `sqlite_stat1` ("fixed results of ANALYZE", lang_analyze §2.2): the same plans in
tests and in production, independent of what a household's data looks like.

### 1.6 WAL and the page cache

- With `synchronous=NORMAL` in WAL mode there is no fsync per commit, only at checkpoints; the
  database stays consistent and loses only the last commits on power loss. Ours is right
  ([pragma synchronous](https://sqlite.org/pragma.html#pragma_synchronous)).
- A checkpoint runs inside the committing thread once the WAL passes 1,000 pages, and can reset
  the WAL only when no reader holds an older snapshot. A read transaction always open (an
  unclosed `*sql.Rows`, a stream to a slow client inside a read) starves checkpoints and the WAL
  grows without bound, slowing every read ([wal §6](https://sqlite.org/wal.html)). Without
  `journal_size_limit` the WAL file stays at its high-water size.
- `cache_size` defaults to about 2 MB per connection; PocketBase uses 32 MB, practitioners
  32–200 MB. `mmap_size` saves a copy per read but turns an I/O error into SIGBUS, which crashes
  a Go process: a measured trade-off, not a default.

### 1.7 FTS5

External-content tables (ours) need the application to pass the exact old text on delete, which
FTS re-tokenizes; an update is a delete plus an insert. The trigram tokenizer emits about one
token per character, so rewriting a row is several times the cost of the word index. Each
transaction that touches an FTS5 index adds a segment; at `crisismerge` (16) segments on a level
the next write merges them all, inside whoever wrote. `automerge=0` with a background `'merge'`
moves that work off the write path; `detail=none` or `detail=column` shrinks a trigram index that
only serves substring search; `contentless_delete=1` (3.43+) deletes without re-tokenizing, at
the cost of `snippet()` and `highlight()` ([fts5](https://sqlite.org/fts5.html) §4.3.4, §4.4,
§6).

### 1.8 Counting, statements, foreign keys

- `COUNT(*)` walks a B-tree every time; SQLite keeps no row count. Count in SQL over a covering
  index, or keep a counter in the same transaction as its source; never fetch rows to count them.
- modernc prepares and finalizes every statement on every `database/sql` call: about 2.7 µs for a
  point query and 75 µs for a four-table join (measured). sqlc's `emit_prepared_queries` would
  prepare each once per connection. A CPU saving on hot reads; it does nothing for waiting.
- When a parent row is deleted or its key changes, SQLite looks up each child table by the child
  key; without an index there, that is a scan of the child per parent row
  ([foreignkeys §3](https://sqlite.org/foreignkeys.html)). **Our schema has 30 foreign keys with
  no index and 6 with only a partial one**, `sort_key(item_id) → item ON DELETE CASCADE` among
  them (checked over `pragma_foreign_key_list` and `pragma_index_list`).
- Many small queries are cheap in-process: SQLite's own page renders with about 200
  ([np1queryprob](https://sqlite.org/np1queryprob.html)). N+1 is a CPU cost here, not a network
  one; what matters is that the count does not grow with the library.

## 2. What comparable projects learned

**No one fixed "the scan holds the database" with a faster driver or a different lock.**
Everyone who had it fixed it by making the scan write less, in short transactions.

- **Navidrome** (Go, the closest analogue; [source](https://github.com/navidrome/navidrome)):
  - The scan commits one transaction per folder, and re-reads a folder only when a stored hash
    of its file list, sizes and modification times differs.
  - A later phase rebuilds only the albums earlier phases "touched", compares each with what is
    stored, skips the unchanged ones, and commits one album at a time.
  - Statistics are refreshed only when changes were detected.
  - Its 0.64 regression was a scan holding the lock "for many seconds per folder" while a new
    artwork worker wrote. The fixes were to pause the artwork worker during scans, run maintenance
    under a lock that never competes with a scan, and run `ANALYZE` per index
    ([#6201](https://github.com/navidrome/navidrome/pull/6201)).
  - A user's last-access time is written at most once a minute per user, with a 1-second timeout,
    and a failure is logged and the request served.
- **Jellyfin** ([locking post](https://jellyfin.org/posts/SQLite-locking/),
  [#14047](https://github.com/jellyfin/jellyfin/pull/14047)) blamed over-scheduled parallel
  writes and "very long running and frankly unoptimized transactions", and added an app-level
  lock (the pessimistic mode blocks all reads during a write, which discards WAL's main benefit).
  Its symptoms match deferred transactions upgrading to writes, which `BEGIN IMMEDIATE` avoids.
  - Scans in 10.11 became up to 200× slower ([#15070](https://github.com/jellyfin/jellyfin/issues/15070),
    [#14386](https://github.com/jellyfin/jellyfin/issues/14386)).
  - The fixes that helped were query-shape ones: batched saves
    ([#13889](https://github.com/jellyfin/jellyfin/pull/13889)), missing indexes and one grouped
    count instead of many ([#15986](https://github.com/jellyfin/jellyfin/pull/15986)).
  - [#18274](https://github.com/jellyfin/jellyfin/issues/18274) is our finding 2 almost word
    for word: a scan of an unchanged 52,140-item library rewrote 3,980 items (4,770 UPDATEs).
    The cause was a timestamp the scan itself sets being part of the comparison. The fix
    ([#18278](https://github.com/jellyfin/jellyfin/pull/18278)) left bookkeeping fields out of
    the change comparison.
- **Plex** logs "Sleeping for 200ms to retry busy DB" during scans on slow storage. The
  community restarts the server from cron when it floods
  ([forum](https://forums.plex.tv/t/sqlite3-sleeping-for-200ms-to-retry-busy-db/722286)), and one
  version wrote 157 million bandwidth rows a month into the library database
  ([forum](https://forums.plex.tv/t/library-db-size-more-than-doubled-in-latest-version/918851)).
  The structure was never fixed, so users manage it.
- **Emby** keeps users and authentication in their own database files, so a library scan's
  writes never block a sign-in ([support](https://emby.media/support/articles/Corrupt-Database.html)).
  PocketBase writes request logs in batches into a separate `auxiliary.db`, and Rails 8 puts
  cache, queue and cable in their own files. All three split write churn away from the main
  writer.
- **Drivers** ([go-sqlite-bench](https://github.com/cvilsmeier/go-sqlite-bench)): modernc, ncruces
  and mattn are within about 20% on small-transaction workloads and 1.5–3× on bulk inserts. None
  changes the single writer. Stay on modernc.

## 3. Proposed principles

These are proposals for the owner. Each says what it prevents (the audit finding, as **A1–A8**)
and what would enforce it. Accepted ones would go into [sql.md](../../standards/sql.md) and the
[audit checklist](../../audit-checklist.md), with the gates of §4.

### Schema and indexes

- **DL1. Each table's access paths are written down, each with the index that serves it.** For
  every query that reads the table: its filter, its order and the index it uses. A new query
  names its path, or adds one with its index.
  - Why: SQLite always has a fallback, the scan, so a missing path works in development and fails
    at scale (DeBrie's discipline, which DynamoDB forces).
  - Enforced by: a generated table of every query's plan, committed beside the queries, so a
    changed plan is a reviewed diff. *(A1)*
- **DL2. A second lookup path gets its own index; a key ordered for listing does not serve
  lookups.** Equality columns first, then one range or the ORDER BY columns. A column with a
  handful of values (locale, kind, a state) never leads an index unless every query binds the
  next column too.
  - Enforced by: the plan check of §4.2 and the weak-prefix check of §4.4. *(A1)*
- **DL3. Every foreign key's child column leads an index** (a partial index does not count),
  unless the child table is small by nature and listed with its reason.
  - Enforced by: a schema test over the pragmas (§4.1). *(A1: it would have caught `sort_key`
    with no data at all.)*
- **DL4. A partial index lists the queries it serves, and each repeats its condition
  literally. The other side of a tombstone has its own index** wherever a query reads removed
  rows.
  - Enforced by: the plan check asserting the named index for each such query. *(A6)*
- **DL5. No speculative indexes.** Every index serves a named path, because each costs a write
  on every insert and every update of its columns. Columns updated often (last-used, progress)
  stay out of indexes.
  - Enforced by: a check that every index appears in some query's plan.

### Queries

- **DL6. Nothing a request triggers grows with the library.** Every list a request reads has a
  `LIMIT` and pages by keyset (sql.md §11 already). Counts are `COUNT` in SQL over an index, or a
  counter kept in the same transaction as its source, never rows fetched to count. A request's
  number of statements is constant as the library grows.
  - Enforced by: §4.3 and §4.5. *(A7)*
- **DL7. Bulk work is set-based or batched, never one statement per item over the library.**
  Small bounded N+1 on a read path is acceptable in SQLite if it stays constant in N.
  - Enforced by: the statement counts of §4.5 at two library sizes.

### Writes and transactions

- **DL8. A write transaction holds the writer for milliseconds: bounded chunks, never the
  library.** Read and compute outside it (through the reader pool, in bounded read
  transactions), then write the result. A long job commits in chunks of roughly 50–100 ms or a
  few hundred rows. Each chunk leaves the database valid and is safe to run again, and the writer
  is released between chunks.
  - Enforced by: the hold budget of §4.6, and the slow-write log in production. *(A3)*
- **DL9. A write that changes nothing writes nothing.** Every `ON CONFLICT DO UPDATE` and
  derivation `UPDATE` carries a change guard (`WHERE col IS NOT excluded.col`). Every trigger
  that maintains derived data is `AFTER UPDATE OF <its columns> WHEN old.x IS NOT new.x`.
  Comparisons leave out the fields the writer stamps itself, the Jellyfin trap.
  - Enforced by: the second-pass-writes-zero test (§4.6), and a `sqlc vet` rule for unguarded
    upserts (§4.7). *(A2)*
- **DL10. Reading never waits for the writer.** Nothing on a read request's path writes. A
  bookkeeping value such as last-used or last-seen is kept in memory and written later in a
  batch, or kept outside the main database.
  - Enforced by: "a GET makes no write" in the API tests (§4.5). *(A5)*
- **DL11. The writer's queue has an order: people before background work.** A request's or a
  person's write goes before a pass's, a fetch's or a status report's whenever the writer
  changes hands. A high-rate background writer (node status, artwork, read-throughs) coalesces
  its writes and pauses while a long job runs.
  - Why: Go's pool picks a random waiter today.
  - Enforced by: a store test that queues background writes and asserts an interactive write
    goes next.

### Derived data

- **DL12. Each derived table names its sources, its derivation and what invalidates which
  rows. A change re-derives what it touched, not the library.** A full re-derivation is the
  backstop: on a slow schedule, after an upgrade, and on request. It is cheap because of DL9 and
  DL13.
  - Why: Kleppmann's derived data; Navidrome's "touched" albums.
  - Enforced by: a test that changing one item touches O(1) rows, and a test that the
    incremental result equals a full rebuild. *(A4)*
- **DL13. A derivation can tell, without recomputing, that its inputs have not changed.** A
  fingerprint of the inputs is stored with the derived rows: the file's identity, the parsed
  name, the provider record's version, the locale, and a rules version that is bumped when the
  derivation's code changes. An equal fingerprint skips the item.
  - Why: Navidrome's folder hash.
- **DL14. Requests to re-derive are coalesced.** A burst of N events marks work and wakes one
  worker, which runs once for the burst after a short quiet period.
  - Enforced by: a `clocktest` test firing N events and counting runs. *(A4)*

### Operations and observability

- **DL15. The planner gets statistics that do not mislead it, and every connection loads them.**
  Our proposal, with the reasoning in §1.5: a committed `sqlite_stat1` describing a reference
  library, loaded by every connection. That gives the same plans in tests and production, and no
  `ANALYZE` holds the writer at a household's expense. The alternative is `ANALYZE` per index
  after migrations and large changes, Navidrome-style. *(A8)*
- **DL16. The connection settings follow measured practice:** a `cache_size` of tens of MB per
  connection, a `journal_size_limit`, and read transactions closed promptly, so checkpoints are
  never starved. FTS5 merging runs in the background (`automerge=0` plus `'merge'` steps), not
  inside whoever writes. Each setting is read back at open, as sql.md §7 already does.
- **DL17. Slow work names itself.** A write that waited names its writer, the caller holding it
  and how long it held. A slow request reports its statements by query name and its writes. A
  slow statement reports its name, time and pages touched. The thresholds are settings.
  - Enforced by: tests of each log line. Logging every waiting write is in place since
    v0.2.0-nas.6; naming the holder is not.

## 4. Proposed gates, in the order they pay

Measures that don't depend on the clock (pages touched, statements run, rows written) carry the
gates. Time budgets are only a backstop, since wall time on shared machines varies 10–30%; SQLite
measures its own performance in CPU cycles for the same reason ([cpu](https://sqlite.org/cpu.html)).

What modernc exposes decides the mechanism:
- **Not reachable:** `sqlite3_stmt_status` (per-statement full-scan and step counters) and
  `sqlite3_trace_v2`. They exist in its `lib` package, but the connection handle is private.
- **Public:** `sqlite.DBStatus`, through `sql.Conn.Raw`. Page-cache hits plus misses before and
  after a statement give **pages touched**. On a 20,000-row `sort_key` shaped like ours, the
  delete by item touched **179 pages without the index and 4 with it** (measured). That count
  doesn't depend on the machine.
- **Also public:** the pre-update hook, which counts rows written, through `Raw`.

1. **Every foreign key has an index** (unit tier, milliseconds). A test walks
   `pragma_foreign_key_list` against `pragma_index_list` and `pragma_index_info`. It keeps an
   allowlist with reasons, in the established `fullScansAllowed` style. It flags `sort_key` today
   with no data. *(DL3)*
2. **Every query's plan is read, not only `List…`.** `TestReferenceScale` checks 84 of 297
   queries; check all of them, plus search's three hand-written statements and cache.db's
   queries. Also fail on `AUTOMATIC … INDEX`, and on `TEMP B-TREE FOR ORDER BY` in a paged
   query. This catches the tombstone lookup (A6). *(DL1, DL2, DL4)*
3. **A GET writes nothing, and each request's statements are counted.** A wrapper on sqlc's
   `DBTX` seam in `internal/store` tallies each call by its query name, which every generated
   statement begins with. The tally lives in the request's context. The API harness then asserts:
   - a GET makes zero writes;
   - no query runs more than a few times in one request;
   - a list route's count does not grow when the fixture grows (GitLab's control-count method).

   The same tally goes into the slow-request log line. *(DL6, DL10, DL17)*
4. **Weak prefixes are found from statistics.** On the scale fixture, after `ANALYZE` (or with
   the committed statistics of DL15), each `SEARCH`'s rows-per-probe comes from `sqlite_stat1`
   for the number of equality terms the plan names. It fails over a bound, except for keyset page
   walks with a `LIMIT`. This is the check that sees `(locale=?)`. *(DL2)*
5. **Pages touched do not grow with the library.** Hot queries and jobs run at n and 2n items.
   Pages touched (DBStatus) and statements run must grow no faster than the work. This catches
   quadratic shapes on any machine. *(DL2, DL7)*
6. **Identification pass gates,** on a generator of scanned, unidentified files in
   `identifytest`. (The reference fixture holds identified rows, so the pass has nothing to do
   there.)
   - A second pass over an unchanged library writes zero rows, counted by the pre-update hook.
   - Changing one item writes O(1) rows.
   - No transaction holds the writer past a budget. This one fails today, by design.

   *(DL8, DL9, DL12)*
7. **`sqlc vet` refuses an unguarded upsert:** a `DO UPDATE` with no `WHERE`, beside the existing
   `no-unbounded-write` rule. `sqlc.yaml` is a gate file, so this one is the owner's change.
   *(DL9)*
8. **A store-level statement wrapper** (`database/sql/driver`, standard library, about 200 lines)
   times each statement to its close and reads its pages touched. That gives production its
   slow-statement log and per-query counters, in place of the trace profile modernc does not
   expose. Build it once item 3 shows the need. *(DL17)*

## 5. What this changes in the audit's fix list

The audit's fixes stand, with these refinements:

- **Fix 1, indexes:** add `sort_key(item_id, locale)` and an index for removed files' payload
  hash, plus whatever gate 1 flags among the 36 unindexed foreign keys whose tables grow:
  `play_position(file_id)`, `play_event(file_id)`, `collection_member(member_title)`,
  `edition(title_id)`, `extra_relation(extra_title)`, `item_tag(item_id)`, `file(library_id)`,
  `file(attached_to_file)`.
- **Fix 2, write only what changed:** guard every derivation upsert and update, and narrow
  `item_text_au` to `UPDATE OF … WHEN …`. Add input fingerprints with a rules version (DL13), so
  an unchanged item is not even recomputed.
- **Fix 3, authentication:** session last-used is kept in memory and flushed in a batch every few
  minutes and at shutdown. The idle check reads the memory first. A crash costs at worst a few
  minutes of a days-long idle window.
- **Fix 4, one pass for many fetches:** a fetch marks the series dirty and wakes the pass worker,
  which runs after a quiet period over what was marked.
- **Fix 5, the pass:** reads through the reader pool in bounded read transactions, writes per
  title or a few hundred rows, and releases the writer between chunks. The priority order (DL11)
  goes in with it, and background writers coalesce.
- **Fix 8, statistics:** replaced by DL15. Do not adopt `PRAGMA optimize` as the audit
  proposed; Navidrome's evidence is against it as the only mechanism.
- **New: FTS5 upkeep.** `automerge=0` with a background merge. Consider `detail=none` on the
  trigram index if it serves only substring search.
- **Not recommended:** a driver switch; `BEGIN CONCURRENT`/`wal2`; locks that block reads during
  writes; longer busy timeouts, which turn fast errors into slow ones.

The owner allows a fresh database on staging and the production NAS where that gives the better design
(2026-10-15). That makes it natural to redesign `sort_key`, the FTS triggers, the fingerprints
and session storage as a new baseline, rather than as steps on top of version 36.

## 6. Questions for the owner

1. Which of DL1–DL17 become standard, and do any need changing?
2. **Statistics (DL15):** committed statistics for a reference library, or `ANALYZE` per index
   after migrations and large changes?
3. **Sessions' last-used time:** in memory with a periodic flush (no schema change), or moved to
   `cache.db` (Emby, PocketBase and Rails style)?
4. **The gates of §4** that touch gate files: `sqlc.yaml` (item 7), `internal/archtest` (item 1,
   if it lives there), and the committed statistics if DL15 goes that way.

## Sources

Read and cached under `/yoloai/cache/research/db/`, where the four threads' full notes are kept
(`sqlite-internals.md`, `go-sqlite-practice.md`, `design-principles.md`, `testing-gates.md`).

- SQLite: [query planner](https://sqlite.org/queryplanner.html),
  [optimizer overview](https://sqlite.org/optoverview.html),
  [next-generation planner and checklist](https://sqlite.org/queryplanner-ng.html),
  [EXPLAIN QUERY PLAN](https://sqlite.org/eqp.html), [partial indexes](https://sqlite.org/partialindex.html),
  [WITHOUT ROWID](https://sqlite.org/withoutrowid.html), [ANALYZE](https://sqlite.org/lang_analyze.html),
  [PRAGMA optimize](https://sqlite.org/pragma.html#pragma_optimize), [WAL](https://sqlite.org/wal.html),
  [transactions](https://sqlite.org/lang_transaction.html), [UPSERT](https://sqlite.org/lang_upsert.html),
  [triggers](https://sqlite.org/lang_createtrigger.html), [FTS5](https://sqlite.org/fts5.html),
  [foreign keys](https://sqlite.org/foreignkeys.html), [N+1 in SQLite](https://sqlite.org/np1queryprob.html),
  [statement counters](https://sqlite.org/c3ref/c_stmtstatus_counter.html),
  [measuring CPU](https://sqlite.org/cpu.html).
- Markus Winand, [Use The Index, Luke](https://use-the-index-luke.com) (concatenated keys,
  partial indexes, the "most selective first" myth, keyset pagination, index-only scans).
- Martin Kleppmann, *Designing Data-Intensive Applications* ch. 11–12, and
  [Turning the database inside-out](https://martin.kleppmann.com/2015/03/04/turning-the-database-inside-out.html).
- Campbell and Majors, *Database Reliability Engineering* (summaries; not read in full).
- Alex DeBrie, [single-table design](https://www.alexdebrie.com/posts/dynamodb-single-table/)
  and [no bad queries](https://www.alexdebrie.com/posts/dynamodb-no-bad-queries/).
- Brandur Leach, [soft deletion](https://brandur.org/soft-deletion) and
  [atomic phases](https://brandur.org/idempotency-keys).
- Stephen Margheim, [SQLite on Rails](https://fractaledmind.github.io/2024/04/15/sqlite-on-rails-the-how-and-why-of-optimal-performance/);
  Bert Hubert, [database locked despite timeout](https://berthub.eu/articles/posts/a-brief-post-on-sqlite3-database-locked-despite-timeout/);
  [Litestream tips](https://litestream.io/tips/).
- Navidrome [source](https://github.com/navidrome/navidrome), [#5740](https://github.com/navidrome/navidrome/pull/5740),
  [#6201](https://github.com/navidrome/navidrome/pull/6201); Jellyfin
  [locking post](https://jellyfin.org/posts/SQLite-locking/), [#14047](https://github.com/jellyfin/jellyfin/pull/14047),
  [#15070](https://github.com/jellyfin/jellyfin/issues/15070), [#15986](https://github.com/jellyfin/jellyfin/pull/15986),
  [#18274](https://github.com/jellyfin/jellyfin/issues/18274); [PocketBase](https://github.com/pocketbase/pocketbase);
  [tailscale/sqlite](https://github.com/tailscale/sqlite).
- [go-sqlite-bench](https://github.com/cvilsmeier/go-sqlite-bench); modernc.org/sqlite v1.59.0
  and Go 1.26.2 `database/sql` source, read locally.
- Gates precedent: [Django assertNumQueries](https://docs.djangoproject.com/en/5.2/topics/testing/tools/),
  [GitLab QueryRecorder](https://docs.gitlab.com/development/database/query_recorder/),
  [Prosopite](https://github.com/charkost/prosopite), [sqlc vet](https://docs.sqlc.dev/en/latest/howto/vet.html).
