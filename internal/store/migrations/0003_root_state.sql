-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each root's last scan and what it found (D208, D41, D42). The device and mount facts are the
-- ones the last complete scan saw, the known-good values a later scan is compared against: a
-- disk unplugged from its mount point has another device id, and its clean, empty listing must
-- not remove the library. The outcome says whether the files are available, and a scan whose
-- removals were withheld waits here for the admin.
-- Additive: an older binary ignores the table, so compatible_version stays where it is.

CREATE TABLE root_state (                                  -- class O
  node_id         INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  path            TEXT NOT NULL,
  device          INTEGER,                -- st_dev at the last complete scan
  mount_point     INTEGER CHECK (mount_point IN (0, 1)),
  outcome         TEXT NOT NULL CHECK (outcome IN ('complete', 'unreadable', 'withheld')),
  outcome_at      INTEGER NOT NULL,
  withheld_since  INTEGER,                -- the withheld scan's start: what it did not see goes
  withheld_reason TEXT,                   -- once the admin confirms
  PRIMARY KEY (node_id, path)
);
