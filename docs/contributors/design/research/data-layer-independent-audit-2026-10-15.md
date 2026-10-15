> **ABOUTME:** An independent audit of the data layer and of the proposed principles DL1–DL17,
> written from a senior DBA's and data architect's position: the proposal fixes speed but not
> correctness once the pass leaves the serial writer; amendments to DL3, DL8, DL11, DL15, DL16;
> thirteen missing principles (M1–M13); fifteen schema and code findings (S1–S15); and an ordered
> plan for the fresh baseline.

*Settled after the audit, 2026-10-15: sqlc 1.31.1 parses `STRICT` tables (generated a `STRICT`
and a `STRICT, WITHOUT ROWID` table with typed fields), so M5 needs no fallback; modernc.org/sqlite
v1.59.0 exposes SQLite's online backup API (`conn.NewBackup`, `Backup.Step(n)`), reachable through
`sql.Conn.Raw`, so S7 has a second route besides a maintenance connection's `VACUUM INTO`.*

# Independent audit of Orpheum's data layer and the proposed principles DL1–DL17

*2026-10-15. Written from the position of a long-serving DBA and data architect who was not
involved in the design. Everything here was read from the repository at an earlier commit or measured in
a scratch directory; nothing in `/home/karl/orpheum` was changed. Claims marked **(measured)** were
reproduced here on CPython's SQLite 3.46.1 (the behaviour concerned is the same in 3.53). Claims
marked **(from memory)** are from long practice and literature I did not re-read for this report;
check them before acting on them. "Verify" marks a point I could not settle here.*

Inputs read: the database access audit and the data-layer practice research of 2026-10-15,
`standards/sql.md`, `plans/schema.md`, `migrations/0036_baseline.sql` and steps 0037–0042,
`cache/schema.sql`, the query files, `store/open.go`, `store/doc.go`, `store/backup.go`,
`identify/pass.go`, `text.go`, `sortkeys.go`, `chooser.go`, `auth/auth.go`'s `Authenticate`,
`library/ingest.go`, and the 103 `.Write(ctx` call sites in `internal/core`.

---

## 1. Verdict

**The proposal is sound in direction, unusually well evidenced, and incomplete in one
dangerous way.** It correctly diagnoses the incident as a service-time problem on a single
server, not a lock or driver problem. Its remedies are the ones every mature shop reaches:
write less, write only deltas, keep transactions short, index the access paths you actually
use, and gate on machine-independent counters rather than wall time. The gates in §4, built on
pages touched, statements counted and rows written, are better than what most commercial teams
have. I would adopt most of DL1–DL17.

The gap is that **the proposal is a performance programme with no correctness programme beside
it.** Today the whole identification pass runs inside one `BEGIN IMMEDIATE` transaction on the
one writer. That is slow, but it is *serializable*: nothing can interleave with it. DL8 ("read
and compute outside it … then write the result … in chunks") deliberately gives that up. The
proposal then never says how the system will avoid the anomalies that appear once it does:

- **Lost updates.** The pass reads a title, a person confirms or overrides it, and the pass
  writes back values computed from the stale read.
- **Read skew.** A multi-query read sees different commits per statement. `Database.Read()`
  hands each sqlc call its own autocommit statement on a pooled connection, so there is no
  snapshot to "read outside the writer" *in*.
- **Duplicate identity.** Two derivations both decide a title does not exist yet. Nothing in the
  schema forbids the second insert.
- **Half-applied derivations.** A chunk boundary falls inside a group of rows that must change
  together.

Each of these is textbook: Berenson et al., *A Critique of ANSI SQL Isolation Levels* (1995,
<https://www.microsoft.com/en-us/research/publication/a-critique-of-ansi-sql-isolation-levels/>),
and Kleppmann, *DDIA* ch. 7. None of them is hypothetical in this code (see §4, S1–S3). **DL8
must not be built until the principles of §3 items M1–M3 are adopted with it.** Otherwise the
project trades a latency incident for silent data corruption, which is the worse trade by every
measure a DBA uses.

The second structural point is that **the pass's granularity is wrong, not only its transaction
length.** It is one function that does two different jobs:

1. **Grouping.** It decides which files make which titles and series. This is cross-item and
   needs a library-wide view.
2. **Resolution.** It computes a title's values from overrides, provider data and local
   fallbacks. This is a pure per-item function.

Today a one-field override (`chooser.rederive`) or one series match (`recordSeriesAndRederive`)
re-runs both over the whole library, synchronously, on the request path in the chooser's case.
Splitting the two is the redesign that makes DL8, DL12 and DL14 easy, and the fresh baseline is
the time to do it.

Third, the schema leans on the application for integrity that the engine could enforce, and the
engine is about to lose the serial execution that made that safe. It has no STRICT tables, no
cross-column CHECKs, no supertype/subtype enforcement, and no declared identity keys beyond
surrogates. Constraints are the only integrity mechanism that survives concurrency, refactoring
and new writers. A fresh baseline should add them.

Smaller verdicts:

- **DL15 (committed statistics)** is defensible; it is plan baselining in the Oracle SPM sense. It
  needs stated consequences: no `PRAGMA optimize`, ever, and tests over more than one data shape.
- **DL16's FTS5 `automerge=0`** is likely over-engineering at household scale.
- **DL3 is over-justified.** Unindexed foreign keys cost nothing at runtime when parents are never
  deleted, which is the tombstone design.

---

## 2. Principle by principle

**DL1: access paths written down, each with its index. *Agree.***
This is DeBrie's and Winand's discipline, and the committed plan table makes plan drift a reviewed
diff. **Amend:**

- The record of each path should also hold its *expected cardinality at reference scale* (rows per
  probe), so a reviewer can see that `SEARCH … (locale=?)` reads 20,000 rows.
- Cover `cache.db` and `search.go` from day one. The job queue has the same quadratic defect as
  `sort_key` (S4), and the audit did not look at it.

**DL2: a second lookup path gets its own index; low-cardinality columns do not lead. *Agree*,**
with Winand's caveat that "selectivity first" is a myth. Order columns by *how queries bind
them*: equality first, then range or order. `sort_key`'s `(locale, listing, collation_key,
item_id)` is correct for its listing path precisely because every listing query binds both
leading columns. The rule should say "unless every query binds it", which the text nearly does.

**DL3: every foreign key's child column leads an index. *Amend*.** SQLite needs the child index
only when a **parent row is deleted or its key updated**. Inserting or updating a child looks up
the parent by its primary key, which is always indexed. Orpheum's parents are tombstoned, not
deleted, under `ON DELETE RESTRICT` (sql.md §6), so most of the 36 unindexed foreign keys cost
nothing at runtime (from memory; <https://sqlite.org/foreignkeys.html> §3).

`sort_key` was slow because of a missing **access path** (DL1/DL2), not foreign-key enforcement:
its `ON DELETE CASCADE` never fires, because items are never deleted. Reword DL3 as:

- "A foreign key whose parent can be deleted, or whose action is CASCADE or SET NULL, has a child
  index."
- "Every other child column is indexed when a query reads by it (DL1)."

That keeps the gate (§4.1) honest and avoids indexes that cost writes and serve nothing, which is
DL5. As written, DL3 and DL5's "every index appears in some plan" gate conflict: a foreign-key
index serves enforcement, which `EXPLAIN QUERY PLAN` of the application's queries never shows.

**DL4: a partial index lists its queries and repeats its condition; tombstones get their own
index. *Agree.*** This is exactly SQLite's implication rule (partialindex §3). Add: **prefer
generated or literal conditions to parameterised ones.** `removed_at IS NULL` works because it is
literal; a query that passes `removed_at IS ?` will never use it.

**DL5: no speculative indexes. *Agree*,** with the foreign-key exemption above. A household
database is read-mostly, so index maintenance cost is small. The real point is to keep hot,
frequently-updated columns (`last_used_at`, `last_seen_at`, `position_ms`) out of indexes, as it
says.

**DL6: nothing a request triggers grows with the library. *Strongly agree.*** This is the single
most valuable rule for an OLTP path, and the statement-count gate (GitLab's QueryRecorder method)
is the right enforcement.

**DL7: bulk work set-based or batched. *Agree.*** Note that "set-based" in SQLite often means one
prepared statement executed N times in one transaction, not one giant SQL statement. Both are
fine; what matters is the constant number of statements per unit.

**DL8: write transactions hold the writer for milliseconds; read outside; chunked. *Agree with
the goal; disagree with the wording as a standalone rule.*** Four amendments, without which I
would oppose it:

1. **Chunk on consistency units, not on rows or milliseconds.** The unit of a transaction is the
   aggregate whose invariants must hold together (Evans, DDD; Vernon, *Effective Aggregate
   Design*, <https://www.dddcommunity.org/library/vernon_2011/>). In Orpheum that is one title
   with its versions, text, sort key and provider ids, or one series with its episodes, ordering
   and positions. "50–100 ms or a few hundred rows" is the *budget* a unit must fit, not the
   boundary. A 300-episode series is still microseconds per row; split it and readers can see a
   series half moved to another ordering.
2. **Read outside, verify inside.** Whatever decision the write depends on is re-checked inside
   the write transaction by cheap point reads, or by a compare-and-set predicate on the
   `UPDATE`. This is optimistic concurrency control (Kung & Robinson, *ACM TODS* 1981). See M1.
3. **"Outside" means inside a read transaction.** The store needs a `ReadTx(ctx, func(q))` that
   pins one connection and one WAL snapshot (`BEGIN` on the reader pool). Today `Read()` returns
   queries over the pool, so consecutive calls may run on different connections at different
   commits. See M2 and S1.
4. **Each chunk is idempotent and resumable**, and the job records which chunks are done, so a
   crash between chunks finishes rather than restarts. The proposal says "safe to run again";
   make it testable: kill the job after chunk k for every k and assert the final state equals an
   uninterrupted run.

Also give the budget a theoretical footing. With one server, the mean wait in an M/G/1 queue is
the Pollaczek–Khinchine formula, `W = λ·E[S²] / (2(1−ρ))` (Harchol-Balter, *Performance Modeling
and Design of Computer Systems*, 2013; from memory). The wait is driven by the **second moment**
of service time, not its mean. One 30 s transaction per 30 s among thousands of 35 µs ones makes
`E[S²]` about 10⁹ times larger than the small writes alone. That is why the cure is a cap on the
*maximum* service time, which is what DL8 is, and not "make the pass faster on average". Dean &
Barroso, *The Tail at Scale* (CACM 2013), make the same argument for tails.

**DL9: a write that changes nothing writes nothing. *Strongly agree.*** The guarded upsert
(`DO UPDATE … WHERE col IS NOT excluded.col`) and narrowed triggers (`AFTER UPDATE OF … WHEN
old.x IS NOT new.x`) are correct and measured. **Amend in two ways:**

- **Name the stamps.** Bookkeeping columns the writer sets itself (`last_seen_at`, `last_asked_at`,
  `updated_at`, `made_at`) are excluded from comparisons. Better still, *don't stamp what can be
  derived*. The scan's `SetFileSeen` writes `file.last_seen_at` for every unchanged file on every
  scan: a whole-library write per scan, which DL9 should forbid (S5). A file seen by the last
  complete scan of its root is already recorded by `root_scan.completed_at` plus the file being
  live. Keep a per-file stamp only when it changes meaning (a revival, a move).
- **Guards are also correctness devices.** `MovePlayPosition`'s `WHERE excluded.position_read_at >
  play_position.position_read_at` is exemplary compare-and-set. DL9's guard and M1's
  compare-and-set are the same mechanism; state them together.

**DL10: reading never waits for the writer. *Agree.*** Session last-used in memory, flushed in
batches, is the right call (question 3: memory, not `cache.db`). Two additions:

- **Make the flush monotonic.** `UpdateGrantLastUsed` today sets the value unconditionally, so a
  slow request can write an older time over a newer one. Use `WHERE last_used_at IS NULL OR
  last_used_at < ?`.
- **Keep security state durable.** Revocation and expiry decisions must never depend on the
  in-memory value alone; a revoke must be durable before it returns.

The GET-writes-nothing gate is excellent.

**DL11: writer queue ordered, people first. *Agree with the goal; amend the mechanism.***

- **Use a writer service, not pool arbitration.** Don't fight `database/sql`'s random hand-off;
  put a small writer service in front of the one connection, with two (or three) priority queues
  and **aging**, so background work cannot starve. Strict priority without aging is a classic
  starvation bug (from memory; any OS scheduling text).
- **Get group commit from the same service.** Many tiny interactive writes, such as preference
  changes, play reports and flushed last-used times, can be committed together in one
  transaction when they queue together. That is the oldest throughput trick in the book (Gray &
  Reuter, *Transaction Processing*, from memory). It needs care: one failing write must not roll
  back its neighbours. Use savepoints per item.
- **Measure two things.** Expose the queue depth per priority and the wait and hold histograms
  (DL17).

**DL12: derived tables name sources, derivation and invalidation; re-derive what was touched.
*Strongly agree*.** This is Kleppmann's derived-data model, and the A/I/O/F/C class taxonomy in
schema.md is already better than what I see in most commercial schemas. **Amend:** the
derivation's granularity must match the invalidation's (see the Verdict). Grouping (files to
titles and series) and resolution (a title's values) are two derivations with different inputs,
different invalidation and different costs. Make them two.

**DL13: input fingerprints with a rules version. *Agree, with a warning.*** A fingerprint that
omits one input produces *silent* staleness. That is the cache-invalidation failure every DBA has
debugged at 3 a.m. Three mitigations, all cheap:

1. A scheduled full rebuild that *compares* rather than writes, logging every item whose
   incremental result differs from a rebuild. That is a consistency checker; see M7.
2. The rules version derived mechanically: a test hashes the derivation's output over a fixed
   corpus and fails when the hash changes without a version bump. Then a code change cannot
   forget the bump.
3. A fingerprint that hashes the canonical *values* of the inputs, not row ids or timestamps.

**DL14: coalesce re-derivation requests. *Agree*.** It is debouncing with a dirty set. Persist the
dirty set (a small table) or make it derivable at startup, or a restart loses pending work.

**DL15: committed statistics. *Agree with conditions.*** This is plan baselining, as in Oracle
SQL Plan Management (from memory: <https://docs.oracle.com/en/database/oracle/oracle-database/19/tgsql/overview-of-sql-plan-management.html>).
Conditions:

- **No `PRAGMA optimize` and no `ANALYZE`, anywhere.** As I recall, `PRAGMA optimize` re-analyses
  a table whose row count has drifted far from `sqlite_stat1`'s, so on a real household it would
  overwrite the committed values. That includes the audit's own fix 7, and operators running the
  CLI. State this and test it (from memory; verify the drift threshold in the 3.53 docs).
- **Commit `sqlite_stat1` only, and delete `sqlite_stat4`.** Its samples are data-specific and
  hard to fix by hand.
- **Test plans over several data shapes**: film-heavy, series-heavy (the nas.example shape), tiny,
  and 100k files. With fixed statistics, the risk moves from "the planner guesses" to "our guess
  is wrong for this household".
- **Use the hints you already use.** `CROSS JOIN` ordering is already used in `browse.sql`, and
  `stat1` takes the `unordered` and `noskipscan` hints (from memory). Pinning hot plans explicitly
  is more robust than tuning numbers.

Navidrome's evidence does justify distrust of runtime `ANALYZE` on slow NAS storage.

**DL16: connection settings. *Agree*** on `cache_size` and `journal_size_limit`. **Disagree** with
FTS5 `automerge=0` as a default:

- At about 100k short names both FTS indexes are a few megabytes; their merges cost milliseconds.
- With `automerge=0` the index fragments unless the background merger reliably runs.
- My recollection is that `crisismerge` still fires on the write path regardless (verify).

Once DL9 stops the four FTS writes per item per pass, the merge load nearly vanishes. Measure
after DL9, then decide. Add a rule that **no read transaction stays open for long** (a streamed
response, an unclosed `Rows`), because it pins the WAL and starves checkpoints. Enforce it with a
"longest-open read transaction" metric.

**DL17: slow work names itself. *Agree*,** and expand it into the metrics list of M6.

---

## 3. Missing principles, ranked

**M1. Decide outside, verify inside: optimistic concurrency for every read-then-write.**
*(Highest. DL8 creates the hazard.)*

- Any write whose content depends on a read made outside its transaction carries a
  compare-and-set predicate (`WHERE revision = ?`, `WHERE input_fingerprint IS ?`, or a domain
  guard like `MovePlayPosition`'s), or re-reads its premises inside the transaction.
- A zero-row result is a conflict to retry or drop, never success.
- Authored and mixed rows (`title`, `collection`, `item_text`, `field_override`, `artwork`'s
  choice) get a `revision INTEGER NOT NULL DEFAULT 0` incremented on every write.
- Enforce with a concurrency test: interleave a person's confirm or override between the pass's
  read and write, and assert the person wins.

**M2. Multi-statement reads use one snapshot.** Add `Database.ReadTx` (and `Cache.ReadTx`). A page
or decision built from more than one query reads inside one read transaction. `Authenticate`
today reads the grant, the user and the role's reach as three separate snapshots. The anomaly is
benign there; on a title page joining versions, files and play state it is not.

**M3. Identity is declared, not inferred.** Every entity that must not be duplicated has a
`UNIQUE` constraint on its natural or business key beside its surrogate key. Surrogate keys are
right (sql.md is correct to forbid UUIDs and reissue). But a surrogate key says nothing about
*sameness*, and only a unique constraint stops two concurrent derivations from both creating "The
Unknown (1982)" (Date, *An Introduction to Database Systems*; Karwin, *SQL Antipatterns*, from
memory). Today title identity lives in the pass's in-memory `titleIDByKey` map, which is only
safe because the pass is the sole writer for 30 s at a time.

**M4. Transactions follow aggregates.** Name each aggregate and its invariants in schema.md, and
make "one aggregate per write transaction" the rule DL8 budgets. Cross-aggregate effects happen
through coalesced follow-up work (DL14), not wider transactions. Helland's *Life beyond
Distributed Transactions* (CIDR 2007, <https://www.cidrdb.org/cidr2007/papers/cidr07p15.pdf>) is
the clearest statement, and it applies inside one process as much as across machines.

**M5. Constraints are the first line of integrity.**

- `STRICT` on every table (SQLite 3.37+, <https://sqlite.org/stricttables.html>). Verify that sqlc
  parses it; if it does not, add a schema test asserting `typeof()` of every BLOB key column.
- Cross-column `CHECK`s for every rule a row must obey alone.
- Supertype/subtype integrity via composite foreign keys (S9).
- `NOT NULL` wherever a state has no "absent" meaning.

Review cannot keep these once there are many writers; the engine can.

**M6. Operability: the database is observable, checked and recoverable.**

- **Stated RPO and RTO.** For example: RPO one day with `VACUUM INTO` nightly, minutes with
  Litestream; RTO the time to restore and migrate.
- **Restore tested in CI and drilled.**
- **`PRAGMA quick_check`** in the background after start and **`integrity_check`** weekly on the
  backup copy, so the long read is not on the live file. Add the FTS5 `'integrity-check'` and
  `foreign_key_check` too.
- **Metrics:** writer wait and hold histograms by caller and priority; queue depth; WAL size and
  checkpoint results (busy, log, checkpointed); age of the oldest open read transaction;
  `page_count`, `freelist_count`, file sizes; rows per table for the growing tables; cache hit
  ratio from `DBStatus`; slow statements by name.
- **A runbook** for "database is slow", "disk full", "corrupt", "WAL huge" and "restore".

Campbell & Majors, *Database Reliability Engineering* (O'Reilly 2017), and SQLite's
<https://sqlite.org/howtocorrupt.html> are the references.

**M7. Invariants are executable.** Write the data invariants as SQL queries that must return zero
rows. Examples:

- Every confirmed title has a `followed_source_id`.
- Every live named item has exactly one `sort_key` in the current locale.
- No live file belongs to a removed version.
- Every open job's target exists in `persistent.db`.
- The FTS `integrity-check` passes.

Run them after every test (a test-harness hook), after migrations, and periodically in production
read-only, logging violations. This is DBCC CHECKDB or fsck for the domain, and the
cheapest-to-build safety net there is. It is also the oracle for property-based tests and crash
tests (M9).

**M8. Separate data by volatility and width.** Volatile bookkeeping goes in memory or `cache.db`:
node status, last-used times, served-art recency (already there). Wide, rarely-read payloads
(probe JSON, NFO bytes, provider bodies) go in side tables, so that hot rows stay narrow and
updates to hot columns stay cheap (S5, S6). This is vertical partitioning, standard since the
1970s, and SQLite's overflow-page storage makes it matter more, not less.

**M9. Test like the data matters.**

- Property- or model-based tests: random libraries plus random operation sequences, compared
  against a full rebuild and checked with M7's invariants.
- Crash injection: kill the process mid-pass or mid-chunk in a loop, restart, check invariants.
  SQLite's own crash testing is the model (<https://sqlite.org/testing.html>).
- Fixtures with *skewed* distributions, not uniform ones.
- Migration tests from every golden: these exist and are good.

**M10. Every growing table has a retention policy.**

- `play_event`, `play_span`: append-only, kept forever by design. Fine, but say so and estimate.
- `file` tombstones: every replaced rip leaves one.
- `artwork` rows, provider `season/N` parts, finished jobs: `DeleteFinishedJobs` exists, good.

State per table: keep forever, keep N days, or compact. At household scale the volume is small,
and the policy's value is that nothing grows unexamined. Plex's 157 million bandwidth rows were
an absence of this principle.

**M11. Cross-file consistency is designed, not assumed.**

- Two files means no cross-file transaction. `ATTACH` does not help: in WAL mode a multi-file
  commit is atomic per file only (<https://sqlite.org/lang_attach.html>, from memory of its
  wording).
- So every protocol that writes both `persistent.db` and `cache.db` fixes an order, normally
  persistent first, as the fact, then cache as the derived note.
- The second write is idempotent, and a sweep repairs orphans in both directions (a job whose
  item is gone; an item owed work with no job). D231 already derives work from facts, which is
  exactly right. Make the principle explicit and add the orphan checks to M7.

**M12. Personal data has an inventory and a deletion story.**

- **What holds it:** `user.name`, `admin_note`, play history, device names, `grant` rows.
- **Today's gap:** tombstoning with `ON DELETE RESTRICT` means "delete this person" leaves their
  history.
- **The fix:** a household may not need GDPR Art. 17 erasure, but a self-hosted product will be
  asked. Decide now whether deletion means pseudonymise (null the name, keep ids) or erase
  (cascade through history). The baseline's foreign-key actions follow from that decision.

**M13. Temporal data is chosen deliberately.** For authored data (`field_override`, `setting`,
role changes), decide whether history matters: who changed a title's name and when, or undo.
Today the last write wins and the previous value is lost. If history matters, a small
transaction-time history table per authored table is cheap (Snodgrass, *Developing Time-Oriented
Database Applications in SQL*, <https://www2.cs.arizona.edu/~rts/tdbbook.pdf>). If not, say so.

---

## 4. Schema and code findings, by severity

### High

**S1. There is no read transaction, so "read outside the writer" has no snapshot.**
`store/open.go`: `Read()` returns `sqlcgen.New(database.readers)`, so each call is an autocommit
statement on whichever pooled connection is free. Consequences:

- Every multi-query read is non-repeatable. `Authenticate` (auth.go:543–577) reads
  `GetLiveGrant`, `liveUser` and `reachOfRole` separately.
- DL8's plan to load the library through the reader pool would see a library that changes
  between its own queries: files from one commit, titles from another.

Fix: add `ReadTx` before DL8 is built, and use it wherever a decision combines reads (M2).

**S2. A lost update is waiting in `UpdateTitleResolved`.** `queries/identify.sql`:

```sql
UPDATE title SET year = …, runtime_ms = …, followed_source = sqlc.arg(followed_source),
  followed_source_id = sqlc.arg(followed_source_id),
  identification_state = CASE WHEN identification_state = 'confirmed' THEN identification_state ELSE sqlc.arg(identification_state) END
WHERE id = sqlc.arg(id);
```

It protects the *state* of a confirmed title but overwrites the *source and id* unconditionally.
That is safe today only because the pass reads and writes inside one serial transaction. Once the
pass computes outside the writer (DL8), a person can confirm match X between the pass's read and
its write. The pass then writes the stale `followed_source_id` under a state that still says
`confirmed`: an internally inconsistent row that no error reports.

Fix: compare-and-set on a `revision` or input fingerprint (M1), and a CHECK or invariant that a
confirmed title has a source id. The same audit applies to every `UPDATE` the pass issues
(`MoveVersionToTitle`, `MoveEpisodeToSeries`, `UpsertPosition`, `UpsertCollectionMember`).

**S3. The derivation has no per-item entry point.** `chooser.rederive` (chooser.go:395) answers a
person's single-field override by calling `pass.IdentifyLibrary` for the whole library, inside
the request. `recordSeriesAndRederive` and `refresh.go:266` do the same per fetch (audit A4). The
root cause is that grouping and resolution are one function (Verdict). With the split:

- An override, a match or a refresh re-resolves **one item**: point reads, one short transaction.
- A scan delta re-groups **the affected folders**.
- The whole-library run becomes the slow-schedule backstop DL12 describes.

This removes A3 and A4 by construction rather than by budget.

**S4. The job queue in `cache.db` has the audit's quadratic shape, unaudited.** Measured with
`EXPLAIN QUERY PLAN` on `cache/schema.sql`:

- `ListOpenJobsOfTarget` (run on every enqueue, to keep "at most one open job per target") is
  **`SCAN job`**, and `SetJobWanted` has the same predicate. A sweep that enqueues N fetches does
  N full scans of the queue: O(N²).
- `NextQueuedJob` uses `job_next (state=?)`, whose leading column is constant inside its own
  partial index. It walks every queued job of *every* kind in priority order to find one of the
  requested kind.
- The one-open-job-per-target rule is enforced by read-then-insert in Go, which is correct only
  because `cache.db` also has one serial writer.

Fix for the fresh `cache.db`:

- Replace the eight nullable target columns with `target_kind TEXT, target_id INTEGER`. The CHECK
  that at most one is set disappears with them. The "polymorphic association" antipattern is
  acceptable here, since no foreign key can cross files anyway.
- `CREATE UNIQUE INDEX job_open_target ON job(kind, target_kind, target_id) WHERE state IN
  ('queued','running','assigned')`, and enqueue with `INSERT … ON CONFLICT DO UPDATE`, which also
  replaces `MergeQueuedJob`'s read-modify-write.
- `CREATE INDEX job_claim ON job(kind, priority DESC, runnable_at, id) WHERE state = 'queued'`.

### Medium-high

**S5. `file` is a wide row with hot columns, and the scan rewrites it every time.** `file` holds
`probe_json` (ffprobe output, typically several kB), `nfo_body`, `error_detail_json` and the path
twice (text and raw bytes), beside `last_seen_at`, `filing` and `version_id`, which change often.
SQLite stores a row as one record with overflow pages, so:

- **(measured)** Updating a same-width integer on 2,000 rows with 12 kB blobs wrote **2,000 WAL
  pages**: one cell page per row, because each row fills its page. The same update with the blob
  in a side table wrote **40**.
- **(measured)** An update that changes a column's *size* rewrote the overflow chain: **6,003
  pages**, three per row.
- **(measured)** A scan of the narrow table took 0.28 ms; the wide one 7.3 ms.

The code paths affected:

- `SetFileSeen` runs for every unchanged file on every scan: a whole-library, page-per-file write.
- `UpdateObservedFile` rewrites `probe_json` even when it is unchanged.
- `ListIdentifyFiles` selects `probe_json` for every file in every pass, only so that
  `probeRuntimeMs` can parse the JSON for one number.
- `ListRecentPlayPositions` calls `json_extract(file.probe_json, '$.format.durationMs')` per row.

Fix:

- Split into `file` (narrow: identity, path, filing, version, tombstone) and `file_probe(file_id
  PK, probe_json, duration_ms, …)` with the few fields queries need extracted into typed columns
  at ingest.
- Add `file_nfo(file_id PK, body)`.
- Write `last_seen_at` only on change of meaning, or drop it in favour of the root's scan record
  (DL9 amendment).

`provider_response` has the same shape, with `last_asked_at` beside `body_gzip`. Split it too, or
keep the stamp the same width.

**S6. Telemetry rides the main writer and the backup.** `nodes.takeStatus` writes
`node.last_status_json` and `last_status_at` into `persistent.db` on every status report. It
competes with requests for the one writer (it appears among the audit's waiters), churns the WAL,
and lands in backups, for data whose only use is "is the node alive now". Keep it in memory, with
a coarse `last_seen` written rarely, or in `cache.db`. Session `last_used_at` (DL10) is the same
class of data.

### Medium

**S7. The backup holds the writer.** `BackupInto` runs `VACUUM INTO` on `database.writer`, a pool
of one connection, so every write waits for the backup's duration. That is 177 ms at the
reference scale, and seconds to minutes on a large library on slow disks. It cannot simply move
to the reader pool: **(measured)** `VACUUM INTO` on a `query_only` connection fails with "attempt
to write a readonly database". Two fixes:

- A dedicated maintenance connection, not `query_only` and not the writer, whose `VACUUM INTO` is
  a read transaction and does not block the writer.
- SQLite's online backup API, if modernc exposes it through `Conn.Raw` (verify).

The same connection should run checkpoints, `quick_check` and any FTS merge.

**S8. No STRICT tables; BLOB keys are exposed to affinity.** `payload_hash`, `collation_key`,
`secret_sha256`, `source_hash` and `locate_hash` are declared `BLOB`. Without STRICT, a caller
that passes a hex `string` instead of `[]byte` stores TEXT, which never compares equal to the BLOB
form. The result is silent misses: a duplicate file, a lost loudness row, a session that never
matches. `sort_key.collation_key` would *sort* differently too, since TEXT sorts before BLOB.
STRICT turns these into errors. If sqlc cannot parse `STRICT`, a schema test asserting
`typeof(col) = 'blob'` over fixtures is the fallback.

**S9. Supertype/subtype integrity is not enforced.**

- `title.id` and `collection.id` reference `item(id)`, but nothing requires `item.kind = 'title'`
  or `'collection'`, nor prevents one item having both a `title` and a `collection` row.
- The standard fix (Celko, *SQL for Smarties*, from memory) is `UNIQUE (id, kind)` on `item`, plus
  in each subtype a `kind` column with a CHECK fixing its value and a composite foreign key
  `(id, kind) REFERENCES item(id, kind)`.

Related gaps in cross-column rules:

- `collection.identification_state` is nullable even for `kind='series'`, which is what is
  identified (D209).
- `library.check_every_hours` should be `NOT NULL` if and only if `change_detection = 'interval'`.
- `title.followed_source_id` should require `followed_source`.
- `"grant"` carries nine kinds with kind-specific nullable columns (`device_name`, `device_id`,
  `locate_hash`, `offer_key`, `link_sealed`) and no CHECK tying them to kind. schema.md already
  notes that `device_name` and `device_id` are dead.
- `preference.scope_id` is a polymorphic reference with `0` as a sentinel and no integrity at all.

**S10. Identity uniqueness is undeclared.**

- `item_provider_id` has only `PRIMARY KEY (item_id, provider, provider_id)` and a non-unique
  lookup index, so one TMDB id can belong to several live items. The code expects and handles
  this (`ListTitlesByProviderID … ORDER BY`), but nothing defines when it is legitimate.
- Titles have no natural key at all.

Decide whether "one live item per (provider, provider_id) per library" is an invariant. If so,
make it a partial unique index. If not, write down why. Either way, M3 must be settled before the
pass leaves the serial writer.

**S11. No integrity checking, checkpoint control or WAL bound anywhere.** A grep of
`internal/**/*.go` finds no `quick_check`, `integrity_check`, `wal_checkpoint`,
`journal_size_limit` or `cache_size`. `foreign_key_check` runs only in migrations. See M6.

### Low

**S12. `item_text`'s FTS link relies on an implicit rowid.** The FTS tables use
`content_rowid='rowid'` over `item_text`, whose key is `(item_id, language)` with no `INTEGER
PRIMARY KEY`. SQLite documents that VACUUM *may* renumber such rowids
(<https://sqlite.org/lang_vacuum.html>). **(measured)** It did not on 3.46.1, VACUUM or VACUUM
INTO, but relying on undocumented behaviour in the path backups take is avoidable. sql.md's
rebuild rule already knows the hazard. Give `item_text` an `id INTEGER PRIMARY KEY` and point the
FTS tables at it.

**S13. `UpdateGrantLastUsed` is not monotonic** (DL10 above).

**S14. Fractional ordering in REAL** (`position.sort_order`, `collection_member.position`) runs
out of precision after about 50 midpoint insertions at one spot (from memory: binary64 has a
52-bit mantissa). Renumber on exhaustion, or use integer gaps. Rare in practice.

**S15. Multi-valued and stringly typed fields.**

- `role.capabilities` is a comma-separated set: Karwin's "jaywalking". It is tolerable only
  because the set is closed and validated in Go; a `role_capability(role_id, capability CHECK
  (…))` table costs nothing and lets SQL ask "who can administer".
- `setting.value_json` is a deliberate registry-backed key-value store. Acceptable as designed
  (D200).

### Things done well that the fresh baseline should keep

- **The class taxonomy (A/I/O/F/C), per column on mixed tables.** It is the right foundation for
  "derived data is rebuildable", and rarer in industry than it should be.
- **Tombstones with partial unique indexes and `live_*` views.**
- **`CHECK` vocabularies mirrored by exhaustive Go types.**
- **Integer milliseconds from an injectable clock.**
- **The engine setup:** `application_id`, pragmas read back at open, one writer with `BEGIN
  IMMEDIATE`, and a `query_only` reader pool.
- **The migration runner:** step checksums, a snapshot before migrating, free-space preflight,
  resume after interruption.
- **"One writing package per table"** (`TestArch_EachTableHasOneWritingPackage`), the
  application-level equivalent of a stored-procedure boundary.
- **`MovePlayPosition`'s guard**, the model for M1.
- **D231's "work derived from facts, not held as jobs"**, which is exactly how to make `cache.db`
  disposable.
- **The measured refusal of the `title_watch` cache** in schema.md: the right instinct, measure
  before denormalising.

---

## 5. Recommendations for the fresh baseline

In order. The first four are prerequisites for DL8.

1. **Store API first.**
   - `ReadTx` on both databases.
   - A writer service in front of each writer connection: priority queues with aging, optional
     group commit with per-item savepoints, and wait/hold metrics by caller.
   - A maintenance connection for backup, checkpoints, `quick_check` and merges.
   - `cache_size` (for example `-32000`, 32 MB) and `journal_size_limit` in the DSN, read back
     like the rest.
2. **Integrity in the schema.**
   - `STRICT` on every table, if sqlc permits.
   - Supertype/subtype composite keys on `item` → `title` and `collection`.
   - Cross-column CHECKs (S9).
   - `revision INTEGER NOT NULL DEFAULT 0` on `title`, `collection`, `item_text`,
     `field_override` and `artwork`.
   - Declared identity uniques (S10).
   - `item_text.id INTEGER PRIMARY KEY` as the FTS content rowid.
3. **Split the derivation.**
   - *Grouping* (files to titles and series): per folder, on scan deltas, transactions per
     aggregate.
   - *Resolution* (a title's or series' values, text, sort key and search fold): per item, a pure
     function of stored inputs, with `input_fingerprint` and `rules_version` columns on the
     resolved rows and guarded writes.
   - Overrides, matches and refreshes call resolution for one item. A coalescing worker (DL14)
     runs grouping. The whole-library run is the backstop, in compare mode.
4. **Physical layout by volatility and width.**
   - Narrow `file`; `file_probe` with typed extracted fields; `file_nfo`.
   - `provider_response` body separated from its stamps.
   - `sort_key` with `CREATE INDEX sort_key_item ON sort_key(item_id, locale)`. Consider
     carrying `library_id` in the listing key if per-library browsing for restricted roles proves
     sparse; measure first.
   - Node status and session last-used out of `persistent.db`'s write path.
   - An `identification_state` index (or the count query of audit fix 6), and the tombstone
     payload-hash index (A6).
5. **`cache.db` queue redesign (S4):**
   - `target_kind`/`target_id`.
   - The partial unique index on open jobs.
   - The claim index with `kind` first.
   - `ON CONFLICT` enqueue.
6. **Triggers and guards.**
   - `item_text_au` becomes `AFTER UPDATE OF name, search_alternates, overview … WHEN old.name IS
     NOT new.name OR …`.
   - Every upsert gets the `IS NOT excluded.x` guard (DL9), and `sqlc vet` enforces it.
7. **Creation-time pragmas,** which cannot be changed cheaply later:
   - `auto_vacuum=INCREMENTAL` on `cache.db`, which churns, with periodic `incremental_vacuum` from
     the maintenance connection. Optional on `persistent.db`, which mostly grows.
   - Keep `page_size` 4096.
8. **Statistics (DL15):** commit `sqlite_stat1` built from the reference fixture. Gate that nothing
   runs `ANALYZE` or `PRAGMA optimize`. Read plans over three data shapes in `TestReferenceScale`.
9. **Executable invariants (M7)** as a `.sql` file of zero-row assertions, run after every store
   and identify test, after migrations, and nightly in production. Include `foreign_key_check`,
   FTS `integrity-check` and the cross-file orphan checks (M11).
10. **Operations (M6):**
    - Write down RPO and RTO.
    - A nightly backup through the maintenance connection, with `integrity_check` on the copy.
    - A restore drill as a scenario test.
    - A metrics list exposed on the System page.
    - A runbook page.
11. **Retention (M10) and personal data (M12):** one line per growing table in schema.md, and a
    decision on what deleting a person does, reflected in the foreign-key actions of `play_*`,
    `device`, `grant` and `field_override.set_by_user_id`.
12. **Tests (M9):**
    - Model-based random-operation tests against a full rebuild.
    - A kill-and-resume loop for the chunked grouping job.
    - The person-wins-over-the-pass interleaving test (M1).
    - The pass gates of §4.6.

### Answers to the proposal's owner questions, from this seat

1. **Adopt DL1, DL2, DL4–DL7, DL9, DL10, DL12–DL14 and DL17.** Amend DL3 (deletable parents and
   cascades only), DL8 (aggregates, read-tx, compare-and-set, resumable), DL11 (writer service
   with aging), DL15 (no optimize, multi-shape tests) and DL16 (defer `automerge=0` until
   measured). Add M1–M7 as standard; M8–M13 as design rules for the baseline.
2. **Statistics:** committed `sqlite_stat1`, under the conditions in DL15.
3. **Session last-used:** in memory with a monotonic, batched flush; not `cache.db`.
4. **Gates:** in addition to the proposed ones:
   - the invariant suite (M7);
   - the person-wins interleaving test (M1);
   - a schema test for STRICT or BLOB typing;
   - a check that `cache.db` queries are inside the plan gate;
   - a gate that no code path runs `ANALYZE` or `PRAGMA optimize`.

---

## Sources

- SQLite documentation: [STRICT tables](https://sqlite.org/stricttables.html),
  [ATTACH](https://sqlite.org/lang_attach.html), [atomic commit](https://sqlite.org/atomiccommit.html),
  [WAL](https://sqlite.org/wal.html), [isolation](https://sqlite.org/isolation.html),
  [VACUUM](https://sqlite.org/lang_vacuum.html), [backup API](https://sqlite.org/backup.html),
  [PRAGMA](https://sqlite.org/pragma.html) (`quick_check`, `integrity_check`, `auto_vacuum`,
  `journal_size_limit`, `optimize`), [ANALYZE](https://sqlite.org/lang_analyze.html),
  [foreign keys](https://sqlite.org/foreignkeys.html), [partial indexes](https://sqlite.org/partialindex.html),
  [FTS5](https://sqlite.org/fts5.html), [how to corrupt](https://sqlite.org/howtocorrupt.html),
  [testing](https://sqlite.org/testing.html), [file format: overflow pages](https://sqlite.org/fileformat2.html).
- H. Berenson, P. Bernstein, J. Gray et al., *A Critique of ANSI SQL Isolation Levels*, SIGMOD 1995.
- H. T. Kung, J. T. Robinson, *On Optimistic Methods for Concurrency Control*, ACM TODS 6(2), 1981.
- J. Gray, A. Reuter, *Transaction Processing: Concepts and Techniques*, 1993 (from memory).
- M. Kleppmann, *Designing Data-Intensive Applications*, ch. 7 (transactions), 11–12 (derived data).
- P. Helland, *Life beyond Distributed Transactions*, CIDR 2007.
- E. Evans, *Domain-Driven Design*, 2003; V. Vernon, *Effective Aggregate Design*, 2011.
- C. J. Date, *An Introduction to Database Systems*; J. Celko, *SQL for Smarties* (subtypes);
  B. Karwin, *SQL Antipatterns* (jaywalking, polymorphic associations, EAV). All from memory.
- R. Snodgrass, *Developing Time-Oriented Database Applications in SQL*, 2000.
- M. Harchol-Balter, *Performance Modeling and Design of Computer Systems*, 2013 (M/G/1,
  Pollaczek–Khinchine); J. Dean, L. Barroso, *The Tail at Scale*, CACM 2013.
- L. Campbell, C. Majors, *Database Reliability Engineering*, O'Reilly 2017.
- S. Ambler, P. Sadalage, *Refactoring Databases*, 2006; M. Fowler,
  [Evolutionary Database Design](https://martinfowler.com/articles/evodb.html) and
  [Parallel Change](https://martinfowler.com/bliki/ParallelChange.html) (expand/contract).
- M. Winand, [Use The Index, Luke](https://use-the-index-luke.com).
- Oracle, [SQL Plan Management](https://docs.oracle.com/en/database/oracle/oracle-database/19/tgsql/overview-of-sql-plan-management.html) (plan baselines; from memory).
- [Litestream](https://litestream.io) (continuous WAL shipping, for an RPO of seconds).
