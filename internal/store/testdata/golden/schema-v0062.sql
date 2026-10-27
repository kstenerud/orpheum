-- application_id 1447646258
-- user_version 62
-- compatible_version 48
-- sqlite_sequence invitation 0
-- sqlite_sequence library 0
-- sqlite_sequence role 2

-- index artwork_chosen
CREATE UNIQUE INDEX artwork_chosen ON artwork(item_id, kind) WHERE is_chosen = 1;

-- index artwork_item
CREATE INDEX artwork_item ON artwork(item_id, kind);

-- index artwork_source
CREATE INDEX artwork_source ON artwork(source_sha256) WHERE source_sha256 IS NOT NULL;

-- index collection_member_title
CREATE INDEX collection_member_title ON collection_member(member_title) WHERE member_title IS NOT NULL;

-- index collection_series_found
CREATE INDEX collection_series_found ON collection(found_at, found_file_modified_at) WHERE kind = 'series';

-- index collection_series_released
CREATE INDEX collection_series_released ON collection(released_on) WHERE kind = 'series';

-- index device_owner
CREATE INDEX device_owner ON device(user_id) WHERE removed_at IS NULL;

-- index fetched_subtitle_once
CREATE UNIQUE INDEX fetched_subtitle_once ON fetched_subtitle(video_file, provider, provider_file_id);

-- index fetched_subtitle_video
CREATE INDEX fetched_subtitle_video ON fetched_subtitle(video_file);

-- index file_attached
CREATE INDEX file_attached ON file(attached_to_file);

-- index file_copy
CREATE INDEX file_copy ON file(copy_of) WHERE copy_of IS NOT NULL;

-- index file_errored
CREATE INDEX file_errored ON file(filing, node_id, path) WHERE filing = 'errored' AND removed_at IS NULL;

-- index file_filing_version
CREATE INDEX file_filing_version ON file_filing(version_id) WHERE version_id IS NOT NULL;

-- index file_live_path
CREATE UNIQUE INDEX file_live_path ON file(node_id, reported_path_raw) WHERE removed_at IS NULL;

-- index file_payload
CREATE INDEX file_payload ON file(payload_hash) WHERE removed_at IS NULL;

-- index file_removed_at
CREATE INDEX file_removed_at ON file(removed_at) WHERE removed_at IS NOT NULL;

-- index file_removed_payload
CREATE INDEX file_removed_payload ON file(node_id, payload_hash, removed_at) WHERE removed_at IS NOT NULL;

-- index grant_device
CREATE INDEX grant_device ON "grant"(device_id);

-- index grant_locate_hash
CREATE UNIQUE INDEX grant_locate_hash ON "grant"(locate_hash) WHERE locate_hash IS NOT NULL;

-- index grant_parent
CREATE INDEX grant_parent ON "grant"(parent_secret_sha256);

-- index grant_user
CREATE INDEX grant_user ON "grant"(user_id) WHERE revoked_at IS NULL;

-- index invitation_link_invitation
CREATE INDEX invitation_link_invitation ON invitation_link(invitation_id);

-- index invitation_made_user
CREATE INDEX invitation_made_user ON invitation(made_user_id) WHERE made_user_id IS NOT NULL;

-- index invitation_role
CREATE INDEX invitation_role ON invitation(role_id);

-- index item_filing_library
CREATE INDEX item_filing_library ON item(filing_library_id, kind) WHERE removed_at IS NULL;

-- index item_provider_lookup
CREATE INDEX item_provider_lookup ON item_provider_id(provider, provider_id);

-- index item_text_original
CREATE UNIQUE INDEX item_text_original ON item_text(item_id) WHERE is_original = 1;

-- index library_root_node
CREATE INDEX library_root_node ON library_root(node_id, root_path) WHERE retired_at IS NULL;

-- index other_video_file_name
CREATE INDEX other_video_file_name ON other_video(file_name_key, title_id);

-- index other_video_recorded
CREATE INDEX other_video_recorded ON other_video(recorded_at, title_id);

-- index play_event_device
CREATE INDEX play_event_device ON play_event(device_id);

-- index play_event_file
CREATE INDEX play_event_file ON play_event(file_id);

-- index play_event_user_file
CREATE INDEX play_event_user_file ON play_event(user_id, file_id, happened_at);

-- index play_position_device
CREATE INDEX play_position_device ON play_position(device_id);

-- index play_position_file
CREATE INDEX play_position_file ON play_position(file_id);

-- index play_span_device
CREATE INDEX play_span_device ON play_span(device_id);

-- index play_span_file
CREATE INDEX play_span_file ON play_span(file_id);

-- index play_span_user
CREATE INDEX play_span_user ON play_span(user_id, started_at);

-- index position_order
CREATE INDEX position_order ON position(ordering_id, sort_order);

-- index position_title
CREATE INDEX position_title ON position(title_id);

-- index preference_scope
CREATE INDEX preference_scope ON preference(scope, scope_id);

-- index purged_watching_version
CREATE INDEX purged_watching_version ON purged_watching(version_id);

-- index realm_key_master
CREATE INDEX realm_key_master ON realm_key(master_realm);

-- index role_library_library
CREATE INDEX role_library_library ON role_library(library_id);

-- index secret_name
CREATE UNIQUE INDEX secret_name ON secret(scope, ifnull(user_id, 0), name);

-- index sort_key_listing
CREATE INDEX sort_key_listing ON sort_key(locale, listing, collation_key, item_id);

-- index stream_media
CREATE INDEX stream_media ON stream(video_file, kind);

-- index title_ambiguous
CREATE INDEX title_ambiguous ON title(id) WHERE identification_state = 'ambiguous';

-- index title_film_found
CREATE INDEX title_film_found ON title(found_at, found_file_modified_at) WHERE kind = 'movie';

-- index title_film_released
CREATE INDEX title_film_released ON title(released_on) WHERE kind = 'movie';

-- index title_other_found
CREATE INDEX title_other_found ON title(found_at, found_file_modified_at) WHERE kind = 'other';

-- index title_waiting
CREATE INDEX title_waiting ON title(id)
  WHERE identification_state = 'unidentified' AND followed_source = 'none' AND kind != 'episode';

-- index unavailable_place_root
CREATE INDEX unavailable_place_root ON unavailable_place(node_id, root_path);

-- index user_live_name
CREATE UNIQUE INDEX user_live_name ON user(name) WHERE removed_at IS NULL;

-- index version_title
CREATE INDEX version_title ON version(title_id);

-- table artwork
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
  CHECK (is_missing_at_source IN (0, 1))) STRICT;

-- table collection
CREATE TABLE collection (                                  -- class A or O
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  -- A collection is an item of kind 'collection' (D303, M5): the pair names the item and its kind.
  item_kind   TEXT NOT NULL DEFAULT 'collection' CHECK (item_kind = 'collection'),
  kind        TEXT NOT NULL CHECK (kind IN ('series','box_set','user')),
  membership  TEXT NOT NULL CHECK (membership IN ('explicit','predicate','external')),
  predicate   TEXT,                      -- D75's serialised form, for 'predicate'
  created_by  TEXT NOT NULL CHECK (created_by IN ('scanner','provider','user')),
  -- For shows, the series is what is identified (D209): it carries the year its search is keyed
  -- on and the match state a title carries.
  year        INTEGER,                                                        -- O: resolved
  followed_source    TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')),  -- F/A
  followed_source_id TEXT,                                                    -- F/A
  identification_state TEXT CHECK (identification_state IN
    ('identified','ambiguous','unidentified','confirmed')), found_at INTEGER NOT NULL DEFAULT 0, found_file_modified_at INTEGER NOT NULL DEFAULT 0, released_on INTEGER CHECK (released_on IS NULL OR released_on >= 10000),                   -- F/A (D172)
  CHECK (followed_source_id IS NULL OR followed_source IS NOT NULL),  -- an id is an id at a source
  FOREIGN KEY (id, item_kind) REFERENCES item(id, kind) ON DELETE RESTRICT
) STRICT;

-- table collection_member
CREATE TABLE collection_member (                           -- D159, D160
  collection_id     INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  member_title      INTEGER REFERENCES title(id) ON DELETE RESTRICT,
  member_collection INTEGER REFERENCES collection(id) ON DELETE RESTRICT,
  position          REAL,                -- NULL: a member without a place (Machete order)
  CHECK ((member_title IS NULL) <> (member_collection IS NULL)),
  UNIQUE (collection_id, member_title),
  UNIQUE (collection_id, member_collection)
) STRICT;

-- table device
CREATE TABLE device (                                      -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,
  platform      TEXT NOT NULL,
  audio_output  TEXT CHECK (audio_output IN ('built_in_speakers','receiver','headphones')),
                                         -- NULL until asked on first playback (D184)
  added_at      INTEGER NOT NULL,
  last_signed_in_at INTEGER NOT NULL,
  removed_at    INTEGER
, sound_offset_ms INTEGER NOT NULL DEFAULT 0
  CHECK (sound_offset_ms BETWEEN -2000 AND 2000)) STRICT;

-- table device_reader_limit
CREATE TABLE device_reader_limit (                          -- D240 section 5
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE CASCADE,
  device_key   TEXT NOT NULL,             -- the node's name for it: 'zfs:tank', 'disk:sdc'
  reader_limit INTEGER NOT NULL CHECK (reader_limit BETWEEN 1 AND 64),
  set_at       INTEGER NOT NULL,
  PRIMARY KEY (node_id, device_key)
) WITHOUT ROWID, STRICT;

-- table edition
CREATE TABLE edition (                                     -- class A when a person made it
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  created_by  TEXT NOT NULL CHECK (created_by IN ('provider','user')),  -- D158
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
) STRICT;

-- table edition_text
CREATE TABLE edition_text (                                -- class A or O
  edition_id  INTEGER NOT NULL REFERENCES edition(id) ON DELETE RESTRICT,
  language    TEXT NOT NULL,
  name        TEXT NOT NULL,
  PRIMARY KEY (edition_id, language)
) STRICT;

-- table extra_relation
CREATE TABLE extra_relation (                              -- class O, or A when asserted
  owner_title   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  owner_edition INTEGER REFERENCES edition(id) ON DELETE RESTRICT,  -- only a person sets it
  extra_title   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  extra_type    TEXT NOT NULL CHECK (extra_type IN ('trailer','teaser','clip','featurette',
                  'behind_the_scenes','deleted_scene','interview','short','scene',
                  'theme_song','theme_video','unknown')),       -- D97, D162: eleven + unknown
  position      INTEGER,
  asserted_by   TEXT NOT NULL CHECK (asserted_by IN ('scanner','user')),
  PRIMARY KEY (owner_title, extra_title)
) STRICT;

-- table favourite
CREATE TABLE favourite (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, added_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID, STRICT;

-- table fetch_failure
CREATE TABLE fetch_failure (                               -- class F
  item_id   INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  failure   TEXT NOT NULL,               -- what the provider or the job said
  failed_at INTEGER NOT NULL
) STRICT;

-- table fetched_subtitle
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

-- table field_override
CREATE TABLE field_override (                              -- class A (D183)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  field       TEXT NOT NULL,             -- a field-registry name
  language    TEXT NOT NULL DEFAULT '',  -- '' for language-invariant fields
  override_value TEXT NOT NULL,          -- a frozen copy
  origin      TEXT NOT NULL,             -- 'typed', 'nfo', 'tmdb', 'tvdb' — display only
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (item_id, field, language)
) STRICT;

-- table file
CREATE TABLE file (                                        -- class I (identity columns) + O
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id       INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path          TEXT NOT NULL,           -- NFC form, for display and search
  size_bytes    INTEGER,
  modified_at   INTEGER,
  payload_hash  BLOB,                    -- D179: anchored on the media payload
  payload_bytes INTEGER,
  filing_reason TEXT,                    -- the rule verbatim, the error code, what was tried
  error_detail_json TEXT,                -- owner, group, mode; the platform's text (D163)
  attached_to_file INTEGER REFERENCES file(id) ON DELETE RESTRICT,  -- a sidecar's video
  first_seen_at INTEGER NOT NULL,
  last_seen_at  INTEGER NOT NULL,
  removed_at    INTEGER,                 -- tombstone: only on a positive report (D34, D179)
  -- An NFO's bytes, as the node sent them: identification runs in the core, which never opens a
  -- file, and re-derives a title from stored input only (D182). A subtitle's are read when played.
  reported_path_raw BLOB,              -- class I: the bytes the node reported, the file's key (D211)
  opensubtitles_hash TEXT,               -- 16 hex digits of OpenSubtitles' moviehash (D224); NULL until scanned
  is_identity_confirmed INTEGER NOT NULL DEFAULT 1
    CHECK (is_identity_confirmed IN (0, 1)),  -- D179: 0 after a rebuild, until re-seen
  sidecar_kind  TEXT CHECK (sidecar_kind IN ('nfo', 'subtitle', 'image'))
, filesystem_device INTEGER, inode INTEGER, filing TEXT NOT NULL DEFAULT 'video'
  CHECK (filing IN ('video','copy','attached','skipped','errored')), copy_of INTEGER REFERENCES file(id) ON DELETE RESTRICT) STRICT;

-- table file_filing
CREATE TABLE file_filing (                                 -- class O; the version is class I
  file_id      INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  library_kind TEXT NOT NULL CHECK (library_kind IN ('movies','shows','other')),
  filing       TEXT NOT NULL CHECK (filing IN ('listed','extra','unidentified')),
  filing_reason TEXT,                    -- the rule verbatim, what was tried
  version_id   INTEGER REFERENCES version(id) ON DELETE RESTRICT,
  part_index   INTEGER,                  -- order within a multi-part Version
  identified_from_path TEXT,             -- the path it was identified from (D208 section 1)
  PRIMARY KEY (file_id, library_kind)
) STRICT;

-- table file_nfo
CREATE TABLE file_nfo (                                    -- class O: a rescan sends it again
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  body        BLOB NOT NULL              -- the NFO's bytes, as the node sent them (D182)
) STRICT;

-- table file_probe
CREATE TABLE file_probe (                                  -- class O: what the node probed (D59)
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  probe_json  TEXT NOT NULL,             -- the raw probe output, the input D59 replays
  duration_ms INTEGER NOT NULL DEFAULT 0 -- its format's duration; 0 when it gave none
) STRICT;

-- table grant
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

-- table group_text
CREATE TABLE "group_text" (                              -- a season's name, a disc's, a part's
  ordering_id   INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  group_number  INTEGER NOT NULL,
  language      TEXT NOT NULL,
  name          TEXT,
  overview      TEXT,
  PRIMARY KEY (ordering_id, group_number, language)
) STRICT;

-- table guessed_match
CREATE TABLE guessed_match (                               -- class F (D301)
  item_id     INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE CASCADE,
  guessed_at  INTEGER NOT NULL
) STRICT;

-- table invitation
CREATE TABLE "invitation" (                              -- class A (D246 section 2, D318)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  admin_note    TEXT NOT NULL CHECK (admin_note <> ''),
  issued_at     INTEGER NOT NULL,
  made_user_id  INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- the account its use made
  made_at       INTEGER,
  CHECK ((made_user_id IS NULL) = (made_at IS NULL))
) STRICT;

-- table invitation_link
CREATE TABLE invitation_link (                             -- class A (D318): one of an invitation's links
  secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  invitation_id INTEGER NOT NULL REFERENCES invitation(id) ON DELETE RESTRICT
) WITHOUT ROWID, STRICT;

-- table item
CREATE TABLE item (                                        -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('title','collection','person')),
  filing_library_id  INTEGER REFERENCES library(id) ON DELETE RESTRICT,
  origin_server TEXT,                    -- NULL = this server; else who gave it (D174)
  merged_into INTEGER REFERENCES item(id) ON DELETE RESTRICT,  -- D173
  added_at    INTEGER NOT NULL,
  removed_at  INTEGER,
  UNIQUE (id, kind)                      -- a subtype row names its item and the item's kind (D303, M5)
) STRICT;

-- table item_fts
CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alternates, overview, content='item_text', content_rowid='id',
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
  name, search_alternates, content='item_text', content_rowid='id', tokenize='trigram'
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
) WITHOUT ROWID, STRICT;

-- table item_tag
CREATE TABLE item_tag (
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  tag_id      INTEGER NOT NULL REFERENCES tag(id) ON DELETE RESTRICT,
  PRIMARY KEY (tag_id, item_id)
) WITHOUT ROWID, STRICT;

-- table item_text
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
  search_alternates TEXT,
  UNIQUE (item_id, language)
) STRICT;

-- table library
CREATE TABLE "library" (                                 -- class A
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('movies','shows','other')),
  -- the three questions asked at creation (D184), never defaulted:
  metadata_source  TEXT NOT NULL CHECK (metadata_source IN ('nfo_then_tmdb','tmdb','nfo')),
  change_detection TEXT NOT NULL CHECK (change_detection IN ('watch','interval','manual')),
  check_every_hours INTEGER,             -- only for 'interval'
  is_writable      INTEGER NOT NULL CHECK (is_writable IN (0,1)),
  created_at       INTEGER NOT NULL,
  removed_at       INTEGER,
  -- Hours between checks belong to the interval and nothing else (D303, M5).
  CHECK ((change_detection = 'interval') = (check_every_hours IS NOT NULL)),
  -- An other library is never identified online: it reads NFO files alone (D349 section 10).
  CHECK (kind <> 'other' OR metadata_source = 'nfo')
) STRICT;

-- table library_root
CREATE TABLE library_root (                                -- class O: as the node reports it (D107)
  library_id INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id    INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path  TEXT NOT NULL, retired_at INTEGER,
  PRIMARY KEY (library_id, node_id, root_path)
) STRICT;

-- table locator
CREATE TABLE locator (                                     -- class A (discovery-core.md K7)
  session_secret_sha256 BLOB PRIMARY KEY REFERENCES "grant"(secret_sha256) ON DELETE RESTRICT,
  hash        BLOB NOT NULL UNIQUE,      -- SHA-256 of the locator; never the locator
  issued_at   INTEGER NOT NULL
) WITHOUT ROWID, STRICT;

-- table loudness
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

-- table node
CREATE TABLE node (                                        -- class I + O (the last three)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  enrolled_at INTEGER NOT NULL,
  removed_at  INTEGER) STRICT;

-- table ordering
CREATE TABLE ordering (                                    -- D96, D159
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  collection_id INTEGER NOT NULL REFERENCES collection(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,           -- 'aired', 'dvd', 'absolute', a person's; 'release' for an album
  provider      TEXT, kind TEXT, source_id TEXT, label TEXT, listed_count INTEGER,                    -- NULL when user-authored
  UNIQUE (collection_id, name)
) STRICT;

-- table other_video
CREATE TABLE other_video (                                 -- class C (D349 section 4)
  title_id      INTEGER PRIMARY KEY REFERENCES title(id) ON DELETE RESTRICT,
  recorded_at   INTEGER NOT NULL,          -- Unix milliseconds
  recorded_from TEXT NOT NULL CHECK (recorded_from IN ('tag','name','file','person')),
  file_name     TEXT NOT NULL,             -- its file's name, the last part of its path
  file_name_key BLOB NOT NULL              -- file_name's CLDR sort key in the sort_locale, BINARY order
) STRICT;

-- table play_event
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

-- table play_position
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

-- table play_span
CREATE TABLE play_span (                                   -- class A (D165)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  device_id   INTEGER REFERENCES device(id) ON DELETE RESTRICT,
  from_position_ms INTEGER NOT NULL,
  to_position_ms   INTEGER NOT NULL,
  started_at  INTEGER NOT NULL,
  ended_at    INTEGER NOT NULL,
  is_unterminated INTEGER NOT NULL DEFAULT 0 CHECK (is_unterminated IN (0,1)),
  is_unattended   INTEGER NOT NULL DEFAULT 0 CHECK (is_unattended IN (0,1))  -- after "still watching?" went unanswered (D188): never completes anything
) STRICT;

-- table position
CREATE TABLE position (                                    -- season/episode, or disc/track
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  group_number INTEGER,                  -- season, or disc
  number_in_group INTEGER,               -- episode, or track
  sort_order  REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, title_id)
) STRICT;

-- table preference
CREATE TABLE preference (                                  -- class A (D24, D57, D90, D103, D230)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL CHECK (scope IN ('user','library','work','device','file')),
  scope_id    INTEGER NOT NULL DEFAULT 0,  -- 0 for 'user'; a work's is its series' or its title's item
  axis        TEXT NOT NULL,               -- a file's 'subtitle_offset_ms', 'subtitle_used',
                                           -- 'audio_used'; a work's 'subtitle_language',
                                           -- 'audio_language'; a person's own 'dialogue_boost',
                                           -- 'levelling'
  value_json  TEXT NOT NULL,               -- JSON; a switch is true or false, and unset is not false
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID, STRICT;

-- table progress_hide
CREATE TABLE progress_hide (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, hidden_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID, STRICT;

-- table provider_response
CREATE TABLE provider_response (                           -- class F: the stored input (D170)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  part        TEXT NOT NULL DEFAULT '',  -- '' the item itself; 'season/N' a series' season listing
  fetched_at  INTEGER NOT NULL,          -- when body_gzip was fetched: the body's age
  last_asked_at INTEGER NOT NULL,        -- when the provider was last asked, with or without a new body
  body_gzip   BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
  PRIMARY KEY (item_id, provider, part)
) STRICT;

-- table purged_watching
CREATE TABLE purged_watching (                             -- class A (D306 section 8)
  user_id         INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  version_id      INTEGER NOT NULL REFERENCES version(id) ON DELETE RESTRICT,
  completed_count INTEGER NOT NULL CHECK (completed_count >= 0),  -- the 'completed' events of its deleted files
  watched_ms      INTEGER NOT NULL CHECK (watched_ms >= 0),       -- the time in their spans
  last_watched_at INTEGER NOT NULL,                               -- the latest of either
  PRIMARY KEY (user_id, version_id)
) WITHOUT ROWID, STRICT;

-- table realm_key
CREATE TABLE realm_key (                                   -- class A (D297)
  user_id       INTEGER PRIMARY KEY REFERENCES user(id) ON DELETE CASCADE,
  master_realm  TEXT NOT NULL,           -- the master realm the key was made for (proof.MasterRealm)
  key           BLOB NOT NULL,           -- proof.RealmKey, 32 bytes
  made_at       INTEGER NOT NULL
) STRICT;

-- table refresh_request
CREATE TABLE refresh_request (                             -- class A
  item_id      INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  requested_at INTEGER NOT NULL,
  -- Whether to ask again for the item's images that were missing or refused.
  should_fetch_images INTEGER NOT NULL CHECK (should_fetch_images IN (0, 1))
) STRICT;

-- table role
CREATE TABLE role (                                        -- class A (D130)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL UNIQUE,
  is_builtin    INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0,1)),
  capabilities  TEXT NOT NULL,           -- a set from the closed enum, e.g. 'administer,download'
  predicate     TEXT                     -- NULL = sees everything; invalid = sees nothing (D164)
) STRICT;

-- table role_library
CREATE TABLE role_library (                                -- class A (D243 section 4)
  role_id          INTEGER NOT NULL REFERENCES role(id) ON DELETE CASCADE,
  library_id       INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  may_rebroadcast  INTEGER NOT NULL DEFAULT 0 CHECK (may_rebroadcast IN (0,1)),
  PRIMARY KEY (role_id, library_id)
) WITHOUT ROWID, STRICT;

-- table root_mount
CREATE TABLE root_mount (                                  -- class O
  node_id           INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path         TEXT NOT NULL,
  mount_path        TEXT NOT NULL,       -- NFC, absolute on the node, like file.path
  filesystem_device INTEGER NOT NULL,    -- st_dev when it was last seen where expected
  is_present        INTEGER NOT NULL CHECK (is_present IN (0, 1)),
  PRIMARY KEY (node_id, mount_path)
) STRICT;

-- table root_scan
CREATE TABLE root_scan (                                   -- class O
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path    TEXT NOT NULL,
  completed_at INTEGER NOT NULL,         -- the last scan of this root that finished
  PRIMARY KEY (node_id, root_path)
) STRICT;

-- table root_state
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
) STRICT;

-- table schema_meta
CREATE TABLE schema_meta (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  compatible_version INTEGER NOT NULL  -- oldest schema whose code can still use this file
) STRICT;

-- table schema_step
CREATE TABLE schema_step (                                 -- class I: the runner's own record
  version INTEGER PRIMARY KEY,
  sha256  TEXT NOT NULL                  -- hex, of the step file's bytes
) STRICT;

-- table secret
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

-- table setting
CREATE TABLE setting (                                     -- class A: the server's (D200)
  key         TEXT NOT NULL PRIMARY KEY, -- a name from the settings registry in code
  value_json  TEXT NOT NULL,             -- JSON
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL: the setup wizard
  set_at      INTEGER NOT NULL,
  -- Whether a person set it (D201 section 1): any manual set marks it, even to its starting
  -- value; "set to default" clears it; a release that changes a starting value moves every
  -- unmarked row to the new one.
  is_modified INTEGER NOT NULL DEFAULT 0 CHECK (is_modified IN (0, 1))
) STRICT;

-- table sort_key
CREATE TABLE "sort_key" (                            -- class C (D47)
  item_id       INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
  locale        TEXT NOT NULL,
  listing       TEXT NOT NULL,           -- the list it sorts in: 'movie', 'series', 'album'…
  collation_key BLOB NOT NULL,           -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (item_id, locale)
) WITHOUT ROWID, STRICT;

-- table sqlite_sequence
CREATE TABLE sqlite_sequence(name,seq);

-- table stream
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

-- table subtitle_text
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

-- table tag
CREATE TABLE tag (                                         -- class A (D164)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL UNIQUE
) STRICT;

-- table title
CREATE TABLE title (                                       -- mixed: see the columns
  id             INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  -- A title is an item of kind 'title' (D303, M5): the pair names the item and its kind.
  item_kind      TEXT NOT NULL DEFAULT 'title' CHECK (item_kind = 'title'),
  media_kind     TEXT NOT NULL DEFAULT 'video' CHECK (media_kind IN ('video','audio')),
  year           INTEGER,                                    -- O: resolved
  runtime_ms     INTEGER,                                    -- O: resolved
  original_language TEXT,                                    -- O: resolved
  -- D170/D183: the one source this title follows, and its id there. F, or A once a person
  -- chose it: a match is a fetch, and a migration never redoes one.
  followed_source    TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')),  -- F/A
  followed_source_id TEXT,                                                          -- F/A
  identification_state TEXT NOT NULL CHECK (identification_state IN
                   ('identified','ambiguous','unidentified','confirmed')), found_at INTEGER NOT NULL DEFAULT 0, found_file_modified_at INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL DEFAULT 'movie'
  CHECK (kind IN ('movie','episode','extra','other')), released_on INTEGER CHECK (released_on IS NULL OR released_on >= 10000),   -- F/A (D172)
  CHECK (followed_source_id IS NULL OR followed_source IS NOT NULL),  -- an id is an id at a source
  FOREIGN KEY (id, item_kind) REFERENCES item(id, kind) ON DELETE RESTRICT
) STRICT;

-- table title_watched
CREATE TABLE title_watched (                                -- class A (D370)
  user_id    INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  title_id   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  -- When it became watched, by playing or by hand; NULL while it is not.
  watched_at INTEGER,
  -- When its person last marked it by hand, watched or unwatched; NULL for never. A position in one
  -- of its files watched at or before it is not where it resumes.
  marked_at  INTEGER,
  -- A row says something: watched, or marked unwatched.
  CHECK (watched_at IS NOT NULL OR marked_at IS NOT NULL),
  PRIMARY KEY (user_id, title_id)
) WITHOUT ROWID, STRICT;

-- table unavailable_place
CREATE TABLE unavailable_place (                           -- class O (D328): a place that cannot be read now
  node_id   INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path      TEXT NOT NULL,             -- the root, or the disk's mount point inside it; NFC, absolute
  root_path TEXT NOT NULL,             -- the root it is, or is inside
  since     INTEGER NOT NULL,          -- the first time in a row it was found so
  cause     TEXT NOT NULL CHECK (cause IN ('not_answering', 'unread', 'missing', 'not_named')),
  PRIMARY KEY (node_id, path)
) STRICT;

-- table user
CREATE TABLE user (                                        -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  password_hash BLOB,
  created_at    INTEGER NOT NULL,
  removed_at    INTEGER,
  admin_note    TEXT                     -- who the account is for, the admin's alone (D246 section 2)
, admin_away TEXT NOT NULL DEFAULT 'never'
  CHECK (admin_away IN ('never', 'always'))) STRICT;

-- table user_deny
CREATE TABLE user_deny (                                   -- class A: narrows only
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  PRIMARY KEY (user_id, item_id)
) WITHOUT ROWID, STRICT;

-- table user_rating
CREATE TABLE user_rating (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, rating INTEGER NOT NULL,
  rated_at INTEGER NOT NULL, PRIMARY KEY (user_id, item_id)) WITHOUT ROWID, STRICT;

-- table version
CREATE TABLE version (                                     -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  edition_id  INTEGER REFERENCES edition(id) ON DELETE RESTRICT,  -- NULL: cut unknown (D155)
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
) STRICT;

-- table video_timeline
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

-- table watchlist
CREATE TABLE watchlist (user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  item_id INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT, added_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, item_id)) WITHOUT ROWID, STRICT;

-- table whole_file_hash
CREATE TABLE whole_file_hash (                             -- class C
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  sha256      BLOB,                      -- NULL when the node could not read it through
  size_bytes  INTEGER,                   -- the file's, when it was read
  modified_at INTEGER,
  failure     TEXT,                      -- why it could not be read through
  hashed_at   INTEGER NOT NULL
) STRICT;

-- trigger collection_found_ai
CREATE TRIGGER collection_found_ai AFTER INSERT ON collection BEGIN
  UPDATE collection SET
    found_at = (SELECT derived.found_at FROM collection_found AS derived WHERE derived.collection_id = collection.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM collection_found AS derived
      WHERE derived.collection_id = collection.id)
  WHERE collection.id = new.id
    AND EXISTS (
      SELECT 1 FROM collection_found AS derived WHERE derived.collection_id = collection.id
        AND (collection.found_at IS NOT derived.found_at
          OR collection.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger collection_member_found_ad
CREATE TRIGGER collection_member_found_ad AFTER DELETE ON collection_member WHEN old.member_title IS NOT NULL BEGIN
  UPDATE collection SET
    found_at = (SELECT derived.found_at FROM collection_found AS derived WHERE derived.collection_id = collection.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM collection_found AS derived
      WHERE derived.collection_id = collection.id)
  WHERE collection.id = old.collection_id
    AND EXISTS (
      SELECT 1 FROM collection_found AS derived WHERE derived.collection_id = collection.id
        AND (collection.found_at IS NOT derived.found_at
          OR collection.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger collection_member_found_ai
CREATE TRIGGER collection_member_found_ai AFTER INSERT ON collection_member WHEN new.member_title IS NOT NULL BEGIN
  UPDATE collection SET
    found_at = (SELECT derived.found_at FROM collection_found AS derived WHERE derived.collection_id = collection.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM collection_found AS derived
      WHERE derived.collection_id = collection.id)
  WHERE collection.id = new.collection_id
    AND EXISTS (
      SELECT 1 FROM collection_found AS derived WHERE derived.collection_id = collection.id
        AND (collection.found_at IS NOT derived.found_at
          OR collection.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger collection_member_found_au
CREATE TRIGGER collection_member_found_au AFTER UPDATE OF collection_id, member_title ON collection_member
WHEN old.collection_id IS NOT new.collection_id OR old.member_title IS NOT new.member_title BEGIN
  UPDATE collection SET
    found_at = (SELECT derived.found_at FROM collection_found AS derived WHERE derived.collection_id = collection.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM collection_found AS derived
      WHERE derived.collection_id = collection.id)
  WHERE collection.id IN (old.collection_id, new.collection_id)
    AND EXISTS (
      SELECT 1 FROM collection_found AS derived WHERE derived.collection_id = collection.id
        AND (collection.found_at IS NOT derived.found_at
          OR collection.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger file_filing_found_ad
CREATE TRIGGER file_filing_found_ad AFTER DELETE ON file_filing WHEN old.version_id IS NOT NULL BEGIN
  UPDATE title SET
    found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
  WHERE title.id = (SELECT version.title_id FROM version WHERE version.id = old.version_id)
    AND EXISTS (
      SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
        AND (title.found_at IS NOT derived.found_at
          OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger file_filing_found_ai
CREATE TRIGGER file_filing_found_ai AFTER INSERT ON file_filing WHEN new.version_id IS NOT NULL BEGIN
  UPDATE title SET
    found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
  WHERE title.id = (SELECT version.title_id FROM version WHERE version.id = new.version_id)
    AND EXISTS (
      SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
        AND (title.found_at IS NOT derived.found_at
          OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger file_filing_found_au
CREATE TRIGGER file_filing_found_au AFTER UPDATE OF version_id ON file_filing
WHEN old.version_id IS NOT new.version_id BEGIN
  UPDATE title SET
    found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
  WHERE title.id IN (SELECT version.title_id FROM version WHERE version.id IN (old.version_id, new.version_id))
    AND EXISTS (
      SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
        AND (title.found_at IS NOT derived.found_at
          OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger file_found_au
CREATE TRIGGER file_found_au AFTER UPDATE OF removed_at, modified_at ON file
WHEN old.removed_at IS NOT new.removed_at OR old.modified_at IS NOT new.modified_at BEGIN
  UPDATE title SET
    found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
  WHERE title.id IN (
      SELECT version.title_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id = new.id)
    AND EXISTS (
      SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
        AND (title.found_at IS NOT derived.found_at
          OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger item_text_ad
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.id, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.id, old.name, old.search_alternates);
END;

-- trigger item_text_ai
CREATE TRIGGER item_text_ai AFTER INSERT ON item_text BEGIN
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.id, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.id, new.name, new.search_alternates);
END;

-- trigger item_text_au
CREATE TRIGGER item_text_au AFTER UPDATE OF name, search_alternates, overview ON item_text
WHEN old.name IS NOT new.name OR old.search_alternates IS NOT new.search_alternates
  OR old.overview IS NOT new.overview BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.id, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.id, new.name, new.search_alternates, new.overview);
END;

-- trigger item_text_au_trigram
CREATE TRIGGER item_text_au_trigram AFTER UPDATE OF name, search_alternates ON item_text
WHEN old.name IS NOT new.name OR old.search_alternates IS NOT new.search_alternates BEGIN
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.id, old.name, old.search_alternates);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.id, new.name, new.search_alternates);
END;

-- trigger title_found_ai
CREATE TRIGGER title_found_ai AFTER INSERT ON title BEGIN
  UPDATE title SET
    found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
  WHERE title.id = new.id
    AND EXISTS (
      SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
        AND (title.found_at IS NOT derived.found_at
          OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger title_found_au
CREATE TRIGGER title_found_au AFTER UPDATE OF found_at, found_file_modified_at ON title
WHEN old.found_at IS NOT new.found_at OR old.found_file_modified_at IS NOT new.found_file_modified_at BEGIN
  UPDATE collection SET
    found_at = (SELECT derived.found_at FROM collection_found AS derived WHERE derived.collection_id = collection.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM collection_found AS derived
      WHERE derived.collection_id = collection.id)
  WHERE collection.id IN (
      SELECT collection_member.collection_id FROM collection_member WHERE collection_member.member_title = new.id)
    AND EXISTS (
      SELECT 1 FROM collection_found AS derived WHERE derived.collection_id = collection.id
        AND (collection.found_at IS NOT derived.found_at
          OR collection.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger version_found_ai
CREATE TRIGGER version_found_ai AFTER INSERT ON version BEGIN
  UPDATE title SET
    found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
  WHERE title.id = new.title_id
    AND EXISTS (
      SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
        AND (title.found_at IS NOT derived.found_at
          OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- trigger version_found_au
CREATE TRIGGER version_found_au AFTER UPDATE OF title_id, removed_at ON version
WHEN old.title_id IS NOT new.title_id OR old.removed_at IS NOT new.removed_at BEGIN
  UPDATE title SET
    found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
  WHERE title.id IN (old.title_id, new.title_id)
    AND EXISTS (
      SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
        AND (title.found_at IS NOT derived.found_at
          OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- view collection_found
CREATE VIEW collection_found AS
SELECT collection.id AS collection_id,
  coalesce((
    SELECT title.found_at FROM collection_member CROSS JOIN title ON title.id = collection_member.member_title
    WHERE collection_member.collection_id = collection.id
    ORDER BY title.found_at DESC, title.found_file_modified_at DESC LIMIT 1
  ), 0) AS found_at,
  coalesce((
    SELECT title.found_file_modified_at FROM collection_member
    CROSS JOIN title ON title.id = collection_member.member_title
    WHERE collection_member.collection_id = collection.id
    ORDER BY title.found_at DESC, title.found_file_modified_at DESC LIMIT 1
  ), 0) AS found_file_modified_at
FROM collection;

-- view library_place
CREATE VIEW library_place AS
SELECT library_root.library_id, library.kind AS library_kind, library_root.node_id, library_root.root_path,
  place.id AS place_id, place.copy_of, coalesce(place.copy_of, place.id) AS file_id, place.path,
  place.reported_path_raw, place.filing, place.first_seen_at
FROM library_root
CROSS JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
CROSS JOIN file AS place ON place.node_id = library_root.node_id AND place.removed_at IS NULL
  AND place.reported_path_raw > CAST(CASE WHEN library_root.root_path = '/' THEN '/'
    ELSE library_root.root_path || '/' END AS BLOB)
  AND place.reported_path_raw < CAST(CASE WHEN library_root.root_path = '/' THEN '0'
    ELSE library_root.root_path || '0' END AS BLOB)
WHERE library_root.retired_at IS NULL;

-- view live_file
CREATE VIEW live_file AS SELECT * FROM file WHERE removed_at IS NULL;

-- view live_item
CREATE VIEW live_item AS SELECT * FROM item WHERE removed_at IS NULL;

-- view place_library
CREATE VIEW place_library AS
SELECT place.id AS place_id, place.copy_of, coalesce(place.copy_of, place.id) AS file_id, place.node_id, place.first_seen_at,
  library_root.library_id, library.kind AS library_kind, library_root.root_path
FROM file AS place
CROSS JOIN library_root ON library_root.node_id = place.node_id AND library_root.retired_at IS NULL
CROSS JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
WHERE place.removed_at IS NULL
  AND (library_root.root_path = '/'
    OR substr(place.reported_path_raw, 1, length(CAST(library_root.root_path AS BLOB)) + 1)
      = CAST(library_root.root_path || '/' AS BLOB));

-- view title_found
CREATE VIEW title_found AS
SELECT title.id AS title_id,
  coalesce((
    SELECT file.first_seen_at FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file ON file.id = file_filing.file_id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
    ORDER BY file.first_seen_at, coalesce(file.modified_at, 0) DESC LIMIT 1
  ), 0) AS found_at,
  coalesce((
    SELECT coalesce(file.modified_at, 0) FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file ON file.id = file_filing.file_id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
    ORDER BY file.first_seen_at, coalesce(file.modified_at, 0) DESC LIMIT 1
  ), 0) AS found_file_modified_at
FROM title;

-- view version_library
CREATE VIEW version_library AS
SELECT file_filing.version_id, file_filing.file_id, place.id AS place_id, library_root.library_id
FROM file_filing
CROSS JOIN file AS place ON (place.id = file_filing.file_id OR place.copy_of = file_filing.file_id)
  AND place.removed_at IS NULL
CROSS JOIN library_root ON library_root.node_id = place.node_id AND library_root.retired_at IS NULL
CROSS JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
  AND library.kind = file_filing.library_kind
WHERE file_filing.version_id IS NOT NULL
  AND (library_root.root_path = '/'
    OR substr(place.reported_path_raw, 1, length(CAST(library_root.root_path AS BLOB)) + 1)
      = CAST(library_root.root_path || '/' AS BLOB));
