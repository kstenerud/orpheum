-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D363: libraries may overlap. A file no longer belongs to a library: it is found at one or more
-- places, a node and a path, and a library covers places by its folders, so a file is in every
-- library that covers one of its places. A file row is a place: a byte-identical copy confirmed at a
-- second place is a row of its own, filed 'copy', naming the file it is a copy of (copy_of), and
-- what lies beside it stays attached to it, the place's (section 3). How identification filed a file
-- moves to file_filing, one row for each kind of library that filed it, so a films library and an
-- other library over one folder each give it the item of their kind (section 5). An item keeps the
-- library whose pass files it, filing_library_id, which says nothing of where it is listed. The mixed
-- kind of library, which no library has, is gone.

-- The library is rebuilt with its kind's new check, as step 0059 rebuilt it: nothing names it in a
-- trigger or a view; its ids and their sequence are kept. A mixed library is refused, loudly, by a
-- guard that lives only while the step runs: D363 removes the kind, and none exists to carry over.
CREATE TABLE library_new (                                 -- class A
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
CREATE TEMP TRIGGER refuse_a_mixed_library BEFORE INSERT ON library_new WHEN new.kind = 'mixed'
BEGIN
  SELECT RAISE(ABORT, 'D363 removes the mixed kind of library, and a library of it exists: make it a films or a series library, then migrate');
END;
INSERT INTO library_new (id, name, kind, metadata_source, change_detection, check_every_hours, is_writable,
  created_at, removed_at)
SELECT id, name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at, removed_at
FROM library;
DROP TRIGGER refuse_a_mixed_library;
INSERT INTO sqlite_sequence (name, seq)
SELECT 'library_new', seq FROM sqlite_sequence
WHERE name = 'library' AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'library_new');
UPDATE sqlite_sequence SET seq = max(seq, (SELECT seq FROM sqlite_sequence WHERE name = 'library'))
WHERE name = 'library_new' AND EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'library');
DROP TABLE library;
ALTER TABLE library_new RENAME TO library;

-- How each kind of library filed a file (D163, D363 section 5): listed, an extra, or unidentified,
-- with why; the version that holds it there and its part; and the path it was identified from, so a
-- rename is seen as one (D208 section 1). Class A/O as the file's columns it replaces: identification
-- writes it, through library.FileAs and library.PutInVersion. Each file of a library is carried over
-- as its library's kind filed it, tombstones too, whose versions keep what was watched of them.
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
INSERT INTO file_filing (file_id, library_kind, filing, filing_reason, version_id, part_index, identified_from_path)
SELECT file.id, library.kind, file.filing, file.filing_reason, file.version_id, file.part_index,
  file.identified_from_path
FROM file
JOIN library ON library.id = file.library_id
WHERE file.filing IN ('listed', 'extra', 'unidentified');
CREATE INDEX file_filing_version ON file_filing(version_id) WHERE version_id IS NOT NULL;

-- What named the columns that leave the file: step 0058's view, and the triggers that read it, made
-- again below, and the indexes, made again without them.
DROP TRIGGER file_found_ai;
DROP TRIGGER file_found_au;
DROP TRIGGER version_found_ai;
DROP TRIGGER version_found_au;
DROP TRIGGER title_found_ai;
DROP VIEW title_found;
DROP INDEX file_errored;
DROP INDEX file_library;
DROP INDEX file_removed_payload;
DROP INDEX file_version;

-- A file's own filing is the scan's: a video, a copy of one at another place, a sidecar attached to
-- one, or a path skipped or that could not be read. Its check is the column's own, so the column is
-- added anew, copied and renamed into the old one's place, as step 0059 did a title's kind. The
-- default is only what adding a column needs.
ALTER TABLE file ADD COLUMN filing_new TEXT NOT NULL DEFAULT 'video'
  CHECK (filing_new IN ('video','copy','attached','skipped','errored'));
UPDATE file SET filing_new = CASE WHEN filing IN ('listed', 'extra', 'unidentified') THEN 'video' ELSE filing END
WHERE filing_new IS NOT CASE WHEN filing IN ('listed', 'extra', 'unidentified') THEN 'video' ELSE filing END;
UPDATE file SET filing_reason = NULL WHERE filing IN ('listed', 'extra', 'unidentified') AND filing_reason IS NOT NULL;
ALTER TABLE file DROP COLUMN filing;
ALTER TABLE file RENAME COLUMN filing_new TO filing;
ALTER TABLE file DROP COLUMN library_id;
ALTER TABLE file DROP COLUMN version_id;
ALTER TABLE file DROP COLUMN part_index;
ALTER TABLE file DROP COLUMN identified_from_path;
-- The file this place is a confirmed copy of, filed 'copy': its first place, which holds its identity,
-- its filings, its tracks and what was watched of it (D363 section 2). NULL for the file itself.
ALTER TABLE file ADD COLUMN copy_of INTEGER REFERENCES file(id) ON DELETE RESTRICT;
CREATE INDEX file_errored ON file(filing, node_id, path) WHERE filing = 'errored' AND removed_at IS NULL;
CREATE INDEX file_removed_payload ON file(node_id, payload_hash, removed_at) WHERE removed_at IS NOT NULL;
CREATE INDEX file_copy ON file(copy_of) WHERE copy_of IS NOT NULL;

-- A library's folders on a node, for the places they cover, read from the place.
CREATE INDEX library_root_node ON library_root(node_id, root_path) WHERE retired_at IS NULL;

-- The library whose identification pass files an item: the one that governs its files, the
-- outermost folder of their kind's libraries covering them (D363 section 5). It is identification's
-- bookkeeping; which libraries list the item is their coverage of its files' places.
DROP INDEX item_library;
ALTER TABLE item RENAME COLUMN library_id TO filing_library_id;
CREATE INDEX item_filing_library ON item(filing_library_id, kind) WHERE removed_at IS NULL;

-- D116's whole-file pass, run for a candidate copy alone (D363 section 2): the SHA-256 of a file's
-- every byte, as it was at its size and modified time. Class C: a later read finds it again.
CREATE TABLE whole_file_hash (                             -- class C
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  sha256      BLOB,                      -- NULL when the node could not read it through
  size_bytes  INTEGER,                   -- the file's, when it was read
  modified_at INTEGER,
  failure     TEXT,                      -- why it could not be read through
  hashed_at   INTEGER NOT NULL
) STRICT;

-- Each live place and each live library whose live folder covers it (D363 sections 1, 4): a folder of
-- "/" holds every place on its node, and one holds the places below it by the bytes their node
-- reported (D211). file_id is the file the place is of: itself, or the file it is a copy of. Read
-- from the place outward, by CROSS JOIN: the folders on its node, then their libraries.
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

-- The same coverage read from the folder: each live folder of each live library and each live place
-- under it, by the range of the bytes their node reported that file_live_path serves: after the
-- folder's path and a slash, before its path and the character after the slash ("/" holds every
-- place on its node). A folder's places, and so a library's, without reading every place.
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

-- Each version and each library that lists it: one of its kind covering a live place of a live file
-- it holds (D363 sections 4, 5). An item two folders, two libraries or two copies reach is one row
-- per library, and a list asks only whether one exists (section 6). Read from the version outward, by
-- CROSS JOIN, as place_library is.
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

-- Step 0058's derivation, through how the file was filed.
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

-- A file goes, comes back or changes, or is filed in, out of or into another version: each title it
-- is filed in is derived again. Each writes only on a change (D303, DL9).
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

-- Step 0058's, as they were: a version made after its file names it, or a title after its version,
-- as a fixture may, and a version moved to another title or removed.
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
