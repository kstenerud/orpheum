-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D349: a kind of library, other, for the videos that are neither films nor series (home movies,
-- concerts, lectures, clips). Such a library takes NFO files only, so nothing of it is ever matched
-- online, and every video in it is a title of the kind other, with the date it was recorded and its
-- file's name, which its lists sort by.

-- The library is rebuilt with its new kind and the check that holds an other library to NFO files.
-- Nothing names it in a trigger or a view; its ids and their sequence are kept.
CREATE TABLE library_new (                                 -- class A
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('movies','shows','mixed','other')),
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
INSERT INTO library_new (id, name, kind, metadata_source, change_detection, check_every_hours, is_writable,
  created_at, removed_at)
SELECT id, name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at, removed_at
FROM library;
-- The ids of libraries deleted before are never issued again (D173): the copy takes the old table's
-- sequence, which the rows copied may be below.
INSERT INTO sqlite_sequence (name, seq)
SELECT 'library_new', seq FROM sqlite_sequence
WHERE name = 'library' AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'library_new');
UPDATE sqlite_sequence SET seq = max(seq, (SELECT seq FROM sqlite_sequence WHERE name = 'library'))
WHERE name = 'library_new' AND EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'library');
DROP TABLE library;
ALTER TABLE library_new RENAME TO library;

-- A title's kind gains other. Its check is the column's own, so the column is added anew with the
-- new check, copied, and renamed into the old one's place; the two indexes that name the kind are
-- made again around it. The default is only what adding a column needs: every title is inserted
-- with its kind.
DROP INDEX title_waiting;
DROP INDEX title_film_found;
ALTER TABLE title ADD COLUMN kind_new TEXT NOT NULL DEFAULT 'movie'
  CHECK (kind_new IN ('movie','episode','extra','other'));
UPDATE title SET kind_new = kind WHERE kind_new IS NOT kind;
ALTER TABLE title DROP COLUMN kind;
ALTER TABLE title RENAME COLUMN kind_new TO kind;
CREATE INDEX title_waiting ON title(id)
  WHERE identification_state = 'unidentified' AND followed_source = 'none' AND kind != 'episode';
CREATE INDEX title_film_found ON title(found_at, found_file_modified_at) WHERE kind = 'movie';
-- An other library's newest added, as the films' (step 0058).
CREATE INDEX title_other_found ON title(found_at, found_file_modified_at) WHERE kind = 'other';

-- What an other video's lists sort by, besides its name: when it was recorded, and where that came
-- from (its container's tag, a date in its file's or a folder's name, the file's own time, or a
-- person's correction, the override recorded_at), and its file's name with that name's sort key in
-- the sort_locale (D47). Class C: the identification pass's, from the file, its probe and the
-- override.
CREATE TABLE other_video (                                 -- class C (D349 section 4)
  title_id      INTEGER PRIMARY KEY REFERENCES title(id) ON DELETE RESTRICT,
  recorded_at   INTEGER NOT NULL,          -- Unix milliseconds
  recorded_from TEXT NOT NULL CHECK (recorded_from IN ('tag','name','file','person')),
  file_name     TEXT NOT NULL,             -- its file's name, the last part of its path
  file_name_key BLOB NOT NULL              -- file_name's CLDR sort key in the sort_locale, BINARY order
) STRICT;
CREATE INDEX other_video_recorded ON other_video(recorded_at, title_id);
CREATE INDEX other_video_file_name ON other_video(file_name_key, title_id);
