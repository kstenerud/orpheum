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
  below uses the current names. Units are in names (`size_bytes`, `mtime_ms`), times end in
  `_at`, booleans are predicates, and abbreviations are spelled out (`identification`,
  `group_number`, `search_alternates`, `started_at` for play spans).
- **Depends on:** [library-model.md](library-model.md) (the *why*), [migrations.md](migrations.md)
  (how this changes), [backup-restore.md](backup-restore.md).
- **Scope:** [Cut A](../../../ROADMAP.md) — two people, one remote, watching together with
  subtitles ([D178](../../decisions/working-notes.md)). Tables that are **additive** and belong to
  later features are listed at the end, not sketched
  ([D180](../../decisions/working-notes.md)).

> **IMPLEMENTED — see `internal/store/migrations/0001_initial.sql`** (M1): every table above
> the `cache.db` section, verbatim, plus the two built-in roles. `cache.db` arrives in M2
> ([D198](../../decisions/working-notes.md)).

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

CREATE TABLE schema_step (                                 -- migration 0009 (plan-deviations P17)
  version INTEGER PRIMARY KEY,
  sha256  TEXT NOT NULL                  -- of the step file as it was applied
);
-- The runner refuses a database where an applied step's text has since changed.
```

## Libraries and nodes

```sql
CREATE TABLE node (                                        -- class I + O (the last three)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  enrolled_at INTEGER NOT NULL,
  removed_at  INTEGER,
  report_seq  INTEGER NOT NULL DEFAULT 0,  -- last journal sequence the core ingested (D59)
  status      TEXT,                        -- the node's last status record, JSON
  status_at   INTEGER                      -- when it arrived; liveness is judged from this
);

CREATE TABLE library (                                     -- class A
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('movies','shows','mixed')),
  -- the three questions asked at creation (D184), never defaulted:
  metadata_source  TEXT NOT NULL CHECK (metadata_source IN ('nfo_then_tmdb','tmdb','nfo')),
  change_detection TEXT NOT NULL CHECK (change_detection IN ('watch','interval','manual')),
  check_every_h    INTEGER,          -- only for 'interval'
  writable         INTEGER NOT NULL CHECK (writable IN (0,1)),
  created_at       INTEGER NOT NULL,
  removed_at       INTEGER
);

CREATE TABLE library_root (                                -- class O: as the node reports it (D107)
  library_id INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id    INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path       TEXT NOT NULL,
  PRIMARY KEY (library_id, node_id, path)
);
```

A node's roots are set on the node and never remotely ([D107](../../decisions/working-notes.md));
the core records what the node says it serves.

```sql
CREATE TABLE root_scan (                                   -- class O
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path         TEXT NOT NULL,
  completed_at INTEGER NOT NULL,         -- the last scan of this root that finished
  PRIMARY KEY (node_id, path)
);
```

**A file is tombstoned only by a completed scan of its root** ([D179](../../decisions/working-notes.md)),
and this is where "completed" is recorded, in the same transaction as the tombstones. After a
rebuild it is empty, so nothing is tombstoned until a scan completes — the safe direction.

```sql
CREATE TABLE root_state (                                  -- class O; migrations 0003 and 0008
  node_id         INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path            TEXT NOT NULL,
  filesystem_device          INTEGER,                -- st_dev at the last complete scan
  is_mount_point     INTEGER CHECK (is_mount_point IN (0, 1)),
  outcome         TEXT NOT NULL CHECK (outcome IN ('complete', 'unreadable', 'withheld')),
  outcome_at      INTEGER NOT NULL,
  withheld_scan_started_at  INTEGER,                -- the withheld scan's start: what it did not see goes
  withheld_reason TEXT,                   -- once the admin confirms
  removals_approved INTEGER NOT NULL DEFAULT 0 CHECK (removals_approved IN (0, 1)),  -- class A
  PRIMARY KEY (node_id, path)
);
CREATE TABLE root_mount (                                  -- class O; migration 0004
  node_id  INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path     TEXT NOT NULL,
  path     TEXT NOT NULL,                -- NFC, absolute on the node, like file.path
  filesystem_device   INTEGER NOT NULL,             -- st_dev when it was last seen where expected
  is_present  INTEGER NOT NULL CHECK (is_present IN (0, 1)),
  PRIMARY KEY (node_id, path)
);
```

**A root's removals can be withheld, and a disk can go missing without its files going**
([D208](../../decisions/working-notes.md)). `root_state` is how the last scan of each root
ended: complete; unreadable, so nothing was removed; or withheld, because it would have
removed more than `removal_withhold_percent` of the root, until an admin approves
(`removals_approved`, the one authored column here, cleared by the scan it lets remove).
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
  path_raw      BLOB,                    -- the bytes, when they differ (D49)
  path_key      BLOB,                    -- migration 0011: the reported bytes, always; the live-file key (D211)
  size_bytes          INTEGER,
  mtime_ms         INTEGER,
  payload_hash  BLOB,                    -- D179: anchored on the media payload
  payload_bytes   INTEGER,
  identity      TEXT NOT NULL DEFAULT 'confirmed'
                  CHECK (identity IN ('confirmed','unconfirmed')),   -- D179: after a rebuild
  filing        TEXT NOT NULL CHECK (filing IN
                  ('listed','extra','attached','skipped','errored','unidentified')),
  filing_reason TEXT,                    -- the rule verbatim, the error code, what was tried
  error_detail  TEXT,                    -- owner, group, mode; the platform's text (D163)
  probe         TEXT,                    -- raw probe output, JSON; the input D59 replays
  version_id    INTEGER REFERENCES version(id) ON DELETE RESTRICT,
  part_index    INTEGER,                 -- order within a multi-part Version
  attached_to   INTEGER REFERENCES file(id) ON DELETE RESTRICT,  -- a sidecar's video
  first_seen_at    INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL,
  removed_at    INTEGER                  -- tombstone: only on a positive report (D34, D179)
);
CREATE UNIQUE INDEX file_live_path ON file(node_id, path_key) WHERE removed_at IS NULL;  -- 0011: was (node_id, path)
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
  lang_raw    TEXT,                      -- as the container carried it
  lang        TEXT,                      -- BCP 47, normalised (D24)
  title_raw   TEXT,
  disposition INTEGER NOT NULL DEFAULT 0,  -- FFmpeg's flags, as found
  channels    INTEGER,
  UNIQUE (bytes_file, stream_index)
);
CREATE INDEX stream_media ON stream(video_file, kind);

CREATE TABLE fetched_subtitle (                            -- class A: quota-limited, kept
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  video_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_ref TEXT NOT NULL,
  lang        TEXT NOT NULL,
  format      TEXT NOT NULL,
  body        BLOB NOT NULL,             -- a few kB; in the database so the backup carries it
  fetched_by  INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  fetched_at  INTEGER NOT NULL
);
```

A fetched subtitle is **stored in the database, not the media tree**: it cost one of the user's
20 daily downloads, the library may be read-only ([principles §2](../../principles/README.md)),
and in the database the ordinary backup carries it with no second file class.

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
  origin      TEXT,                      -- NULL = this server; else who gave it (D174)
  merged_into INTEGER REFERENCES item(id) ON DELETE RESTRICT,  -- D173
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
);
CREATE INDEX item_library ON item(library_id, kind) WHERE removed_at IS NULL;
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
  original_lang  TEXT,                                       -- O: resolved
  -- D170/D183: the one source this title follows, and its id there. F, or A once a person
  -- chose it: a match is a fetch, and a migration never redoes one.
  source         TEXT CHECK (source IN ('nfo','tmdb','tvdb','none')),       -- F/A
  source_id      TEXT,                                                      -- F/A
  identification          TEXT NOT NULL CHECK (identification IN
                   ('identified','ambiguous','unidentified','confirmed'))    -- F/A (D172)
);
CREATE INDEX title_ambiguous ON title(id) WHERE identification = 'ambiguous';

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
  type          TEXT NOT NULL CHECK (type IN ('trailer','teaser','clip','featurette',
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
  lang        TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT,
  search_alternates  TEXT,                      -- migration 0010: the name, folded for search (D48)
  PRIMARY KEY (item_id, lang)
);
CREATE UNIQUE INDEX item_text_original ON item_text(item_id) WHERE is_original = 1;

CREATE TABLE edition_text (                                -- class A or O
  edition_id  INTEGER NOT NULL REFERENCES edition(id) ON DELETE RESTRICT,
  lang        TEXT NOT NULL,
  name        TEXT NOT NULL,
  PRIMARY KEY (edition_id, lang)
);

CREATE TABLE sort_key (                                    -- class C (D47)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
  locale      TEXT NOT NULL,
  listing     TEXT NOT NULL,             -- the list it sorts in: 'movie', 'series', 'album'…
  key         BLOB NOT NULL,             -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (locale, listing, key, item_id)
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
  INSERT INTO item_fts(rowid, name, overview) VALUES (new.rowid, new.name, new.overview);
  INSERT INTO item_fts_tri(rowid, name) VALUES (new.rowid, new.name);
END;
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, overview) VALUES ('delete', old.rowid, old.name, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name) VALUES ('delete', old.rowid, old.name);
END;
CREATE TRIGGER item_text_au AFTER UPDATE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, overview) VALUES ('delete', old.rowid, old.name, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name) VALUES ('delete', old.rowid, old.name);
  INSERT INTO item_fts(rowid, name, overview) VALUES (new.rowid, new.name, new.overview);
  INSERT INTO item_fts_tri(rowid, name) VALUES (new.rowid, new.name);
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
  lang        TEXT NOT NULL DEFAULT '',  -- '' for language-invariant fields
  value       TEXT NOT NULL,             -- a frozen copy
  origin      TEXT NOT NULL,             -- 'typed', 'nfo', 'tmdb', 'tvdb' — display only
  set_by      INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (item_id, field, lang)
);

CREATE TABLE artwork (                                     -- class O/F; A when chosen
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('poster','backdrop','logo','banner','thumb','disc')),
  lang        TEXT,
  source      TEXT NOT NULL,             -- 'tmdb', 'local' (beside the media, D215), 'embedded', 'user'
  source_ref  TEXT,
  content_hash BLOB,                     -- the derived sizes live in cache/ under this
  chosen      INTEGER NOT NULL DEFAULT 0 CHECK (chosen IN (0,1)),  -- a person picked it
  removed_at  INTEGER
);
CREATE UNIQUE INDEX artwork_chosen ON artwork(item_id, kind) WHERE chosen = 1;
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
  source      TEXT CHECK (source IN ('nfo','tmdb','tvdb','none')),         -- F/A: its own values
  source_id   TEXT,                                                          -- F/A: its match
  identification       TEXT CHECK (identification IN ('identified','ambiguous','unidentified','confirmed'))  -- F/A
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
  number      INTEGER,                   -- episode, or track
  sort        REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, title_id)
);
CREATE INDEX position_sort ON position(ordering_id, sort);
CREATE INDEX position_title ON position(title_id);
CREATE TABLE group_text (                                  -- a season's name, a disc's subtitle
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  group_number         INTEGER NOT NULL,
  lang        TEXT NOT NULL,
  name        TEXT,
  overview    TEXT,
  PRIMARY KEY (collection_id, group_number, lang)
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
  builtin       INTEGER NOT NULL DEFAULT 0 CHECK (builtin IN (0,1)),
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
  outsider      INTEGER NOT NULL DEFAULT 0 CHECK (outsider IN (0,1)),  -- D174: sees no household
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
  audio_output  TEXT CHECK (audio_output IN ('tv_speakers','receiver','headphones')),
                                         -- NULL until asked on first playback (D184)
  first_seen_at    INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL,
  removed_at    INTEGER
);

CREATE TABLE "grant" (                                     -- class A (D154)
  hash        BLOB PRIMARY KEY,          -- SHA-256 of the secret; never the secret
  kind        TEXT NOT NULL CHECK (kind IN ('session','link','cast','join','api_key',
                'adapter','node','relay','invite')),
  subject     INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL,
  parent      BLOB REFERENCES "grant"(hash) ON DELETE RESTRICT,  -- D177: derived grants
  issued_at   INTEGER NOT NULL,
  expires_at  INTEGER,
  last_seen_at   INTEGER,
  revoked_at  INTEGER,
  label       TEXT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT
) WITHOUT ROWID;
CREATE INDEX grant_subject ON "grant"(subject) WHERE revoked_at IS NULL;
```

`invite` is a ninth grant kind, added for [D174](../../decisions/working-notes.md): single use,
and redeeming it creates the outsider's user with the role **asked when the invite was made**
([D184](../../decisions/working-notes.md)); ceilings and a lease join that question when they
exist.

```sql
CREATE TABLE setting (                                     -- class A
  scope       TEXT NOT NULL CHECK (scope IN ('server','user')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL for 'server'
  key         TEXT NOT NULL,             -- a name from the settings registry in code
  value       TEXT NOT NULL,             -- JSON
  set_by      INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL: the setup wizard
  set_at      INTEGER NOT NULL,
  CHECK ((scope = 'server') = (user_id IS NULL))
);
CREATE UNIQUE INDEX setting_key ON setting(scope, ifnull(user_id, 0), key);
```

**Everything changeable at runtime lives here** ([D181](../../decisions/working-notes.md),
[D182](../../decisions/working-notes.md) §3): the TMDB key, each user's OpenSubtitles login,
play-state thresholds, backup retention, the update-check answer. A key's type, bounds, starting
value and whether the settings page shows it come from one registry in code. **Every key has a
row** ([D200](../../decisions/working-notes.md)): the server writes each missing key's starting
value at start, reads only rows, and treats a missing row as an error that names the key and its
starting value. Migration 0002 adds `is_modified INTEGER NOT NULL DEFAULT 0` ([D201](../../decisions/working-notes.md)):
a person's set marks it, "set to default" clears it, and an unmarked row follows its starting value
when a release changes it. Per-item preferences stay in their own table.

```sql
CREATE TABLE secret (                                      -- class A, sealed (D194)
  scope       TEXT NOT NULL CHECK (scope IN ('server','user')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL for 'server'
  name        TEXT NOT NULL,             -- 'tmdb_api_key', 'opensubtitles_password', 'turn_secret'…
  key_id      INTEGER NOT NULL,          -- which secret.key sealed it (rotation)
  nonce       BLOB NOT NULL,             -- 24 bytes, XChaCha20-Poly1305
  ciphertext      BLOB NOT NULL,             -- ciphertext; the associated data is (scope, user_id, name)
  hint        TEXT,                      -- last secret_hint_chars characters of an API key; NULL for a password
  set_by      INTEGER REFERENCES user(id) ON DELETE RESTRICT,
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
  seq         INTEGER NOT NULL,          -- the guard: an older report does not move it
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  updated_at  INTEGER NOT NULL,
  PRIMARY KEY (user_id, file_id)
) WITHOUT ROWID;

CREATE TABLE play_event (                                  -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('started','paused','seeked','completed','abandoned')),
  position_ms INTEGER NOT NULL,
  server_at   INTEGER NOT NULL,
  client_mono_ms INTEGER
);
CREATE INDEX play_event_user_file ON play_event(user_id, file_id, server_at);

CREATE TABLE play_span (                                   -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  from_position_ms INTEGER NOT NULL,
  to_position_ms   INTEGER NOT NULL,
  started_at      INTEGER NOT NULL,
  ended_at        INTEGER NOT NULL,
  unterminated INTEGER NOT NULL DEFAULT 0 CHECK (unterminated IN (0,1)),
  unattended  INTEGER NOT NULL DEFAULT 0 CHECK (unattended IN (0,1))  -- after "still watching?" went unanswered (D188): never completes anything
);
CREATE INDEX play_span_user ON play_span(user_id, started_at);
CREATE INDEX play_span_file ON play_span(file_id);

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

CREATE TABLE preference (                                  -- class A (D24, D57, D90, D103)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL CHECK (scope IN ('user','library','series','device','file')),
  scope_id    INTEGER NOT NULL DEFAULT 0,  -- 0 for 'user'
  axis        TEXT NOT NULL,               -- 'audio_lang', 'subtitle_set', 'subtitle_offset_ms',
                                           -- 'embedded_fonts', 'caption_size', 'dialogue_boost'…
  value       TEXT NOT NULL,               -- tri-valued where the axis says so: 'off' ≠ unset
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID;

CREATE TABLE subtitle_quota (                              -- class C: a count per day
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
  provider    TEXT NOT NULL,
  day         TEXT NOT NULL,
  used        INTEGER NOT NULL,
  PRIMARY KEY (user_id, provider, day)
) WITHOUT ROWID;
```

**A sync offset is a preference at `file` scope**, because it is expressed in seconds of one
file ([D173](../../decisions/working-notes.md)); a series-wide offset is the same axis at
`series` scope, which is what D90 asked for.

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
  fetched_at  INTEGER NOT NULL,
  body        BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
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
> version 10 added `subtitle_text` and the `extract_subtitle` job with subtitle extraction in M3.

```sql
CREATE TABLE job (                                         -- the one work queue (D193)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('fetch','artwork','prepare','extract_subtitle','scan','backup')),  -- artwork: D212
  subject     INTEGER,                   -- item_id, file_id, library_id, stream_id — per kind
  args        TEXT NOT NULL DEFAULT '{}',  -- JSON: provider, profile, reason
  priority    INTEGER NOT NULL DEFAULT 0,  -- higher runs first; "play this now" is highest
  state       TEXT NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued','running','done','failed','cancelled')),
  node_id     INTEGER,                   -- where it runs, for node work
  runnable_at INTEGER NOT NULL,          -- not before this: rate limits and retry back-off (D170)
  attempts    INTEGER NOT NULL DEFAULT 0,
  progress    REAL,                      -- 0..1 when known
  reason      TEXT,                      -- why it exists, shown to the user
  last_error  TEXT,                      -- FFmpeg's exit and stderr tail, the provider's reply
  created_at  INTEGER NOT NULL,
  finished_at INTEGER
);
CREATE INDEX job_next ON job(state, priority DESC, runnable_at) WHERE state = 'queued';
CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  item_id     INTEGER NOT NULL,          -- a film, or a series (D209)
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, poster_ref TEXT,
  PRIMARY KEY (item_id, rank)
);
CREATE TABLE art_served (                                  -- D212 §6: recency, to the day
  hash        TEXT PRIMARY KEY,          -- an image's directory under cache/art/
  last_served_epoch_day INTEGER NOT NULL  -- days since the Unix epoch it was last served
) WITHOUT ROWID;
CREATE TABLE prepared_rendition (                          -- D175, D180, D217 §5
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  source_file  INTEGER NOT NULL,         -- library.db's file; play state records against it
  payload_hash BLOB NOT NULL,            -- the source as it was when prepared (D179)
  profile      TEXT NOT NULL CHECK (profile IN ('remux','h264-8m')),
  node_id      INTEGER NOT NULL,         -- the node that holds it, beside its source
  state        TEXT NOT NULL DEFAULT 'queued'
                 CHECK (state IN ('queued','running','complete','failed')),
  segment_length_s INTEGER NOT NULL,     -- seconds per segment; another setting remakes it
  run_id       TEXT NOT NULL,            -- names this making's directory on the node; new each time
  prepared_ms  INTEGER NOT NULL DEFAULT 0,  -- how much of the source is prepared
  size_bytes   INTEGER,                  -- its size once complete
  error        TEXT,                     -- why it failed: FFmpeg's exit and stderr tail
  created_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  UNIQUE (source_file, profile)
);
```

A rendition's files live on its node, in `<file_id>/<profile>/<run_id>/` under the node's prepared
directory, which the node alone deletes from (D217 §5); this row is the core's note of what it
asked for and what the node said. Its prepare job's subject is the row's id (D217 §3).

```sql
CREATE TABLE subtitle_text (                               -- D217 §6: by the video's payload hash
  payload_hash BLOB NOT NULL,            -- the video the track is in, as it was when extracted
  stream_index INTEGER NOT NULL,         -- the track's stream index in that video
  state        TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','complete','failed')),
  ass          BLOB,                     -- once complete: for JASSUB
  vtt          BLOB,                     -- once complete: for the cast receiver
  error        TEXT,                     -- why it failed: FFmpeg's exit and stderr tail
  created_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  PRIMARY KEY (payload_hash, stream_index)
);
```

A text track is extracted when its video is first played (D217 §6): a track inside the video,
or a subtitle file beside it, converted whole. Its text is kept by the payload hash of the file
its bytes are in and its stream index there, a subtitle file's one track being its stream 0, so
a move or a rescan keeps it. Its extraction job's subject is the `stream` row (D217 §3).
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
backups differ in what runs them and in `args`, not in how they wait, retry, report progress or
get cancelled; `fetch_queue` was the first of what would have become four.

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
| 13. a grant may have a parent | `"grant".parent` |
| 14. every item carries its source | `item.origin` |

Obligations 5, 6 and 8–12 are satisfied by additive tables or by client behaviour and are
unaffected by this sketch.
