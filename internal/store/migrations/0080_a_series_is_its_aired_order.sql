-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- A series' members are its aired order's places (D426 section 6). sequence_member held each
-- episode a second time, beside its place in the aired order, at the same sort order: every writer
-- wrote both (identify's positionEpisode and positionUnnumbered) and moved both (a series joining
-- another). Its other role, a sequence of sequences (member_sequence, a box set's), no writer ever
-- filled, and the box set returns with its feature (D426 sections 2 and 8). So it goes, and
-- series_member reads the membership from the aired order.
--
-- No membership is lost: a member with no place in its series' aired order gets one, in no season, at
-- its member sort order (an unplaced one after the numbered specials, as an unnumbered special is),
-- and a series with members and no aired order is given it, as the scanner makes it. A series' found
-- time is a reading of its members, so the triggers that kept it move to the places, and it is read
-- again.
--
-- An older binary writes sequence_member, so the compatible version rises.

INSERT INTO ordering (sequence_id, name, kind)
SELECT DISTINCT sequence_member.sequence_id, 'aired', 'aired' FROM sequence_member
WHERE sequence_member.member_work IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM ordering WHERE ordering.sequence_id = sequence_member.sequence_id AND ordering.name = 'aired');
INSERT INTO position (ordering_id, work_id, grp_id, number_in_group, sort_order)
SELECT ordering.id, sequence_member.member_work, NULL, NULL, coalesce(sequence_member.position, 9999)
FROM sequence_member
JOIN ordering ON ordering.sequence_id = sequence_member.sequence_id AND ordering.name = 'aired'
WHERE sequence_member.member_work IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM position
    WHERE position.ordering_id = ordering.id AND position.work_id = sequence_member.member_work);

DROP TRIGGER sequence_member_found_ai;
DROP TRIGGER sequence_member_found_ad;
DROP TRIGGER sequence_member_found_au;
DROP TRIGGER item_found_au;
DROP VIEW item_found;
DROP INDEX sequence_member_work;
DROP TABLE sequence_member;

-- A series' members: the titles placed in its aired order (D426 section 6).
CREATE VIEW series_member AS
SELECT ordering.sequence_id, position.work_id AS member_work, position.sort_order
FROM ordering
JOIN position ON position.ordering_id = ordering.id
WHERE ordering.name = 'aired';

CREATE VIEW item_found AS
SELECT item.id AS item_id,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_at FROM series_member CROSS JOIN item AS member ON member.id = series_member.member_work
    WHERE series_member.sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT place.first_seen_at FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN place ON place.id = file_filing.file_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL AND place.removed_at IS NULL
    ORDER BY place.first_seen_at, coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_at,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_file_modified_at FROM series_member
    CROSS JOIN item AS member ON member.id = series_member.member_work
    WHERE series_member.sequence_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT coalesce(place.modified_at, 0) FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN place ON place.id = file_filing.file_id
    WHERE version.work_id = item.id AND version.removed_at IS NULL AND place.removed_at IS NULL
    ORDER BY place.first_seen_at, coalesce(place.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_file_modified_at
FROM item;

CREATE TRIGGER item_found_au AFTER UPDATE OF found_at, found_file_modified_at ON item
WHEN old.found_at IS NOT new.found_at OR old.found_file_modified_at IS NOT new.found_file_modified_at BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (SELECT series_member.sequence_id FROM series_member WHERE series_member.member_work = new.id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- A place in an aired order made, gone or moved changes its series' members, so its found time.
CREATE TRIGGER position_found_ai AFTER INSERT ON position BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT ordering.sequence_id FROM ordering WHERE ordering.id = new.ordering_id AND ordering.name = 'aired')
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER position_found_ad AFTER DELETE ON position BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT ordering.sequence_id FROM ordering WHERE ordering.id = old.ordering_id AND ordering.name = 'aired')
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER position_found_au AFTER UPDATE OF ordering_id, work_id ON position
WHEN old.ordering_id IS NOT new.ordering_id OR old.work_id IS NOT new.work_id BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT ordering.sequence_id FROM ordering
      WHERE ordering.id IN (old.ordering_id, new.ordering_id) AND ordering.name = 'aired')
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

-- The places this step made are members already, so a series' found time is as it was; it is read
-- again all the same, as the triggers would have kept it.
UPDATE item SET
  found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
  found_file_modified_at = (
    SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
WHERE item.kind = 'series'
  AND EXISTS (
    SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
      AND (item.found_at IS NOT derived.found_at
        OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
