-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- orpheum:baseline folds 803a4a36b9554ff00a637bcd2cd4ec97fe302a4eda74f7f6acc10f5e2e0303c3
-- The baseline (D228, D460): persistent.db's schema at version 86 in one step, which builds a new
-- database and runs on no other. It is the migration origin: production ran steps 0049 to 0086
-- on the baseline before it (0086 is names_are_camel_case, whose checksum is above), so a database
-- at 86 carries on from here, and one at 85 or below, which no installation outside production
-- holds, is refused, or set aside and started over where database.when_unmigratable says so (D249
-- section 4). Migrations are live from here (LIVE): every later schema change is a step after
-- this one, never an edit of it.
-- Each statement is the text SQLite holds for that object in a database migrated through the
-- steps, so a new database and production's hold the same schema, word for word. The tables are
-- documented in docs/contributors/design/plans/schema.md. The runner owns application_id,
-- user_version and compatible_version; this file holds DDL and the two built-in roles only.

-- The tables.

CREATE TABLE schema_meta (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  compatible_version INTEGER NOT NULL  -- oldest schema whose code can still use this file
) STRICT;

CREATE TABLE schema_step (                                 -- class I: the runner's own record
  version INTEGER PRIMARY KEY,
  sha256  TEXT NOT NULL                  -- hex, of the step file's bytes
) STRICT;

CREATE TABLE artwork (                                     -- class O/F; A when chosen
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('poster','backdrop','logo','banner','still','disc')),
  language    TEXT,
  source      TEXT NOT NULL,             -- 'nfo', 'tmdb', 'local'
  source_ref  TEXT,
  source_sha256 BLOB,                    -- the SHA-256 of the source image; its sizes live in cache/ under it
  is_chosen   INTEGER NOT NULL DEFAULT 0 CHECK (is_chosen IN (0,1)),  -- a person picked it
  removed_at  INTEGER
, refused_at INTEGER, refusal TEXT, is_missing_at_source INTEGER NOT NULL DEFAULT 0
  CHECK (is_missing_at_source IN (0, 1)), grp_id INTEGER REFERENCES grp(id) ON DELETE RESTRICT) STRICT;

CREATE TABLE device (                                      -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,
  platform      TEXT NOT NULL,
  added_at      INTEGER NOT NULL,
  last_signed_in_at INTEGER NOT NULL,
  removed_at    INTEGER
, sound_offset_ms INTEGER NOT NULL DEFAULT 0
  CHECK (sound_offset_ms BETWEEN -2000 AND 2000)) STRICT;

CREATE TABLE device_reader_limit (                          -- D240 section 5
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE CASCADE,
  device_key   TEXT NOT NULL,             -- the node's name for it: 'zfs:tank', 'disk:sdc'
  reader_limit INTEGER NOT NULL CHECK (reader_limit BETWEEN 1 AND 64),
  set_at       INTEGER NOT NULL,
  PRIMARY KEY (node_id, device_key)
) WITHOUT ROWID, STRICT;

CREATE TABLE fetch_failure (                               -- class F
  item_id   INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  failure   TEXT NOT NULL,               -- what the provider or the job said
  failed_at INTEGER NOT NULL
) STRICT;

CREATE TABLE fetched_subtitle (                            -- class A: quota-limited, kept
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  video_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_file_id TEXT NOT NULL,
  language    TEXT NOT NULL,
  format      TEXT NOT NULL,
  body_gzip   BLOB NOT NULL,             -- a few kB; in the database so the backup carries it
  fetched_by_user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  fetched_at  INTEGER NOT NULL,
  release_name TEXT                      -- the provider's file name for it (D224)
) STRICT;

CREATE TABLE file_nfo (                                    -- class O: a rescan sends it again
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  body        BLOB NOT NULL              -- the NFO's bytes, as the node sent them (D182)
) STRICT;

CREATE TABLE file_probe (                                  -- class O: what the node probed (D59)
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  probe_json  TEXT NOT NULL,             -- the raw probe output, the input D59 replays
  duration_ms INTEGER NOT NULL DEFAULT 0 -- its format's duration; 0 when it gave none
) STRICT;

CREATE TABLE "grant" (                                   -- class A (D154)
  secret_sha256 BLOB PRIMARY KEY,        -- SHA-256 of the secret; never the secret
  -- issued_secret: a secret handed to a person to pass on, a registration invitation or a device
  -- code, which its scope names.
  kind        TEXT NOT NULL CHECK (kind IN ('session','link','cast','join','api_key',
                'adapter','node','relay','issued_secret')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL,
  parent_secret_sha256 BLOB REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,  -- D177: derived grants
  issued_at   INTEGER NOT NULL,
  expires_at  INTEGER,
  last_used_at INTEGER,
  revoked_at  INTEGER,
  device_name TEXT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT
, locate_hash BLOB, offer_key BLOB, link_sealed BLOB) WITHOUT ROWID, STRICT;

CREATE TABLE guessed_match (                               -- class F (D301)
  item_id     INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE CASCADE,
  guessed_at  INTEGER NOT NULL
) STRICT;

CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alternates, overview, content='item_text', content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);

CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, search_alternates, content='item_text', content_rowid='id', tokenize='trigram'
);

CREATE TABLE item_provider_id (                            -- class I; several per provider (ISRCs)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  PRIMARY KEY (item_id, provider, provider_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE "item_text" (                             -- class O/F, resolved (D45)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,  -- the full-text indexes' key for the row
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  language    TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT,
  -- class C: the name folded by the one normaliser in Go (diacritics removed, and the atomic
  -- letters no Unicode option decomposes, such as Ø and ß, spelled out), so "Bjork" finds
  -- "Bjørk" (D48).
  search_alternates TEXT, sort_title TEXT,
  UNIQUE (item_id, language)
) STRICT;

CREATE TABLE locator (                                     -- class A (discovery-core.md K7)
  session_secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  hash        BLOB NOT NULL UNIQUE,      -- SHA-256 of the locator; never the locator
  issued_at   INTEGER NOT NULL
) WITHOUT ROWID, STRICT;

CREATE TABLE loudness (                                    -- class O: what the node measured
  payload_hash    BLOB NOT NULL,
  stream_index    INTEGER NOT NULL,
  measured_mix    TEXT NOT NULL CHECK (measured_mix IN ('multichannel','stereo')),  -- as it is, or as stereo
  integrated_lufs REAL,                  -- -70 for silence; NULL when it failed
  range_lu        REAL,
  true_peak_dbtp  REAL,                  -- NULL for silence, which has no peak, or a failure
  failure         TEXT,                  -- FFmpeg's words when it could not be measured
  measured_at     INTEGER NOT NULL,
  PRIMARY KEY (payload_hash, stream_index, measured_mix)
) WITHOUT ROWID, STRICT;

CREATE TABLE node (                                        -- class I + O (the last three)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  enrolled_at INTEGER NOT NULL,
  removed_at  INTEGER) STRICT;

CREATE TABLE ordering (                                    -- D96, D159
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  sequence_id INTEGER NOT NULL REFERENCES "sequence"(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,           -- 'aired', 'dvd', 'absolute', a person's; 'release' for an album
  provider      TEXT, kind TEXT, source_id TEXT, label TEXT, listed_count INTEGER,                    -- NULL when user-authored
  UNIQUE (sequence_id, name)
) STRICT;

CREATE TABLE play_event (                                  -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('started','paused','seeked','completed','abandoned')),
  position_ms INTEGER NOT NULL,
  happened_at INTEGER NOT NULL,
  happened_mono_ms INTEGER
) STRICT;

CREATE TABLE play_position (                               -- class A (D81, D157)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  position_ms INTEGER NOT NULL,
  report_seq  INTEGER NOT NULL,          -- the report's number within its playback: a repeat is not new
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  updated_at  INTEGER NOT NULL,
  -- The moment of watching the position is for (D217 section 7): the device's monotonic time
  -- anchored to the server's clock when the report arrived (D165). The guard: a report about an
  -- earlier moment does not move it.
  position_read_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, file_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE play_span (                                   -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  from_position_ms INTEGER NOT NULL,
  to_position_ms   INTEGER NOT NULL,
  started_at  INTEGER NOT NULL,
  ended_at    INTEGER NOT NULL,
  is_unattended   INTEGER NOT NULL DEFAULT 0 CHECK (is_unattended IN (0,1))  -- after "still watching?" went unanswered (D188): never completes anything
) STRICT;

CREATE TABLE "item_hidden" (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, hidden_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID, STRICT;

CREATE TABLE provider_response (                           -- class F: the stored input (D170)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  part        TEXT NOT NULL DEFAULT '',  -- '' the item itself; 'season/N' a series' season listing
  fetched_at  INTEGER NOT NULL,          -- when body_gzip was fetched: the body's age
  last_asked_at INTEGER NOT NULL,        -- when the provider was last asked, with or without a new body
  body_gzip   BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
  PRIMARY KEY (item_id, provider, part)
) STRICT;

CREATE TABLE realm_key (                                   -- class A (D297)
  user_id       INTEGER PRIMARY KEY REFERENCES user(id) ON DELETE CASCADE,
  master_realm  TEXT NOT NULL,           -- the master realm the key was made for (proof.MasterRealm)
  key           BLOB NOT NULL,           -- proof.RealmKey, 32 bytes
  made_at       INTEGER NOT NULL
) STRICT;

CREATE TABLE refresh_request (                             -- class A
  item_id      INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  requested_at INTEGER NOT NULL,
  -- Whether to ask again for the item's images that were missing or refused.
  should_fetch_images INTEGER NOT NULL CHECK (should_fetch_images IN (0, 1))
) STRICT;

CREATE TABLE role (                                        -- class A (D130)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL UNIQUE,
  is_builtin    INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0,1)),
  capabilities  TEXT NOT NULL, revision INTEGER NOT NULL DEFAULT 0) STRICT;

CREATE TABLE role_library (                                -- class A (D243 section 4)
  role_id          INTEGER NOT NULL REFERENCES role(id) ON DELETE CASCADE,
  library_id       INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  may_rebroadcast  INTEGER NOT NULL DEFAULT 0 CHECK (may_rebroadcast IN (0,1)),
  PRIMARY KEY (role_id, library_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE secret (                                      -- class A, sealed (D194)
  scope       TEXT NOT NULL CHECK (scope IN ('server','user')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL for 'server'
  name        TEXT NOT NULL,             -- 'tmdb_api_key', 'opensubtitles_password', 'turn_secret'…
  key_id      INTEGER NOT NULL,          -- which secret.key sealed it (rotation)
  nonce       BLOB NOT NULL,             -- 24 bytes, XChaCha20-Poly1305
  ciphertext  BLOB NOT NULL,             -- ciphertext; the associated data is (scope, user_id, name)
  last_chars  TEXT,                      -- last four characters of an API key; NULL for a password
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  CHECK ((scope = 'server') = (user_id IS NULL))
) STRICT;

CREATE TABLE setting (                                     -- class A: the server's (D200)
  key         TEXT NOT NULL PRIMARY KEY, -- a name from the settings registry in code
  value_json  TEXT NOT NULL,             -- JSON
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL: the setup wizard
  set_at      INTEGER NOT NULL,
  -- Whether a person set it (D201 section 1): any manual set marks it, even to its starting
  -- value; "set to default" clears it; a release that changes a starting value moves every
  -- unmarked row to the new one.
  is_modified INTEGER NOT NULL DEFAULT 0 CHECK (is_modified IN (0, 1))
, revision INTEGER NOT NULL DEFAULT 0) STRICT;

CREATE TABLE stream (                                      -- class O; not "track" (D186)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  bytes_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT, -- where the bytes are
  video_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT, -- the video it serves
  stream_index INTEGER,                  -- NULL for a whole-file sidecar
  kind        TEXT NOT NULL CHECK (kind IN ('video','audio','subtitle','attachment')),
  codec       TEXT,
  container_language TEXT,               -- as the container carried it
  language    TEXT,                      -- BCP 47, normalised (D24)
  container_title TEXT,
  disposition_flags INTEGER NOT NULL DEFAULT 0,  -- FFmpeg's flags, as found
  channel_count INTEGER,
  UNIQUE (bytes_file, stream_index)
) STRICT;

CREATE TABLE subtitle_text (                               -- class O: what the node made
  source_hash  BLOB NOT NULL,
  stream_index INTEGER NOT NULL,         -- the track's stream index in that file; 0 for a fetched subtitle
  ass          BLOB,                     -- for JASSUB; NULL when it could not be made
  vtt          BLOB,                     -- for the cast receiver; NULL when it could not be made
  failure      TEXT,                     -- why it could not be made: FFmpeg's exit and stderr tail
  made_at      INTEGER NOT NULL,
  PRIMARY KEY (source_hash, stream_index),
  CHECK ((failure IS NULL) = (ass IS NOT NULL AND vtt IS NOT NULL))
) STRICT;

CREATE TABLE user (                                        -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  password_hash BLOB,
  created_at    INTEGER NOT NULL,
  removed_at    INTEGER,
  admin_note    TEXT                     -- who the account is for, the admin's alone (D246 section 2)
, admin_away TEXT NOT NULL DEFAULT 'never'
  CHECK (admin_away IN ('never', 'always')), revision INTEGER NOT NULL DEFAULT 0) STRICT;

CREATE TABLE video_timeline (                              -- class O: what the node read (D247 section 10)
  payload_hash BLOB NOT NULL,            -- the bytes it was read from, as file.payload_hash (D179)
  video_stream_index INTEGER NOT NULL,   -- the picture's stream, as ffprobe numbers every stream
  keyframes_ms_json TEXT NOT NULL DEFAULT '[]',  -- JSON: its keyframes' times in ms, ascending: where a chunk can begin
  bytes_in_each_second_json TEXT NOT NULL DEFAULT '[]',  -- JSON: its bytes in each second of the film, by the second
  failure     TEXT,                      -- FFmpeg's words when the file could not be read through; the JSON is then empty
  made_at     INTEGER NOT NULL,
  read_failure TEXT,                     -- FFmpeg's words when a full read failed after the index gave the keyframes; NULL otherwise
  PRIMARY KEY (payload_hash, video_stream_index)
) WITHOUT ROWID, STRICT;

CREATE TABLE purged_watching (                             -- class A (D306 section 8)
  user_id         INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  version_id      INTEGER NOT NULL REFERENCES version(id) ON DELETE RESTRICT,
  completed_count INTEGER NOT NULL CHECK (completed_count >= 0),  -- the 'completed' events of its deleted files
  watched_ms      INTEGER NOT NULL CHECK (watched_ms >= 0),       -- the time in their spans
  last_watched_at INTEGER NOT NULL,                               -- the latest of either
  PRIMARY KEY (user_id, version_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE invitation_link (                             -- class A (D318): one of an invitation's links
  secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  invitation_id INTEGER NOT NULL REFERENCES invitation(id) ON DELETE RESTRICT
) WITHOUT ROWID, STRICT;

CREATE TABLE unavailable_place (                           -- class O (D328): a place that cannot be read now
  node_id   INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path      TEXT NOT NULL,             -- the root, or the disk's mount point inside it; NFC, absolute
  root_path TEXT NOT NULL,             -- the root it is, or is inside
  since     INTEGER NOT NULL,          -- the first time in a row it was found so
  cause     TEXT NOT NULL CHECK (cause IN ('not_answering', 'unread', 'missing', 'not_named')),
  PRIMARY KEY (node_id, path)
) STRICT;

CREATE TABLE whole_file_hash (                             -- class C
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  sha256      BLOB,                      -- NULL when the node could not read it through
  size_bytes  INTEGER,                   -- the file's, when it was read
  modified_at INTEGER,
  failure     TEXT,                      -- why it could not be read through
  hashed_at   INTEGER NOT NULL
) STRICT;

CREATE TABLE "work_watched" (                                -- class A (D370)
  user_id    INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  work_id   INTEGER NOT NULL REFERENCES "work"(id) ON DELETE RESTRICT,
  -- When it became watched, by playing or by hand; NULL while it is not.
  watched_at INTEGER,
  -- When its person last marked it by hand, watched or unwatched; NULL for never. A position in one
  -- of its files watched at or before it is not where it resumes.
  marked_at  INTEGER,
  -- A row says something: watched, or marked unwatched.
  CHECK (watched_at IS NOT NULL OR marked_at IS NOT NULL),
  PRIMARY KEY (user_id, work_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE genre (                                        -- class F
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,                 -- as its first source spelled it
  name_key    TEXT NOT NULL UNIQUE           -- folded (D48): "Sci-Fi" from two sources is one
) STRICT;

CREATE TABLE item_genre (                                   -- class C, derived by the pass
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  genre_id    INTEGER NOT NULL REFERENCES genre(id) ON DELETE RESTRICT,
  ordinal     INTEGER NOT NULL CHECK (ordinal >= 0),  -- its place in its source's list
  PRIMARY KEY (item_id, genre_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE credit (                                       -- class C, derived by the pass
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  job        TEXT NOT NULL CHECK (job IN ('cast', 'director', 'writer')),
  ordinal     INTEGER NOT NULL CHECK (ordinal >= 0),  -- billing order within the role
  person_id   INTEGER NOT NULL REFERENCES person(id) ON DELETE RESTRICT,
  character   TEXT,                          -- a cast member's part; NULL for crew
  PRIMARY KEY (item_id, job, ordinal)
) WITHOUT ROWID, STRICT;

CREATE TABLE library (                                 -- class A
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('movie','series','other')),
  -- the three questions asked at creation (D184), never defaulted:
  metadata_source  TEXT NOT NULL CHECK (metadata_source IN ('nfo_then_tmdb','tmdb','nfo')),
  change_detection TEXT NOT NULL CHECK (change_detection IN ('watch','interval','manual')),
  check_every_hours INTEGER,             -- only for 'interval'
  is_writable      INTEGER NOT NULL CHECK (is_writable IN (0,1)),
  created_at       INTEGER NOT NULL,
  removed_at       INTEGER, revision INTEGER NOT NULL DEFAULT 0,
  -- Hours between checks belong to the interval and nothing else (D303, M5).
  CHECK ((change_detection = 'interval') = (check_every_hours IS NOT NULL)),
  -- An other library is never identified online: it reads NFO files alone (D349 section 10).
  CHECK (kind <> 'other' OR metadata_source = 'nfo')
) STRICT;

CREATE TABLE file_filing (                                 -- class O; the version is class I
  file_id      INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  library_kind TEXT NOT NULL CHECK (library_kind IN ('movie','series','other')),
  filing       TEXT NOT NULL CHECK (filing IN ('listed','extra','unidentified')),
  filing_reason TEXT,                    -- the rule verbatim, what was tried
  version_id   INTEGER REFERENCES version(id) ON DELETE RESTRICT,
  part_index   INTEGER,                  -- order within a multi-part Version
  identified_from_path TEXT,             -- the path it was identified from (D208 section 1)
  PRIMARY KEY (file_id, library_kind)
) STRICT;

CREATE TABLE other_video (                                 -- class C (D349 section 4)
  work_id      INTEGER PRIMARY KEY REFERENCES "work"(id) ON DELETE RESTRICT,
  recorded_at   INTEGER NOT NULL,          -- Unix milliseconds
  recorded_from TEXT NOT NULL CHECK (recorded_from IN ('tag','name','file','user')),
  file_name     TEXT NOT NULL,             -- its file's name, the last part of its path
  file_name_key BLOB NOT NULL              -- file_name's CLDR sort key in the sort_locale, BINARY order
) STRICT;

CREATE TABLE invitation (                                  -- class A (D246 section 2, D318)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  admin_note    TEXT NOT NULL CHECK (admin_note <> ''),
  issued_at     INTEGER NOT NULL,
  made_user_id  INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- the user its use made
  made_at       INTEGER,
  CHECK ((made_user_id IS NULL) = (made_at IS NULL))
) STRICT;

CREATE TABLE version (                                     -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  work_id    INTEGER NOT NULL REFERENCES "work"(id) ON DELETE RESTRICT,
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
) STRICT;

CREATE TABLE item (                                        -- class I, and O/F/A by column
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  -- What it is, in the API's words (D416, D426 section 2); a title, a collection or a person names
  -- it again, tied here by (id, kind).
  kind        TEXT NOT NULL CHECK (kind IN ('movie','series','episode','extra','other','person')),
  filing_library_id  INTEGER REFERENCES library(id) ON DELETE RESTRICT,
  merged_into INTEGER REFERENCES item(id) ON DELETE RESTRICT,  -- D173
  added_at    INTEGER NOT NULL,
  removed_at  INTEGER,
  -- For shows, the series is what is identified (D209): it carries the year its search is keyed on
  -- and the match state a film carries.
  year        INTEGER,                                    -- O: resolved
  -- The day it was released as yyyymmdd, of its year (D366, released.go); NULL undated.
  released_on INTEGER CHECK (released_on IS NULL OR (released_on >= 10000 AND released_on / 10000 = year)),  -- F/A (D172)
  followed_source    TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')),  -- F/A
  followed_source_id TEXT,                                                          -- F/A
  -- Every kind but a person is identified.
  identification_state TEXT CHECK (identification_state IN
                   ('identified','ambiguous','unidentified','confirmed')),
  -- When it was found, derived by the triggers below as item_found defines it (step 0058), class C:
  -- a work's first-found live file's first_seen_at and that file's modified_at, a series' newest
  -- episode's; 0 with none. A new item has none, so each starts at 0 and the triggers move it.
  found_at               INTEGER NOT NULL DEFAULT 0,
  found_file_modified_at INTEGER NOT NULL DEFAULT 0,
  CHECK (followed_source_id IS NULL OR followed_source IS NOT NULL),  -- an id is an id at a source
  CHECK ((kind = 'person') = (identification_state IS NULL)),
  UNIQUE (id, kind)                      -- a subtype row names its item and the item's kind (D303, M5)
) STRICT;

CREATE TABLE "work" (                                       -- mixed: see the columns
  id             INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  -- A title is a movie, an episode, an extra or an other video, as its item says (D426 section 2).
  kind           TEXT NOT NULL CHECK (kind IN ('movie','episode','extra','other')),
  runtime_ms     INTEGER,                                    -- O: resolved
  FOREIGN KEY (id, kind) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE RESTRICT
) STRICT;

CREATE TABLE "sequence" (                                  -- class A or O
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  -- A collection is a series, as its item says (D426 section 2); a box set returns with its feature.
  kind        TEXT NOT NULL CHECK (kind = 'series'),
  created_by  TEXT NOT NULL CHECK (created_by IN ('scanner','provider','user')),
  FOREIGN KEY (id, kind) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE RESTRICT
) STRICT;

CREATE TABLE person (                                       -- class F, an item of kind 'person'
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind = 'person'),       -- the pair names the item and its kind
  -- Their id at TMDB, whose person ids are numbered apart from its films', so kept here and not in
  -- item_provider_id; NULL for one only an NFO names.
  tmdb_id     TEXT,
  -- Their name as the one normaliser folds it, which finds the person an NFO names again; the
  -- name itself is the item's text, which the full-text indexes read (D48).
  name_key    TEXT NOT NULL,
  -- Their picture as their source names it, a TMDB profile path or an NFO's thumb; not fetched.
  picture_ref TEXT,
  FOREIGN KEY (id, kind) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE RESTRICT
) STRICT;

CREATE TABLE sort_key (                                     -- class C (D47)
  item_id       INTEGER NOT NULL,
  locale       TEXT NOT NULL,
  listing       TEXT NOT NULL,           -- the list it sorts in: its item's kind (audit 1)
  collation_key BLOB NOT NULL,           -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (item_id, locale),
  FOREIGN KEY (item_id, listing) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE CASCADE
) WITHOUT ROWID, STRICT;

CREATE TABLE link_memory (                                  -- class C (D257 section 1); persistent.db again since step 0073 (D427)
  -- Who measured it, "device 3" or a session with no device's "session <key>", and on which
  -- network, the address the core saw its request come from, as clientnet.Of names it.
  player           TEXT NOT NULL,
  network          TEXT NOT NULL,
  -- Its speed, kbit/s, and the spread of the natural logs of the samples it rested on; NULL while
  -- it has said only its steadiness.
  speed_kbps       INTEGER CHECK (speed_kbps > 0),
  speed_log_spread REAL CHECK (speed_log_spread >= 0),
  speed_at         INTEGER,
  -- Its steadiness level (D352 section 3); NULL while it has said only its speed.
  steadiness       INTEGER CHECK (steadiness BETWEEN 0 AND 3),
  steadiness_at    INTEGER,
  -- When it last said anything of the link, the later of the two: what a stale row is found by.
  said_at          INTEGER NOT NULL,
  CHECK ((speed_kbps IS NULL) = (speed_at IS NULL)),
  CHECK ((steadiness IS NULL) = (steadiness_at IS NULL)),
  CHECK (speed_at IS NOT NULL OR steadiness_at IS NOT NULL),
  PRIMARY KEY (player, network)
) WITHOUT ROWID, STRICT;

CREATE TABLE library_root (                                -- class O: as the node reports it (D107)
  library_id INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id    INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path  TEXT NOT NULL, retired_at INTEGER,
  PRIMARY KEY (library_id, node_id, root_path)
) WITHOUT ROWID, STRICT;

CREATE TABLE root_scan (                                   -- class O
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path    TEXT NOT NULL,
  completed_at INTEGER NOT NULL,         -- the last scan of this root that finished
  PRIMARY KEY (node_id, root_path)
) WITHOUT ROWID, STRICT;

CREATE TABLE root_state (                                  -- class O
  node_id           INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path         TEXT NOT NULL,
  filesystem_device INTEGER,             -- st_dev at the last complete scan
  is_mount_point    INTEGER CHECK (is_mount_point IN (0, 1)),
  last_scan_outcome TEXT NOT NULL CHECK (last_scan_outcome IN ('complete', 'unreadable', 'withheld')),
  last_scan_at      INTEGER NOT NULL,
  withheld_scan_started_at INTEGER,      -- the withheld scan's start: what it did not see goes
  withheld_reason   TEXT,                --   once the admin confirms
  are_removals_approved INTEGER NOT NULL DEFAULT 0
    CHECK (are_removals_approved IN (0, 1)),  -- class A: the admin said the files are gone
  PRIMARY KEY (node_id, root_path)
) WITHOUT ROWID, STRICT;

CREATE TABLE root_mount (                                  -- class O
  node_id           INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path         TEXT NOT NULL,
  mount_path        TEXT NOT NULL,       -- NFC, absolute on the node, like file.path
  filesystem_device INTEGER NOT NULL,    -- st_dev when it was last seen where expected
  is_present        INTEGER NOT NULL CHECK (is_present IN (0, 1)),
  PRIMARY KEY (node_id, mount_path)
) WITHOUT ROWID, STRICT;

CREATE TABLE field_override (                              -- class A (D183)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  field       TEXT NOT NULL,             -- a field-registry name
  language    TEXT NOT NULL DEFAULT '',  -- '' for language-invariant fields
  override_value TEXT NOT NULL,          -- a frozen copy
  origin      TEXT NOT NULL,             -- 'typed', 'nfo', 'tmdb', 'tvdb' — display only
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  revision    INTEGER NOT NULL DEFAULT 0,  -- raised by every write (sql.md M1)
  PRIMARY KEY (item_id, field, language)
) WITHOUT ROWID, STRICT;

CREATE TABLE extra_relation (                              -- class O, or A when asserted
  owner_work    INTEGER NOT NULL REFERENCES work(id) ON DELETE RESTRICT,
  extra_work    INTEGER NOT NULL REFERENCES work(id) ON DELETE RESTRICT,
  extra_type    TEXT NOT NULL CHECK (extra_type IN ('trailer','teaser','clip','featurette',
                  'behind_the_scenes','deleted_scene','interview','short','scene',
                  'theme_song','theme_video','unknown')),       -- D97, D162: eleven + unknown
  asserted_by   TEXT NOT NULL CHECK (asserted_by IN ('scanner','user')),
  PRIMARY KEY (owner_work, extra_work)
) WITHOUT ROWID, STRICT;

CREATE TABLE preference (                                  -- class A (D24, D57, D90, D103, D230, D428)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL CHECK (scope IN ('user','library','item','device','file')),
  scope_id    INTEGER NOT NULL DEFAULT 0,  -- 0 for 'user'; an item's is its film's or its series'
  axis        TEXT NOT NULL,               -- a file's 'subtitle_offset_ms', 'subtitle_used',
                                           -- 'audio_used'; an item's 'subtitle_language',
                                           -- 'audio_language', a series' 'episode_ordering';
                                           -- a user's own 'dialogue_boost', 'levelling'
  value_json  TEXT NOT NULL,               -- JSON; a switch is true or false, and unset is not false
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID, STRICT;

CREATE TABLE file (                                        -- class I (identity columns) + O (D426 section 4)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,  -- the id of the place it was first found at
  size_bytes    INTEGER,
  payload_hash  BLOB,                    -- D179: anchored on the media payload; NULL until it is read
  payload_bytes INTEGER,
  opensubtitles_hash TEXT,               -- 16 hex digits of OpenSubtitles' moviehash (D224); NULL until scanned
  sidecar_kind  TEXT CHECK (sidecar_kind IN ('nfo', 'subtitle', 'image'))  -- NULL: a media file
) STRICT;

CREATE TABLE place (                                       -- class I (identity columns) + O (D363 section 2)
  -- Its file's id where the file was first found; a confirmed copy keeps the id it was found with.
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id       INTEGER REFERENCES file(id) ON DELETE RESTRICT,  -- what is here
  node_id       INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path          TEXT NOT NULL,           -- NFC form, for display and search
  reported_path_raw BLOB NOT NULL,       -- class I: the bytes the node reported, the place's key (D211)
  filesystem_device INTEGER,
  inode         INTEGER,
  modified_at   INTEGER,
  first_seen_at INTEGER NOT NULL,
  last_seen_at  INTEGER NOT NULL,
  removed_at    INTEGER,                 -- tombstone: only on a positive report (D34, D179)
  filing        TEXT NOT NULL CHECK (filing IN ('read', 'skipped', 'errored')),
  filing_reason TEXT,                    -- the rule verbatim, the error code, what was tried
  error_detail_json TEXT,                -- owner, group, mode; the platform's text (D163)
  attached_to_place INTEGER REFERENCES place(id) ON DELETE RESTRICT, is_unconfirmed INTEGER NOT NULL DEFAULT 0 CHECK (is_unconfirmed IN (0, 1)),  -- a sidecar's video's place
  CHECK (file_id IS NOT NULL OR filing IN ('skipped', 'errored'))
) STRICT;

CREATE TABLE grp (                                         -- a season, or a disc (D426 section 5)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  number      INTEGER NOT NULL,          -- the season's number, or the disc's
  UNIQUE (ordering_id, number)
) STRICT;

CREATE TABLE "position" (                                -- season/episode, or disc/track
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  work_id    INTEGER NOT NULL REFERENCES "work"(id) ON DELETE RESTRICT,
  grp_id      INTEGER REFERENCES grp(id) ON DELETE RESTRICT,  -- its season, or disc; NULL: none
  number_in_group INTEGER,               -- episode, or track
  sort_order  REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, work_id)
) STRICT;

CREATE TABLE "group_text" (                              -- a season's name, a disc's, a part's
  grp_id      INTEGER NOT NULL REFERENCES grp(id) ON DELETE RESTRICT,
  language    TEXT NOT NULL,
  name        TEXT,
  PRIMARY KEY (grp_id, language)
) WITHOUT ROWID, STRICT;

CREATE TABLE system_library (                              -- class A: the server's own (D435)
  kind       TEXT NOT NULL PRIMARY KEY CHECK (kind IN ('system')),
  library_id INTEGER NOT NULL UNIQUE REFERENCES library(id) ON DELETE RESTRICT
) WITHOUT ROWID, STRICT;

CREATE TABLE node_unconfined_leave (                       -- class A: an admin's acceptance (D450)
  node_id             INTEGER NOT NULL PRIMARY KEY REFERENCES node(id) ON DELETE RESTRICT,
  accepted_by_user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  accepted_at         INTEGER NOT NULL
) STRICT;

-- The two built-in roles (cut-a section 1): every capability on admin; the user role may watch
-- together and reach the server from outside the house (D178).
INSERT INTO role (name, is_builtin, capabilities) VALUES
  ('admin', 1, 'administer,deleteMedia,download,editMetadata,hostWatchTogether,remoteAccess'),
  ('user', 1, 'hostWatchTogether,remoteAccess');

-- The views.

CREATE VIEW library_place AS
SELECT library_root.library_id, library.kind AS library_kind, library_root.node_id, library_root.root_path,
  place.id AS place_id, place.file_id, place.path, place.reported_path_raw, place.filing, place.first_seen_at
FROM place
JOIN library_root ON library_root.node_id = place.node_id AND library_root.retired_at IS NULL
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
WHERE place.removed_at IS NULL
  AND place.reported_path_raw > CAST(CASE WHEN library_root.root_path = '/' THEN '/'
    ELSE library_root.root_path || '/' END AS BLOB)
  AND place.reported_path_raw < CAST(CASE WHEN library_root.root_path = '/' THEN '0'
    ELSE library_root.root_path || '0' END AS BLOB);

CREATE VIEW series_member AS
SELECT ordering.sequence_id, position.work_id AS member_work, position.sort_order
FROM ordering
JOIN position ON position.ordering_id = ordering.id
WHERE ordering.name = 'aired';

CREATE VIEW file_place AS
SELECT place.file_id, place.id AS place_id
FROM place
WHERE place.removed_at IS NULL AND place.id = (
  SELECT first.id FROM place AS first
  WHERE first.file_id = place.file_id AND first.removed_at IS NULL
  ORDER BY first.id <> first.file_id, first.first_seen_at, first.id LIMIT 1);

CREATE VIEW item_found AS
SELECT item.id AS item_id,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_at FROM series_member CROSS JOIN item AS member ON member.id = series_member.member_work
    WHERE series_member.sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT (SELECT min(found.first_seen_at) FROM place AS found WHERE found.file_id = file_place.file_id) AS first_found
    FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file_place ON file_place.file_id = file_filing.file_id
    CROSS JOIN place ON place.id = file_place.place_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
    ORDER BY first_found, coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_at,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_file_modified_at FROM series_member
    CROSS JOIN item AS member ON member.id = series_member.member_work
    WHERE series_member.sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT coalesce(place.modified_at, 0)
    FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file_place ON file_place.file_id = file_filing.file_id
    CROSS JOIN place ON place.id = file_place.place_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
    ORDER BY (SELECT min(found.first_seen_at) FROM place AS found WHERE found.file_id = file_place.file_id),
      coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_file_modified_at
FROM item;

CREATE VIEW version_library AS
SELECT file_filing.version_id, file_filing.file_id, library_place.place_id, library_place.library_id
FROM file_filing
JOIN library_place ON library_place.file_id = file_filing.file_id
  AND library_place.library_kind = file_filing.library_kind
WHERE file_filing.version_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM system_library WHERE system_library.library_id = library_place.library_id);

-- The indexes.

CREATE UNIQUE INDEX artwork_chosen ON artwork(item_id, kind) WHERE is_chosen = 1;

CREATE INDEX artwork_item ON artwork(item_id, kind);

CREATE INDEX artwork_source ON artwork(source_sha256) WHERE source_sha256 IS NOT NULL;

CREATE INDEX device_owner ON device(user_id) WHERE removed_at IS NULL;

CREATE UNIQUE INDEX fetched_subtitle_once ON fetched_subtitle(video_file, provider, provider_file_id);

CREATE UNIQUE INDEX grant_locate_hash ON "grant"(locate_hash) WHERE locate_hash IS NOT NULL;

CREATE INDEX grant_user ON "grant"(user_id) WHERE revoked_at IS NULL;

CREATE INDEX item_provider_lookup ON item_provider_id(provider, provider_id);

CREATE UNIQUE INDEX item_text_original ON item_text(item_id) WHERE is_original = 1;

CREATE INDEX play_event_user_file ON play_event(user_id, file_id, happened_at);

CREATE INDEX play_span_file ON play_span(file_id);

CREATE INDEX play_span_user ON play_span(user_id, started_at);

CREATE INDEX realm_key_master ON realm_key(master_realm);

CREATE INDEX role_library_library ON role_library(library_id);

CREATE UNIQUE INDEX secret_name ON secret(scope, ifnull(user_id, 0), name);

CREATE INDEX stream_media ON stream(video_file, kind);

CREATE UNIQUE INDEX user_live_name ON user(name) WHERE removed_at IS NULL;

CREATE INDEX play_event_device ON play_event(device_id);

CREATE INDEX play_position_device ON play_position(device_id);

CREATE INDEX play_span_device ON play_span(device_id);

CREATE INDEX grant_device ON "grant"(device_id);

CREATE INDEX grant_parent ON "grant"(parent_secret_sha256);

CREATE INDEX purged_watching_version ON purged_watching(version_id);

CREATE INDEX play_position_file ON play_position(file_id);

CREATE INDEX play_event_file ON play_event(file_id);

CREATE INDEX invitation_link_invitation ON invitation_link(invitation_id);

CREATE INDEX unavailable_place_root ON unavailable_place(node_id, root_path);

CREATE INDEX item_genre_genre ON item_genre(genre_id, item_id);

CREATE INDEX credit_person ON credit(person_id, item_id);

CREATE INDEX file_filing_version ON file_filing(version_id) WHERE version_id IS NOT NULL;

CREATE INDEX other_video_file_name ON other_video(file_name_key, work_id);

CREATE INDEX other_video_recorded ON other_video(recorded_at, work_id);

CREATE INDEX invitation_made_user ON invitation(made_user_id) WHERE made_user_id IS NOT NULL;

CREATE INDEX invitation_role ON invitation(role_id);

CREATE INDEX item_filing_library ON item(filing_library_id, kind) WHERE removed_at IS NULL;

CREATE INDEX item_kind_found ON item(kind, found_at, found_file_modified_at, id);

CREATE INDEX item_kind_released ON item(kind, released_on);

CREATE INDEX item_identification ON item(identification_state, followed_source, kind);

CREATE INDEX item_ambiguous ON item(identification_state, kind) WHERE identification_state = 'ambiguous';

CREATE INDEX item_waiting ON item(identification_state, followed_source, kind)
  WHERE identification_state = 'unidentified' AND followed_source = 'none' AND kind != 'episode';

CREATE INDEX person_name_key ON person(name_key);

CREATE UNIQUE INDEX person_tmdb ON person(tmdb_id) WHERE tmdb_id IS NOT NULL;

CREATE INDEX sort_key_listing ON sort_key(locale, listing, collation_key, item_id);

CREATE INDEX sort_key_item ON sort_key(item_id, listing);

CREATE INDEX link_memory_said ON link_memory(said_at);

CREATE INDEX version_work ON version(work_id);

CREATE INDEX library_root_node ON library_root(node_id, root_path) WHERE retired_at IS NULL;

CREATE INDEX preference_scope ON preference(scope, scope_id);

CREATE INDEX file_payload ON file(payload_hash) WHERE payload_hash IS NOT NULL;

CREATE INDEX place_file ON place(file_id);

CREATE INDEX place_attached ON place(attached_to_place);

CREATE INDEX place_errored ON place(filing, node_id, path) WHERE filing = 'errored' AND removed_at IS NULL;

CREATE UNIQUE INDEX place_live_path ON place(node_id, reported_path_raw) WHERE removed_at IS NULL;

CREATE INDEX place_removed_at ON place(removed_at) WHERE removed_at IS NOT NULL;

CREATE INDEX position_order ON position(ordering_id, sort_order);

CREATE INDEX position_work ON position(work_id);

CREATE UNIQUE INDEX node_live_name ON node(name) WHERE removed_at IS NULL;

-- The triggers.

CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.id, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.id, old.name, old.search_alternates);
END;

CREATE TRIGGER item_text_ai AFTER INSERT ON item_text BEGIN
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.id, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.id, new.name, new.search_alternates);
END;

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

CREATE TRIGGER file_filing_found_ad AFTER DELETE ON file_filing WHEN old.version_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT version.work_id FROM version WHERE version.id = old.version_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER file_filing_found_ai AFTER INSERT ON file_filing WHEN new.version_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT version.work_id FROM version WHERE version.id = new.version_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER file_filing_found_au AFTER UPDATE OF version_id ON file_filing
WHEN old.version_id IS NOT new.version_id BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (SELECT version.work_id FROM version WHERE version.id IN (old.version_id, new.version_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER version_found_ai AFTER INSERT ON version BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = new.work_id
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER version_found_au AFTER UPDATE OF work_id, removed_at ON version
WHEN old.work_id IS NOT new.work_id OR old.removed_at IS NOT new.removed_at BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (old.work_id, new.work_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER item_found_au AFTER UPDATE OF found_at, found_file_modified_at ON item
WHEN old.found_at IS NOT new.found_at OR old.found_file_modified_at IS NOT new.found_file_modified_at BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (SELECT series_member.sequence_id FROM series_member WHERE series_member.member_work = new.id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER position_found_ai AFTER INSERT ON position BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT ordering.sequence_id FROM ordering WHERE ordering.id = new.ordering_id AND ordering.name = 'aired')
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER position_found_ad AFTER DELETE ON position BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT ordering.sequence_id FROM ordering WHERE ordering.id = old.ordering_id AND ordering.name = 'aired')
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER position_found_au AFTER UPDATE OF ordering_id, work_id ON position
WHEN old.ordering_id IS NOT new.ordering_id OR old.work_id IS NOT new.work_id BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT ordering.sequence_id FROM ordering
      WHERE ordering.id IN (old.ordering_id, new.ordering_id) AND ordering.name = 'aired')
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER place_found_ai AFTER INSERT ON place WHEN new.file_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.work_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id IN (new.file_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER place_found_ad AFTER DELETE ON place WHEN old.file_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.work_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id IN (old.file_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER place_found_au AFTER UPDATE OF removed_at, modified_at, file_id ON place
WHEN old.removed_at IS NOT new.removed_at OR old.modified_at IS NOT new.modified_at
  OR old.file_id IS NOT new.file_id BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.work_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id IN (old.file_id, new.file_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
