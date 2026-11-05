-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- One kind vocabulary (D416 section 5, D425 section 5): a library's kind is singular, as an item's
-- is, movie, series or other, where it was movies, shows or other. The library and file_filing,
-- the two tables that keep a library's kind, are rebuilt with the new check and their rows carried
-- over with the kind mapped. Each is held aside in a temporary table, dropped and created again
-- under its own name, so no rename has to resolve the views and triggers that name it; the
-- library's id sequence is kept, and file_filing's own index and triggers are made again as they
-- were.

CREATE TEMP TABLE library_held AS SELECT * FROM library;
CREATE TEMP TABLE library_sequence_held AS SELECT seq FROM sqlite_sequence WHERE name = 'library';
DROP TABLE library;
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
  removed_at       INTEGER,
  -- Hours between checks belong to the interval and nothing else (D303, M5).
  CHECK ((change_detection = 'interval') = (check_every_hours IS NOT NULL)),
  -- An other library is never identified online: it reads NFO files alone (D349 section 10).
  CHECK (kind <> 'other' OR metadata_source = 'nfo')
) STRICT;
INSERT INTO library (id, name, kind, metadata_source, change_detection, check_every_hours, is_writable,
  created_at, removed_at)
SELECT id, name, CASE kind WHEN 'movies' THEN 'movie' WHEN 'shows' THEN 'series' ELSE kind END,
  metadata_source, change_detection, check_every_hours, is_writable, created_at, removed_at
FROM library_held;
DELETE FROM sqlite_sequence WHERE name = 'library';
INSERT INTO sqlite_sequence (name, seq) SELECT 'library', seq FROM library_sequence_held;
DROP TABLE library_held;
DROP TABLE library_sequence_held;

CREATE TEMP TABLE file_filing_held AS SELECT * FROM file_filing;
DROP TABLE file_filing;
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
INSERT INTO file_filing (file_id, library_kind, filing, filing_reason, version_id, part_index,
  identified_from_path)
SELECT file_id, CASE library_kind WHEN 'movies' THEN 'movie' WHEN 'shows' THEN 'series' ELSE library_kind END,
  filing, filing_reason, version_id, part_index, identified_from_path
FROM file_filing_held;
DROP TABLE file_filing_held;
CREATE INDEX file_filing_version ON file_filing(version_id) WHERE version_id IS NOT NULL;

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
