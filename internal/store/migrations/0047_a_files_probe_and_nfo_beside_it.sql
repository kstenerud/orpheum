-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D303 (M8; the independent audit's S5): a file's row stays narrow. Its probe output and an NFO's
-- bytes, kilobytes each, sat beside columns every scan rewrites (last_seen_at, filing), so a scan's
-- stamp of an unchanged file wrote a page for each file, and a probe's change of size rewrote its
-- overflow pages. They move to rows of their own, written only when they change, and the probe's
-- duration, the one number the pass and Continue Watching read of it, is a column of its own.

CREATE TABLE file_probe (                                  -- class O: what the node probed (D59)
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  probe_json  TEXT NOT NULL,             -- the raw probe output, the input D59 replays
  duration_ms INTEGER NOT NULL DEFAULT 0 -- its format's duration; 0 when it gave none
);
INSERT INTO file_probe (file_id, probe_json, duration_ms)
  SELECT id, probe_json, CAST(coalesce(json_extract(probe_json, '$.format.durationMs'), 0) AS INTEGER)
  FROM file WHERE probe_json IS NOT NULL;

CREATE TABLE file_nfo (                                    -- class O: a rescan sends it again
  file_id     INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE RESTRICT,
  body        BLOB NOT NULL              -- the NFO's bytes, as the node sent them (D182)
);
INSERT INTO file_nfo (file_id, body) SELECT id, nfo_body FROM file WHERE nfo_body IS NOT NULL;

ALTER TABLE file DROP COLUMN probe_json;
ALTER TABLE file DROP COLUMN nfo_body;
