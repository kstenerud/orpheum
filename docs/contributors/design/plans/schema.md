> **ABOUTME:** The Cut A schema — every table the first release needs, as SQL that actually runs,
> with each table's class (authored, observed, fetched, identity, cache), what references it, and
> the decision behind it. The single place the tables are listed.

# Schema (Cut A)

- **Status:** SKETCH — 2026-09-23, from the data-architecture critique
  ([D179](../../decisions/working-notes.md)–[D185](../../decisions/working-notes.md); this sketch is D185). The SQL
  blocks below are loaded verbatim by the benchmark in
  `/yoloai/cache/bench/schema/` (SQLite 3.53.4 via modernc), so they are valid as written.
- **Depends on:** [library-model.md](library-model.md) (the *why*), [migrations.md](migrations.md)
  (how this changes), [backup-restore.md](backup-restore.md).
- **Scope:** [Cut A](../../../ROADMAP.md) — two people, one remote, watching together with
  subtitles ([D178](../../decisions/working-notes.md)). Tables that are **additive** and belong to
  later features are listed at the end, not sketched
  ([D180](../../decisions/working-notes.md)).

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
  splitting it by table bought nothing. `cache.db` holds fetched responses and working queues,
  is never backed up, and may be deleted at any time ([principles §14](../../principles/README.md)).

**Class**, used in every table below, is what [D181](../../decisions/working-notes.md) and
[D182](../../decisions/working-notes.md) need to know to migrate it:

| Class | Meaning | A migration may |
| --- | --- | --- |
| **A** authored | a person made it | migrate in place; nothing re-derives it |
| **I** identity | ids and the evidence that binds them to files | migrate in place; a rebuild matches against it |
| **O** observed | derived from stored input (probe output, filenames, NFO, cached responses) | drop and re-derive |
| **F** fetched | provider values with no stored response | migrate in place; never refetch |
| **C** cache | anything rebuildable from the above | drop |

## Bookkeeping

```sql
PRAGMA application_id = 1447646258;  -- 'VPMS'; a file without it is refused, not migrated

CREATE TABLE schema_meta (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  compatible_version INTEGER NOT NULL  -- oldest schema whose code can still use this file
);
-- PRAGMA user_version is the schema version, set in each step's transaction (D181).
```

## Libraries and nodes

```sql
CREATE TABLE node (                                        -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  enrolled_at INTEGER NOT NULL,
  removed_at  INTEGER
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
  size          INTEGER,
  mtime         INTEGER,
  payload_hash  BLOB,                    -- D179: anchored on the media payload
  payload_len   INTEGER,
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
  first_seen    INTEGER NOT NULL,
  last_seen     INTEGER NOT NULL,
  removed_at    INTEGER                  -- tombstone: only on a positive report (D34, D179)
);
CREATE UNIQUE INDEX file_live_path ON file(node_id, path) WHERE removed_at IS NULL;
CREATE INDEX file_payload ON file(payload_hash) WHERE removed_at IS NULL;  -- moves, duplicates
CREATE INDEX file_version ON file(version_id) WHERE version_id IS NOT NULL;
CREATE VIEW live_file AS SELECT * FROM file WHERE removed_at IS NULL;
```

**`attached` is a sixth filing**, added here: a subtitle or NFO file that belongs to a video
beside it is neither listed nor an extra. D163 said a new reason would be visible with no new UI;
this is the first.

```sql
CREATE TABLE track (                                       -- class O
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT, -- where the bytes are
  media_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT, -- the video it serves
  stream_index INTEGER,                  -- NULL for a whole-file sidecar
  kind        TEXT NOT NULL CHECK (kind IN ('video','audio','subtitle','attachment')),
  codec       TEXT,
  lang_raw    TEXT,                      -- as the container carried it
  lang        TEXT,                      -- BCP 47, normalised (D24)
  title_raw   TEXT,
  disposition INTEGER NOT NULL DEFAULT 0,  -- FFmpeg's flags, as found
  channels    INTEGER,
  UNIQUE (file_id, stream_index)
);
CREATE INDEX track_media ON track(media_file, kind);

CREATE TABLE fetched_subtitle (                            -- class A: quota-limited, kept
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  media_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
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

## The work: Title → Edition → Version → File

```sql
CREATE TABLE title (                                       -- class I (id, provider ids) + O/F
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  library_id     INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  kind           TEXT NOT NULL CHECK (kind IN ('movie','episode','extra')),
  media_kind     TEXT NOT NULL DEFAULT 'video' CHECK (media_kind IN ('video','audio')),
  year           INTEGER,
  runtime_ms     INTEGER,
  original_lang  TEXT,
  -- D170/D183: the one source this title follows, and its id there
  source         TEXT CHECK (source IN ('nfo','tmdb','tvdb','none')),
  source_id      TEXT,
  ident          TEXT NOT NULL CHECK (ident IN
                   ('identified','ambiguous','unidentified','confirmed')),   -- D172
  origin         TEXT,                   -- NULL = this server; else who gave it (D174)
  merged_into    INTEGER REFERENCES title(id) ON DELETE RESTRICT,  -- D173
  created_at     INTEGER NOT NULL,
  removed_at     INTEGER
);
CREATE INDEX title_library ON title(library_id, kind) WHERE removed_at IS NULL;
CREATE INDEX title_ambiguous ON title(library_id) WHERE ident = 'ambiguous';

CREATE TABLE title_provider_id (                           -- class I
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  PRIMARY KEY (title_id, provider)
);
CREATE INDEX title_provider_lookup ON title_provider_id(provider, provider_id);

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
CREATE TABLE title_text (                                  -- class O/F, resolved (D45)
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  lang        TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT,
  PRIMARY KEY (title_id, lang)
);
CREATE UNIQUE INDEX title_text_original ON title_text(title_id) WHERE is_original = 1;

CREATE TABLE edition_text (                                -- class A or O
  edition_id  INTEGER NOT NULL REFERENCES edition(id) ON DELETE RESTRICT,
  lang        TEXT NOT NULL,
  name        TEXT NOT NULL,
  PRIMARY KEY (edition_id, lang)
);

CREATE TABLE sort_key (                                    -- class C (D47)
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE CASCADE,
  locale      TEXT NOT NULL,
  key         BLOB NOT NULL,             -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (locale, key, title_id)
) WITHOUT ROWID;

CREATE VIRTUAL TABLE title_fts USING fts5(                 -- class C (D48)
  name, overview, content='title_text', content_rowid='rowid',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE title_fts_tri USING fts5(
  name, content='title_text', content_rowid='rowid', tokenize='trigram'
);
```

`sort_key` and the FTS tables are caches and may `CASCADE`; nothing authored references them.
Triggers keeping the FTS in step with `title_text` are omitted from the sketch.

## What a person set: overrides, artwork, tags

```sql
CREATE TABLE field_override (                              -- class A (D183)
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  field       TEXT NOT NULL,             -- a field-registry name
  lang        TEXT NOT NULL DEFAULT '',  -- '' for language-invariant fields
  value       TEXT NOT NULL,             -- a frozen copy
  origin      TEXT NOT NULL,             -- 'typed', 'nfo', 'tmdb', 'tvdb' — display only
  set_by      INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (title_id, field, lang)
);

CREATE TABLE artwork (                                     -- class O/F; A when chosen
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('poster','backdrop','logo','banner','thumb','disc')),
  lang        TEXT,
  source      TEXT NOT NULL,             -- 'nfo', 'tmdb', 'embedded', 'user'
  source_ref  TEXT,
  content_hash BLOB,                     -- the derived sizes live in cache/ under this
  chosen      INTEGER NOT NULL DEFAULT 0 CHECK (chosen IN (0,1)),  -- a person picked it
  removed_at  INTEGER
);
CREATE UNIQUE INDEX artwork_chosen ON artwork(title_id, kind) WHERE chosen = 1;
CREATE INDEX artwork_title ON artwork(title_id, kind);

CREATE TABLE tag (                                         -- class A (D164)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL UNIQUE
);
CREATE TABLE title_tag (
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  tag_id      INTEGER NOT NULL REFERENCES tag(id) ON DELETE RESTRICT,
  PRIMARY KEY (tag_id, title_id)
) WITHOUT ROWID;
```

The columns in `title` and `title_text` are **the resolved values** — override, else the
title's source, else local fallbacks ([D183](../../decisions/working-notes.md)) — and are
rewritten whenever a layer changes. They are what everything reads.

## Series, collections and orderings

```sql
CREATE TABLE collection (                                  -- class A or O
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  library_id  INTEGER REFERENCES library(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('series','box_set','user')),
  membership  TEXT NOT NULL CHECK (membership IN ('explicit','predicate','external')),
  predicate   TEXT,                      -- D75's serialised form, for 'predicate'
  created_by  TEXT NOT NULL CHECK (created_by IN ('scanner','provider','user')),
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
);
CREATE TABLE collection_text (
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  lang        TEXT NOT NULL,
  name        TEXT NOT NULL,
  overview    TEXT,
  PRIMARY KEY (collection_id, lang)
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
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  series_id   INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  name        TEXT NOT NULL,             -- 'aired', 'dvd', 'absolute', or a person's
  provider    TEXT,                      -- NULL when user-authored
  UNIQUE (series_id, name)
);
CREATE TABLE episode_position (
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  season      INTEGER,
  number      INTEGER,
  sort        REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, title_id)
);
CREATE INDEX episode_sort ON episode_position(ordering_id, sort);
CREATE TABLE season_text (
  series_id   INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  season      INTEGER NOT NULL,
  lang        TEXT NOT NULL,
  name        TEXT,
  overview    TEXT,
  PRIMARY KEY (series_id, season, lang)
);
```

**There is no series table and no season table.** A series is a Collection whose members are
episodes; a season is a number within an ordering, with optional text
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
  predicate     TEXT,                    -- NULL = sees everything; invalid = sees nothing (D164)
  max_bitrate_kbps INTEGER,              -- F31; NULL = no ceiling
  max_streams   INTEGER,
  download_lease_days INTEGER            -- NULL = indefinite (D169); shown on the role's page
);

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
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  PRIMARY KEY (user_id, title_id)
) WITHOUT ROWID;

CREATE TABLE device (                                      -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,
  platform      TEXT NOT NULL,
  audio_output  TEXT CHECK (audio_output IN ('tv_speakers','receiver','headphones')),
                                         -- NULL until asked on first playback (D184)
  first_seen    INTEGER NOT NULL,
  last_seen     INTEGER NOT NULL,
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
  last_seen   INTEGER,
  revoked_at  INTEGER,
  label       TEXT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT
) WITHOUT ROWID;
CREATE INDEX grant_subject ON "grant"(subject) WHERE revoked_at IS NULL;
```

`invite` is a ninth grant kind, added for [D174](../../decisions/working-notes.md): single use,
and redeeming it creates the outsider's user with the role, ceilings and lease **asked when the
invite was made** ([D184](../../decisions/working-notes.md)).

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
  pos_from_ms INTEGER NOT NULL,
  pos_to_ms   INTEGER NOT NULL,
  t_from      INTEGER NOT NULL,
  t_to        INTEGER NOT NULL,
  unterminated INTEGER NOT NULL DEFAULT 0 CHECK (unterminated IN (0,1))
);
CREATE INDEX play_span_user ON play_span(user_id, t_from);
CREATE INDEX play_span_file ON play_span(file_id);

CREATE TABLE favourite (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  title_id INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT, at INTEGER NOT NULL,
  PRIMARY KEY (user_id, title_id)) WITHOUT ROWID;
CREATE TABLE user_rating (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  title_id INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT, rating INTEGER NOT NULL,
  at INTEGER NOT NULL, PRIMARY KEY (user_id, title_id)) WITHOUT ROWID;
CREATE TABLE watchlist (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  title_id INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT, at INTEGER NOT NULL,
  PRIMARY KEY (user_id, title_id)) WITHOUT ROWID;

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

## `cache.db` — never backed up

```sql
CREATE TABLE provider_response (                           -- class C: the stored input (D170)
  title_id    INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  fetched_at  INTEGER NOT NULL,
  body        BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
  PRIMARY KEY (title_id, provider)
);
CREATE TABLE fetch_queue (                                 -- rate-limited work (D170)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  reason      TEXT NOT NULL,             -- 'first', 'user', 'thin', 'score_on_view'
  not_before  INTEGER NOT NULL,
  attempts    INTEGER NOT NULL DEFAULT 0,
  last_error  TEXT
);
CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  title_id    INTEGER NOT NULL,
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, poster_ref TEXT,
  PRIMARY KEY (title_id, rank)
);
CREATE TABLE prepared_rendition (                          -- D175, D180
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  source_file INTEGER NOT NULL,          -- play state records against this
  profile     TEXT NOT NULL,
  path        TEXT NOT NULL,             -- under cache/
  bytes       INTEGER,
  created_at  INTEGER NOT NULL
);
```

No foreign keys cross into `library.db` — SQLite cannot enforce them across files, and nothing
here is referenced by anything authored. **Losing `cache.db` costs refetches the user asks for,
re-queued work and re-transcodes; it loses nothing** — and a lost `match_candidate` means an
ambiguous title's choice screen searches once more.

## Not in Cut A, and additive

Each arrives with its feature as a new table or column, precluding nothing
([D180](../../decisions/working-notes.md)): people and credits (Cut A reads cast from
`provider_response`), certifications and the descriptor vector, external scores, media segments,
loudness columns on `track`, the speech map and subtitle verification columns, trickplay, home
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
| 7. the track model is not three axes | `track.kind` is a closed set that widens by rebuild |
| 13. a grant may have a parent | `"grant".parent` |
| 14. every item carries its source | `title.origin` |

Obligations 5, 6 and 8–12 are satisfied by additive tables or by client behaviour and are
unaffected by this sketch.
