> **ABOUTME:** The migration policy — how every persisted format this project writes is versioned,
> upgraded, tested and, when something goes wrong, recovered. The database first, then files,
> then formats read by another version on another machine.

# Migrations

- **Status:** PLANNED — policy decided 2026-09-23 ([D181](../../decisions/working-notes.md)) on
  [research/migrations.md](../research/migrations.md).
- **Depends on:** [library-model.md](library-model.md), [backup-restore.md](backup-restore.md)
- **Deadline:** before the first release that stores anyone's data (Cut A).

## The premise

Some decision in the first schema will be wrong, and a real user's database will have to be
reshaped. The question is not whether we migrate but whether a migration can lose anything. The
answer this plan gives: **it can fail, it can be interrupted, it can be regretted — and none of
those loses data.**

## What is persisted, by class

| Class | Examples | Policy |
| --- | --- | --- |
| **Irreplaceable, in the database** | watch state, spans, preferences, tags, roles, grants, identity rows ([D179](../../decisions/working-notes.md)) | **migrated**, carefully — everything below |
| **Derived from stored input** | the library index, sort keys, FTS, filings — anything computed from raw probe output, verbatim filenames, cached provider responses or NFO files | **dropped and re-derived** from that input ([D59](../../decisions/working-notes.md)), never transformed in place |
| **Fetched, with no stored response** | provider values whose raw response is gone | **migrated in place; never refetched** by a migration — a refetch drifts, floods, and is the user's act ([D170](../../decisions/working-notes.md), [D182](../../decisions/working-notes.md)) |
| **Authored files, read before the database opens** | startup config, a node's roots ([D107](../../decisions/working-notes.md)) | versioned; old versions read forever; **never rewritten by us without being asked** |
| **Our files, read later** | trash manifests ([D145](../../decisions/working-notes.md)), backups, sidecars | versioned; every past version readable forever |
| **Formats read by another version** | node ↔ core exchange, offline span uploads, the API ([D85](../../decisions/working-notes.md)) | versioned per message; tolerant readers; a stated skew window |
| **Disposable** | `cache/`, consumed scan reports, logs | **versioned by discard**: a format change deletes and regenerates |

**Re-deriving has two costs, and the step says which.** Re-interpreting stored input — the verbatim
filename and raw probe output the index keeps ([D59](../../decisions/working-notes.md)) — needs no
node and takes seconds. Re-probing media wakes nodes and disks and is priced like a scan pass
([D94](../../decisions/working-notes.md)). A step prefers the first, and **identity rows are never
derived data**: they are kept and matched against ([D179](../../decisions/working-notes.md)).

Anything changeable at runtime lives in the database — library settings, provider keys, plugin
state (one namespace per plugin) — so it is backed up and migrated with everything else. Files hold
only what must exist before the database opens.

## The database

> **Until migrations are live, a baseline and one step ([D228](../../decisions/working-notes.md#d228--until-migrations-are-live-librarydbs-schema-is-a-baseline-and-one-step-and-a-schema-change-folds-the-last-step-into-the-baseline)).** Nothing is released, so no
> database but the owner's development one has ever been migrated. The steps are a baseline,
> `NNNN_baseline.sql`, which builds a new database at its version in one step, and the one step
> after it. A schema change folds the last step into the baseline and adds the change as the
> next step, so a development database is one step behind and really migrates through it. The
> baseline names the checksum of the step it folded last, which a database that reached its
> version through that step recorded; a database below its version is refused. Everything below
> applies to every step; *every step is kept, forever* and *a golden for every version* wait
> until the owner declares migrations live, by committing `internal/store/migrations/LIVE`.

> **IMPLEMENTED — see `internal/store`** (M1): the two integers, the runner and its checks, the
> pre-migration snapshot and free-space preflight, the newer-database refusal, `orpheum migrate`,
> `/health` while migrating, and tests for the golden, interruption, failure and refusal. The
> reference-scale database and its 10 s step budget arrived after M2 (`TestReferenceScale`, in
> `internal/store`'s integration tier). **Not yet:** golden fixtures with rows, which matter from
> the second schema version on.

### Bookkeeping: two integers, and nothing clever

- **`PRAGMA user_version` is the schema version**, set in the **same transaction** as the step's
  DDL, so the version and the schema cannot disagree.
- **A one-row `schema_meta` table holds `compatible_version`**: the oldest release's schema that
  can still use this file (Chromium's `last_compatible_version`). An additive step leaves it
  alone; a step that drops, renames or reshapes raises it.
- **`PRAGMA application_id`** is set once, and a file without it is refused rather than migrated.
- **The ledger never changes format.** Jellyfin's worst incident was translating one ledger into
  another. Two integers have no translation.

### The runner is ours, and small

About 150 lines, because the tools each fail one of our requirements (research §3): goose writes
the version outside a no-transaction step; golang-migrate cannot run our rebuilds; Atlas's free
edition mangles views, triggers and FTS5. For each pending step, in order:

1. `PRAGMA foreign_keys=OFF` on a **pinned connection**, outside any transaction, and assert it
   took.
2. `BEGIN IMMEDIATE`.
3. Record `sqlite_sequence`.
4. Run the step's SQL file.
5. **Checks the runner owns**, because a SQL file cannot fail itself: `foreign_key_check` returns
   no rows; no `sqlite_sequence` value went down; every view still compiles.
6. Set `user_version` (and `compatible_version` if the step declares it). `COMMIT`.
7. `PRAGMA foreign_keys=ON`, and assert it took.

**Each step commits on its own**, so an interruption resumes at the last completed step (Home
Assistant, Gitea). Every intermediate version is a real one with a golden fixture, so a partly
upgraded file is a known state, not a half-state.

### A step is a SQL file, frozen to its era

- **Plain SQL, read by sqlc**, so the generated queries always match the migrated schema. A step
  never calls generated query code or current Go constants (Room, Apple).
- **Rebuilds follow one documented pattern** — create new, copy **with explicit `rowid`** where FTS
  content depends on it, drop old, rename new, recreate indexes, triggers and views — and views,
  and every other table's trigger, are dropped before the rename: a rename checks every trigger,
  and step 0058's triggers on `version`, `title` and `collection_member` read `file` through the
  views `title_found` and `collection_found`. `CREATE TABLE … AS SELECT` is not used.
- **The runner records each applied step's checksum** (`schema_step`), in the step's own
  transaction, and refuses a database where an applied step's text has since changed. A committed step never
  changes; a draft applied by the dev loop and then edited before it was committed does, and
  leaves a schema that is not the step's (plan-deviations P17).
- **No filesystem, no network, no path parsing.** A step that would need them is instead a step
  that invalidates derived data and schedules a rescan or re-interpretation.
- **Go code only for a data transform SQL cannot express**, and such a step changes no schema, so
  sqlc stays right.
- **SQLite 3.53's `ALTER COLUMN` forms are not used until sqlc parses them**; a rebuild does the
  same job in a second.

### Before migrating: a snapshot that outlives success

- **`VACUUM INTO` before the first pending step** (177 ms at the reference scale), named with the
  from-version: `pre-migrate-v41.sqlite`. **One per from-version, not one per attempt** — a
  restart loop must not fill the disk (Jellyfin #15189).
- **It is not deleted when the migration succeeds.** That is when people want to go back. Pruned
  by count with the other backups.
- **A free-space preflight**: the snapshot plus a rebuild's worst case (twice the largest table).
  Refused with the numbers if short — a refusal, not a partial run.

### While migrating: never look dead

- **The health endpoint answers `migrating: step 3 of 7`** with a success status, so no container
  manager kills us for being unhealthy (Jellyfin #12035).
- **`migrate` is also a command**, so an operator can run it without starting the server
  (Jellyfin 12, Gitea).
- **Progress is per step**, which at our scale is about a second each. A step is budgeted: CI fails
  one that takes over **10 s** on the reference-scale database.

### When a step fails

- **The step rolls back and the server does not start normally.** It serves a status page naming
  the step, the error, and the snapshot.
- **Transient errors are retried, not diagnosed**: busy, locked, disk full, out of memory
  (Firefox).
- **A migration error is never called corruption.** Corruption is claimed only when
  `integrity_check` fails, and even then **the file is never moved aside or replaced
  automatically** — Home Assistant did that to a healthy database and the history was *"gone"*.
- **No destructive fallback for the irreplaceable set.** Dropping and re-deriving is the ordinary
  path for derived tables, never an emergency one.

### A newer database

- **If `compatible_version` ≤ this binary's version, it runs.** The newer release declared that it
  left the file usable by us — which is what expand/contract buys.
- **Otherwise it refuses and names the way back**: *"This database was upgraded by version 1.6 and
  cannot be used by 1.5. The snapshot `pre-migrate-v41.sqlite`, taken before that upgrade, can be
  restored — you will lose what happened since."* Never rewind the version, never run anyway.
- **No stored rollback SQL** (Plex's approach): the snapshot is the rollback, and it is tested.

### Expand, then contract, across releases

- **Additive changes do not raise `compatible_version`**: a nullable column, a new table, a new
  index. An older release keeps working.
- **Removing or reshaping is done a release later than adding**, where it can be: add the new
  column and write both; drop the old one in the next release, which raises the floor.
- **A release that raises `compatible_version` is a one-way door and its version number says so**
  (a major bump), because release notes do not reach people on `latest`.
- **Patch releases carry no schema steps.** A security fix must never be held behind a migration
  (Jellyfin 10.11.7).

### Upgrading from anything

**Every step is kept, forever** — once migrations are live ([D228](../../decisions/working-notes.md#d228--until-migrations-are-live-librarydbs-schema-is-a-baseline-and-one-step-and-a-schema-change-folds-the-last-step-into-the-baseline)). Each is small, pure SQL and fixture-tested, and our users skip
releases. If the chain is ever long enough to matter, a fresh install may start from a schema
snapshot (Grafana) — upgrades still walk the chain. A minimum upgrade-from version is introduced
only for a measured reason, and then it is a named constant with its reason beside it.

### Testing

- **A golden fixture for every released schema version**, committed as `.dump` text with a small
  set of realistic rows (Chromium's format, Home Assistant's data). **CI fails if the current
  version has none.**
- **Every golden migrates to current, and the result's schema equals a fresh install's** —
  tables, columns, indexes, views, triggers, `sqlite_sequence` (Room's validation).
- **Every data-transforming step has assertions on the rows**, not only on the schema.
- **Interruption:** kill the runner after each step in turn; the next run resumes and ends
  identical.
- **Newer database:** raise `compatible_version` past the binary and assert the refusal and its
  message.
- **Restore:** migrate, restore the pre-migration snapshot, start the old binary.
- **Scale:** a reference-scale database (≈46 MB) in CI, and the 10 s step budget.

## Files

- **A new version needing more than an old file says** ([D182](../../decisions/working-notes.md)):
  a **default**, only if it is safe to be wrong and shown as *default* ([principles §15](../../principles/15-safe-visible-defaults.md)); else a **derivation** from the old keys, in memory, with a warning; else it is
  **not a file key** — the server starts and asks in the admin UI; else, only for a value needed
  before the database opens, a **refusal naming the key** and a `config upgrade` command that
  writes the new file when run, keeping the old one. Anyone adding a key says which of the four.
- **Every authored file carries a `version`**, and every past version is readable. Deprecated
  keys are accepted with a warning naming the replacement.
- **We never rewrite a file a person wrote** unless they changed a setting through our UI, and then
  the previous copy is kept beside it.
- **Unknown keys are preserved, never dropped** — the rule
  [D141](../../decisions/working-notes.md) states for NFO, applied to our own files.
- **Backups:** a backup from an older version is restored and then migrated by the ordinary
  runner; one from a newer version is refused by the same `compatible_version` rule. This
  replaces backup-restore.md's *"restore refuses a gap"*.

## Formats read by another version

- **Core ↔ node** ([D59](../../decisions/working-notes.md)): every exchange file and message carries a
  format version. **Skew window: the core accepts a node one minor release behind or ahead; outside
  it, each side refuses with a named reason** ([F37](../../../FEATURES.md)'s honest refusal) rather
  than misreading. Readers ignore unknown fields.
- **Offline uploads**: a device can be offline indefinitely
  ([D169](../../decisions/working-notes.md)), so **every past span-upload format is accepted
  forever**; they are small and few.
- **The API** is [api-compatibility.md](api-compatibility.md)'s contract.

## Done when

The runner and its checks exist; a golden per released version migrates to a schema identical to
a fresh install's; an interrupted migration resumes; a newer database is refused with the snapshot
named; a pre-migration snapshot survives success and restores into a working older release; and a
real-scale database migrates within budget.
