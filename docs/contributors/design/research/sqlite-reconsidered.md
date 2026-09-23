> **ABOUTME:** A re-examination of the Postgres decision against the actual workload — which
> corrects a misreading of the original evidence, finds most of the claimed Postgres advantages
> evaporate, and concludes SQLite with specific discipline.

# SQLite Reconsidered

Researched **2026-09-18**. [D5](../../decisions/working-notes-archive.md)'s own status line said it was
"the one most worth re-examining", so this is following that note rather than reversing on a
whim.

## 1. I misread the original evidence

D5's central argument was that SQLite's single-writer lock collides with scan-while-streaming,
citing three failures. Re-read, they do not say what I claimed:

- **Audiobookshelf #5349** — a library scan hit `SQLITE_BUSY` while another write held the
  lock, "and that rejection was **unhandled** — which took down the entire server." That is an
  **application bug**, not a SQLite limit. `SQLITE_BUSY` is the documented, expected signal to
  wait and retry; crashing on it is the same class of error as not checking a return code.
- **Jellyfin #11589 and the ZFS/LXC lock group** — real user pain, but **unattributed**. Both
  arrived alongside Jellyfin's EF Core migration, whose other symptom was transactions and
  result-set enumeration being held open far too long. Whether the lock contention was SQLite's
  semantics or Jellyfin's usage of them is not established, and I presented it as though it
  were.
- **"SQLite core dump on systems without SSE4.1"** — a **native-library** failure, not a
  database-semantics one. It is an argument about which *driver* you link, and it disappears
  entirely with a pure-Go driver (§4).

The honest summary: one of the three was an app bug, one is unattributed, and one is a driver
issue. That is a much weaker case than D5 made.

## 2. The concurrency argument does not survive the actual numbers

The relevant metric is not simultaneous *users*; it is simultaneous *writers* and the write
rate. For a household server:

- ~20 concurrent streams × one progress update every ~10 s ≈ **2 writes per second**.
- Scanning is bursty but batchable into transactions.
- SQLite in WAL mode handles **thousands** of batched writes per second on modest hardware.

That is roughly three orders of magnitude of headroom. The write pattern that worried D5 is not
a load problem; it is a *contention* problem, and contention is a design question.

**Navidrome — Go, SQLite, a media library, a scanner, and a Jellyfin-compatible API — has
already solved it in production.** From its source:

- DSN: `_busy_timeout=15000&_journal_mode=WAL&_foreign_keys=on&synchronous=normal`
- A shared pool sized `max(4, runtime.NumCPU())`, with this comment, which is the whole design
  brief in three sentences:

  > "It bounds concurrent *readers*: SQLite serializes writers on a single database-wide write
  > lock, so more connections buy no write parallelism. A connection is held while blocked on
  > disk I/O or on a slow HTTP client, neither of which is CPU-bound… the floor is what matters
  > on small machines."

- And the sharp edge they learned the hard way, worth stealing verbatim: **`SQLITE_BUSY` and
  `SQLITE_BUSY_SNAPSHOT` share the message "database is locked", but `busy_timeout` can retry
  the first and can *never* retry the second.** A deferred read transaction upgraded to a write
  after someone else wrote has a stale snapshot; the only recovery is to roll back and retry the
  whole transaction. Navidrome inspects the extended result code specifically to tell them
  apart.

If the only Go+SQLite media server in the survey has production patterns for this, it is an
engineering discipline, not a wall.

## 3. Most of the claimed Postgres advantages evaporate

D5 also argued Postgres brings capability we would otherwise add separately. Checked:

| Claimed | Reality |
| --- | --- |
| Full-text search | **SQLite FTS5 is excellent.** No advantage. |
| JSONB | **SQLite has JSONB** (3.45+). No advantage. |
| `LISTEN`/`NOTIFY` for job wakeups | **Only needed because Postgres is a separate process.** In one binary this is a Go channel. This argues *for* SQLite. |
| `pgvector` | `sqlite-vec` exists, and vectors are a photos feature we are not building ([D1](../../decisions/working-notes.md)). No advantage. |
| `pg_trgm` fuzzy matching | **The one real advantage.** SQLite has no built-in trigram similarity. But filename→title matching belongs in the identification component in Go anyway ([scanner.md](../plans/scanner.md)), where the library ecosystem is good. Minor loss. |

Four of five do not survive. That was the weakest part of D5 and I should have checked it then.

## 4. The driver choice matters as much as the engine, and one option breaks a promise

Not considered in D5 at all, and it interacts directly with
[D9](../../decisions/working-notes.md)'s "single static binary, trivial cross-compilation to
amd64/arm64":

| Driver | Nature | Consequence |
| --- | --- | --- |
| `mattn/go-sqlite3` | **cgo** | Fastest and most used (9.2k stars) — but **cgo breaks trivial cross-compilation**, which is one of D9's stated reasons for choosing Go, and it reintroduces the native-crash class that bit Jellyfin. |
| `modernc.org/sqlite` | **Pure Go** (transpiled C) | v1.59.0, actively released through September 2026. Preserves cross-compilation and static linking. Slower than cgo. |
| `ncruces/go-sqlite3` | **Pure Go** (Wasm runtime) | Actively maintained, notably few open issues. Different performance profile again. |

**A pure-Go driver is close to required**, because choosing cgo would silently give up the
cross-compilation property that helped justify Go in the first place. Pick between `modernc` and
`ncruces` with a benchmark on the low-end target, not from reputation.

## 5. What SQLite buys that D5 under-weighted

- **One binary, no second service.** For self-hosted software this is not a convenience, it is
  a large part of the product. Navidrome's reputation — *"could run on my raspberry pi with my
  library"* — is built on it.
- **Backup becomes copying a file.** This closes a gap flagged in our own tree:
  [backup-restore.md](../plans/backup-restore.md) records "Postgres is not a file the user can
  copy… that is the cost of that decision and it has not been paid yet." With SQLite the cost
  simply does not arise.
- **No major-version upgrade dance.** Postgres major upgrades are a recurring source of pain for
  self-hosters, and "upgrades must be boring" is one of the strongest findings in
  [sentiment.md](sentiment.md).
- **RAM on the low end.** Our own honest low-end target is an 8 GB Celeron NAS
  ([test-hardware.md](test-hardware.md)); Postgres there costs memory that the transcoder and
  page cache want.

## 6. The multi-process objection, and why it dissolves

The one genuinely new argument for a client/server database since D5 was written:
[D19](../../decisions/working-notes.md) and [D26](../../decisions/working-notes.md) put the
transcoder and the media worker in **separate processes, possibly on separate machines**. Two
processes writing one SQLite file over a network share would be a serious mistake.

It dissolves because those workers **should not touch the database at all**. They receive a job
and return results over the core's API; the core remains the sole writer. That is also exactly
what D26's threat model wants — a confined process parsing hostile media should not hold
database credentials. **The architecture that makes the security story good also makes SQLite
viable**, which is a pleasing alignment rather than a coincidence.

## 7. The risks that remain, and what to do about them

1. **Never on a network filesystem.** SQLite's locking over NFS/SMB is genuinely unsafe. The
   database lives in the config directory, not the media directory, so this only bites someone
   who puts their config on a share — which must be **detected at startup and refused with a
   clear message**, not left to corrupt quietly.
2. **`SQLITE_BUSY_SNAPSHOT` needs retry-the-transaction logic**, distinct from `busy_timeout`.
   Build it into the repository layer once, per Navidrome's pattern.
3. **A single-writer discipline.** Serialise writes through one path so the application never
   contends with itself, and batch scanner writes into transactions.
4. **Adding Postgres later is not free** — `sqlc` generates dialect-specific code, so a port
   means a second set of queries. Bounded if every query lives in the repository layer, which
   it does regardless.

## 8. Conclusion

**SQLite, with a pure-Go driver.** The concurrency argument does not survive the numbers, the
capability arguments mostly evaporate, the evidence I cited was partly misread, and the
deployment benefits are large for exactly this kind of product.

Postgres remains the right answer for a genuinely multi-tenant or multi-node deployment. That is
not this product, and if it ever becomes so, it becomes so with a repository layer already in
place and a real requirement to design against — which is a far better position than paying the
operational cost now against a workload that does not exist.
