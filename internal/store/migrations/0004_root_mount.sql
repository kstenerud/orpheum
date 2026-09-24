-- SPDX-License-Identifier: GPL-3.0-or-later
-- The mount boundaries inside each root (D208, D42): a directory on another device than its
-- parent, as the last complete scan saw it. With /mnt as a root and disks at /mnt/a and /mnt/b,
-- unplugging one leaves an empty directory that lists cleanly, and nothing about the root itself
-- changes; this is what tells the core that the subtree is missing rather than emptied.
-- A boundary a later scan does not see where it was, or sees on another device, is kept with
-- present = 0: nothing under it is removed until it returns or the admin says it is gone.
-- Additive: an older binary ignores the table, so compatible_version stays where it is.

CREATE TABLE root_mount (                                  -- class O
  node_id  INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root     TEXT NOT NULL,
  path     TEXT NOT NULL,                -- NFC, absolute on the node, like file.path
  device   INTEGER NOT NULL,             -- st_dev when it was last seen where expected
  present  INTEGER NOT NULL CHECK (present IN (0, 1)),
  PRIMARY KEY (node_id, path)
);
