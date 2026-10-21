-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D328: availability is kept per place, a root or a disk mounted inside one, in one table. A root's
-- unavailable_since and a mount's missing_since move here; a root's scan outcome and a mount's
-- is_present stay the scans' facts, which say nothing of availability.
CREATE TABLE unavailable_place (                           -- class O (D328): a place that cannot be read now
  node_id   INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path      TEXT NOT NULL,             -- the root, or the disk's mount point inside it; NFC, absolute
  root_path TEXT NOT NULL,             -- the root it is, or is inside
  since     INTEGER NOT NULL,          -- the first time in a row it was found so
  cause     TEXT NOT NULL CHECK (cause IN ('not_answering', 'unread', 'missing', 'not_named')),
  PRIMARY KEY (node_id, path)
) STRICT;
CREATE INDEX unavailable_place_root ON unavailable_place(node_id, root_path);
-- A root unavailable after a complete scan was so for its disk not answering, or for its hello;
-- after one that did not complete, for that scan.
INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
SELECT node_id, root_path, root_path, unavailable_since,
  CASE WHEN last_scan_outcome = 'complete' THEN 'not_answering' ELSE 'unread' END
FROM root_state WHERE unavailable_since IS NOT NULL;
INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
SELECT node_id, mount_path, root_path, coalesce(missing_since, 0), 'missing'
FROM root_mount WHERE is_present = 0
ON CONFLICT (node_id, path) DO NOTHING;
ALTER TABLE root_state DROP COLUMN unavailable_since;
ALTER TABLE root_mount DROP COLUMN missing_since;
