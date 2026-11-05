-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The subtypes take D426 section 3's names: title becomes work, the playable thing (a movie, an
-- episode, an extra, an other video), and collection becomes sequence, the ordered container (a
-- series; a box set later). Every table, column, index and trigger named for them follows:
-- title_watched is work_watched, collection_member is sequence_member, a title_id is a work_id, a
-- collection_id a sequence_id, member_title and member_collection are member_work and
-- member_sequence, and an extra's owner_title and extra_title are owner_work and extra_work.
-- Renames keep every row, every foreign key and the views and triggers that name them, which the
-- engine rewrites; the indexes and triggers named for the old words are made again under the new.

DROP TRIGGER collection_member_found_ad;
DROP TRIGGER collection_member_found_ai;
DROP TRIGGER collection_member_found_au;
DROP INDEX collection_member_title;
DROP INDEX position_title;
DROP INDEX version_title;

ALTER TABLE title RENAME TO work;
ALTER TABLE collection RENAME TO sequence;
ALTER TABLE collection_member RENAME TO sequence_member;
ALTER TABLE title_watched RENAME TO work_watched;

ALTER TABLE other_video RENAME COLUMN title_id TO work_id;
ALTER TABLE position RENAME COLUMN title_id TO work_id;
ALTER TABLE version RENAME COLUMN title_id TO work_id;
ALTER TABLE work_watched RENAME COLUMN title_id TO work_id;
ALTER TABLE sequence_member RENAME COLUMN collection_id TO sequence_id;
ALTER TABLE sequence_member RENAME COLUMN member_title TO member_work;
ALTER TABLE sequence_member RENAME COLUMN member_collection TO member_sequence;
ALTER TABLE ordering RENAME COLUMN collection_id TO sequence_id;
ALTER TABLE extra_relation RENAME COLUMN owner_title TO owner_work;
ALTER TABLE extra_relation RENAME COLUMN extra_title TO extra_work;

CREATE INDEX sequence_member_work ON sequence_member(member_work) WHERE member_work IS NOT NULL;
CREATE INDEX position_work ON position(work_id);
CREATE INDEX version_work ON version(work_id);

CREATE TRIGGER sequence_member_found_ad AFTER DELETE ON sequence_member WHEN old.member_work IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = old.sequence_id
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER sequence_member_found_ai AFTER INSERT ON sequence_member WHEN new.member_work IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = new.sequence_id
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;

CREATE TRIGGER sequence_member_found_au AFTER UPDATE OF sequence_id, member_work ON sequence_member
WHEN old.sequence_id IS NOT new.sequence_id OR old.member_work IS NOT new.member_work BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (old.sequence_id, new.sequence_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
