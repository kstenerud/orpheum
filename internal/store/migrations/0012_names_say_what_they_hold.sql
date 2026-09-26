-- SPDX-License-Identifier: GPL-3.0-or-later
-- Names say what they hold (standards/go.md section 3; agent-failures A8): the naming audit of
-- 2026-09-26 renamed every column a reader had to look up. Units go in the name (size_bytes,
-- mtime_ms), times end in _at, booleans read as predicates, and no abbreviation needs decoding
-- (ident, grp, t_from). The owner chose to rename now, before anything is released.
-- The stored JSON of file.probe and file.error_detail is rewritten with the node's renamed
-- keys: bitRate is bitRateBps, sampleRate is sampleRateHz, and an error's owner, group and
-- count are ownerUid, groupGid and repeatCount. Both are written by this project in one fixed
-- form, so the keys are found by their quoted names.
-- An older binary names the old columns, so the step raises compatible_version.
-- orpheum:raises-compatible-version

ALTER TABLE file RENAME COLUMN size TO size_bytes;
ALTER TABLE file RENAME COLUMN mtime TO mtime_ms;
ALTER TABLE file RENAME COLUMN payload_len TO payload_bytes;
ALTER TABLE file RENAME COLUMN first_seen TO first_seen_at;
ALTER TABLE file RENAME COLUMN last_seen TO last_seen_at;

ALTER TABLE stream RENAME COLUMN file_id TO bytes_file;
ALTER TABLE stream RENAME COLUMN media_file TO video_file;
ALTER TABLE fetched_subtitle RENAME COLUMN media_file TO video_file;

ALTER TABLE title RENAME COLUMN ident TO identification;
ALTER TABLE collection RENAME COLUMN ident TO identification;

ALTER TABLE position RENAME COLUMN grp TO group_number;
ALTER TABLE group_text RENAME COLUMN grp TO group_number;

ALTER TABLE device RENAME COLUMN first_seen TO first_seen_at;
ALTER TABLE device RENAME COLUMN last_seen TO last_seen_at;
ALTER TABLE "grant" RENAME COLUMN last_seen TO last_seen_at;

ALTER TABLE secret RENAME COLUMN sealed TO ciphertext;

ALTER TABLE play_span RENAME COLUMN t_from TO started_at;
ALTER TABLE play_span RENAME COLUMN t_to TO ended_at;
ALTER TABLE play_span RENAME COLUMN pos_from_ms TO from_position_ms;
ALTER TABLE play_span RENAME COLUMN pos_to_ms TO to_position_ms;

ALTER TABLE favourite RENAME COLUMN at TO added_at;
ALTER TABLE user_rating RENAME COLUMN at TO rated_at;
ALTER TABLE watchlist RENAME COLUMN at TO added_at;
ALTER TABLE progress_hide RENAME COLUMN at TO hidden_at;

ALTER TABLE root_state RENAME COLUMN device TO filesystem_device;
ALTER TABLE root_state RENAME COLUMN mount_point TO is_mount_point;
ALTER TABLE root_state RENAME COLUMN withheld_since TO withheld_scan_started_at;
ALTER TABLE root_mount RENAME COLUMN device TO filesystem_device;
ALTER TABLE root_mount RENAME COLUMN present TO is_present;
ALTER TABLE root_mount RENAME COLUMN root TO root_path;

UPDATE file SET probe = replace(replace(probe, '"bitRate":', '"bitRateBps":'), '"sampleRate":', '"sampleRateHz":')
WHERE probe IS NOT NULL;
UPDATE file SET error_detail = replace(replace(replace(error_detail,
  '"owner":', '"ownerUid":'), '"group":', '"groupGid":'), '"count":', '"repeatCount":')
WHERE error_detail IS NOT NULL;

-- search_alt is search_alternates. The two FTS tables name their content table's columns, so
-- they and their triggers are rebuilt, which is safe because both are derivable (class C).
DROP TRIGGER item_text_ai;
DROP TRIGGER item_text_ad;
DROP TRIGGER item_text_au;
DROP TABLE item_fts;
DROP TABLE item_fts_tri;

ALTER TABLE item_text RENAME COLUMN search_alt TO search_alternates;

CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alternates, overview, content='item_text', content_rowid='rowid',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, search_alternates, content='item_text', content_rowid='rowid', tokenize='trigram'
);

CREATE TRIGGER item_text_ai AFTER INSERT ON item_text BEGIN
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.rowid, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.rowid, new.name, new.search_alternates);
END;
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.rowid, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.rowid, old.name, old.search_alternates);
END;
CREATE TRIGGER item_text_au AFTER UPDATE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.rowid, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.rowid, old.name, old.search_alternates);
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.rowid, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.rowid, new.name, new.search_alternates);
END;

INSERT INTO item_fts(item_fts) VALUES ('rebuild');
INSERT INTO item_fts_tri(item_fts_tri) VALUES ('rebuild');
