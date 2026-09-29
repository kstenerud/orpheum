-- application_id 1447646258
-- user_version 23
-- compatible_version 23
-- sqlite_sequence device 0
-- sqlite_sequence role 2

-- index artwork_chosen
CREATE UNIQUE INDEX artwork_chosen ON artwork(item_id, kind) WHERE is_chosen = 1;

-- index artwork_item
CREATE INDEX artwork_item ON artwork(item_id, kind);

-- index device_owner
CREATE INDEX device_owner ON device(user_id) WHERE removed_at IS NULL;

-- index fetched_subtitle_video
CREATE INDEX fetched_subtitle_video ON fetched_subtitle(video_file);

-- index file_live_path
CREATE UNIQUE INDEX file_live_path ON file(node_id, reported_path_bytes) WHERE removed_at IS NULL;

-- index file_payload
CREATE INDEX file_payload ON file(payload_hash) WHERE removed_at IS NULL;

-- index file_version
CREATE INDEX file_version ON file(version_id) WHERE version_id IS NOT NULL;

-- index grant_user
CREATE INDEX grant_user ON "grant"(user_id) WHERE revoked_at IS NULL;

-- index item_library
CREATE INDEX item_library ON item(library_id, kind) WHERE removed_at IS NULL;

-- index item_provider_lookup
CREATE INDEX item_provider_lookup ON item_provider_id(provider, provider_id);

-- index item_text_original
CREATE UNIQUE INDEX item_text_original ON item_text(item_id) WHERE is_original = 1;

-- index play_event_user_file
CREATE INDEX play_event_user_file ON play_event(user_id, file_id, happened_at);

-- index play_span_file
CREATE INDEX play_span_file ON play_span(file_id);

-- index play_span_user
CREATE INDEX play_span_user ON play_span(user_id, started_at);

-- index position_order
CREATE INDEX position_order ON position(ordering_id, sort_order);

-- index position_title
CREATE INDEX position_title ON position(title_id);

-- index secret_name
CREATE UNIQUE INDEX secret_name ON secret(scope, ifnull(user_id, 0), name);

-- index setting_key
CREATE UNIQUE INDEX setting_key ON setting(scope, ifnull(user_id, 0), key);

-- index stream_media
CREATE INDEX stream_media ON stream(video_file, kind);

-- index title_ambiguous
CREATE INDEX title_ambiguous ON title(id) WHERE identification = 'ambiguous';

-- index user_live_name
CREATE UNIQUE INDEX user_live_name ON user(name) WHERE removed_at IS NULL;

-- index version_title
CREATE INDEX version_title ON version(title_id) WHERE removed_at IS NULL;

-- table artwork
CREATE TABLE artwork (                                     -- class O/F; A when chosen
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('poster','backdrop','logo','banner','thumb','disc')),
  lang        TEXT,
  source      TEXT NOT NULL,             -- 'nfo', 'tmdb', 'embedded', 'user'
  source_ref  TEXT,
  content_hash BLOB,                     -- the derived sizes live in cache/ under this
  is_chosen      INTEGER NOT NULL DEFAULT 0 CHECK (is_chosen IN (0,1)),  -- a person picked it
  removed_at  INTEGER
);

-- table collection
CREATE TABLE collection (                                  -- class A or O
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('series','box_set','user')),
  membership  TEXT NOT NULL CHECK (membership IN ('explicit','predicate','external')),
  predicate   TEXT,                      -- D75's serialised form, for 'predicate'
  created_by  TEXT NOT NULL CHECK (created_by IN ('scanner','provider','user'))
, year INTEGER, followed_source TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')), followed_source_id TEXT, identification TEXT CHECK (identification IN
  ('identified','ambiguous','unidentified','confirmed')));

-- table collection_member
CREATE TABLE collection_member (                           -- D159, D160
  collection_id     INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  member_title      INTEGER REFERENCES title(id) ON DELETE RESTRICT,
  member_collection INTEGER REFERENCES collection(id) ON DELETE RESTRICT,
  position          REAL,                -- NULL: a member without a place (Machete order)
  CHECK ((member_title IS NULL) <> (member_collection IS NULL)),
  UNIQUE (collection_id, member_title),
  UNIQUE (collection_id, member_collection)
);

-- table device
CREATE TABLE "device" (                                  -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,
  platform      TEXT NOT NULL,
  audio_output  TEXT CHECK (audio_output IN ('built_in_speakers','receiver','headphones')),
                                         -- NULL until asked on first playback (D184)
  first_seen_at    INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL,
  removed_at    INTEGER
);

-- table edition
CREATE TABLE edition (                                     -- class A when a person made it
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  created_by  TEXT NOT NULL CHECK (created_by IN ('provider','user')),  -- D158
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
);

-- table edition_text
CREATE TABLE edition_text (                                -- class A or O
  edition_id  INTEGER NOT NULL REFERENCES edition(id) ON DELETE RESTRICT,
  lang        TEXT NOT NULL,
  name        TEXT NOT NULL,
  PRIMARY KEY (edition_id, lang)
);

-- table extra_relation
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

-- table favourite
CREATE TABLE favourite (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, added_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;

-- table fetched_subtitle
CREATE TABLE fetched_subtitle (                            -- class A: quota-limited, kept
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  video_file  INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_file_id TEXT NOT NULL,
  lang        TEXT NOT NULL,
  format      TEXT NOT NULL,
  body_gzip        BLOB NOT NULL,             -- a few kB; in the database so the backup carries it
  fetched_by  INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  fetched_at  INTEGER NOT NULL
, release_name TEXT);

-- table field_override
CREATE TABLE field_override (                              -- class A (D183)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  field       TEXT NOT NULL,             -- a field-registry name
  lang        TEXT NOT NULL DEFAULT '',  -- '' for language-invariant fields
  override_value       TEXT NOT NULL,             -- a frozen copy
  origin      TEXT NOT NULL,             -- 'typed', 'nfo', 'tmdb', 'tvdb' — display only
  set_by_user_id      INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (item_id, field, lang)
);

-- table file
CREATE TABLE file (                                        -- class I (identity columns) + O
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  library_id    INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id       INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path          TEXT NOT NULL,           -- NFC form
  size_bytes          INTEGER,
  mtime_ms         INTEGER,
  payload_hash  BLOB,                    -- D179: anchored on the media payload
  payload_bytes   INTEGER,
  filing        TEXT NOT NULL CHECK (filing IN
                  ('listed','extra','attached','skipped','errored','unidentified')),
  filing_reason TEXT,                    -- the rule verbatim, the error code, what was tried
  error_detail  TEXT,                    -- owner, group, mode; the platform's text (D163)
  probe_json         TEXT,                    -- raw probe output, JSON; the input D59 replays
  version_id    INTEGER REFERENCES version(id) ON DELETE RESTRICT,
  part_index    INTEGER,                 -- order within a multi-part Version
  attached_to_file   INTEGER REFERENCES file(id) ON DELETE RESTRICT,  -- a sidecar's video
  first_seen_at    INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL,
  removed_at    INTEGER                  -- tombstone: only on a positive report (D34, D179)
, nfo_body BLOB, identified_from_path TEXT, reported_path_bytes BLOB, opensubtitles_hash TEXT, is_identity_confirmed INTEGER NOT NULL DEFAULT 1
  CHECK (is_identity_confirmed IN (0, 1)), sidecar_kind TEXT CHECK (sidecar_kind IN ('nfo', 'subtitle', 'image')));

-- table grant
CREATE TABLE "grant" (                                     -- class A (D154)
  secret_sha256        BLOB PRIMARY KEY,          -- SHA-256 of the secret; never the secret
  kind        TEXT NOT NULL CHECK (kind IN ('session','link','cast','join','api_key',
                'adapter','node','relay','invite')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL,
  parent_secret_sha256      BLOB REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,  -- D177: derived grants
  issued_at   INTEGER NOT NULL,
  expires_at  INTEGER,
  last_seen_at   INTEGER,
  revoked_at  INTEGER,
  device_name       TEXT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT
) WITHOUT ROWID;

-- table group_text
CREATE TABLE group_text (                                  -- a season's name, a disc's subtitle
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  group_number         INTEGER NOT NULL,
  lang        TEXT NOT NULL,
  name        TEXT,
  overview    TEXT,
  PRIMARY KEY (collection_id, group_number, lang)
);

-- table item
CREATE TABLE item (                                        -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('title','collection','person')),
  library_id  INTEGER REFERENCES library(id) ON DELETE RESTRICT,
  origin_server      TEXT,                      -- NULL = this server; else who gave it (D174)
  merged_into INTEGER REFERENCES item(id) ON DELETE RESTRICT,  -- D173
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
);

-- table item_fts
CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alternates, overview, content='item_text', content_rowid='rowid',
  tokenize='unicode61 remove_diacritics 2'
);

-- table item_fts_config
CREATE TABLE 'item_fts_config'(k PRIMARY KEY, v) WITHOUT ROWID;

-- table item_fts_data
CREATE TABLE 'item_fts_data'(id INTEGER PRIMARY KEY, block BLOB);

-- table item_fts_docsize
CREATE TABLE 'item_fts_docsize'(id INTEGER PRIMARY KEY, sz BLOB);

-- table item_fts_idx
CREATE TABLE 'item_fts_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID;

-- table item_fts_tri
CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, search_alternates, content='item_text', content_rowid='rowid', tokenize='trigram'
);

-- table item_fts_tri_config
CREATE TABLE 'item_fts_tri_config'(k PRIMARY KEY, v) WITHOUT ROWID;

-- table item_fts_tri_data
CREATE TABLE 'item_fts_tri_data'(id INTEGER PRIMARY KEY, block BLOB);

-- table item_fts_tri_docsize
CREATE TABLE 'item_fts_tri_docsize'(id INTEGER PRIMARY KEY, sz BLOB);

-- table item_fts_tri_idx
CREATE TABLE 'item_fts_tri_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID;

-- table item_provider_id
CREATE TABLE item_provider_id (                            -- class I; several per provider (ISRCs)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  PRIMARY KEY (item_id, provider, provider_id)
) WITHOUT ROWID;

-- table item_tag
CREATE TABLE item_tag (
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  tag_id      INTEGER NOT NULL REFERENCES tag(id) ON DELETE RESTRICT,
  PRIMARY KEY (tag_id, item_id)
) WITHOUT ROWID;

-- table item_text
CREATE TABLE item_text (                                   -- class O/F, resolved (D45)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  lang        TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT, search_alternates TEXT,
  PRIMARY KEY (item_id, lang)
);

-- table library
CREATE TABLE library (                                     -- class A
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('movies','shows','mixed')),
  -- the three questions asked at creation (D184), never defaulted:
  metadata_source  TEXT NOT NULL CHECK (metadata_source IN ('nfo_then_tmdb','tmdb','nfo')),
  change_detection TEXT NOT NULL CHECK (change_detection IN ('watch','interval','manual')),
  check_every_h    INTEGER,          -- only for 'interval'
  is_writable         INTEGER NOT NULL CHECK (is_writable IN (0,1)),
  created_at       INTEGER NOT NULL,
  removed_at       INTEGER
);

-- table library_root
CREATE TABLE library_root (                                -- class O: as the node reports it (D107)
  library_id INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id    INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path       TEXT NOT NULL,
  PRIMARY KEY (library_id, node_id, path)
);

-- table loudness
CREATE TABLE "loudness" (                                -- class O: what the node measured
  payload_hash    BLOB NOT NULL,
  stream_index    INTEGER NOT NULL,
  measured_mix    TEXT NOT NULL CHECK (measured_mix IN ('multichannel','stereo')),  -- as it is, or as stereo
  integrated_lufs REAL,                  -- -70 for silence; NULL when it failed
  range_lu        REAL,
  true_peak_dbtp  REAL,                  -- NULL for silence, which has no peak, or a failure
  failure         TEXT,                  -- FFmpeg's words when it could not be measured
  measured_at     INTEGER NOT NULL,
  PRIMARY KEY (payload_hash, stream_index, measured_mix)
) WITHOUT ROWID;

-- table node
CREATE TABLE node (                                        -- class I + O (the last three)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  enrolled_at INTEGER NOT NULL,
  removed_at  INTEGER,
  last_ingested_report_seq  INTEGER NOT NULL DEFAULT 0,  -- last journal sequence the core ingested (D59)
  last_status_json      TEXT,                        -- the node's last status record, JSON
  last_status_at   INTEGER                      -- when it arrived; liveness is judged from this
);

-- table ordering
CREATE TABLE ordering (                                    -- D96, D159
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,           -- 'aired', 'dvd', 'absolute', a person's; 'release' for an album
  provider      TEXT,                    -- NULL when user-authored
  UNIQUE (collection_id, name)
);

-- table play_event
CREATE TABLE play_event (                                  -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('started','paused','seeked','completed','abandoned')),
  position_ms INTEGER NOT NULL,
  happened_at   INTEGER NOT NULL,
  client_mono_ms INTEGER
);

-- table play_position
CREATE TABLE play_position (                               -- class A (D81, D157)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  position_ms INTEGER NOT NULL,
  report_seq         INTEGER NOT NULL,          -- the guard: an older report does not move it
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  updated_at  INTEGER NOT NULL, position_read_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, file_id)
) WITHOUT ROWID;

-- table play_span
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

-- table position
CREATE TABLE position (                                    -- season/episode, or disc/track
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  group_number         INTEGER,                   -- season, or disc
  number_in_group      INTEGER,                   -- episode, or track
  sort_order        REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, title_id)
);

-- table preference
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

-- table progress_hide
CREATE TABLE progress_hide (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, hidden_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;

-- table provider_response
CREATE TABLE "provider_response" (                       -- class F: the stored input (D170)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  part        TEXT NOT NULL DEFAULT '',  -- '' the item itself; 'season/N' a series' season listing
  fetched_at  INTEGER NOT NULL,
  body_gzip        BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
  PRIMARY KEY (item_id, provider, part)
);

-- table role
CREATE TABLE role (                                        -- class A (D130)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL UNIQUE,
  is_builtin       INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0,1)),
  capabilities  TEXT NOT NULL,           -- a set from the closed enum, e.g. 'administer,download'
  predicate     TEXT                     -- NULL = sees everything; invalid = sees nothing (D164)
);

-- table root_mount
CREATE TABLE root_mount (                                  -- class O
  node_id  INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path     TEXT NOT NULL,
  mount_path     TEXT NOT NULL,                -- NFC, absolute on the node, like file.path
  filesystem_device   INTEGER NOT NULL,             -- st_dev when it was last seen where expected
  is_present  INTEGER NOT NULL CHECK (is_present IN (0, 1)),
  PRIMARY KEY (node_id, mount_path)
);

-- table root_scan
CREATE TABLE root_scan (                                   -- class O
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path         TEXT NOT NULL,
  completed_at INTEGER NOT NULL,         -- the last scan of this root that finished
  PRIMARY KEY (node_id, root_path)
);

-- table root_state
CREATE TABLE root_state (                                  -- class O
  node_id         INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path            TEXT NOT NULL,
  filesystem_device          INTEGER,                -- st_dev at the last complete scan
  is_mount_point     INTEGER CHECK (is_mount_point IN (0, 1)),
  outcome         TEXT NOT NULL CHECK (outcome IN ('complete', 'unreadable', 'withheld')),
  outcome_at      INTEGER NOT NULL,
  withheld_scan_started_at  INTEGER,                -- the withheld scan's start: what it did not see goes
  withheld_reason TEXT, are_removals_approved INTEGER NOT NULL DEFAULT 0
  CHECK (are_removals_approved IN (0, 1)),                   -- once the admin confirms
  PRIMARY KEY (node_id, root_path)
);

-- table schema_meta
CREATE TABLE schema_meta (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  compatible_version INTEGER NOT NULL  -- oldest schema whose code can still use this file
);

-- table schema_step
CREATE TABLE schema_step (                                 -- class I: the runner's own record
  version INTEGER PRIMARY KEY,
  sha256  TEXT NOT NULL                  -- hex, of the step file's bytes
);

-- table secret
CREATE TABLE secret (                                      -- class A, sealed (D194)
  scope       TEXT NOT NULL CHECK (scope IN ('server','user')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL for 'server'
  name        TEXT NOT NULL,             -- 'tmdb_api_key', 'opensubtitles_password', 'turn_secret'…
  key_id      INTEGER NOT NULL,          -- which secret.key sealed it (rotation)
  nonce       BLOB NOT NULL,             -- 24 bytes, XChaCha20-Poly1305
  ciphertext      BLOB NOT NULL,             -- ciphertext; the associated data is (scope, user_id, name)
  hint        TEXT,                      -- last four characters of an API key; NULL for a password
  set_by_user_id      INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  CHECK ((scope = 'server') = (user_id IS NULL))
);

-- table setting
CREATE TABLE setting (                                     -- class A
  scope       TEXT NOT NULL CHECK (scope IN ('server','user')),
  user_id     INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL for 'server'
  key         TEXT NOT NULL,             -- a name from the settings registry in code
  value_json       TEXT NOT NULL,             -- JSON
  set_by_user_id      INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL: the setup wizard
  set_at      INTEGER NOT NULL, is_modified INTEGER NOT NULL DEFAULT 0 CHECK (is_modified IN (0, 1)),
  CHECK ((scope = 'server') = (user_id IS NULL))
);

-- table sort_key
CREATE TABLE sort_key (                                    -- class C (D47)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
  locale      TEXT NOT NULL,
  listing     TEXT NOT NULL,             -- the list it sorts in: 'movie', 'series', 'album'…
  collation_key         BLOB NOT NULL,             -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (locale, listing, collation_key, item_id)
) WITHOUT ROWID;

-- table sqlite_sequence
CREATE TABLE sqlite_sequence(name,seq);

-- table stream
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
  disposition_flags INTEGER NOT NULL DEFAULT 0,  -- FFmpeg's flags, as found
  channel_count    INTEGER,
  UNIQUE (bytes_file, stream_index)
);

-- table tag
CREATE TABLE tag (                                         -- class A (D164)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL UNIQUE
);

-- table title
CREATE TABLE title (                                       -- mixed: see the columns
  id             INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  kind           TEXT NOT NULL CHECK (kind IN ('movie','episode','extra')),
  media_kind     TEXT NOT NULL DEFAULT 'video' CHECK (media_kind IN ('video','audio')),
  year           INTEGER,                                    -- O: resolved
  runtime_ms     INTEGER,                                    -- O: resolved
  original_lang  TEXT,                                       -- O: resolved
  -- D170/D183: the one source this title follows, and its id there. F, or A once a person
  -- chose it: a match is a fetch, and a migration never redoes one.
  followed_source         TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')),       -- F/A
  followed_source_id      TEXT,                                                      -- F/A
  identification          TEXT NOT NULL CHECK (identification IN
                   ('identified','ambiguous','unidentified','confirmed'))    -- F/A (D172)
);

-- table user
CREATE TABLE user (                                        -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  password_hash BLOB,
  is_outsider      INTEGER NOT NULL DEFAULT 0 CHECK (is_outsider IN (0,1)),  -- D174: sees no household
  created_at    INTEGER NOT NULL,
  removed_at    INTEGER
);

-- table user_deny
CREATE TABLE user_deny (                                   -- class A: narrows only
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  PRIMARY KEY (user_id, item_id)
) WITHOUT ROWID;

-- table user_rating
CREATE TABLE user_rating (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, rating INTEGER NOT NULL,
  rated_at INTEGER NOT NULL, PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;

-- table version
CREATE TABLE version (                                     -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  edition_id  INTEGER REFERENCES edition(id) ON DELETE RESTRICT,  -- NULL: cut unknown (D155)
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
);

-- table watchlist
CREATE TABLE watchlist (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, added_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID;

-- trigger item_text_ad
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.rowid, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.rowid, old.name, old.search_alternates);
END;

-- trigger item_text_ai
CREATE TRIGGER item_text_ai AFTER INSERT ON item_text BEGIN
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.rowid, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.rowid, new.name, new.search_alternates);
END;

-- trigger item_text_au
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

-- view live_file
CREATE VIEW live_file AS SELECT * FROM file WHERE removed_at IS NULL;

-- view live_item
CREATE VIEW live_item AS SELECT * FROM item WHERE removed_at IS NULL;
