-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- A file is split from its places (D426 section 4). Until now one row was both: a file's first place
-- was the file itself, and a confirmed copy at another place a row filed 'copy' naming it (D363
-- section 2). file now holds the payload (its hash, size, OpenSubtitles hash, and, for a sidecar, its
-- kind; NULL is a media file), and place holds where it was seen: the node, the path and the bytes
-- reported (the key, D211), the device and inode, the modified and seen times, the removal, the
-- place's filing and the file it holds. What lies beside a place is that place's (D363 section 3), so
-- a sidecar attaches to a place.
--
-- Every id is kept. A row that was no copy becomes a file and a place, both with its id: a file's id
-- is the id of the place it was first found at. A copy becomes a place holding the file it was a copy
-- of; what it held of its own was moved onto that file when it was confirmed (copies.go's merge and
-- the folds: watching, choices, fetched subtitles), so its id is a place's alone, and the whole-file
-- hash read of it, a cache, goes. A sidecar attached to a copy is attached to the copy's place, by the
-- same id. A place is read, skipped or errored: 'video' and 'attached' are read places whose file is
-- a media file or a sidecar, and a copy is a read place holding another place's file.
--
-- An older binary writes the one row, so the compatible version rises.

DROP VIEW version_library;
DROP VIEW library_place;

CREATE TEMP TABLE file_held AS SELECT * FROM file;
CREATE TEMP TABLE file_sequence_held AS SELECT seq FROM sqlite_sequence WHERE name = 'file';
-- What a scan read again at a copy's place was kept under the copy's own id: its probe and its own
-- tracks, which its file has the same of, go; the tracks of the subtitles beside it are its file's,
-- as copies.go's merge makes them; a filing of it is its file's where the file has none of that kind
-- of library (MoveFilingToFile), and goes where it has.
CREATE TEMP TABLE copy_held AS SELECT id, copy_of FROM file_held WHERE copy_of IS NOT NULL;
DELETE FROM whole_file_hash WHERE file_id IN (SELECT id FROM copy_held);
DELETE FROM file_probe WHERE file_id IN (SELECT id FROM copy_held);
DELETE FROM file_nfo WHERE file_id IN (SELECT id FROM copy_held);
UPDATE stream SET video_file = (SELECT copy_of FROM copy_held WHERE copy_held.id = stream.video_file)
WHERE video_file IN (SELECT id FROM copy_held) AND bytes_file <> video_file;
DELETE FROM stream WHERE bytes_file IN (SELECT id FROM copy_held);
UPDATE file_filing SET file_id = (SELECT copy_of FROM copy_held WHERE copy_held.id = file_filing.file_id)
WHERE file_id IN (SELECT id FROM copy_held)
  AND NOT EXISTS (
    SELECT 1 FROM file_filing AS kept
    WHERE kept.file_id = (SELECT copy_of FROM copy_held WHERE copy_held.id = file_filing.file_id)
      AND kept.library_kind = file_filing.library_kind);
DELETE FROM file_filing WHERE file_id IN (SELECT id FROM copy_held);
DROP TABLE copy_held;
-- The view the found triggers read goes with the table it reads, and comes back on place below.
DROP VIEW item_found;
DROP TABLE file;

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
  attached_to_place INTEGER REFERENCES place(id) ON DELETE RESTRICT,  -- a sidecar's video's place
  CHECK (file_id IS NOT NULL OR filing IN ('skipped', 'errored'))
) STRICT;

INSERT INTO file (id, size_bytes, payload_hash, payload_bytes, opensubtitles_hash, sidecar_kind)
SELECT id, size_bytes, payload_hash, payload_bytes, opensubtitles_hash, sidecar_kind
FROM file_held WHERE copy_of IS NULL;
INSERT INTO place (id, file_id, node_id, path, reported_path_raw, filesystem_device, inode, modified_at,
  first_seen_at, last_seen_at, removed_at, filing, filing_reason, error_detail_json, attached_to_place)
SELECT id, coalesce(copy_of, id), node_id, path, coalesce(reported_path_raw, CAST(path AS BLOB)),
  filesystem_device, inode, modified_at, first_seen_at, last_seen_at, removed_at,
  CASE WHEN filing IN ('skipped', 'errored') THEN filing ELSE 'read' END, filing_reason, error_detail_json,
  attached_to_file
FROM file_held;

-- A new file takes the next id, and its first place the same: neither sequence is behind any id kept.
DELETE FROM sqlite_sequence WHERE name IN ('file', 'place');
INSERT INTO sqlite_sequence (name, seq)
SELECT 'file', max(coalesce((SELECT seq FROM file_sequence_held), 0), coalesce((SELECT max(id) FROM file_held), 0));
INSERT INTO sqlite_sequence (name, seq) SELECT 'place', seq FROM sqlite_sequence WHERE name = 'file';
DROP TABLE file_sequence_held;
DROP TABLE file_held;

CREATE INDEX file_payload ON file(payload_hash) WHERE payload_hash IS NOT NULL;
CREATE INDEX place_file ON place(file_id);
CREATE INDEX place_attached ON place(attached_to_place);
CREATE INDEX place_errored ON place(filing, node_id, path) WHERE filing = 'errored' AND removed_at IS NULL;
CREATE UNIQUE INDEX place_live_path ON place(node_id, reported_path_raw) WHERE removed_at IS NULL;
CREATE INDEX place_removed_at ON place(removed_at) WHERE removed_at IS NOT NULL;

-- When each item was found, from its file's first place (the place whose id is the file's), as step
-- 0072 derived it from the file's row.
CREATE VIEW item_found AS
SELECT item.id AS item_id,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_at FROM "sequence_member" CROSS JOIN item AS member ON member.id = "sequence_member".member_work
    WHERE "sequence_member".sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT place.first_seen_at FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN place ON place.id = file_filing.file_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL AND place.removed_at IS NULL
    ORDER BY place.first_seen_at, coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_at,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_file_modified_at FROM "sequence_member"
    CROSS JOIN item AS member ON member.id = "sequence_member".member_work
    WHERE "sequence_member".sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT coalesce(place.modified_at, 0) FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN place ON place.id = file_filing.file_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL AND place.removed_at IS NULL
    ORDER BY place.first_seen_at, coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_file_modified_at
FROM item;

CREATE TRIGGER place_found_au AFTER UPDATE OF removed_at, modified_at ON place
WHEN old.removed_at IS NOT new.removed_at OR old.modified_at IS NOT new.modified_at BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.work_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id = new.id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- Each live place and each live library whose live folder covers it, as step 0077 said, read from
-- place: a copy's place holds its file, so the file a place holds is file_id, and a copy is a place
-- whose id is not its file's.
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

CREATE VIEW version_library AS
SELECT file_filing.version_id, file_filing.file_id, library_place.place_id, library_place.library_id
FROM file_filing
JOIN library_place ON library_place.file_id = file_filing.file_id
  AND library_place.library_kind = file_filing.library_kind
WHERE file_filing.version_id IS NOT NULL;
