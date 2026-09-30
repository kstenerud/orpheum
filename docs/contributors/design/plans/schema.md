> **ABOUTME:** The Cut A schema — every table the first release needs, as SQL that actually runs,
> with each table's class (authored, observed, fetched, identity, cache), what references it, and
> the decision behind it. The single place the tables are listed.

# Schema (Cut A)

- **Status:** PLANNED — the first migration of [Cut A](cut-a.md)'s M1. Revised 2026-09-23 by
  the second audit ([audit-2026-09-23.md](../research/audit-2026-09-23.md) §3.1, [D193](../../decisions/working-notes.md)): a
  `setting` table, per-column classes on mixed tables, `provider_response` moved into
  `library.db`, node state, one `job` queue, FTS triggers, and the ceiling columns dropped;
  then the sealed `secret` table ([D194](../../decisions/working-notes.md)).
  First sketched 2026-09-23 from the data-architecture critique
  ([D179](../../decisions/working-notes.md)–[D185](../../decisions/working-notes.md); this sketch is D185). The SQL
  blocks below are loaded verbatim by the benchmark in
  `/yoloai/cache/bench/schema/` (SQLite 3.53.4 via modernc), so they are valid as written.
- **Renamed 2026-09-26** by migration 0012, after a naming audit (agent-failures A8): the SQL
  below uses the current names. Units are in names (`size_bytes`, `runtime_ms`), times end in
  `_at`, booleans are predicates, and abbreviations are spelled out (`identification_state`, `language`,
  `group_number`, `search_alternates`, `started_at` for play spans).
- **Depends on:** [library-model.md](library-model.md) (the *why*), [migrations.md](migrations.md)
  (how this changes), [backup-restore.md](backup-restore.md).
- **Scope:** [Cut A](../../../ROADMAP.md) — two people, one remote, watching together with
  subtitles ([D178](../../decisions/working-notes.md)). Tables that are **additive** and belong to
  later features are listed at the end, not sketched
  ([D180](../../decisions/working-notes.md)).

> **IMPLEMENTED — see `internal/store/migrations/`** (M1): every table above the `cache.db`
> section, plus the two built-in roles. `cache.db` arrives in M2
> ([D198](../../decisions/working-notes.md)). Until migrations are live ([D228](../../decisions/working-notes.md#d228--until-migrations-are-live-librarydbs-schema-is-a-baseline-and-one-step-and-a-schema-change-folds-the-last-step-into-the-baseline)) the steps are a
> baseline, `0027_baseline.sql`, and the one step after it, `0028_choices_follow_the_work.sql`;
> the notes below that name a migration (*migration 0011*, *0023: was …*) record the step that
> added or renamed a column. Steps 0001 to 0027 are folded into the baseline and survive only in
> the repository's history.

## Why this document exists

The schema was prose spread across fifteen documents, and no single place listed the tables.
That is how the critique's largest finding went unnoticed for a week: the backup kept watch state
that referenced ids a rebuild would reissue ([D179](../../decisions/working-notes.md)). A list is
where that kind of hole is visible.

## Conventions

- **Integer keys, never reissued.** Every table something else references uses
  `INTEGER PRIMARY KEY AUTOINCREMENT` ([D173](../../decisions/working-notes.md),
  [D179](../../decisions/working-notes.md)). No UUIDs.
- **Tombstones, not deletes.** A referenced row carries `removed_at`; references from history or
  authored data are `ON DELETE RESTRICT`; uniqueness is a partial index over live rows; reads go
  through `live_*` views ([D173](../../decisions/working-notes.md)).
- **Times** are integer milliseconds since the epoch, server clock, unless named `*_mono_ms`
  (a client's monotonic clock, [D165](../../decisions/working-notes.md)).
- **Closed vocabularies are `CHECK` constraints**, so a typo is an error rather than a new value.
  Widening one is a table rebuild (about a second, [D181](../../decisions/working-notes.md)).
- **Two database files.** `library.db` holds everything below except the last section and is
  backed up **whole** — at the reference scale that is 46 MB and 177 ms by `VACUUM INTO`, so
  splitting it by table bought nothing. `cache.db` holds match candidates, the job queue and the
  index of prepared renditions, is never backed up, and may be deleted at any time
  ([principles §14](../../principles/README.md)). **Fetched provider responses are in
  `library.db`**, because re-deriving a title reads them and D170 forbids refetching them.

**Class**, used in every table below, is what [D181](../../decisions/working-notes.md) and
[D182](../../decisions/working-notes.md) need to know to migrate it:

| Class | Meaning | A migration may |
| --- | --- | --- |
| **A** authored | a person made it | migrate in place; nothing re-derives it |
| **I** identity | ids and the evidence that binds them to files | migrate in place; a rebuild matches against it |
| **O** observed | derived from stored input (probe output, filenames, NFO, cached responses) | drop and re-derive |
| **F** fetched | provider values with no stored response | migrate in place; never refetch |
| **C** cache | anything rebuildable from the above | drop |

**A table that mixes classes names them per column** (`file`, `title`). A migration then
re-derives only its **O** columns and carries every **A**, **I** and **F** column in place — a
person's choice or a fetched fact is never lost because it shares a row with something
derivable.

## Bookkeeping

```sql
PRAGMA application_id = 1447646258;  -- 'VPMS'; a file without it is refused, not migrated

CREATE TABLE schema_meta (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  compatible_version INTEGER NOT NULL  -- oldest schema whose code can still use this file
);
-- PRAGMA user_version is the schema version, set in each step's transaction (D181).

CREATE TABLE schema_step (                                 -- plan-deviations P17; in the baseline
  version INTEGER PRIMARY KEY,
  sha256  TEXT NOT NULL                  -- of the step file as it was applied
);
-- The runner records each step's checksum in its transaction, and refuses a database where an
-- applied step's text has since changed (a baseline also accepts the checksum of the step it folded, D228).
```

## Libraries and nodes

```sql
CREATE TABLE node (                                        -- class I + O (the last three)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  enrolled_at INTEGER NOT NULL,
  removed_at  INTEGER,
  last_ingested_report_seq  INTEGER NOT NULL DEFAULT 0,  -- last journal sequence the core ingested (D59)
  last_status_json      TEXT,                        -- the node's last status record, JSON
  last_status_at   INTEGER                      -- when it arrived; liveness is judged from this
);

CREATE TABLE library (                                     -- class A
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('movies','shows','mixed')),
  -- the three questions asked at creation (D184), never defaulted:
  metadata_source  TEXT NOT NULL CHECK (metadata_source IN ('nfo_then_tmdb','tmdb','nfo')),
  change_detection TEXT NOT NULL CHECK (change_detection IN ('watch','interval','manual')),
  check_every_hours INTEGER,         -- only for 'interval'
  is_writable         INTEGER NOT NULL CHECK (is_writable IN (0,1)),
  created_at       INTEGER NOT NULL,
  removed_at       INTEGER
);

CREATE TABLE library_root (                                -- class O: as the node reports it (D107)
  library_id INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id    INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path       TEXT NOT NULL,
  PRIMARY KEY (library_id, node_id, root_path)
);
```

A node's roots are set on the node and never remotely ([D107](../../decisions/working-notes.md));
the core records what the node says it serves.

```sql
CREATE TABLE root_scan (                                   -- class O
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path         TEXT NOT NULL,
  completed_at INTEGER NOT NULL,         -- the last scan of this root that finished
  PRIMARY KEY (node_id, root_path)
);
```

**A file is tombstoned only by a completed scan of its root** ([D179](../../decisions/working-notes.md)),
and this is where "completed" is recorded, in the same transaction as the tombstones. After a
rebuild it is empty, so nothing is tombstoned until a scan completes — the safe direction.

```sql
CREATE TABLE root_state (                                  -- class O; migrations 0003 and 0008
  node_id         INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path       TEXT NOT NULL,
  filesystem_device          INTEGER,                -- st_dev at the last complete scan
  is_mount_point     INTEGER CHECK (is_mount_point IN (0, 1)),
  last_scan_outcome TEXT NOT NULL CHECK (last_scan_outcome IN ('complete', 'unreadable', 'withheld')),
  last_scan_at    INTEGER NOT NULL,
  withheld_scan_started_at  INTEGER,                -- the withheld scan's start: what it did not see goes
  withheld_reason TEXT,                   -- once the admin confirms
  are_removals_approved INTEGER NOT NULL DEFAULT 0 CHECK (are_removals_approved IN (0, 1)),  -- class A
  PRIMARY KEY (node_id, root_path)
);
CREATE TABLE root_mount (                                  -- class O; migration 0004
  node_id  INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path     TEXT NOT NULL,
  mount_path    TEXT NOT NULL,           -- NFC, absolute on the node, like file.path
  filesystem_device   INTEGER NOT NULL,             -- st_dev when it was last seen where expected
  is_present  INTEGER NOT NULL CHECK (is_present IN (0, 1)),
  PRIMARY KEY (node_id, mount_path)
);
```

**A root's removals can be withheld, and a disk can go missing without its files going**
([D208](../../decisions/working-notes.md)). `root_state` is how the last scan of each root
ended: complete; unreadable, so nothing was removed; or withheld, because it would have
removed more than `scan_withhold_removals_over_percent` of the root and more than
`scan_withhold_removals_over_files` files, until an admin approves (`are_removals_approved`, the one
authored column here, cleared by the scan it lets remove).
`root_mount` is each mount boundary a complete scan crossed: a boundary that comes back as an
empty directory on the root's own device is a disk that is not there, and its files wait
rather than go, until it returns or an admin says it is gone.

**The Cut A node has no enrolment credential.** It is the core's child on a Unix socket whose
permissions are the boundary ([D193](../../decisions/working-notes.md)); a remote node's credential arrives with remote nodes, as a
`grant` of kind `node`.

## Files: every observation, its identity and its filing

One row per observed path, **whatever its filing** — listed, extra, skipped, errored,
unidentified — so the admin view is this table with no filter
([D163](../../decisions/working-notes.md)), and an unidentified file is already a playable row
(obligation #2).

```sql
CREATE TABLE file (                                        -- class I (identity columns) + O
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  library_id    INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id       INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path          TEXT NOT NULL,           -- NFC form
  reported_path_raw        BLOB,                    -- migration 0011: the reported bytes, always; the live-file key (D211); 0027: was reported_path_bytes
  size_bytes          INTEGER,
  modified_at         INTEGER,
  payload_hash  BLOB,                    -- D179: anchored on the media payload
  payload_bytes   INTEGER,
  opensubtitles_hash TEXT,               -- migration 0017: the provider's moviehash, 16 hex digits (D224 §3)
  is_identity_confirmed INTEGER NOT NULL DEFAULT 1
                  CHECK (is_identity_confirmed IN (0, 1)),   -- D179: 0 after a rebuild, until re-seen
  filing        TEXT NOT NULL CHECK (filing IN
                  ('listed','extra','attached','skipped','errored','unidentified')),
  filing_reason TEXT,                    -- the rule verbatim, the error code, what was tried
  sidecar_kind  TEXT CHECK (sidecar_kind IN ('nfo','subtitle','image')),  -- 0023: an attached file's kind
  error_detail_json TEXT,                -- owner, group, mode; the platform's text (D163)
  probe_json    TEXT,                    -- raw probe output, JSON; the input D59 replays
  version_id    INTEGER REFERENCES version(id) ON DELETE RESTRICT,
  part_index    INTEGER,                 -- order within a multi-part Version
  attached_to_file INTEGER REFERENCES file(id) ON DELETE RESTRICT,  -- a sidecar's video
  nfo_body      BLOB,                    -- migration 0005: an NFO's bytes, which identification reads
  identified_from_path TEXT,             -- the path its identification was read from
  first_seen_at    INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL,
  removed_at    INTEGER                  -- tombstone: only on a positive report (D34, D179)
);
CREATE UNIQUE INDEX file_live_path ON file(node_id, reported_path_raw) WHERE removed_at IS NULL;  -- 0011: was (node_id, path)
CREATE INDEX file_payload ON file(payload_hash) WHERE removed_at IS NULL;  -- moves, duplicates
CREATE INDEX file_version ON file(version_id) WHERE version_id IS NOT NULL;
CREATE VIEW live_file AS SELECT * FROM file WHERE removed_at IS NULL;
```

**`attached` is a sixth filing**, added here: a subtitle or NFO file that belongs to a video
beside it is neither listed nor an extra. D163 said a new reason would be visible with no new UI;
this is the first.

**A subtitle file is a track of its video.** Its `file` row carries its payload hash, the file
hashed whole (D179), and once it is attached it has one `stream` row: `bytes_file` the subtitle
file, `video_file` the video, no `stream_index`, and the language and flags its name gives after
the video's name, as Kodi and Jellyfin read them (`Film (2020).en.forced.srt`; `sdh`, `hi` and
`cc` mark a track for the deaf and hard of hearing). A subtitle file that belongs to no video has
no track.

```sql
CREATE TABLE stream (                                      -- class O; not "track" (D186)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  bytes_file     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT, -- where the bytes are
  video_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT, -- the video it serves
  stream_index INTEGER,                  -- NULL for a whole-file sidecar
  kind        TEXT NOT NULL CHECK (kind IN ('video','audio','subtitle','attachment')),
  codec       TEXT,
  container_language    TEXT,                      -- as the container carried it
  language        TEXT,                      -- BCP 47, normalised (D24)
  container_title   TEXT,
  disposition_flags INTEGER NOT NULL DEFAULT 0,  -- FFmpeg's flags, as found
  channel_count INTEGER,
  UNIQUE (bytes_file, stream_index)
);
CREATE INDEX stream_media ON stream(video_file, kind);

CREATE TABLE fetched_subtitle (                            -- class A: quota-limited, kept
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  video_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_file_id TEXT NOT NULL,
  language        TEXT NOT NULL,
  format      TEXT NOT NULL,
  body_gzip        BLOB NOT NULL,             -- gzip; a few kB; in the database so the backup carries it
  fetched_by_user_id  INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  fetched_at  INTEGER NOT NULL,
  release_name TEXT                      -- migration 0017: the provider's name for it (D224)
);
CREATE INDEX fetched_subtitle_video ON fetched_subtitle(video_file);  -- migration 0018
```

A fetched subtitle is **stored in the database, not the media tree**: it cost one of the server's
OpenSubtitles account's daily downloads (20 on a free account, as OpenSubtitles counts them; D224 §4), the library may be read-only ([principles §2](../../principles/README.md)),
and in the database the ordinary backup carries it with no second file class.

**Each audio track's loudness** is measured once by the node (migration 0019,
[D226](../../decisions/working-notes.md)): as it is when it has more than two channels, which a
receiver plays, and as the stereo mix a stereo device hears. It is kept **by payload hash and
stream index**, not by file, because it cost a read of the whole file: a rescan rewrites the
file's streams, a move changes its row, and a second copy is the same bytes. A row outlives its
file, at a few dozen bytes, and serves the bytes again if they come back.

```sql
CREATE TABLE loudness (                                    -- class O: what the node measured
  payload_hash    BLOB NOT NULL,
  stream_index    INTEGER NOT NULL,
  measured_mix           TEXT NOT NULL CHECK (measured_mix IN ('multichannel','stereo')),  -- as it is, or as stereo
  integrated_lufs REAL,                  -- -70 for silence; NULL when it failed
  range_lu        REAL,
  true_peak_dbtp  REAL,                  -- NULL for silence, which has no peak, or a failure
  failure         TEXT,                  -- FFmpeg's words when it could not be measured
  measured_at     INTEGER NOT NULL,
  PRIMARY KEY (payload_hash, stream_index, measured_mix)
) WITHOUT ROWID;
```

## Items: what text, artwork, tags, overrides and favourites attach to

Titles are not the only things with a name, a poster, a sort position, tags, a person's
corrections or a star: **a series, a box set, an album and an artist have all of them**. So they
share one id space ([D186](../../decisions/working-notes.md)), and every table below that describes
"a thing" references `item`, not `title`.

```sql
CREATE TABLE item (                                        -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('title','collection','person')),
  library_id  INTEGER REFERENCES library(id) ON DELETE RESTRICT,
  origin_server      TEXT,                      -- NULL = this server; else who gave it (D174)
  merged_into INTEGER REFERENCES item(id) ON DELETE RESTRICT,  -- D173
  added_at  INTEGER NOT NULL,
  removed_at  INTEGER
);
CREATE INDEX item_library ON item(library_id, kind) WHERE removed_at IS NULL;
CREATE INDEX item_added ON item(added_at) WHERE removed_at IS NULL;  -- migration 0026: newest first
CREATE VIEW live_item AS SELECT * FROM item WHERE removed_at IS NULL;

CREATE TABLE item_provider_id (                            -- class I; several per provider (ISRCs)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  PRIMARY KEY (item_id, provider, provider_id)
) WITHOUT ROWID;
CREATE INDEX item_provider_lookup ON item_provider_id(provider, provider_id);
```

`person` is reserved in the vocabulary and has no table in Cut A — people, credits and artists
arrive together as additive tables.

## The work: Title → Edition → Version → File

```sql
CREATE TABLE title (                                       -- mixed: see the columns
  id             INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  kind           TEXT NOT NULL CHECK (kind IN ('movie','episode','extra')),
  media_kind     TEXT NOT NULL DEFAULT 'video' CHECK (media_kind IN ('video','audio')),
  year           INTEGER,                                    -- O: resolved
  runtime_ms     INTEGER,                                    -- O: resolved
  original_language  TEXT,                                       -- O: resolved
  -- D170/D183: the one source this title follows, and its id there. F, or A once a person
  -- chose it: a match is a fetch, and a migration never redoes one.
  followed_source         TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')),       -- F/A
  followed_source_id      TEXT,                                                      -- F/A
  identification_state          TEXT NOT NULL CHECK (identification_state IN
                   ('identified','ambiguous','unidentified','confirmed'))    -- F/A (D172)
);
CREATE INDEX title_ambiguous ON title(id) WHERE identification_state = 'ambiguous';

CREATE TABLE edition (                                     -- class A when a person made it
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  created_by  TEXT NOT NULL CHECK (created_by IN ('provider','user')),  -- D158
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
);

CREATE TABLE version (                                     -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  edition_id  INTEGER REFERENCES edition(id) ON DELETE RESTRICT,  -- NULL: cut unknown (D155)
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
);
CREATE INDEX version_title ON version(title_id) WHERE removed_at IS NULL;

CREATE TABLE extra_relation (                              -- class O, or A when asserted
  owner_title   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  owner_edition INTEGER REFERENCES edition(id) ON DELETE RESTRICT,  -- only a person sets it
  extra_title   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  extra_type          TEXT NOT NULL CHECK (extra_type IN ('trailer','teaser','clip','featurette',
                  'behind_the_scenes','deleted_scene','interview','short','scene',
                  'theme_song','theme_video','unknown')),       -- D97, D162: eleven + unknown
  position      INTEGER,
  asserted_by   TEXT NOT NULL CHECK (asserted_by IN ('scanner','user')),
  PRIMARY KEY (owner_title, extra_title)
);
```

**An item need not have a File** (obligation #1): a Title with no Version is valid. **A prepared
rendition has no row here** — it is a cache entry in `cache.db`, and play state records against
its source File ([D180](../../decisions/working-notes.md)).

## Text, sort and search

```sql
CREATE TABLE item_text (                                   -- class O/F, resolved (D45)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  language        TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT,
  search_alternates  TEXT,                      -- migration 0010: the name, folded for search (D48)
  PRIMARY KEY (item_id, language)
);
CREATE UNIQUE INDEX item_text_original ON item_text(item_id) WHERE is_original = 1;

CREATE TABLE edition_text (                                -- class A or O
  edition_id  INTEGER NOT NULL REFERENCES edition(id) ON DELETE RESTRICT,
  language        TEXT NOT NULL,
  name        TEXT NOT NULL,
  PRIMARY KEY (edition_id, language)
);

CREATE TABLE sort_key (                                    -- class C (D47)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
  locale      TEXT NOT NULL,
  listing     TEXT NOT NULL,             -- the list it sorts in: 'movie', 'series', 'album'…
  collation_key         BLOB NOT NULL,             -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (locale, listing, collation_key, item_id)
) WITHOUT ROWID;

CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alternates, overview, content='item_text', content_rowid='rowid',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, search_alternates, content='item_text', content_rowid='rowid', tokenize='trigram'
);
```

**Migration 0010 adds D48's alternates**, which 0001 left out: `item_text.search_alternates` (named
`search_alt` until 0012) is the name
folded by the one normaliser in Go (`internal/core/textindex`), and both indexes take it, so
"Bjork" finds "Bjørk". The triggers below index it beside the name. Search's statements are
hand-written in `internal/store/search.go`, because sqlc cannot parse FTS5's `MATCH`.

```sql
CREATE TRIGGER item_text_ai AFTER INSERT ON item_text BEGIN
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.rowid, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.rowid, new.name, new.search_alternates);
END;
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.rowid, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.rowid, old.name, old.search_alternates);
END;
CREATE TRIGGER item_text_au AFTER UPDATE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.rowid, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.rowid, old.name, old.search_alternates);
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.rowid, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.rowid, new.name, new.search_alternates);
END;
```

`sort_key` and the FTS tables are caches and may `CASCADE`; nothing authored references them.
The triggers are the external-content pattern from SQLite's FTS5 documentation; a migration that
rebuilds `item_text` rebuilds the index with `INSERT INTO item_fts(item_fts) VALUES ('rebuild')`. **`listing` is in
the sort key's primary key because it has to be**: measured at 500,000 music tracks, a grid page
over one shared key walked every song and artist to find 50 albums — **9.4 ms**, against **0.35 ms**
with the listing first ([D186](../../decisions/working-notes.md)).

## What a person set: overrides, artwork, tags

```sql
CREATE TABLE field_override (                              -- class A (D183)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  field       TEXT NOT NULL,             -- a field-registry name
  language        TEXT NOT NULL DEFAULT '',  -- '' for language-invariant fields
  override_value       TEXT NOT NULL,             -- a frozen copy
  origin      TEXT NOT NULL,             -- 'typed', 'nfo', 'tmdb', 'tvdb' — display only
  set_by_user_id      INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (item_id, field, language)
);

CREATE TABLE artwork (                                     -- class O/F; A when chosen
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('poster','backdrop','logo','banner','still','disc')),
  language        TEXT,
  source      TEXT NOT NULL,             -- 'tmdb', 'local' (beside the media, D215), 'embedded', 'user'
  source_ref  TEXT,
  source_sha256 BLOB,                     -- the derived sizes live in cache/ under this
  is_chosen   INTEGER NOT NULL DEFAULT 0 CHECK (is_chosen IN (0,1)),  -- a person picked it
  removed_at  INTEGER
);
CREATE UNIQUE INDEX artwork_chosen ON artwork(item_id, kind) WHERE is_chosen = 1;
CREATE INDEX artwork_item ON artwork(item_id, kind);

CREATE TABLE tag (                                         -- class A (D164)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL UNIQUE
);
CREATE TABLE item_tag (
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  tag_id      INTEGER NOT NULL REFERENCES tag(id) ON DELETE RESTRICT,
  PRIMARY KEY (tag_id, item_id)
) WITHOUT ROWID;
```

The columns in `title` and `item_text` are **the resolved values** — override, else the
title's source, else local fallbacks ([D183](../../decisions/working-notes.md)) — and are
rewritten whenever a layer changes. They are what everything reads.

## Series, collections and orderings

```sql
CREATE TABLE collection (                                  -- class A or O
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('series','box_set','user')),
  membership  TEXT NOT NULL CHECK (membership IN ('explicit','predicate','external')),
  predicate   TEXT,                      -- D75's serialised form, for 'predicate'
  created_by  TEXT NOT NULL CHECK (created_by IN ('scanner','provider','user')),
  -- D209, migration 0007: a series is what is identified for shows, so it carries what a title
  -- does. NULL for box sets and a person's collections.
  year        INTEGER,                                                       -- O: resolved
  followed_source      TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')),         -- F/A: its own values
  followed_source_id   TEXT,                                                          -- F/A: its match
  identification_state       TEXT CHECK (identification_state IN ('identified','ambiguous','unidentified','confirmed'))  -- F/A
);
CREATE TABLE collection_member (                           -- D159, D160
  collection_id     INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  member_title      INTEGER REFERENCES title(id) ON DELETE RESTRICT,
  member_collection INTEGER REFERENCES collection(id) ON DELETE RESTRICT,
  position          REAL,                -- NULL: a member without a place (Machete order)
  CHECK ((member_title IS NULL) <> (member_collection IS NULL)),
  UNIQUE (collection_id, member_title),
  UNIQUE (collection_id, member_collection)
);

CREATE TABLE ordering (                                    -- D96, D159
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,           -- 'aired', 'dvd', 'absolute', a person's; 'release' for an album
  provider      TEXT,                    -- NULL when user-authored
  UNIQUE (collection_id, name)
);
CREATE TABLE position (                                    -- season/episode, or disc/track
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  group_number         INTEGER,                   -- season, or disc
  number_in_group      INTEGER,                   -- episode, or track
  sort_order        REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, title_id)
);
CREATE INDEX position_order ON position(ordering_id, sort_order);  -- 0023: was position_sort
CREATE INDEX position_title ON position(title_id);
CREATE TABLE group_text (                                  -- a season's name, a disc's subtitle
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  group_number         INTEGER NOT NULL,
  language        TEXT NOT NULL,
  name        TEXT,
  overview    TEXT,
  PRIMARY KEY (collection_id, group_number, language)
);
```

**There is no series table and no season table.** A series is a Collection whose members are
episodes; a season is a group number within an ordering, with optional text — and the same
tables hold an album's discs and tracks, which is why they are named `position` and `group_number` rather
than `episode_position` and `season` ([D186](../../decisions/working-notes.md))
([D160](../../decisions/working-notes.md)). `kind` omits `season` and `smart` from
library-model.md's list for that reason: a season is not a Collection here, and "smart" is
`membership = 'predicate'`.

## People, roles, devices, credentials

```sql
CREATE TABLE role (                                        -- class A (D130)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL UNIQUE,
  is_builtin       INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0,1)),
  capabilities  TEXT NOT NULL,           -- a set from the closed enum, e.g. 'administer,download'
  predicate     TEXT                     -- NULL = sees everything; invalid = sees nothing (D164)
);
-- Ceilings (F31) and the download lease (D169) arrive with their features as columns here
-- (D180); they were sketched and dropped by the second audit (D193).

CREATE TABLE user (                                        -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  password_hash BLOB,
  is_outsider      INTEGER NOT NULL DEFAULT 0 CHECK (is_outsider IN (0,1)),  -- D174: sees no household
  created_at    INTEGER NOT NULL,
  removed_at    INTEGER
);
CREATE UNIQUE INDEX user_live_name ON user(name) WHERE removed_at IS NULL;

CREATE TABLE user_deny (                                   -- class A: narrows only
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  PRIMARY KEY (user_id, item_id)
) WITHOUT ROWID;

CREATE TABLE device (                                      -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,
  platform      TEXT NOT NULL,
  audio_output  TEXT CHECK (audio_output IN ('built_in_speakers','receiver','headphones')),  -- migration 0020
                                         -- NULL until asked on first playback (D184)
  added_at    INTEGER NOT NULL,
  last_signed_in_at     INTEGER NOT NULL,
  removed_at    INTEGER
);
CREATE INDEX device_owner ON device(user_id) WHERE removed_at IS NULL;  -- a person's devices (0015)

CREATE TABLE "grant" (                                     -- class A (D154)
  secret_sha256        BLOB PRIMARY KEY,          -- SHA-256 of the secret; never the secret
  kind        TEXT NOT NULL CHECK (kind IN ('session','link','cast','join','api_key',
                'adapter','node','relay','invite')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL,
  parent_secret_sha256 BLOB REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,  -- D177: derived grants
  issued_at   INTEGER NOT NULL,
  expires_at  INTEGER,
  last_used_at   INTEGER,                -- 0027: was last_seen_at; the API's lastUsedAt
  revoked_at  INTEGER,
  device_name TEXT,                      -- the device an add-device code makes
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT
) WITHOUT ROWID;
CREATE INDEX grant_user ON "grant"(user_id) WHERE revoked_at IS NULL;  -- 0023: was grant_subject
```

`invite` is a ninth grant kind, added for [D174](../../decisions/working-notes.md): single use,
and redeeming it creates the outsider's user with the role **asked when the invite was made**
([D184](../../decisions/working-notes.md)); ceilings and a lease join that question when they
exist.
[D218](../../decisions/working-notes.md)'s accounts use it too, in two scopes: `register`, a
registration invitation for a person the admin already made, which sets their password (seven days;
a new one withdraws the old), and `add_device`, a device code whose `device_name` is the name the
device will be listed under (ten minutes). Both are single use, and like every grant they are
stored only as a hash.

```sql
CREATE TABLE setting (                                     -- class A: the server's (D200)
  key         TEXT NOT NULL PRIMARY KEY, -- a name from the settings registry in code; 0028: its unit spelled out
  value_json       TEXT NOT NULL,             -- JSON
  set_by_user_id      INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL: the setup wizard
  set_at      INTEGER NOT NULL,
  is_modified INTEGER NOT NULL DEFAULT 0 CHECK (is_modified IN (0, 1))  -- a person set it (D201)
);
-- 0028 dropped scope and user_id: no setting is a person's. A person's choices are preferences
-- (the architecture map's F6), and a user scope nothing defined was removed with its endpoints.
```

**Everything changeable at runtime lives here** ([D181](../../decisions/working-notes.md),
[D182](../../decisions/working-notes.md) §3): the TMDB key, the server's OpenSubtitles login (D224),
play-state thresholds, backup retention, the update-check answer. A key's type, bounds, starting
value and whether the settings page shows it come from one registry in code. **Every key has a
row** ([D200](../../decisions/working-notes.md)): the server writes each missing key's starting
value at start, reads only rows, and treats a missing row as an error that names the key and its
starting value. Migration 0002 adds `is_modified INTEGER NOT NULL DEFAULT 0` ([D201](../../decisions/working-notes.md)):
a person's set marks it, "set to default" clears it, and an unmarked row follows its starting value
when a release changes it. A person's choices are not settings: they are preferences, below.

```sql
CREATE TABLE secret (                                      -- class A, sealed (D194)
  scope       TEXT NOT NULL CHECK (scope IN ('server','user')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL for 'server'
  name        TEXT NOT NULL,             -- 'tmdb_api_key', 'opensubtitles_password', 'turn_secret'…
  key_id      INTEGER NOT NULL,          -- which secret.key sealed it (rotation)
  nonce       BLOB NOT NULL,             -- 24 bytes, XChaCha20-Poly1305
  ciphertext      BLOB NOT NULL,             -- ciphertext; the associated data is (scope, user_id, name)
  last_chars  TEXT,                      -- an API key's last characters, secret_last_chars_shown of them; NULL for a password
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  CHECK ((scope = 'server') = (user_id IS NULL))
);
CREATE UNIQUE INDEX secret_name ON secret(scope, ifnull(user_id, 0), name);
```

**Secrets are not settings** ([D194](../../decisions/working-notes.md)). A third-party credential is sealed with a key kept in
`secret.key` in the state directory ([D198](../../decisions/working-notes.md)), never in this database, so a backup of `library.db`
carries only ciphertext. A restore without the key marks each row *needs re-entering* and loses
nothing else. Only the `secrets` package queries this table. A user's own password is not here:
it is an argon2id PHC string in `user.password_hash`.

## Play state and the rest of a person's data

```sql
CREATE TABLE play_position (                               -- class A (D81, D157)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  position_ms INTEGER NOT NULL,
  report_seq         INTEGER NOT NULL,          -- 0014: the report's number within its playback
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  updated_at  INTEGER NOT NULL,
  position_read_at  INTEGER NOT NULL DEFAULT 0,  -- 0014: the guard, the moment of watching it records
  PRIMARY KEY (user_id, file_id)
) WITHOUT ROWID;

CREATE TABLE play_event (                                  -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('started','paused','seeked','completed','abandoned')),
  position_ms INTEGER NOT NULL,
  happened_at   INTEGER NOT NULL,
  happened_mono_ms INTEGER
);
CREATE INDEX play_event_user_file ON play_event(user_id, file_id, happened_at);

CREATE TABLE play_span (                                   -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  from_position_ms INTEGER NOT NULL,
  to_position_ms   INTEGER NOT NULL,
  started_at      INTEGER NOT NULL,
  ended_at        INTEGER NOT NULL,
  is_unterminated INTEGER NOT NULL DEFAULT 0 CHECK (is_unterminated IN (0,1)),
  is_unattended  INTEGER NOT NULL DEFAULT 0 CHECK (is_unattended IN (0,1))  -- after "still watching?" went unanswered (D188): never completes anything
);
CREATE INDEX play_span_user ON play_span(user_id, started_at);
CREATE INDEX play_span_file ON play_span(file_id);
```

**The guard is the moment of watching, not the order of arrival** (D217 §7, migration 0014). A
player reports on one playback: its report's number within the playback, the device's monotonic
clock when it sent the report and when it read the position, and the changes and spans since its
last report, each at a monotonic time. The server anchors each time to its own clock on arrival
(`server_now − (sent − t)`, D165), keeps events and spans at those times, and moves the position
only when its moment, `position_read_at`, is later than the stored one's. A report about an
earlier moment that arrives late is logged and moves nothing. The report number, `report_seq`,
drops a report sent again.

```sql

CREATE TABLE favourite (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, added_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;
CREATE TABLE user_rating (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, rating INTEGER NOT NULL,
  rated_at INTEGER NOT NULL, PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;
CREATE TABLE watchlist (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, added_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;
-- Hidden from Continue Watching / Next Up (D188). Not a completion; it lapses by itself when
-- the item has activity after `hidden_at`, so there is nothing to clear.
CREATE TABLE progress_hide (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, hidden_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;

CREATE TABLE preference (                                  -- class A (D24, D57, D90, D103, D230)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL CHECK (scope IN ('user','library','work','device','file')),  -- 0028: 'work' for 'series'
  scope_id    INTEGER NOT NULL DEFAULT 0,  -- 0 for 'user'; a work's is its series' or its title's item
  axis        TEXT NOT NULL,               -- a file's 'subtitle_offset_ms', 'subtitle_used',
                                           -- 'audio_used'; a work's 'subtitle_language',
                                           -- 'audio_language'; a person's own 'dialogue_boost',
                                           -- 'levelling'
  value_json  TEXT NOT NULL,               -- JSON; a switch is true or false, and unset is not false
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID;

-- subtitle_quota was here; migration 0017 drops it: the count of downloads left is the
-- provider's own (D224 §4).
```

**A sync offset is a preference at `file` scope**, because it is expressed in seconds of one
file ([D173](../../decisions/working-notes.md)); a series-wide offset would be the same axis at
`work` scope, which is what D90 asked for.

**A subtitle or audio choice is kept twice, and copied nowhere** ([D230](../../decisions/working-notes.md#d230--the-last-subtitle-and-audio-language-chosen-in-a-series-follows-its-unwatched-episodes-worked-out-at-play-time)).
At `work` scope (a series, or a film's title, by its item id) `subtitle_language` and
`audio_language` hold the person's last choice there: `{"language","kind","containerTitle"}`, or
`{"isOff":true}` for subtitles. At `file` scope `subtitle_used` and `audio_used` hold what was
last used in a started file: `{"streamId"|"fetchedSubtitleId","track":{…}}`, the track and what
it is, or `{"isOff":true}`; a choice in the file writes it, and the first play report of a file
with none writes what the playback started with. A file's tracks are matched when it plays: its
own `…_used` found again, else the work's language, of the same kind, then the same title.

## No caches in `library.db` — measured

The watched badge on a poster rolls completion up from File through Version to Title. A
`title_watch(user, title, watched_at, in_progress)` cache was sketched here and **measured out**
(`/yoloai/cache/bench/schema/`, reference scale — 3,500 titles, 10,000 files, 209,781 spans,
126,004 events, 4 users; laptop CPU, median of 200):

| Poster grid, 50 films, keyset page | median | p95 |
| --- | --- | --- |
| with the cache | 0.36 ms | 0.85 ms |
| **computed live**, well-shaped | **0.84 ms** | **1.49 ms** |
| computed live, `EXISTS` shaped badly (a `SCAN` of `version`) | 7.2 ms | 9.1 ms |

The page budget is **50 ms** ([performance-targets.md](performance-targets.md)). The cache buys
0.5 ms for a table, an invalidation path on every completion, and a staleness class — the same
verdict [D164](../../decisions/working-notes.md) reached for the role cache. **Not built.** What
the measurement did show is that **query shape matters ten times more than caching**: the
correlated in-progress check must be a `LIMIT 1` scalar subquery driven by `version_title`, not an
`EXISTS` the planner turns into a scan. That is a rule for the query review, not for the schema.

## Fetched responses — in `library.db`

```sql
CREATE TABLE provider_response (                           -- class F: the stored input (D170)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  part        TEXT NOT NULL DEFAULT '',  -- '' the item itself; 'season/N' a series' season listing
  fetched_at  INTEGER NOT NULL,          -- when body_gzip was fetched: the body's age
  last_asked_at INTEGER NOT NULL,        -- 0027: when the provider was last asked, with or without a new body (D213 §4)
  body_gzip   BLOB NOT NULL,             -- gzipped; cast and crew are read from here in Cut A
  PRIMARY KEY (item_id, provider, part)
);
```

**Keyed on `item` since migration 0007** ([D209](../../decisions/working-notes.md)): a series
is matched, not its episodes, and its own response and its season listings are stored against
it. An episode has no response of its own; it takes its values from its season's listing. Before
0007 the table was keyed on `title_id`, which left a series nowhere to keep its match — the one
table that did not follow D186.

**Moved here from `cache.db` by the second audit.** The resolved columns of `title` and
`item_text` re-derive from these bodies, and D170 forbids refetching them, so losing them was
never "only a cache" — deleting `cache.db` would have forced exactly the refetch the design rules
out. At a few kilobytes compressed per title this adds tens of megabytes to the backup at the
reference scale, and removes a failure mode.

## `cache.db` — never backed up

> **Built in M2**, versioned by discard (migrations.md): `internal/store/cache/schema.sql` is the
> one schema, its version on a comment line, and a file whose `user_version` differs is deleted
> and built again. sqlc reads it into its own package, `cachegen`, so a `library.db` query
> cannot name a cache table. `prepared_rendition` came with prepare in M3 (version 8); version 9
> renamed columns to say what they hold (`runnable_at`, `last_served_epoch_day`, `size_bytes`);
> version 10 added `subtitle_text` and the `extract_subtitle` job with subtitle extraction in M3;
> version 11 what made a rendition's video and how fast (D225); version 12 the
> `convert_fetched_subtitle` job (D224); version 13 the `measure_loudness` job and the gains a
> rendition's sound carries (D226); version 14 renamed columns to say what they hold and gave a
> job a column per kind of target (D227); version 15 named a rendition's gains `audio_gains_db_json`;
> version 16 gave each value one name in every layer (D227 §6); version 17 the languages a
> rendition's audio tracks are tagged with, so a relabelled track, which keeps the payload hash
> (D179), has its rendition made again; version 18 corrected the comment naming what a job's
> `args_json` holds; version 19 named the job kinds `fetch_metadata` and `make_artwork_sizes` for
> what they do, and `subtitle_text`'s key `source_hash`, since it holds a fetched subtitle's body
> hash as well as a bytes file's payload hash (M3's closing audit, D227).

```sql
CREATE TABLE job (                                         -- the one work queue (D193)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('fetch_metadata','make_artwork_sizes','prepare','extract_subtitle','convert_fetched_subtitle','measure_loudness','scan','backup')),
  -- What it works on, one column by kind (D227 section 2): fetch_metadata and make_artwork_sizes
  -- an item, prepare a rendition, extract_subtitle a stream, convert_fetched_subtitle a fetched subtitle,
  -- measure_loudness a file. At most one is set; scan and backup, not yet jobs, set none.
  item_id             INTEGER,
  rendition_id        INTEGER,
  stream_id           INTEGER,
  fetched_subtitle_id INTEGER,
  file_id             INTEGER,
  -- A kind's own arguments: fetch_metadata's isRefresh and shouldRetryImages; make_artwork_sizes'
  -- artworkSource, refs, sourceSha256 and sizeNames. The other kinds store '{}'.
  args_json   TEXT NOT NULL DEFAULT '{}',
  priority    INTEGER NOT NULL DEFAULT 0,  -- higher runs first; "play this now" is highest
  state       TEXT NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued','running','done','failed','cancelled')),
  node_id     INTEGER,                   -- where it runs, for node work
  runnable_at INTEGER NOT NULL,          -- not before this: rate limits and retry back-off (D170)
  attempt_count    INTEGER NOT NULL DEFAULT 0,
  progress_fraction REAL,                -- 0..1 when known
  reason      TEXT,                      -- why it exists, shown to the user
  last_failure  TEXT,                      -- FFmpeg's exit and stderr tail, the provider's reply
  created_at  INTEGER NOT NULL,
  finished_at INTEGER,
  CHECK ((item_id IS NOT NULL) + (rendition_id IS NOT NULL) + (stream_id IS NOT NULL) +
         (fetched_subtitle_id IS NOT NULL) + (file_id IS NOT NULL) <= 1)
);
CREATE INDEX job_next ON job(state, priority DESC, runnable_at) WHERE state = 'queued';
CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  item_id     INTEGER NOT NULL,          -- a film, or a series (D209)
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, provider_poster_path TEXT,
  PRIMARY KEY (item_id, rank)
);
CREATE TABLE art_served (                                  -- D212 section 6: recency, to the day
  source_sha256_hex TEXT PRIMARY KEY,     -- an image's directory under cache/art/
  last_served_epoch_day INTEGER NOT NULL  -- days since the Unix epoch it was last served
) WITHOUT ROWID;
CREATE TABLE prepared_rendition (                          -- D175, D180, D217 section 5
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  source_file  INTEGER NOT NULL,         -- library.db's file; play state records against it
  source_payload_hash BLOB NOT NULL,     -- the source as it was when prepared (D179)
  profile      TEXT NOT NULL CHECK (profile IN ('remux','h264-8m')),
  node_id      INTEGER NOT NULL,         -- the node that holds it, beside its source
  state        TEXT NOT NULL DEFAULT 'queued'
                 CHECK (state IN ('queued','running','complete','failed')),
  segment_length_seconds INTEGER NOT NULL,     -- seconds per segment; another setting remakes it
  run_id       TEXT NOT NULL,            -- names this making's directory on the node; new each time
  prepared_through_ms INTEGER NOT NULL DEFAULT 0,  -- how far into the source it is prepared
  size_bytes   INTEGER,                  -- its size once complete
  failure      TEXT,                     -- why it failed: FFmpeg's exit and stderr tail
  video_encoder TEXT,                    -- what made the video once complete: h264_qsv, h264_nvenc, x264, or copy (D225)
  realtime_factor REAL,                  -- seconds of the source per second of running, as FFmpeg last said (D225)
  carried_gains_db_json TEXT NOT NULL DEFAULT '[]',  -- JSON: the gain each audio track's sound carries, by its place (D226)
  audio_languages_json TEXT NOT NULL DEFAULT '[]',  -- JSON: the language each audio track is tagged with, by its place; another remakes it
  created_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  UNIQUE (source_file, profile)
);
```

A rendition's files live on its node, in `<file_id>/<profile>/<run_id>/` under the node's prepared
directory, which the node alone deletes from (D217 §5); this row is the core's note of what it
asked for and what the node said. Its prepare job's target is the row's id, `rendition_id` (D217 §3).

```sql
CREATE TABLE subtitle_text (                               -- D217 §6, D224 §5: by its source
  -- What the text was made from: an embedded track's bytes file, by its payload hash as it was
  -- when extracted (D179), or a fetched subtitle, by the SHA-256 of its stored body_gzip.
  source_hash  BLOB NOT NULL,
  stream_index INTEGER NOT NULL,         -- the track's stream index in that file; 0 for a fetched subtitle
  state        TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','complete','failed')),
  ass          BLOB,                     -- once complete: for JASSUB
  vtt          BLOB,                     -- once complete: for the cast receiver
  failure        TEXT,                     -- why it failed: FFmpeg's exit and stderr tail
  created_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  PRIMARY KEY (source_hash, stream_index)
);
```

A text track is extracted when its video is first played (D217 §6): a track inside the video,
or a subtitle file beside it, converted whole. Its text is kept by the payload hash of the file
its bytes are in and its stream index there, a subtitle file's one track being its stream 0, so
a move or a rescan keeps it. Its extraction job's target is the `stream` row, `stream_id` (D217 §3).
A `stream` row keeps its id for as long as its file has that track: a video observed again has
its rows changed in place by stream index, and only a track its probe no longer has is dropped;
a subtitle file's row is changed in place too. A job whose stream is gone fails, and the next
play queues another. The text is a few
hundred kilobytes at most, so it is kept in the row rather than beside the art.

No foreign keys cross into `library.db` — SQLite cannot enforce them across files, and nothing
here is referenced by anything authored. **Losing `cache.db` costs re-queued work and
re-transcodes; it loses nothing** — a lost `match_candidate` means an ambiguous title's choice
screen searches once more, which is a search the user asked for, and running jobs restart from
zero because their output is a cache.

**One queue, not one per kind** (principles §4). Metadata fetches, prepare jobs, scans and
backups differ in what runs them and in `args_json`, not in how they wait, retry, report progress or
get cancelled; `fetch_queue` was the first of what would have become four.

## Who writes each table

Each table has one writing package: the one package whose code calls the table's `INSERT`,
`UPDATE` and `DELETE` queries. Any other package that changes the table asks that package, through
a function that takes the caller's transaction (`library.FileAs`) or hands it what it found
(identify's report of an item's images, which the server's wiring gives `artwork.Service.Take`), so
the rules a table's rows keep live in one place (the architecture map's F5, decided 2026-09-30).
`TestArch_EachTableHasOneWritingPackage` reads this table and the queries' callers and fails on a
second writer, or on a writer this table does not name. A table no package writes yet says
*none yet*; its first writer is named here in the change that adds it.

| Table | Database | Writing package | Notes |
| --- | --- | --- | --- |
| `artwork` | library.db | `internal/core/artwork` | identify reports the images it found; the server's wiring hands them to `artwork.Service.Take` |
| `collection` | library.db | `internal/core/identify` | |
| `collection_member` | library.db | `internal/core/identify` | |
| `device` | library.db | `internal/core/auth` | |
| `edition` | library.db | none yet | |
| `edition_text` | library.db | none yet | |
| `extra_relation` | library.db | `internal/core/identify` | |
| `favourite` | library.db | none yet | |
| `fetched_subtitle` | library.db | `internal/core/playback` | |
| `field_override` | library.db | `internal/core/identify` | |
| `file` | library.db | `internal/core/library` | identification files through `library.FileAs` and `library.PutInVersion` |
| `grant` | library.db | `internal/core/auth` | |
| `group_text` | library.db | none yet | |
| `item` | library.db | `internal/core/identify` | |
| `item_fts`, `item_fts_tri` | library.db | `internal/core/identify` | kept by `item_text`'s triggers |
| `item_provider_id` | library.db | `internal/core/identify` | |
| `item_tag` | library.db | none yet | |
| `item_text` | library.db | `internal/core/identify` | |
| `library` | library.db | `internal/core/library` | |
| `library_root` | library.db | `internal/core/library` | |
| `loudness` | library.db | `internal/core/playback` | |
| `node` | library.db | `internal/core/nodes` | |
| `ordering` | library.db | `internal/core/identify` | |
| `play_event` | library.db | `internal/core/playback` | |
| `play_position` | library.db | `internal/core/playback` | |
| `play_span` | library.db | `internal/core/playback` | |
| `position` | library.db | `internal/core/identify` | |
| `preference` | library.db | `internal/core/preferences` | |
| `progress_hide` | library.db | `internal/core/playback` | |
| `provider_response` | library.db | `internal/core/identify` | |
| `role` | library.db | none yet | the baseline seeds the built-in roles |
| `root_mount` | library.db | `internal/core/library` | |
| `root_scan` | library.db | `internal/core/library` | |
| `root_state` | library.db | `internal/core/library` | |
| `schema_meta` | library.db | `internal/store` | the migration runner's own statements |
| `schema_step` | library.db | `internal/store` | the migration runner's own statements |
| `secret` | library.db | `internal/core/secrets` | |
| `setting` | library.db | `internal/core/settings` | |
| `sort_key` | library.db | `internal/core/identify` | |
| `stream` | library.db | `internal/core/library` | |
| `tag` | library.db | none yet | |
| `title` | library.db | `internal/core/identify` | |
| `user` | library.db | `internal/core/auth` | |
| `user_deny` | library.db | none yet | |
| `user_rating` | library.db | none yet | |
| `version` | library.db | `internal/core/identify` | |
| `watchlist` | library.db | none yet | |
| `art_served` | cache.db | `internal/core/artwork` | |
| `job` | cache.db | `internal/core/jobs` | |
| `match_candidate` | cache.db | `internal/core/identify` | |
| `prepared_rendition` | cache.db | `internal/core/playback` | |
| `subtitle_text` | cache.db | `internal/core/playback` | |

## Not in Cut A, and additive

Each arrives with its feature as a new table or column, precluding nothing
([D180](../../decisions/working-notes.md)): people and credits (Cut A reads cast from
`provider_response`) — **as `person` items with a credit table keyed on `item`**, so a film's
cast, an album's artist and a track's featured artist are one table ([D186](../../decisions/working-notes.md)); certifications and the descriptor vector, external scores, media segments,
loudness columns on `stream`, the speech map and subtitle verification columns, trickplay, home
rows, the rules field registry, retention marks, download registrations, the change journal
([D166](../../decisions/working-notes.md)), and Watch Together's persisted state if it ever needs
any (sessions live in memory and end with the process — [D119](../../decisions/working-notes.md)'s
*"for the session and not one second longer"*).

## Checked against the obligations

| Obligation (FEATURES.md) | Where |
| --- | --- |
| 1. an item need not have a File | a `title` with no `version` |
| 2. playback addressable by File | every observation is a `file` row |
| 3. referenced rows tombstoned | `removed_at` + `ON DELETE RESTRICT` + `live_*` |
| 4. collection membership is three kinds | `collection.membership` |
| 7. the track model is not three axes | `stream.kind` is a closed set that widens by rebuild |
| 13. a grant may have a parent | `"grant".parent_secret_sha256` |
| 14. every item carries its source | `item.origin_server` |

Obligations 5, 6 and 8–12 are satisfied by additive tables or by client behaviour and are
unaffected by this sketch.
