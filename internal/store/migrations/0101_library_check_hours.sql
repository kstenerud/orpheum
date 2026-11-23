-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- Every library keeps its hours between checks, whatever it notices changes by, and they are used only
-- while it checks on an interval (D541 §1): a library that stops checking on an interval and starts
-- again has the hours it had. A library that had none, one watching or checked only when asked, takes
-- 6, a new library's starting value; one under 1 takes 1, the least, which no API ever stored. A
-- rebuild, since a CHECK cannot be changed in place (standards/sql.md §5); it raises the compatible
-- version, since an older binary writes a library that is not checked on an interval with no hours,
-- which the column no longer takes (migrations.md, Expand then contract). The views over the table
-- are dropped before it and made again after the rename, which checks every view; its sequence is
-- carried, so an id is never given twice.
DROP VIEW version_library;
DROP VIEW library_place;
CREATE TABLE library_new (                                 -- class A
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('movie','series','other')),
  -- the three questions asked at creation (D184), never defaulted:
  metadata_source  TEXT NOT NULL CHECK (metadata_source IN ('nfo_then_tmdb','tmdb','nfo')),
  change_detection TEXT NOT NULL CHECK (change_detection IN ('watch','interval','manual')),
  -- Hours between checks, kept whatever change_detection is, used only for 'interval' (D541 §1).
  check_every_hours INTEGER NOT NULL CHECK (check_every_hours >= 1),
  is_writable      INTEGER NOT NULL CHECK (is_writable IN (0,1)),
  created_at       INTEGER NOT NULL,
  removed_at       INTEGER,
  revision         INTEGER NOT NULL DEFAULT 0,
  -- An other library is never identified online: it reads NFO files alone (D349 section 10).
  CHECK (kind <> 'other' OR metadata_source = 'nfo')
) STRICT;
INSERT INTO library_new (id, name, kind, metadata_source, change_detection, check_every_hours, is_writable,
  created_at, removed_at, revision)
SELECT id, name, kind, metadata_source, change_detection, max(coalesce(check_every_hours, 6), 1), is_writable,
  created_at, removed_at, revision
FROM library;
DELETE FROM sqlite_sequence WHERE name = 'library_new';
INSERT INTO sqlite_sequence (name, seq) SELECT 'library_new', seq FROM sqlite_sequence WHERE name = 'library';
DROP TABLE library;
ALTER TABLE library_new RENAME TO library;
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
WHERE file_filing.version_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM system_library WHERE system_library.library_id = library_place.library_id);
