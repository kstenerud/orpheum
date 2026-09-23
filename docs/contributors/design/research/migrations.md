> **ABOUTME:** How the field migrates persisted data — the big commercial players' embedded
> databases, the self-hosted servers and what goes wrong for their users, and SQLite's mechanics
> measured at our scale. The evidence behind the migration policy.

# Migrations: research

- **Date:** 2026-09-23. Requested by the project owner after the data-architecture critique:
  *"we must assume that mistakes will be made and we will have to migrate. So we need a
  migration policy, and I want it well-researched."*
- **Decision:** [D181](../../decisions/working-notes.md). **Plan:** [migrations.md](../plans/migrations.md).
- **Working notes** (quotes, file paths, every fetched page cached): `/yoloai/cache/research/migrations-{bigco,selfhosted,sqlite}.md`, `/yoloai/cache/http/migrations/`, benchmarks in `/yoloai/cache/bench/migrate/`. Not in the repo.

## 1. The big commercial players

Four shipped embedded databases, read from source and primary docs.

| | Version bookkeeping | Newer DB opened by an older build | Oldest upgradable-from | Old-version fixtures | Pre-migration backup | Failure |
| --- | --- | --- | --- | --- | --- | --- |
| **Chromium** | `meta` table: `version` **and `last_compatible_version`** | opens it if `compatible ≤ mine`, else `INIT_TOO_NEW` | **per database, by value of the data**: history v15 (effectively forever); Web Data v82, tied to Sync's support window; top sites "deprecated after two years… this database is a cache" | a `.sql` dump committed **for every version**; CI fails if the current version has none | none | whole chain in one transaction; rollback; feature disabled |
| **Firefox Places** | `PRAGMA user_version` | profile layer refuses: *"You've launched an older version"*, override `--allow-downgrade`; DB layer rewinds the version, so **every migration must be re-runnable** | schema 52 = Firefox 68 **ESR**; older is treated as corrupt, kept as `.corrupt`, replaced | binary fixtures plus `test_current_from_downgraded.js` | only on corruption | transient (busy, locked, disk full, OOM) retried; anything else → corrupt path |
| **Android Room** | `user_version` + schema identity hash; **JSON schema history committed to git** | throws unless a down-migration exists — which an old binary cannot contain | whatever chain is kept | `MigrationTestHelper` builds old DBs from the JSON history, then **validates the migrated schema against the current one** | none | one `BEGIN EXCLUSIVE` transaction; rollback; exception |
| **Apple Core Data / SwiftData** | structural hashes | refuses (`IncompatibleVersionHashError`) | every model version stays in the bundle: *"My users can upgrade from any version"* | not prescribed | heavyweight only: old store renamed with `~` | error |

What they agree on:

- **Nobody runs down-migrations in the field.** Room allows them, but an old binary cannot
  contain a migration from a future version. Downgrade safety comes from designing the *upgrade*:
  Chromium's **compatible version** — *"the lowest current version embedded in Chrome code that
  can still use this database… adding a column without a NOT NULL constraint"* keeps it low;
  history is at v70 and still readable by v16.
- **The oldest supported upgrade-from version is a named constant, anchored to something outside
  the code, and set by how replaceable the data is.** Caches get short windows and are razed;
  irreplaceable user data keeps migration code practically forever.
- **Fixtures for every shipped version**, with a CI guard (Chromium's
  `VerifyTestSQLFileForCurrentVersionAlreadyExists`) and schema validation after migrating (Room).
- **Migration code is frozen to its era.** Room: *"use full queries instead of referencing
  constants"*. Apple: use generic objects because the class *"may not exist as expected during the
  migration"*. For us: migrations never call generated query code.
- **All-or-nothing per transaction**, never a half-migrated file.
- **Destructive fallback is for derivable data only.** Room deprecated the no-argument
  `fallbackToDestructiveMigration()` in 2.7 for silently turning a missing migration into total
  data loss.

Server-side practice (Stripe's online migrations, gh-ost) exists to avoid locking a live fleet;
we own the only process, so it does not apply. **Expand/contract across releases does**, reread:
not for zero downtime but for **downgrade** — add now, remove a release later, raise the compatible
floor only then.

## 2. The self-hosted field, and what goes wrong

| Project | Bookkeeping | Upgrade-from | Pre-migration backup | Newer DB | Migration tests |
| --- | --- | --- | --- | --- | --- |
| Jellyfin 10.11+ | EF Core history, schema and data "routines" in one ledger | 10.10.7 (a forced stepping stone) | automatic — **deleted on success** | no guard | only "model has no pending changes" |
| Plex | `schema_migrations(version, rollback_sql, …)` | very old | every 3 days, not tied to upgrade | **rolls back** with SQL the newer version stored | closed |
| Home Assistant | integer version, all 53 migrators kept | schema 0 | opt-in, off by default | **refuses** with a message | **a frozen model per version** (`db_schema_N.py`), parametrised from-version tests |
| Navidrome (Go, goose) | goose table, 35 SQL + ~95 Go | 2020 | none | runs anyway | a few |
| Sonarr / Radarr | FluentMigrator, 233 / 140 | 001 | only via the built-in updater — not Docker | runs anyway → *"Error parsing column 45"* | ~40 of 233 / 24 of 140 |
| Gitea | one-row integer | `minDBVersion = 70` (2018) | none | **refuses**: *"Gitea will exit to keep your database safe and unchanged"* | 56 fixture tests |

The failure taxonomy, with evidence:

1. **The ledger breaks, not the schema.** Jellyfin 10.11's largest incident (#15027, 121
   comments, 38 reactions, reproduced on a fresh install) was translating its old `migrations.xml`
   into EF's history table: a one-shot migration re-ran and died on *"no such table:
   ActivityLog"*. #15388 is the same class and still open. Plex had bogus rows in its ledger; a
   system SQLite default change broke Sonarr's ledger query (#5464), reported as *"Database file
   … is corrupt"*.
2. **The supervisor kills the process mid-migration.** Jellyfin #12035: a Docker healthcheck
   killed it *"30+ times"*, leaving a corrupt database; #15189: each restart made a new backup
   *"eventually filling up all the disk space"*. The maintainer: *"if you or your container
   manager is not patient enough… it will kill your db file."* Mitigations seen: commit per step
   and resume (Home Assistant, Gitea); a health endpoint reporting *degraded* rather than failing;
   a migrate-only mode (Jellyfin 12 `--mode MigrateSystem`).
3. **Latent bad data surfaces.** Unknown type rows (Jellyfin #15060), a malformed JSON row
   (Plex), non-UTF-8 paths (Navidrome #3787).
4. **Migrations that touch the environment fail.** Walking the filesystem (Navidrome #3763),
   moving files out of disk space (Jellyfin #14602), a `/tmp` preflight (#15036). Navidrome's
   alternative — *invalidate and rescan* — appears in 23 of ~95 of its Go migrations.
5. **Migration errors are reported as corruption.** Home Assistant #92650 moved a healthy database
   aside and started empty — *"The complete history is gone"* — after a race; `integrity_check`
   on the moved file said `ok`. Sonarr maps every SQLite exception to "corrupt".
6. **No progress on long migrations.** Home Assistant on MariaDB: 11 hours, 28% (#123179).
   Jellyfin: *"we cannot currently provide progress to that step as its just a simple sql query."*
7. **A patch release carrying a data migration blocked security fixes** (Jellyfin 10.11.7,
   #16531: *"will there be a v10.11.8 soon… so I can upgrade for the security fixes?"*, +15).
8. **Release notes do not reach users.** Jellyfin's maintainer: *"Every single announcement we
   did had big, bold text saying 'read the release notes' and 'make a backup', and yet here we
   are… Most, however, seem to just use `latest`."*
9. **Downgrade is "not supported, restore a backup" nearly everywhere**, and users downgrade
   anyway — when a migration fails at startup, and when a release regresses. Jellyfin's
   automatic backup could not serve them, because it was deleted once the migration succeeded.

## 3. SQLite, measured at our scale

modernc.org/sqlite v1.59.0 bundles **SQLite 3.53.4**. A reference-scale database — 3,500 titles,
10,000 files, 30,000 tracks, **279,453 spans**, 87,000 tag rows, views, partial unique indexes,
an FTS5 external-content table — is **45.9 MB**. Laptop CPU, SSD, median of five:

| Operation | Time |
| --- | --- |
| ADD COLUMN (any table) | **8.5 ms** |
| 3.53 `ALTER COLUMN … SET NOT NULL` (scans) | 15 ms |
| DROP COLUMN, spans | 136 ms |
| **12-step rebuild of spans**, 3 indexes, 2 views, FK check | **1.006 s** |
| 12-step rebuild of `file`, children checked | 0.532 s |
| CREATE INDEX on spans | 221–275 ms |
| **`VACUUM INTO` backup** | **177 ms**, 44.5 MB |
| FTS5 rebuild, 14,000 documents | 77 ms |
| `integrity_check` / `quick_check` | 1.08 s / 156 ms |

**At this scale the mechanics are free and correctness is the whole risk.** The largest term in a
rebuild is `foreign_key_check` (382 ms), not copying rows (173 ms).

Verified mechanics and pitfalls, each reproduced:

- **DDL is fully transactional**: ADD COLUMN, a rebuild, CREATE INDEX, an FTS5 rebuild and a
  `user_version` write in one transaction roll back completely. Outside a transaction only:
  `VACUUM` / `VACUUM INTO` (error), and `PRAGMA foreign_keys` / `journal_mode` — which inside a
  transaction **return OK and do nothing**.
- **A rebuild inside a tool's implicit transaction fails**: `foreign_keys=OFF` is ignored and
  dropping a parent hits `ON DELETE RESTRICT`. golang-migrate's default mode fails exactly so,
  and leaves the database marked dirty. `defer_foreign_keys` does not rescue it.
- **Renaming the rebuilt table fails while a view references it**; views are dropped and
  recreated around a rebuild.
- **A naive rebuild loses the `AUTOINCREMENT` high-water mark** and reissues ids — which
  [D173](../../decisions/working-notes.md) forbids.
- **Rebuilding an FTS5 external-content table without copying `rowid`** silently corrupts search
  (2,610 wrong hits for one term).
- **`CREATE TABLE … AS SELECT` drops the primary key**, and child foreign keys then mismatch.
- **`PRAGMA foreign_key_check` in a SQL file never fails the migration**; it only returns rows.
  The runner has to check.

Tooling:

| | Go migrations | per-migration transaction control | notes |
| --- | --- | --- | --- |
| **goose** v3.28 | yes | per file, `NO TRANSACTION` | fits best of the three; its version row for a no-transaction file is written *after*, outside it |
| **golang-migrate** v4.20 | no | one global setting | fails the rebuild case as above |
| **Atlas** (free) | — | — | **cannot handle views, triggers or FTS5**: its diff turned our FTS5 table into plain tables and its generated rebuild failed |
| **sqlc** v1.31 | — | — | reads SQL migration files correctly, ignores down sections; **cannot see Go migrations**; does not yet parse 3.53's `ALTER COLUMN` |

## 4. What this means for us

Three things make our position unusually good, and the policy should lean on them:

1. **The database is mostly a rebuildable index** ([D7](../../decisions/working-notes.md),
   [D179](../../decisions/working-notes.md)). A migration of derived data can be *drop and
   re-derive* — Navidrome's lesson, generalised — leaving careful migration only for the
   irreplaceable set and the identity rows.
2. **Everything is fast.** One second is the worst operation measured, so a snapshot before every
   migration is free and no migration needs to run online.
3. **One process, one bundled SQLite.** No lock-free schema change, no system-library drift.

And three lessons are not optional: **the ledger is the riskiest code**, **something will kill us
mid-migration**, and **the backup must outlive a successful migration**, because that is when
people want to go back.
