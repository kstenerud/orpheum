-- SPDX-License-Identifier: GPL-3.0-or-later
-- M3's closing audit, under D227: a session's last use is last_used_at, as the API already names
-- it, and the setting that paces its write follows; a file's reported path is reported_path_raw,
-- the bytes the node gave, since a column ending in _bytes counts bytes; and a stored provider
-- response keeps the time its body was fetched apart from the time the provider was last asked,
-- which a refresh that finds the item gone upstream stamps without a new body (D213 section 4).
-- provider_response is rebuilt so the new column needs no default; a response fetched before
-- this step was last asked when it was fetched, as far as anything recorded says. An older
-- binary names the old columns, so the step raises compatible_version.
-- orpheum:raises-compatible-version

ALTER TABLE "grant" RENAME COLUMN last_seen_at TO last_used_at;
ALTER TABLE file RENAME COLUMN reported_path_bytes TO reported_path_raw;

CREATE TABLE provider_response_new (                       -- class F: the stored input (D170)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  part        TEXT NOT NULL DEFAULT '',  -- '' the item itself; 'season/N' a series' season listing
  fetched_at  INTEGER NOT NULL,          -- when body_gzip was fetched: the body's age
  last_asked_at INTEGER NOT NULL,        -- when the provider was last asked, with or without a new body
  body_gzip   BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
  PRIMARY KEY (item_id, provider, part)
);
INSERT INTO provider_response_new (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
SELECT item_id, provider, part, fetched_at, fetched_at, body_gzip FROM provider_response;
DROP TABLE provider_response;
ALTER TABLE provider_response_new RENAME TO provider_response;

UPDATE setting SET key = 'session_last_used_write_interval_s' WHERE key = 'session_last_seen_write_interval_s';
