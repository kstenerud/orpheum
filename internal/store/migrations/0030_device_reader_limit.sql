-- SPDX-License-Identifier: GPL-3.0-or-later
-- D240 section 5: how many may read one of a node's devices at once, as an admin set it for that
-- device, over the setting for its kind; a device with no row takes its kind's.

CREATE TABLE device_reader_limit (                          -- D240 section 5
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE CASCADE,
  device_key   TEXT NOT NULL,             -- the node's name for it: 'zfs:tank', 'disk:sdc'
  reader_limit INTEGER NOT NULL CHECK (reader_limit BETWEEN 1 AND 64),
  set_at       INTEGER NOT NULL,
  PRIMARY KEY (node_id, device_key)
) WITHOUT ROWID;
