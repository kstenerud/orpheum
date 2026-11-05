-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- The schema loses what nothing writes or reads (D426 section 8): the tables for editions, tags,
-- favourites, ratings, a user's denied items and the watchlist, which no code ever wrote; the
-- columns nothing wrote (an item's origin server, a title's original language and media kind, a
-- role's and a collection's predicate and a collection's membership, a device's audio output, a
-- span's unterminated mark, a group's overview, an extra's owner edition and position, a version's
-- edition, a file's identity confirmation); the index fetched_subtitle_once already covers; and the
-- views live_item, which nothing read, and live_file, whose two queries now say removed_at IS NULL
-- themselves. A dropped column held one value, or none, on every row, so no row loses anything
-- written. The versions, the extras and the files are rebuilt as step 0068 does, since a column
-- under a foreign key or a table's check cannot be dropped in place.

DROP VIEW live_item;
DROP VIEW live_file;
DROP INDEX fetched_subtitle_video;

ALTER TABLE item DROP COLUMN origin_server;
ALTER TABLE title DROP COLUMN original_language;
ALTER TABLE title DROP COLUMN media_kind;
ALTER TABLE role DROP COLUMN predicate;
ALTER TABLE collection DROP COLUMN predicate;
ALTER TABLE collection DROP COLUMN membership;
ALTER TABLE device DROP COLUMN audio_output;
ALTER TABLE play_span DROP COLUMN is_unterminated;
ALTER TABLE group_text DROP COLUMN overview;

CREATE TEMP TABLE version_held AS SELECT * FROM version;
CREATE TEMP TABLE version_sequence_held AS SELECT seq FROM sqlite_sequence WHERE name = 'version';
DROP TABLE version;
CREATE TABLE version (                                     -- class I
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id    INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  created_at  INTEGER NOT NULL,
  removed_at  INTEGER
) STRICT;
INSERT INTO version (id, title_id, created_at, removed_at)
SELECT id, title_id, created_at, removed_at FROM version_held;
DELETE FROM sqlite_sequence WHERE name = 'version';
INSERT INTO sqlite_sequence (name, seq) SELECT 'version', seq FROM version_sequence_held;
DROP TABLE version_sequence_held;
DROP TABLE version_held;
CREATE INDEX version_title ON version(title_id);
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

CREATE TEMP TABLE extra_relation_held AS SELECT * FROM extra_relation;
DROP TABLE extra_relation;
CREATE TABLE extra_relation (                              -- class O, or A when asserted
  owner_title   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  extra_title   INTEGER NOT NULL REFERENCES title(id) ON DELETE RESTRICT,
  extra_type    TEXT NOT NULL CHECK (extra_type IN ('trailer','teaser','clip','featurette',
                  'behind_the_scenes','deleted_scene','interview','short','scene',
                  'theme_song','theme_video','unknown')),       -- D97, D162: eleven + unknown
  asserted_by   TEXT NOT NULL CHECK (asserted_by IN ('scanner','user')),
  PRIMARY KEY (owner_title, extra_title)
) STRICT;
INSERT INTO extra_relation (owner_title, extra_title, extra_type, asserted_by)
SELECT owner_title, extra_title, extra_type, asserted_by FROM extra_relation_held;
DROP TABLE extra_relation_held;

CREATE TEMP TABLE file_held AS SELECT * FROM file;
CREATE TEMP TABLE file_sequence_held AS SELECT seq FROM sqlite_sequence WHERE name = 'file';
DROP TABLE file;
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
  sidecar_kind  TEXT CHECK (sidecar_kind IN ('nfo', 'subtitle', 'image'))
, filesystem_device INTEGER, inode INTEGER, filing TEXT NOT NULL DEFAULT 'video'
  CHECK (filing IN ('video','copy','attached','skipped','errored')), copy_of INTEGER REFERENCES file(id) ON DELETE RESTRICT) STRICT;
INSERT INTO file (id, node_id, path, size_bytes, modified_at, payload_hash, payload_bytes, filing_reason, error_detail_json, attached_to_file, first_seen_at, last_seen_at, removed_at, reported_path_raw, opensubtitles_hash, sidecar_kind, filesystem_device, inode, filing, copy_of)
SELECT id, node_id, path, size_bytes, modified_at, payload_hash, payload_bytes, filing_reason, error_detail_json, attached_to_file, first_seen_at, last_seen_at, removed_at, reported_path_raw, opensubtitles_hash, sidecar_kind, filesystem_device, inode, filing, copy_of FROM file_held;
DELETE FROM sqlite_sequence WHERE name = 'file';
INSERT INTO sqlite_sequence (name, seq) SELECT 'file', seq FROM file_sequence_held;
DROP TABLE file_sequence_held;
DROP TABLE file_held;
CREATE INDEX file_attached ON file(attached_to_file);
CREATE INDEX file_copy ON file(copy_of) WHERE copy_of IS NOT NULL;
CREATE INDEX file_errored ON file(filing, node_id, path) WHERE filing = 'errored' AND removed_at IS NULL;
CREATE UNIQUE INDEX file_live_path ON file(node_id, reported_path_raw) WHERE removed_at IS NULL;
CREATE INDEX file_payload ON file(payload_hash) WHERE removed_at IS NULL;
CREATE INDEX file_removed_at ON file(removed_at) WHERE removed_at IS NOT NULL;
CREATE INDEX file_removed_payload ON file(node_id, payload_hash, removed_at) WHERE removed_at IS NOT NULL;
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

DROP TABLE edition_text;
DROP TABLE edition;
DROP TABLE item_tag;
DROP TABLE tag;
DROP TABLE user_deny;
DROP TABLE favourite;
DROP TABLE user_rating;
DROP TABLE watchlist;
