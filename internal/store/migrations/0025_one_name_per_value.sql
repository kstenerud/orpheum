-- SPDX-License-Identifier: GPL-3.0-or-later
-- One value, one name, in every layer (D227 §6): the columns take the names the API and the node
-- already use or now use, a language is a language everywhere, the artwork kind 'thumb' is the
-- 'still' the API names, a stored probe's keys and a person's switches are rewritten, and the
-- setting keys named as commands read as predicates. The artwork CHECK changes, so artwork is
-- rebuilt, keeping its ids and its id sequence. An older binary would write 'thumb', which the new
-- CHECK refuses, so the step raises compatible_version.
-- orpheum:raises-compatible-version

ALTER TABLE artwork RENAME COLUMN lang TO language;
ALTER TABLE edition_text RENAME COLUMN lang TO language;
ALTER TABLE fetched_subtitle RENAME COLUMN lang TO language;
ALTER TABLE field_override RENAME COLUMN lang TO language;
ALTER TABLE group_text RENAME COLUMN lang TO language;
ALTER TABLE item_text RENAME COLUMN lang TO language;
ALTER TABLE stream RENAME COLUMN lang TO language;
ALTER TABLE stream RENAME COLUMN lang_raw TO container_language;
ALTER TABLE stream RENAME COLUMN title_raw TO container_title;
ALTER TABLE title RENAME COLUMN original_lang TO original_language;
ALTER TABLE title RENAME COLUMN identification TO identification_state;
ALTER TABLE collection RENAME COLUMN identification TO identification_state;
ALTER TABLE device RENAME COLUMN first_seen_at TO added_at;
ALTER TABLE device RENAME COLUMN last_seen_at TO last_signed_in_at;
ALTER TABLE item RENAME COLUMN created_at TO added_at;
ALTER TABLE fetched_subtitle RENAME COLUMN fetched_by TO fetched_by_user_id;
ALTER TABLE file RENAME COLUMN mtime_ms TO modified_at;
ALTER TABLE play_event RENAME COLUMN client_mono_ms TO happened_mono_ms;
ALTER TABLE secret RENAME COLUMN hint TO last_chars;
ALTER TABLE library_root RENAME COLUMN path TO root_path;

CREATE TABLE artwork_new (                                 -- class O/F; A when chosen
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind IN ('poster','backdrop','logo','banner','still','disc')),
  language    TEXT,
  source      TEXT NOT NULL,             -- 'nfo', 'tmdb', 'local'
  source_ref  TEXT,
  source_sha256 BLOB,                    -- the SHA-256 of the source image; its sizes live in cache/ under it
  is_chosen      INTEGER NOT NULL DEFAULT 0 CHECK (is_chosen IN (0,1)),  -- a person picked it
  removed_at  INTEGER
);
INSERT INTO artwork_new (id, item_id, kind, language, source, source_ref, source_sha256, is_chosen, removed_at)
SELECT id, item_id, CASE kind WHEN 'thumb' THEN 'still' ELSE kind END, language, source, source_ref, content_hash,
  is_chosen, removed_at
FROM artwork;
UPDATE sqlite_sequence SET seq = (SELECT seq FROM sqlite_sequence WHERE name = 'artwork')
WHERE name = 'artwork_new' AND EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'artwork');
DROP TABLE artwork;
ALTER TABLE artwork_new RENAME TO artwork;
CREATE UNIQUE INDEX artwork_chosen ON artwork(item_id, kind) WHERE is_chosen = 1;
CREATE INDEX artwork_item ON artwork(item_id, kind);

UPDATE file SET probe_json = replace(replace(replace(probe_json,
    '"languageTag":', '"language":'), '"title":', '"containerTitle":'), '"names":', '"formatNames":')
WHERE probe_json IS NOT NULL;

UPDATE preference SET axis = 'levelling' WHERE axis = 'loudness_levelling';
UPDATE preference SET value_json = CASE value_json WHEN 'on' THEN 'true' WHEN 'off' THEN 'false' ELSE value_json END
WHERE axis IN ('dialogue_boost', 'levelling');

UPDATE setting SET key = 'should_check_for_updates' WHERE key = 'check_for_updates';
UPDATE setting SET key = 'should_serve_debug_pprof' WHERE key = 'serve_debug_pprof';
UPDATE setting SET key = 'should_serve_debug_metrics' WHERE key = 'serve_debug_metrics';
UPDATE setting SET key = 'should_scan_on_node_hello' WHERE key = 'scan_on_node_hello';
UPDATE setting SET key = 'secret_last_chars_shown' WHERE key = 'secret_hint_chars';
