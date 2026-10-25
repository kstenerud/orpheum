> **ABOUTME:** The Cut A schema — every table the first release needs, as SQL that actually runs,
> with each table's class (authored, observed, fetched, identity, cache), what references it, and
> the decision behind it. The single place the tables are listed.

# Schema (Cut A)

- **Status:** PLANNED — the first migration of [Cut A](cut-a.md)'s M1. Revised 2026-09-23 by
  the second audit ([audit-2026-09-23.md](../research/audit-2026-09-23.md) §3.1, [D193](../../decisions/working-notes.md)): a
  `setting` table, per-column classes on mixed tables, `provider_response` moved into
  `persistent.db`, node state, one `job` queue, FTS triggers, and the ceiling columns dropped;
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
> ([D198](../../decisions/working-notes.md)). The steps are a baseline, `0048_baseline.sql`, a new
> line D303 started (D303 §2: the data before it was discarded), and any step after it, kept since
> the checkpoint ([D272](../../decisions/working-notes.md) §5); the notes below that name a migration
> (*migration 0011*, *0023: was …*, *step 0045*) record the step that added or renamed a column.
> Steps 0001 to 0047 are folded into the baseline and survive only in the repository's history.

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
- **Every table is `STRICT`** (D303, M5; the baseline at 0048), in both databases, so a value of
  the wrong type is refused, not kept as another type that never compares equal. **A subtype row
  is tied to its supertype's kind**: `title` and `collection` carry `item_kind`, fixed by a
  `CHECK`, and a composite foreign key `(id, item_kind)` to `item(id, kind)`, which `item` makes
  unique. **A rule a row obeys alone is a `CHECK`**: an id at a source only with its source
  (`followed_source_id` needs `followed_source`), a library's `check_every_hours` only for checking
  by interval. The DDL below shows the tables as the design reads them; the baseline,
  `internal/store/migrations/0048_baseline.sql`, is the schema that runs.
- **Closed vocabularies are `CHECK` constraints**, so a typo is an error rather than a new value.
  Widening one is a table rebuild (about a second, [D181](../../decisions/working-notes.md)).
- **Two database files.** `persistent.db` holds everything below except the last section and is
  backed up **whole** — at the reference scale that is 46 MB and 177 ms by `VACUUM INTO`, so
  splitting it by table bought nothing. `cache.db` holds match candidates, the job queue and the
  index of prepared renditions, is never backed up, and may be deleted at any time
  ([principles §14](../../principles/14-a-cache-may-always-be-wrong.md)). **Fetched provider responses are in
  `persistent.db`**, because re-deriving a title reads them and D170 forbids refetching them.

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
CREATE TABLE node (                                        -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  enrolled_at INTEGER NOT NULL,
  removed_at  INTEGER
  -- Its last status is kept in memory alone (D303, step 0049): a report every few seconds wrote it
  -- here, and nothing read it back.
);

-- How many may read one of a node's devices at once, as an admin set it for that device, over the
-- setting for its kind (D240 section 5); a device with no row takes its kind's. Migration 0030.
CREATE TABLE device_reader_limit (                          -- class A
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE CASCADE,
  device_key   TEXT NOT NULL,             -- the node's name for it: 'zfs:tank', 'disk:sdc'
  reader_limit INTEGER NOT NULL CHECK (reader_limit BETWEEN 1 AND 64),
  set_at       INTEGER NOT NULL,
  PRIMARY KEY (node_id, device_key)
) WITHOUT ROWID;

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
  retired_at INTEGER,                    -- an admin retired it (D231 section 7); migration 0029
  PRIMARY KEY (library_id, node_id, root_path)
);
```

A node's roots are set on the node and never remotely ([D107](../../decisions/working-notes.md));
the core records what the node says it serves.

**A root, a node and a library each have a retire action** ([D231](../../decisions/working-notes.md)
§7), a library's being its delete. Retiring tombstones the files ([D173](../../decisions/working-notes.md):
their history stays on their titles) and cancels their work. A retired root keeps its
`library_root` row with `retired_at`, and is scanned no more; a retired node is `node.removed_at`,
and its name enrols a new node if it ever says hello again; a deleted library is
`library.removed_at`, and its items are tombstoned (`item.removed_at` with no `merged_into`), hidden
with their history, so a film of it that reappears in another library brings its title back
([D208](../../decisions/working-notes.md) §2).

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
CREATE TABLE unavailable_place (                           -- class O; migration 0054 (D328)
  node_id   INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path      TEXT NOT NULL,             -- the root, or the disk's mount point inside it
  root_path TEXT NOT NULL,             -- the root it is, or is inside
  since     INTEGER NOT NULL,          -- the first time in a row it was found so
  cause     TEXT NOT NULL CHECK (cause IN ('not_answering', 'unread', 'missing', 'not_named')),
  PRIMARY KEY (node_id, path)
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
rather than go, until it returns or an admin says it is gone. These are the scans' facts.

**Availability is kept per place, in `unavailable_place`** (D328): a root, or a disk mounted inside
one, that cannot be read now, since when and why: its disk did not answer, a scan could not read
it, a scan found its disk gone from its mount point, or its node's hello no longer names it. A file
under one is not there to read. `since` is the System page's "offline since", and the admin's
reminder after `offline_reminder_after_days` ([D41](../../decisions/working-notes.md), D231 §7). A
place comes back when a scan reads it, or when its disk answers with the facts its last complete
scan recorded in `root_state` or `root_mount` (D326, D327). Only `internal/core/library`'s
`availability.go` writes it.

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
  filesystem_device INTEGER,             -- migration 0050: where the node last read it, with inode,
  inode         INTEGER,                 --   which tell it moved on its disk (D304 §5)
  is_identity_confirmed INTEGER NOT NULL DEFAULT 1
                  CHECK (is_identity_confirmed IN (0, 1)),   -- D179: 0 after a rebuild, until re-seen
  filing        TEXT NOT NULL CHECK (filing IN
                  ('listed','extra','attached','skipped','errored','unidentified')),
  filing_reason TEXT,                    -- the rule verbatim, the error code, what was tried
  sidecar_kind  TEXT CHECK (sidecar_kind IN ('nfo','subtitle','image')),  -- 0023: an attached file's kind
  error_detail_json TEXT,                -- owner, group, mode; the platform's text (D163)
  version_id    INTEGER REFERENCES version(id) ON DELETE RESTRICT,
  part_index    INTEGER,                 -- order within a multi-part Version
  attached_to_file INTEGER REFERENCES file(id) ON DELETE RESTRICT,  -- a sidecar's video
  identified_from_path TEXT,             -- the path its identification was read from
  first_seen_at    INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL,
  removed_at    INTEGER                  -- tombstone: only on a positive report (D34, D179)
);
CREATE UNIQUE INDEX file_live_path ON file(node_id, reported_path_raw) WHERE removed_at IS NULL;  -- 0011: was (node_id, path)
-- D303 (step 0045): the paths queries take that none served.
CREATE INDEX file_removed_payload ON file(library_id, payload_hash, removed_at) WHERE removed_at IS NOT NULL;
CREATE INDEX file_library ON file(library_id, path) WHERE removed_at IS NULL;
CREATE INDEX file_errored ON file(library_id, path) WHERE filing = 'errored' AND removed_at IS NULL;
CREATE INDEX file_payload ON file(payload_hash) WHERE removed_at IS NULL;  -- moves, duplicates
CREATE INDEX file_version ON file(version_id) WHERE version_id IS NOT NULL;
CREATE VIEW live_file AS SELECT * FROM file WHERE removed_at IS NULL;

-- D303 (M8, step 0047): what is wide and rarely read sits beside the file, so the row a scan stamps
-- stays narrow, and each is written only when it changed.
CREATE TABLE file_probe (                                  -- class O: what the node probed (D59)
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  probe_json  TEXT NOT NULL,             -- the raw probe output, the input D59 replays
  duration_ms INTEGER NOT NULL DEFAULT 0 -- its format's duration; 0 when it gave none
);
CREATE TABLE file_nfo (                                    -- class O: a rescan sends it again
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  body        BLOB NOT NULL              -- the NFO's bytes, which identification reads (D182)
);
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
-- Migration 0032 (D244 section 4, S90): one per file and provider's file, since each cost a download.
CREATE UNIQUE INDEX fetched_subtitle_once ON fetched_subtitle(video_file, provider, provider_file_id);
```

A fetched subtitle is **stored in the database, not the media tree**: it cost one of the server's
OpenSubtitles account's daily downloads (20 on a free account, as OpenSubtitles counts them; D224 §4), the library may be read-only ([principles §2](../../principles/02-the-users-files.md)),
and in the database the ordinary backup carries it with no second file class.
A subtitle is **fetched for a file once**: a second ask for the same provider's file is given the
one kept, and one made while the first is underway waits for it, so neither spends a download
(D244 §4).

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

**Each picture's timeline** is read in the same read as the loudness (step 0035,
[D247](../../decisions/working-notes.md) §10, chunks.md §2): its keyframes, where a chunk of a copy
of it can begin, and its bytes in each second of the film, its bit-rate profile. It is kept beside
the loudness, by payload hash and the picture's stream index, for the same reason: making it again
reads the whole file. It was in `cache.db` for one version, where a discard would have had every
video read through again (F1).

```sql
CREATE TABLE video_timeline (                              -- class O: what the node read (D247 section 10)
  payload_hash BLOB NOT NULL,            -- the bytes it was read from, as file.payload_hash (D179)
  video_stream_index INTEGER NOT NULL,   -- the picture's stream, as ffprobe numbers every stream
  keyframes_ms_json TEXT NOT NULL DEFAULT '[]',  -- JSON: its keyframes' times in ms, ascending: where a chunk can begin
  bytes_in_each_second_json TEXT NOT NULL DEFAULT '[]',  -- JSON: its bytes in each second of the film, by the second
  failure     TEXT,                      -- FFmpeg's words when the file could not be read through; the JSON is then empty
  made_at     INTEGER NOT NULL,
  read_failure TEXT,                     -- 0036: FFmpeg's words when a read through failed after the index gave the keyframes, which are kept (D270 §1)
  PRIMARY KEY (payload_hash, video_stream_index)
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
-- What waits for a person's match after a search found nothing: CountWaitingForAPerson (D303, step 0046).
CREATE INDEX title_waiting ON title(id)
  WHERE identification_state = 'unidentified' AND followed_source = 'none' AND kind != 'episode';

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
CREATE INDEX version_title ON version(title_id);     -- every version, removed too, which a merge moves (D303)

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
  id          INTEGER PRIMARY KEY AUTOINCREMENT,  -- the full-text indexes' key for the row (D303)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  language        TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT,
  search_alternates  TEXT,                      -- migration 0010: the name, folded for search (D48)
  UNIQUE (item_id, language)
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
  PRIMARY KEY (item_id, locale)          -- one key per item in each locale, found by its item (D303)
) WITHOUT ROWID;
CREATE INDEX sort_key_listing ON sort_key(locale, listing, collation_key, item_id);  -- the listings' order

CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alternates, overview, content='item_text', content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, search_alternates, content='item_text', content_rowid='id', tokenize='trigram'
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
    VALUES (new.id, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.id, new.name, new.search_alternates);
END;
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.id, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.id, old.name, old.search_alternates);
END;
-- Each index is rewritten only when a column it indexes changed (D303, DL9): SQLite fires an
-- UPDATE trigger whenever a row is written, whatever its values.
CREATE TRIGGER item_text_au AFTER UPDATE OF name, search_alternates, overview ON item_text
WHEN old.name IS NOT new.name OR old.search_alternates IS NOT new.search_alternates
  OR old.overview IS NOT new.overview BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.id, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.id, new.name, new.search_alternates, new.overview);
END;
CREATE TRIGGER item_text_au_trigram AFTER UPDATE OF name, search_alternates ON item_text
WHEN old.name IS NOT new.name OR old.search_alternates IS NOT new.search_alternates BEGIN
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.id, old.name, old.search_alternates);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.id, new.name, new.search_alternates);
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
  removed_at  INTEGER,
  -- 0029: an image that could not be stored, when and why, and whether its source said it was
  -- not there (a 404, rechecked once, D213 section 1)
  refused_at  INTEGER,
  refusal     TEXT,
  is_missing_at_source INTEGER NOT NULL DEFAULT 0 CHECK (is_missing_at_source IN (0, 1))
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

An image that could not be stored is a row with no `source_sha256`: its item holds it, so it is
not asked for again, and an older image of the kind stays shown. Its `refusal` and `refused_at`
are the item's own failure, a fact that a person's refresh with images clears (D231 §4). One its
source said was missing is looked at once more: the refresh sweep asks for the item's record
again once the source's CDN has stopped answering from its cached 404 (twelve hours for TMDB),
derived from this row and never held as a job (D213 §1, D231 §3), and a second miss gives it up.
A refused image beside the media is owed again once a scan has seen its file since, so a disk
that was away brings its images back (G9).

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
  name          TEXT NOT NULL,           -- 'aired', a TMDB episode group's 'tmdb:<group id>', a person's; 'release' for an album
  provider      TEXT,                    -- NULL when user-authored
  kind          TEXT,                    -- 0057: 'aired', a TMDB group's type ('dvd', 'absolute', 'digital',
                                         -- 'story_arc', 'production', 'tv', 'aired_alternate'), 'authored'
  source_id     TEXT,                    -- 0057: the provider's id for it, a TMDB group's; NULL for aired
  label         TEXT,                    -- 0057: its provider's name for it ("DVD Order"); name stays the key
  listed_count  INTEGER,                 -- 0057: how many episodes its provider lists in it, before it is fetched
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
CREATE TABLE group_text (                                  -- a season's name, a disc's, a part's
  ordering_id     INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,  -- 0057: was collection_id
  group_number         INTEGER NOT NULL,
  language        TEXT NOT NULL,
  name        TEXT,
  overview    TEXT,
  PRIMARY KEY (ordering_id, group_number, language)
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

-- Migration 0033 (D243 section 4): the libraries a role's members may browse and play. The
-- built-in admin role reaches every library and has no rows; any other role reaches only those
-- it lists, so a library taken back is gone from every answer at the next request.
CREATE TABLE role_library (                                -- class A (D243 section 4)
  role_id          INTEGER NOT NULL REFERENCES role(id) ON DELETE CASCADE,
  library_id       INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  may_rebroadcast  INTEGER NOT NULL DEFAULT 0 CHECK (may_rebroadcast IN (0,1)),  -- section 5, later
  PRIMARY KEY (role_id, library_id)
) WITHOUT ROWID;
CREATE INDEX role_library_library ON role_library(library_id);

CREATE TABLE user (                                        -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  password_hash BLOB,
  created_at    INTEGER NOT NULL,
  removed_at    INTEGER,
  admin_note    TEXT,                    -- 0034: who it is for, the admin's alone (D246 section 2)
  admin_away    TEXT NOT NULL DEFAULT 'never'  -- 0042: an admin administers from away too, 'always' (D302)
    CHECK (admin_away IN ('never', 'always'))
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
  audio_output  TEXT CHECK (audio_output IN ('built_in_speakers','receiver','headphones')),  -- migration 0020; unused since D336 §4: nothing reads or writes it, and no one is asked
  sound_offset_ms INTEGER NOT NULL DEFAULT 0 CHECK (sound_offset_ms BETWEEN -2000 AND 2000),  -- migration 0056 (0055's sound_delay_ms): the sound moved later (+) or earlier (-) than its picture
  added_at    INTEGER NOT NULL,
  last_signed_in_at     INTEGER NOT NULL,
  removed_at    INTEGER
);
CREATE INDEX device_owner ON device(user_id) WHERE removed_at IS NULL;  -- a person's devices (0015)

CREATE TABLE "grant" (                                     -- class A (D154)
  secret_sha256        BLOB PRIMARY KEY,          -- SHA-256 of the secret; never the secret
  kind        TEXT NOT NULL CHECK (kind IN ('session','link','cast','join','api_key',
                'adapter','node','relay','issued_secret')),  -- 0029: named for what it covers
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL,
  parent_secret_sha256 BLOB REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,  -- D177: derived grants
  issued_at   INTEGER NOT NULL,
  expires_at  INTEGER,
  last_used_at   INTEGER,                -- 0027: was last_seen_at; the API's lastUsedAt
  revoked_at  INTEGER,
  device_name TEXT,                      -- the device an add-device code makes
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  locate_hash BLOB,                      -- 0037: SHA-256 over "orpheum-locate" and a link's secret, what a discovery locates the core by (discovery-core.md K7)
  offer_key   BLOB,                      -- 0037: SHA-256 over "orpheum-offer" and a link's secret, what an offer through it is proven under
  link_sealed BLOB                       -- 0038: the link's secret sealed under secret.key, bound to this row, so its person is shown their own link again (secrets.Store.SealBound)
) WITHOUT ROWID;
CREATE INDEX grant_user ON "grant"(user_id) WHERE revoked_at IS NULL;  -- 0023: was grant_subject
CREATE UNIQUE INDEX grant_locate_hash ON "grant"(locate_hash) WHERE locate_hash IS NOT NULL;  -- 0037

-- A used registration link, or invitation, becomes its person's own link: kind 'link', scope
-- 'own', living 90 days from their last signed-in visit (0037; the third audit's A12).

-- A device locator, one per session, a fresh one on each visit (0037; discovery-core.md K7).
CREATE TABLE locator (                                     -- class A
  session_secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  hash        BLOB NOT NULL UNIQUE,      -- SHA-256 over "orpheum-locator" and the locator; never the locator
  issued_at   INTEGER NOT NULL
) WITHOUT ROWID;

-- A person's realm key, what their realm sign-in is proven by both ways, made whenever the core
-- sees their password, for the master realm the core's host holds (0039; D297 section 3).
CREATE TABLE realm_key (                                   -- class A (D297)
  user_id       INTEGER PRIMARY KEY REFERENCES user(id) ON DELETE CASCADE,
  master_realm  TEXT NOT NULL,           -- the master realm the key was made for (proof.MasterRealm)
  key           BLOB NOT NULL,           -- proof.RealmKey, 32 bytes
  made_at       INTEGER NOT NULL
);
CREATE INDEX realm_key_master ON realm_key(master_realm);  -- a sign-in lists its master realm's (0040)

-- A film or series the matcher settled by a guess among several of its name, marked on its poster
-- until a person confirms or corrects it (0041; D301 section 4).
CREATE TABLE guessed_match (                               -- class F (D301)
  item_id     INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE CASCADE,
  guessed_at  INTEGER NOT NULL
);
```

`issued_secret` is a ninth grant kind, a secret handed to a person to pass on, added for
[D174](../../decisions/working-notes.md) as `invite` and named for what it covers in migration 0029
(the jobs rebuild's Phase 3). It has three scopes, each single use and, like every grant, stored
only as a hash: `invite`, an invitation, with no person yet, whose `invitation` row holds the role
and the admin's note, and which makes the account its person fills in
([D246](../../decisions/working-notes.md) §2; seven days); `register`, a registration link for a
person who forgot their password, which sets a new one
([D218](../../decisions/working-notes.md) §5; every one sent works until one is used, which ends
the others, [D318](../../decisions/working-notes.md)); and `add_device`, a
device code whose `device_name` was the name the device would be listed under (ten minutes). Since
[D288](../../decisions/working-notes.md) no `add_device` grant is made: a device is activated by a
code it asks for, kept in memory, and `device_name` and `device_id` wait to be dropped with the next
schema change.

```sql
CREATE TABLE invitation (                                  -- class A (D246 section 2, D318), step 0053
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  admin_note    TEXT NOT NULL CHECK (admin_note <> ''),
  issued_at     INTEGER NOT NULL,
  made_user_id  INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- the account its use made
  made_at       INTEGER,
  CHECK ((made_user_id IS NULL) = (made_at IS NULL))
);
CREATE INDEX invitation_role ON invitation(role_id);
CREATE INDEX invitation_made_user ON invitation(made_user_id) WHERE made_user_id IS NOT NULL;

CREATE TABLE invitation_link (                             -- class A (D318), step 0053
  secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  invitation_id INTEGER NOT NULL REFERENCES invitation(id) ON DELETE RESTRICT
) WITHOUT ROWID;
CREATE INDEX invitation_link_invitation ON invitation_link(invitation_id);
```

An invitation, with one or more links, each an `issued_secret` grant of scope `invitation`
([D318](../../decisions/working-notes.md)): another link leaves the earlier live. Registering
through any one makes the account, with the name and password its person chose, the role and the
note; that link becomes the person's own, the others are revoked in the same write, and the
invitation keeps `made_user_id` and `made_at`, which the admin's list shows
([D311](../../decisions/working-notes.md) §4). Withdrawing one still waiting, or deleting its role,
revokes its links and deletes it. The retention sweep deletes the links of grants long ended, and a
waiting invitation left with none; removing a person deletes the invitation that made them.

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
`secret.key` in the instance's persistent directory ([D198](../../decisions/working-notes.md), D242), never in this database, so a backup of `persistent.db`
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
CREATE INDEX play_span_device ON play_span(device_id);  -- 0051: a removed person's devices are deleted (D305)

CREATE TABLE purged_watching (                             -- class A (D306 §8), 0052
  user_id         INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  version_id      INTEGER NOT NULL REFERENCES version(id) ON DELETE RESTRICT,
  completed_count INTEGER NOT NULL CHECK (completed_count >= 0),  -- the 'completed' events of its deleted files
  watched_ms      INTEGER NOT NULL CHECK (watched_ms >= 0),       -- the time in their spans
  last_watched_at INTEGER NOT NULL,                               -- the latest of either
  PRIMARY KEY (user_id, version_id)
) WITHOUT ROWID, STRICT;
CREATE INDEX purged_watching_version ON purged_watching(version_id);
```

**What was watched outlives the file** (D306 §8). A removed file is deleted once
`removed_files_kept_days` (90) have passed; before it goes, how often each person finished it and
how long they watched are added to its version's row here, and its spans, events and position go
with it. A version keeps its edition and moves with its title when titles merge, so a new copy of
a film, filed to the same title, finds what was watched of the old one: an unknown cut matches any
cut, and two different known cuts keep their own. The position does not carry: it names a moment
in the deleted file's timeline (play-state.md). Step 0052 also indexes the file by when it was
removed and by the video a sidecar is attached to, and each play and file preference by its file,
since the purge finds and deletes by them (D303, DL3).

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
                                           -- 'audio_language', a series' 'episode_ordering'
                                           -- (the ordering's name as a JSON string); a person's
                                           -- own 'dialogue_boost', 'levelling'
  value_json  TEXT NOT NULL,               -- JSON; a switch is true or false, and unset is not false
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID;

-- subtitle_quota was here; migration 0017 drops it: the count of downloads left is the
-- provider's own (D224 §4).
```

**An admin's dismissal of the offline reminder is a preference at `user` scope**:
`offline_reminder_dismissed_at` holds when, in Unix milliseconds, and hides the reminder from them
for `offline_reminder_interval_days` (D231 §7, P39).

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

## No caches in `persistent.db` — measured

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

## Fetched responses — in `persistent.db`

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

```sql
CREATE TABLE fetch_failure (                               -- class F; migration 0029
  item_id   INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  failure   TEXT NOT NULL,               -- what the provider or the job said
  failed_at INTEGER NOT NULL
);
CREATE TABLE refresh_request (                             -- class A; migration 0029
  item_id      INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  requested_at INTEGER NOT NULL,
  should_fetch_images INTEGER NOT NULL CHECK (should_fetch_images IN (0, 1))
);
```

**Work is derived from these facts, not held as jobs** ([D231](../../decisions/working-notes.md)
§3, §4). A film or series with no answer yet (not matched, not "not found", no failure of its
own) is owed its first match, whatever became of an earlier job for it; one whose host was down
parked rather than failed, and stays owed. A failure that is the item's own, a job's last
attempt included, is a `fetch_failure`: the sweeps pass it over, its page shows it, and a
person's refresh, "try again", clears it, as a fetch that succeeds does. A person's refresh is a
`refresh_request`, which a sweep queues until a fetch that began after it ends; so a request lost
with `cache.db`, or merged into a rolling refresh, still happens, images and all.

**Moved here from `cache.db` by the second audit.** The resolved columns of `title` and
`item_text` re-derive from these bodies, and D170 forbids refetching them, so losing them was
never "only a cache" — deleting `cache.db` would have forced exactly the refetch the design rules
out. At a few kilobytes compressed per title this adds tens of megabytes to the backup at the
reference scale, and removes a failure mode.

## `cache.db` — never backed up

> **Built in M2**, versioned by discard (migrations.md): `internal/store/cache/schema.sql` is the
> one schema, its version on a comment line, and a file whose `user_version` differs is deleted
> and built again. sqlc reads it into its own package, `cachegen`, so a `persistent.db` query
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
> hash as well as a bytes file's payload hash (M3's closing audit, D227); version 20 gave a job
> `is_snoozed`, so a repeat request wakes a job in back-off but not one waiting for a time it was
> given, and `last_attempt_tool_failed`, the tool's failure kept apart from the job's own
> (D231 §8, D233 §6: the jobs rebuild's Phase 1); version 21 gave a job its `destination` (D231
> §5, Phase 2); version 22 moved `subtitle_text` to `persistent.db` (the architecture map's F1, Phase 3);
> version 23 gave a job the `assigned` state and `assigned_until`, a job whose work a node does with
> no runner waiting for it (D234 §7, Phase 4.0); version 24 `node_attempt`, each job given to a node by
> its attempt token, so its node's word resolves it after a restart too (Phase 4c); version 25 a job's
`wanted_until`, past which play-driven work no person waits for falls to background priority (D234
§3, Phase 4d); version 26 `scan` and `scan_batch`, a root's scan in progress and the batches of files
it has its node read, and a job's `scan_id` and `scan_batch_id`, the targets of the `scan` and
`examine` kinds (D240, Phase 4e); version 29 renamed the `measure_loudness` job `read_through`, which
reads a video through once for its loudness and its picture's timeline, and added `video_timeline`,
that timeline by payload hash (D247 §10); version 30 moved `video_timeline` to `persistent.db` (step
0035, below), since making it again reads the whole file: F1's cost, resolved as for subtitle text and
loudness; version 31 added the `make_chunks` kind, a run of one stream's chunks of a film (D247
§10); version 32 gave it a target of its own, the job's `chunk_store_id`, a row of `chunk_store`
(one stream of a film on one grid), so the runs of a film's streams queue side by side; version 33
gave a `node_attempt` its `node_start_id`, the start of the node it was given to, so a node that
started again is known by its hello even when the core started again too (D238 §3).

```sql
CREATE TABLE job (                                         -- the one work queue (D193)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('fetch_metadata','make_artwork_sizes','prepare','extract_subtitle','convert_fetched_subtitle','read_through','make_chunks','scan','examine','backup')),
  -- What it works on, one column by kind (D227 section 2): fetch_metadata and make_artwork_sizes
  -- an item, prepare a rendition, extract_subtitle a stream, convert_fetched_subtitle a fetched subtitle,
  -- read_through and make_chunks a file, scan a scan (its listing), examine a scan's batch. At most one is set;
  -- backup, not yet a job, sets none.
  item_id             INTEGER,
  rendition_id        INTEGER,
  stream_id           INTEGER,
  fetched_subtitle_id INTEGER,
  file_id             INTEGER,
  scan_id             INTEGER,
  scan_batch_id       INTEGER,
  -- A kind's own arguments: fetch_metadata's isRefresh and shouldRetryImages; make_artwork_sizes'
  -- artworkSource, refs, sourceSha256 and sizeNames. The other kinds store '{}'.
  args_json   TEXT NOT NULL DEFAULT '{}',
  priority    INTEGER NOT NULL DEFAULT 0,  -- higher runs first; "play this now" is highest
  -- 'assigned' is a job whose work a node does: no runner waits for it (D234 section 7).
  state       TEXT NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued','running','assigned','done','failed','cancelled')),
  -- What it needs that can be down (D231 section 5): 'tmdb-api', 'node:3', 'root:3:/media/films';
  -- named when it is queued, or the one it was last parked on. A claim passes over the parked ones.
  destination TEXT,
  runnable_at INTEGER NOT NULL,          -- not before this: rate limits and retry back-off (D170)
  -- Whether a queued job waits for a time it was given (a rate limit, a hold), which a repeat
  -- request never brings forward; 0 when it waits on its start or its back-off (D231 section 8).
  is_snoozed  INTEGER NOT NULL DEFAULT 0 CHECK (is_snoozed IN (0, 1)),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  -- Whether its last attempt ended with the tool failing (a stall, a panic): a second in a row is
  -- the job's own failure (D233 section 6).
  last_attempt_tool_failed INTEGER NOT NULL DEFAULT 0 CHECK (last_attempt_tool_failed IN (0, 1)),
  progress_fraction REAL,                -- 0..1 when known
  reason      TEXT,                      -- why it exists, shown to the user
  last_failure TEXT,                     -- FFmpeg's exit and stderr tail, the provider's reply
  created_at  INTEGER NOT NULL,
  finished_at INTEGER,
  -- When an assigned job's time runs out, after which it is its own failure; kept while its
  -- outcome is taken, and NULL once that is written.
  assigned_until INTEGER,
  -- Until when a person waits for play-driven work (D234 section 3, D240 section 6): each play that
  -- asks for it moves it on; past it, the job falls to background priority. NULL for other work.
  wanted_until INTEGER,
  CHECK ((item_id IS NOT NULL) + (rendition_id IS NOT NULL) + (stream_id IS NOT NULL) +
         (fetched_subtitle_id IS NOT NULL) + (file_id IS NOT NULL) + (scan_id IS NOT NULL) +
         (scan_batch_id IS NOT NULL) <= 1)
);
-- The paths the queue's statements take (D303; the independent audit's S4), cache schema 37.
CREATE INDEX job_claim ON job(kind, priority DESC, runnable_at, id) WHERE state = 'queued';
CREATE INDEX job_open_target ON job(kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id,
  scan_id, scan_batch_id, chunk_store_id) WHERE state IN ('queued', 'running', 'assigned');
CREATE INDEX job_state ON job(state, kind, assigned_until);
CREATE INDEX job_wanted ON job(wanted_until) WHERE wanted_until IS NOT NULL;
CREATE INDEX job_finished ON job(finished_at) WHERE finished_at IS NOT NULL;
-- At most one queued, and one running, job per target is the store's rule, kept by cache.db's one
-- writer: a unique index cannot hold it while a target is one of eight nullable columns, which
-- SQLite's unique indexes treat as distinct. One target column would let it (D303, M3; to do).
CREATE TABLE scan (                                        -- D240: a root's scan in progress
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id     INTEGER NOT NULL,          -- persistent.db's node, whose root it is
  root_path   TEXT NOT NULL,
  priority    INTEGER NOT NULL,          -- its jobs': a person's request, or the queue's own
  started_at  INTEGER NOT NULL,
  -- When its listing was taken, NULL before: from then the ingest holds what it compared, which a
  -- restart loses, so such a scan starts again.
  listed_at   INTEGER,
  listed_count  INTEGER NOT NULL DEFAULT 0,  -- the files its listing found (D236 section 2)
  to_examine_count INTEGER NOT NULL DEFAULT 0,  -- of them, those it needs read
  examined_count INTEGER NOT NULL DEFAULT 0, -- and of those, read
  -- Whether a scan was asked for while this one ran: one more follows it (D236 section 3).
  is_another_scan_queued INTEGER NOT NULL DEFAULT 0 CHECK (is_another_scan_queued IN (0, 1)),
  UNIQUE (node_id, root_path)
);
CREATE TABLE scan_batch (                                  -- D240 section 8: a batch to examine
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  scan_id     INTEGER NOT NULL REFERENCES scan(id) ON DELETE CASCADE,
  files_json  TEXT NOT NULL,             -- the files, as the examine command names them
  file_count  INTEGER NOT NULL,
  is_done     INTEGER NOT NULL DEFAULT 0 CHECK (is_done IN (0, 1))
);
CREATE INDEX scan_batch_of_scan ON scan_batch(scan_id);
CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  item_id     INTEGER NOT NULL,          -- a film, or a series (D209)
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, provider_poster_path TEXT,
  votes       INTEGER,                   -- D301: the most voted settles a year shared
  PRIMARY KEY (item_id, rank)
);
CREATE TABLE art_served (                                  -- D212 section 6: recency, to the day
  source_sha256_hex TEXT PRIMARY KEY,     -- an image's directory under cache/art/
  last_served_epoch_day INTEGER NOT NULL  -- days since the Unix epoch it was last served
) WITHOUT ROWID;
CREATE TABLE prepared_rendition (                          -- D175, D180, D217 section 5
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  source_file  INTEGER NOT NULL,         -- persistent.db's file; play state records against it
  source_payload_hash BLOB NOT NULL,     -- the source as it was when prepared (D179)
  profile      TEXT NOT NULL CHECK (profile IN ('remux','h264')),
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

CREATE TABLE node_attempt (                                -- D234 section 7: work given to a node
  attempt_token TEXT PRIMARY KEY,        -- made for this sending; every result names it (D238 section 2)
  job_id      INTEGER NOT NULL,          -- the assigned job it is an attempt at; resolving it forgets this
  job_kind    TEXT NOT NULL,             -- the job's kind, whose answers resolve it
  node_id     INTEGER NOT NULL,          -- persistent.db's node it was sent to
  -- The start of that node it was given to, as its hello named it: a hello naming another start
  -- is a node that started again and lost it, whether or not the core restarted too (D238 section 3).
  node_start_id TEXT NOT NULL,
  work_json   TEXT NOT NULL,             -- JSON: the work as sent, without the bytes it carried
  sent_at     INTEGER NOT NULL
);
CREATE INDEX node_attempt_job ON node_attempt(job_id);
```

A rendition's files live on its node, in `<file_id>/<profile>/<run_id>/` under the node's prepared
directory, which the node alone deletes from (D217 §5); this row is the core's note of what it
asked for and what the node said. Its prepare job's target is the row's id, `rendition_id` (D217 §3).

The text made from a subtitle track is kept in `persistent.db` since version 22 (below, *Subtitle
text*): making it again reads a whole video, which is not "only timing" (the architecture map's
F1).

No foreign keys cross into `persistent.db` — SQLite cannot enforce them across files, and nothing
here is referenced by anything authored. **Losing `cache.db` costs re-queued work and
re-transcodes; it loses nothing** (D231 §3,
`TestScenario_S78_DiscardingCacheDBLosesOnlyTiming`): every job is derived again from `persistent.db`
by its sweep, or asked for again by the next play. A lost `match_candidate` means an ambiguous title's choice
screen searches once more, which is a search the user asked for, and running jobs restart from
zero because their output is a cache.

**One queue, not one per kind** (principles §4). Metadata fetches, prepare jobs, scans and
backups differ in what runs them and in `args_json`, not in how they wait, retry, report progress or
get cancelled; `fetch_queue` was the first of what would have become four.

## Subtitle text — in `persistent.db`

```sql
CREATE TABLE subtitle_text (                               -- class O; migration 0029 (F1)
  source_hash  BLOB NOT NULL,            -- the payload hash of the bytes' file, or a fetched body's SHA-256
  stream_index INTEGER NOT NULL,         -- the track's stream index in that file; 0 for a fetched subtitle
  ass          BLOB,                     -- for JASSUB; NULL when it could not be made
  vtt          BLOB,                     -- for the cast receiver; NULL when it could not be made
  failure      TEXT,                     -- why it could not be made: FFmpeg's exit and stderr tail
  made_at      INTEGER NOT NULL,
  PRIMARY KEY (source_hash, stream_index),
  CHECK ((failure IS NULL) = (ass IS NOT NULL AND vtt IS NOT NULL))
);
```

A text track is extracted when its video is first played (D217 §6): a track inside the video,
or a subtitle file beside it, converted whole. Its text is kept by the payload hash of the file
its bytes are in and its stream index there, a subtitle file's one track being its stream 0, so
a move or a rescan keeps it; a fetched subtitle's by the SHA-256 of its stored body (D224 §5).
A row is only what the node made, or why it could not: a track being made has none, and a play
of a track that could not be made asks again. Its extraction job's target is the `stream` row,
`stream_id` (D217 §3). A `stream` row keeps its id for as long as its file has that track: a
video observed again has its rows changed in place by stream index, and only a track its probe
no longer has is dropped; a subtitle file's row is changed in place too. A job whose stream is
gone fails, and the next play queues another. The text is a few hundred kilobytes at most, so it
is kept in the row rather than beside the art. It was in `cache.db` until migration 0029, where a
routine release discarded it and every embedded track was read whole again (F1).

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
| `artwork` | persistent.db | `internal/core/artwork` | identify reports the images it found; the server's wiring hands them to `artwork.Service.Take` |
| `collection` | persistent.db | `internal/core/identify` | |
| `collection_member` | persistent.db | `internal/core/identify` | |
| `device` | persistent.db | `internal/core/auth` | |
| `device_reader_limit` | persistent.db | `internal/core/nodes` | an admin's limit on one of a node's devices (D240 §5) |
| `edition` | persistent.db | none yet | |
| `edition_text` | persistent.db | none yet | |
| `extra_relation` | persistent.db | `internal/core/identify` | |
| `favourite` | persistent.db | none yet | |
| `fetched_subtitle` | persistent.db | `internal/core/subtitles` | |
| `fetch_failure` | persistent.db | `internal/core/identify` | the fetch_metadata runner's failure recorder, and a person's refresh, which clears it |
| `field_override` | persistent.db | `internal/core/identify` | |
| `file` | persistent.db | `internal/core/library` | identification files through `library.FileAs` and `library.PutInVersion` |
| `file_probe` | persistent.db | `internal/core/library` | a scan's probe of each video, beside its row (D303) |
| `file_nfo` | persistent.db | `internal/core/library` | an NFO's bytes, beside its row (D303) |
| `grant` | persistent.db | `internal/core/auth` | |
| `locator` | persistent.db | `internal/core/auth` | |
| `realm_key` | persistent.db | `internal/core/auth` | |
| `guessed_match` | persistent.db | `internal/core/identify` | |
| `group_text` | persistent.db | `internal/core/identify` | a provider's episode group's parts' names (episode-orderings.md) |
| `item` | persistent.db | `internal/core/identify` | |
| `item_fts`, `item_fts_tri` | persistent.db | `internal/core/identify` | kept by `item_text`'s triggers |
| `item_provider_id` | persistent.db | `internal/core/identify` | |
| `item_tag` | persistent.db | none yet | |
| `item_text` | persistent.db | `internal/core/identify` | |
| `library` | persistent.db | `internal/core/library` | |
| `library_root` | persistent.db | `internal/core/library` | |
| `loudness` | persistent.db | `internal/core/readthrough` | the read-through that measures it; `internal/core/loudness` reads it for the gains |
| `node` | persistent.db | `internal/core/nodes` | |
| `ordering` | persistent.db | `internal/core/identify` | |
| `play_event` | persistent.db | `internal/core/playback` | |
| `play_position` | persistent.db | `internal/core/playback` | |
| `play_span` | persistent.db | `internal/core/playback` | |
| `purged_watching` | persistent.db | `internal/core/playback` | what deleted files left of each person's watching on their versions, folded as library's purge deletes the files (D306 §8) |
| `position` | persistent.db | `internal/core/identify` | |
| `preference` | persistent.db | `internal/core/preferences` | |
| `progress_hide` | persistent.db | `internal/core/playback` | |
| `provider_response` | persistent.db | `internal/core/identify` | |
| `refresh_request` | persistent.db | `internal/core/identify` | |
| `role` | persistent.db | `internal/core/auth` | the baseline seeds the built-in roles; the admin makes the rest (D243 §4) |
| `invitation` | persistent.db | `internal/core/auth` | |
| `invitation_link` | persistent.db | `internal/core/auth` | |
| `role_library` | persistent.db | `internal/core/auth` | step 0033 gives the built-in user role every live library |
| `root_mount` | persistent.db | `internal/core/library` | |
| `root_scan` | persistent.db | `internal/core/library` | |
| `root_state` | persistent.db | `internal/core/library` | |
| `schema_meta` | persistent.db | `internal/store` | the migration runner's own statements |
| `schema_step` | persistent.db | `internal/store` | the migration runner's own statements |
| `secret` | persistent.db | `internal/core/secrets` | |
| `setting` | persistent.db | `internal/core/settings` | |
| `sort_key` | persistent.db | `internal/core/identify` | |
| `stream` | persistent.db | `internal/core/library` | |
| `subtitle_text` | persistent.db | `internal/core/subtitles` | |
| `tag` | persistent.db | none yet | |
| `title` | persistent.db | `internal/core/identify` | |
| `unavailable_place` | persistent.db | `internal/core/library` | only its `availability.go` (D328) |
| `user` | persistent.db | `internal/core/auth` | |
| `user_deny` | persistent.db | none yet | |
| `user_rating` | persistent.db | none yet | |
| `version` | persistent.db | `internal/core/identify` | |
| `watchlist` | persistent.db | none yet | |
| `art_served` | cache.db | `internal/core/artwork` | |
| `chunk_store` | cache.db | `internal/core/chunks` | made on a store's first run; its make_chunks jobs target it (D247 §10) |
| `job` | cache.db | `internal/store` | the job queue's storage, `JobStorage`, which `internal/platform/jobqueue` drives and `internal/core/jobs` uses (D231 §2) |
| `match_candidate` | cache.db | `internal/core/identify` | |
| `node_attempt` | cache.db | `internal/core/nodes` | each job given to a node, by its attempt token (D234 §7) |
| `scan` | cache.db | `internal/core/library` | a root's scan in progress, from its request to its end (D240) |
| `scan_batch` | cache.db | `internal/core/library` | the batches of files a scan has its node read (D240 §8) |
| `prepared_rendition` | cache.db | `internal/core/renditions` | |
| `video_timeline` | persistent.db | `internal/core/readthrough` | each picture's keyframes and bytes in each second, by payload hash; `internal/core/chunks` reads it for the grid (D247 §10) |

## What grows, and what bounds it

Every table that grows has a line here (D303, M10): what bounds it today, and what removes its
rows. Each class that would grow with no bound but time is kept to a limit a setting holds (D304
§6); a removed item's limit is still to be built.

| Grows with | Tables | Bounded by, today |
| --- | --- | --- |
| The library | `file`, `file_probe`, `file_nfo`, `stream`, `item`, `title`, `version`, `collection`, `collection_member`, `ordering`, `position`, `extra_relation`, `item_text`, `sort_key`, `item_provider_id`, `field_override`, `artwork`, `provider_response`, `loudness`, `video_timeline`, `subtitle_text`, `fetched_subtitle` | the files and works the libraries hold, **and every one they ever held**: a removed file is a tombstone, kept so that a file copied back returns with what was read of it (D208 §4, D231 §7), and deleted `removed_files_kept_days` (90) after, with its streams, probe, NFO, subtitles fetched for it and the choices made in it (D306 §8); a removed item, title or version stays, and nothing purges those yet |
| The library's open questions | `fetch_failure`, `refresh_request`, `guessed_match` | one row an item at most; each is deleted when its question is answered |
| People and their devices | `user`, `device`, `"grant"`, `locator` | the people and devices there are, and a removed person's nameless row (D305 §1); a credential that ended, revoked or expired, is deleted `ended_credentials_kept_days` (90) after, with its locator (D304 §6); a removed device stays as a row while a play names it |
| Watching | `play_event`, `play_span`, `play_position`, `progress_hide`, `purged_watching` | every play of the files there are; a deleted file's plays fold into one `purged_watching` row a person a version (D306 §8); all deleted with the person who watched (`ForgetWatchingOf`) |
| Scans | `root_scan`, `root_mount`, `root_state`, `unavailable_place` | one row a root or a disk in it, deleted with the root |
| Settings, roles, invitations, secrets | `setting`, `role`, `role_library`, `invitation`, `invitation_link`, `secret` | what an admin made; deleted when they delete it |

`cache.db` is bounded by its own rules ([below](#cachedb--never-backed-up)): a job that ended is
pruned once its keeping time passes (D233 §8), and renditions and chunks are evicted by their
budgets.

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
