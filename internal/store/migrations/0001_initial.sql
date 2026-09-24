-- SPDX-License-Identifier: GPL-3.0-or-later
-- orpheum:raises-compatible-version
-- The first schema: library.db as docs/contributors/design/plans/schema.md writes it (D185),
-- copied verbatim from its SQL blocks. cache.db's tables arrive with the job queue in M2 (D198).
-- The runner owns application_id, user_version and compatible_version; this file holds DDL and
-- the two built-in roles only.

CREATE TABLE schema_meta (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  compatible_version INTEGER NOT NULL  -- oldest schema whose code can still use this file
);
-- PRAGMA user_version is the schema version, set in each step's transaction (D181).

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

CREATE TABLE root_scan (                                   -- class O
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path         TEXT NOT NULL,
  completed_at INTEGER NOT NULL,         -- the last scan of this root that finished
  PRIMARY KEY (node_id, path)
);

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

CREATE TABLE stream (                                      -- class O; not "track" (D186)
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
CREATE INDEX stream_media ON stream(media_file, kind);

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
  ident          TEXT NOT NULL CHECK (ident IN
                   ('identified','ambiguous','unidentified','confirmed'))    -- F/A (D172)
);
CREATE INDEX title_ambiguous ON title(id) WHERE ident = 'ambiguous';

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

CREATE TABLE item_text (                                   -- class O/F, resolved (D45)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  lang        TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT,
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
  name, overview, content='item_text', content_rowid='rowid',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, content='item_text', content_rowid='rowid', tokenize='trigram'
);

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
  source      TEXT NOT NULL,             -- 'nfo', 'tmdb', 'embedded', 'user'
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

CREATE TABLE collection (                                  -- class A or O
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('series','box_set','user')),
  membership  TEXT NOT NULL CHECK (membership IN ('explicit','predicate','external')),
  predicate   TEXT,                      -- D75's serialised form, for 'predicate'
  created_by  TEXT NOT NULL CHECK (created_by IN ('scanner','provider','user'))
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
  grp         INTEGER,                   -- season, or disc
  number      INTEGER,                   -- episode, or track
  sort        REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, title_id)
);
CREATE INDEX position_sort ON position(ordering_id, sort);
CREATE INDEX position_title ON position(title_id);
CREATE TABLE group_text (                                  -- a season's name, a disc's subtitle
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  grp         INTEGER NOT NULL,
  lang        TEXT NOT NULL,
  name        TEXT,
  overview    TEXT,
  PRIMARY KEY (collection_id, grp, lang)
);

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

CREATE TABLE secret (                                      -- class A, sealed (D194)
  scope       TEXT NOT NULL CHECK (scope IN ('server','user')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL for 'server'
  name        TEXT NOT NULL,             -- 'tmdb_api_key', 'opensubtitles_password', 'turn_secret'…
  key_id      INTEGER NOT NULL,          -- which secret.key sealed it (rotation)
  nonce       BLOB NOT NULL,             -- 24 bytes, XChaCha20-Poly1305
  sealed      BLOB NOT NULL,             -- ciphertext; the associated data is (scope, user_id, name)
  hint        TEXT,                      -- last four characters of an API key; NULL for a password
  set_by      INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  CHECK ((scope = 'server') = (user_id IS NULL))
);
CREATE UNIQUE INDEX secret_name ON secret(scope, ifnull(user_id, 0), name);

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
  unterminated INTEGER NOT NULL DEFAULT 0 CHECK (unterminated IN (0,1)),
  unattended  INTEGER NOT NULL DEFAULT 0 CHECK (unattended IN (0,1))  -- after "still watching?" went unanswered (D188): never completes anything
);
CREATE INDEX play_span_user ON play_span(user_id, t_from);
CREATE INDEX play_span_file ON play_span(file_id);

CREATE TABLE favourite (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;
CREATE TABLE user_rating (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, rating INTEGER NOT NULL,
  at INTEGER NOT NULL, PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;
CREATE TABLE watchlist (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;
-- Hidden from Continue Watching / Next Up (D188). Not a completion; it lapses by itself when
-- the item has activity after `at`, so there is nothing to clear.
CREATE TABLE progress_hide (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, at INTEGER NOT NULL,
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

CREATE TABLE provider_response (                           -- class F: the stored input (D170)
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  fetched_at  INTEGER NOT NULL,
  body        BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
  PRIMARY KEY (title_id, provider)
);

-- The two built-in roles Cut A has (cut-a §1: no roles beyond admin and user). Every
-- capability is on the admin role; the user role may watch together and reach the server from
-- outside the house, which is what Cut A's remote person does (D178).
INSERT INTO role (name, builtin, capabilities, predicate) VALUES
  ('admin', 1, 'administer,delete_media,download,edit_metadata,host_watch_together,remote_access', NULL),
  ('user', 1, 'host_watch_together,remote_access', NULL);
