> **ABOUTME:** How the server should hold its data — database engine, access layer, schema
> stance, search, migrations, on-disk layout and backup — grounded in the specific failures the
> incumbents suffered by choosing badly.

# Technology: Storage

This is where Jellyfin lost a year and PhotoPrism lost eight. The choice is close to
irreversible, so it gets made deliberately and early.

## 1. Engine: SQLite or Postgres

### The evidence

**SQLite failures in this exact workload.** SQLite in WAL mode gives concurrent readers but
**serialises all writes through a single writer lock**. A media server's worst moment is a
library scan writing thousands of rows while transcodes, playback-progress updates and image
generation also write. That is not hypothetical:

- Audiobookshelf #5349: an unhandled `SQLITE_BUSY` during a library scan running alongside
  FFmpeg metadata-embedding jobs **took the whole server down**.
- Jellyfin #11589: `Library Scan Failing Repeatedly after 10.9 ('database is locked')`.
- Jellyfin `[Group] "Database Locked" on LXC containers using ZFS as storage` — +17 reactions
  and **228 comments**. Note the deployment detail: SQLite's locking behaviour is sensitive to
  the filesystem underneath it, and self-hosters run ZFS, btrfs, NFS and network shares.
- Jellyfin `SQLite core dump on systems without SSE4.1` (+24) — a native-dependency failure on
  older hardware, which self-hosters have a lot of.

**Postgres friction.** It is a second service to install, run, back up and upgrade. For a
self-hosted product that is a real cost: Immich requires it (with custom extensions, in a
custom image) and that is the single most common thing people complain about when *installing*
Immich. Audiobookshelf's users want it (+66); PhotoPrism's want it (+189, **eight years
unfulfilled**).

**The asymmetry that decides it:** adding Postgres later is a schema-portability project
someone may never finish (PhotoPrism is the proof). Starting on Postgres and later adding an
embedded option is additive. And the second-most-requested feature in two of the three
surveyed audio/photo projects is "please support Postgres" — nobody is asking for the reverse.

### Recommendation — **SUPERSEDED 2026-09-18**

> **This section is wrong and is kept for the record.** Re-examined in
> [sqlite-reconsidered.md](sqlite-reconsidered.md) and reversed by
> [D31](../../decisions/working-notes.md): the evidence below was partly misread (Audiobookshelf
> #5349 was an unhandled-error bug, the Jellyfin lock complaints are unattributed, and the
> SSE4.1 crash was a native-driver issue), the write rate is ~2/sec rather than contended, and
> four of the five claimed Postgres capabilities exist in SQLite or are moot. **The decision is
> SQLite with a pure-Go driver.**

**Postgres as the primary and only supported engine for v1.** Ship it in the default Docker
Compose so the install is still one command. Keep all SQL behind a repository layer so an
embedded engine can be added later if single-binary deployment proves to matter more than
expected.

Concretely, Postgres also buys things we will want: real full-text search with proper
dictionaries (`tsvector`), `pg_trgm` for fuzzy title matching during scanning, JSONB for
provider payloads we don't want to model yet, `LISTEN`/`NOTIFY` for job wakeups without a
message broker, and `pgvector` if content-based search ever happens. Each of those is a
dependency we'd otherwise add separately.

**Explicit dissent, recorded:** Navidrome, Stash and Kavita all run SQLite happily at real
library sizes, and single-binary-no-dependencies is genuinely valuable to self-hosters. If the
transcode-heavy write pattern turns out not to materialise, SQLite would be the lighter
choice. The tiebreaker is the *scan-while-streaming* write pattern and the ZFS/NFS deployment
reality, not raw throughput.

## 2. Access layer: no ORM

The Jellyfin 10.11 post-mortem is unambiguous: adopting a full ORM (EF Core) late, over a
schema that had never been normalised, produced migrations that hung for hours and queries
that enumerated whole tables into memory because filters weren't pushed into SQL. PhotoPrism
runs the *deprecated v1* of GORM. These are the two ORM users in the survey and both are
cautionary.

The two projects with the cleanest data layers use **typed query builders or generated
type-safe SQL**:

- Immich: `kysely` — compile-time-checked SQL, no runtime magic, schema declared in-tree.
- Kyoo's auth service: `sqlc` — write SQL, get generated typed Go.
- Navidrome: `Masterminds/squirrel` — a query builder, not an ORM.

**Recommendation: `sqlc`.** You write SQL; it generates typed Go structs and methods, verified
against the real schema at generation time. It is the option that best matches the brief's
"strong typing + mechanical feedback" criterion, because a wrong column name is a build
failure, not a runtime one. `squirrel` or `bun` are acceptable where queries must be dynamic
(search filters, smart playlists); `sqlc` plus a small hand-written dynamic-filter builder is
the pragmatic combination. **No GORM, no Ent, no ORM.**

## 3. Schema stance

Five rules, each earned from a specific incumbent failure:

1. **The files are the source of truth; the database is a derived index.** Navidrome's model.
   Metadata the user edits and playback state are the exceptions — those are *ours*, and they
   get written to sidecar files as well, so a user can rebuild from scratch and lose nothing.
   This removes the entire "migration ate my library" fear class.
2. **Normalise from the start.** Jellyfin 12.0's headline change is turning playlist and
   collection membership into real rows with real foreign keys — in 2026, after eight years.
   Model relations as relations on day one.
3. **Change journal in the schema from commit one.** Immich pairs every syncable table with a
   `*_audit` table (UUIDv7 primary key, indexed timestamp) recording deletions, and serves a
   cursor-based delta-sync endpoint over it. An `updatedAt` column cannot express a delete, so
   offline clients need this, and retrofitting it is a migration. We will have offline clients.
4. **Stable external IDs, and provider IDs kept separately.** An item's identity must survive a
   file move, a rename, a re-rip and a metadata-provider change. Use a UUID as the primary key
   and hold `{provider, provider_id}` pairs in their own table.
5. **Multi-version items are a first-class model, not a hack.** 1080p and 4K of the same film,
   multiple cuts, multi-part files, disc images. Jellyfin and Plex both bolt this on and both
   have long-running bugs about it (`Multi disc music albums are displayed as separate albums`,
   71 comments; `Multi-Part Audiobook being treated as folder`, +19). Model `Title → Edition →
   MediaFile` explicitly.

## 4. Migrations

The discipline, taken from the yoloAI template's own research on crash-safe migration and from
watching Jellyfin fail at it:

- **Versioned, ordered, forward-only, in-tree SQL.** `golang-migrate` (Kyoo's choice) or
  `goose`. CI verifies ordering — Immich has a `migrations:verify-order` script precisely
  because out-of-order migrations merged from parallel branches are a real failure mode.
- **Back up before migrating, automatically, and say where the backup is.** Immich ships a
  `database-backup.service`. The Jellyfin 12.0 HN thread contains a user who lost their config
  and said "it's on me for not checking or taking a backup first" — it was not on them.
- **Detect-and-refuse, never auto-convert across a gap.** If the on-disk version is too old to
  migrate directly, refuse with a message naming the exact intermediate version required.
- **Every migration must be tested against a realistically-sized database.** Jellyfin's
  hours-long hangs were not visible on a small test library.

## 5. Search

Library search is the most-used feature after playback and is consistently under-built —
`Search results are limited to 24.` has been open on `jellyfin-web` since 2020.

- **Postgres FTS (`tsvector`) for text**, with `pg_trgm` for fuzzy matching. No Elasticsearch,
  no Meilisearch, no second service.
- **`pg_trgm` also serves the scanner**: matching `The.Movie.2019.1080p.BluRay.x264-GROUP` to a
  provider title is fuzzy string matching, and having it in the database avoids a separate
  library.
- Semantic/vector search is a `pgvector` column added later, if ever. It is a photos feature,
  not a video one.

## 6. On-disk layout

```
<data-root>/
  config.yaml            user-editable, hand-writable
  media.db               (Postgres lives elsewhere; this is the marker/lock)
  cache/
    transcodes/<item>/   segment cache, purgeable, size-capped
    images/              generated thumbnails and posters
    trickplay/           scrub-preview sprite sheets
  metadata/<item>/       extracted subtitles, fonts, chapter data
  backups/               automatic pre-migration dumps
```

Three rules: **everything under `cache/` must be safe to delete at any time** and regenerate;
nothing the user can't reconstruct lives anywhere but the database and the media files
themselves; and the media directories are **mounted read-only by default**. That last one is
not paranoia — an HN commenter on the Jellyfin 12.0 thread noted they only give Jellyfin
read-only access to their movies, and a separate complaint is that the UI says a deletion
"will be removed from Jellyfin" when it in fact deletes the file. Writes to the media tree
should require an explicit opt-in per library.

## 7. What this settles

1. ~~**Postgres, shipped in the default compose file.**~~ **Superseded: SQLite with a pure-Go
   driver** ([D31](../../decisions/working-notes.md)). The repository layer still keeps the door
   open in the other direction.
2. **`sqlc` for typed SQL; no ORM.** (D6)
3. **Files are the source of truth; the database is a rebuildable index**; user-authored data is
   mirrored to sidecars. (D7)
4. **Audit/change-journal tables and a delta-sync cursor exist in the first schema.** (D8)
5. **`Title → Edition → Version → File` is the core model**, not an afterthought. *(Written as
   three levels on 2026-09-17; corrected to four by [D79](../../decisions/working-notes.md) on
   2026-09-19 — the missing level is EIDR's Manifestation, and its absence is where the field's
   multi-version bugs come from.)*
6. **Automatic pre-migration backup; forward-only ordered migrations; detect-and-refuse.**
7. **SQLite FTS5; no separate search service.** Fuzzy filename→title matching moves into the
   identification component in Go, since SQLite has no `pg_trgm` equivalent (D31).
8. **Media read-only by default.**
