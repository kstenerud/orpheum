-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Newest first is the scan that found it, then its file's own time (the owner, 2026-10-25: "we
-- prioritize items from the most recent scan (fast phase where it gets a list of all files), and
-- within a scan batch we look at the file timestamp"). A listing is stored in one write, so every
-- file it finds has that write's time as its first_seen_at (library.TakeListing): the time names the
-- scan. Kept on the title and the series, class C, derived by the triggers below from what they are
-- derived from, so a page walks an index and stops at its LIMIT:
--   title.found_at:                    its first-found live file's first_seen_at, the scan that found
--                                      it; 0 with no live file
--   title.found_file_modified_at:      that file's modified_at, the newest of several found together
--   collection.found_at, ..._modified: its newest episode's: the latest found_at of its members, and
--                                      that member's file time; 0 with none
-- A film is as new as the scan that first found it: a second version found later does not make it
-- new again. A series is as new as its newest episode: a new episode does.
ALTER TABLE title ADD COLUMN found_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE title ADD COLUMN found_file_modified_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE collection ADD COLUMN found_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE collection ADD COLUMN found_file_modified_at INTEGER NOT NULL DEFAULT 0;

-- What each is derived as: the one definition the triggers, the backfill and the invariant read.
CREATE VIEW title_found AS
SELECT title.id AS title_id,
  coalesce((
    SELECT file.first_seen_at FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
    ORDER BY file.first_seen_at, coalesce(file.modified_at, 0) DESC LIMIT 1
  ), 0) AS found_at,
  coalesce((
    SELECT coalesce(file.modified_at, 0) FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
    ORDER BY file.first_seen_at, coalesce(file.modified_at, 0) DESC LIMIT 1
  ), 0) AS found_file_modified_at
FROM title;

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

-- Every title and series as its files say, before the triggers keep them so.
UPDATE title SET
  found_at = (SELECT derived.found_at FROM title_found AS derived WHERE derived.title_id = title.id),
  found_file_modified_at = (
    SELECT derived.found_file_modified_at FROM title_found AS derived WHERE derived.title_id = title.id)
WHERE EXISTS (
    SELECT 1 FROM title_found AS derived WHERE derived.title_id = title.id
      AND (title.found_at IS NOT derived.found_at
        OR title.found_file_modified_at IS NOT derived.found_file_modified_at));
UPDATE collection SET
  found_at = (SELECT derived.found_at FROM collection_found AS derived WHERE derived.collection_id = collection.id),
  found_file_modified_at = (
    SELECT derived.found_file_modified_at FROM collection_found AS derived
    WHERE derived.collection_id = collection.id)
WHERE EXISTS (
    SELECT 1 FROM collection_found AS derived WHERE derived.collection_id = collection.id
      AND (collection.found_at IS NOT derived.found_at
        OR collection.found_file_modified_at IS NOT derived.found_file_modified_at));

-- Newest first, walked from the top: the films', and the series'. item_added served the order this
-- replaces, and nothing else.
CREATE INDEX title_film_found ON title(found_at, found_file_modified_at) WHERE kind = 'movie';
CREATE INDEX collection_series_found ON collection(found_at, found_file_modified_at) WHERE kind = 'series';
DROP INDEX item_added;

-- A title's files come, go, change or move to another of its versions or titles: it is derived again.
-- Each writes only on a change (D303, DL9).
CREATE TRIGGER file_found_ai AFTER INSERT ON file WHEN new.version_id IS NOT NULL BEGIN
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

CREATE TRIGGER file_found_au AFTER UPDATE OF version_id, removed_at, modified_at ON file
WHEN old.version_id IS NOT new.version_id OR old.removed_at IS NOT new.removed_at
  OR old.modified_at IS NOT new.modified_at BEGIN
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

-- A version made after its file names it, or a title after its version, as a fixture may, is
-- derived too.
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

-- A series is derived again when one of its episodes is, or it gains or loses one.
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

-- A series made after its members, as a fixture may, is derived too.
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
