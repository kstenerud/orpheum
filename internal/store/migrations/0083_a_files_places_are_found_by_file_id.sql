-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A file's places are found by place.file_id, never by assuming a place's id is its file's (D432
-- section 6, the relocation audit's check 4). Step 0078 let many places hold one file, and a file
-- keeps its own place's id while that place lives (D363 section 2); a file held only by a place made
-- later, as a repoint that matches files by their hash will make (D429), had no place in item_found
-- or place_found_au, so its items lost their found time though the file was live.
--
-- file_place says which place a file is read from: its own while that is live, else the earliest
-- found of its live places. The queries that read "the file's place" join it.
--
-- An item's found time is its files' earliest place, live or removed, so a second place does not
-- reset it; a file counts only while a place of it is live, and its modified time is that of the
-- place it is read from. The place triggers watch which file a place holds, and a place made or
-- deleted, since each can change the earliest.

CREATE VIEW file_place AS
SELECT place.file_id, place.id AS place_id
FROM place
WHERE place.removed_at IS NULL AND place.id = (
  SELECT first.id FROM place AS first
  WHERE first.file_id = place.file_id AND first.removed_at IS NULL
  ORDER BY first.id <> first.file_id, first.first_seen_at, first.id LIMIT 1);

DROP TRIGGER place_found_au;
DROP VIEW item_found;

CREATE VIEW item_found AS
SELECT item.id AS item_id,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_at FROM series_member CROSS JOIN item AS member ON member.id = series_member.member_work
    WHERE series_member.sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT (SELECT min(found.first_seen_at) FROM place AS found WHERE found.file_id = file_place.file_id) AS first_found
    FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file_place ON file_place.file_id = file_filing.file_id
    CROSS JOIN place ON place.id = file_place.place_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
    ORDER BY first_found, coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_at,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_file_modified_at FROM series_member
    CROSS JOIN item AS member ON member.id = series_member.member_work
    WHERE series_member.sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT coalesce(place.modified_at, 0)
    FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file_place ON file_place.file_id = file_filing.file_id
    CROSS JOIN place ON place.id = file_place.place_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL
    ORDER BY (SELECT min(found.first_seen_at) FROM place AS found WHERE found.file_id = file_place.file_id),
      coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_file_modified_at
FROM item;

CREATE TRIGGER place_found_ai AFTER INSERT ON place WHEN new.file_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.work_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id IN (new.file_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER place_found_ad AFTER DELETE ON place WHEN old.file_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.work_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id IN (old.file_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER place_found_au AFTER UPDATE OF removed_at, modified_at, file_id ON place
WHEN old.removed_at IS NOT new.removed_at OR old.modified_at IS NOT new.modified_at
  OR old.file_id IS NOT new.file_id BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.work_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id IN (old.file_id, new.file_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- While every file is held by its own place the found times are as they were; they are read again
-- all the same, as the triggers would have kept them.
UPDATE item SET
  found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
  found_file_modified_at = (
    SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
WHERE EXISTS (
  SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
    AND (item.found_at IS NOT derived.found_at
      OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
