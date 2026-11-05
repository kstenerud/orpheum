-- SPDX-License-Identifier: AGPL-3.0-or-later
-- What each player last said of its link returns to persistent.db (D427, amending D426 section 9):
-- a release that rebuilds cache.db would otherwise make every player measure its link again. The
-- table changes only when a player measures, so the hourly backup still skips an unchanged
-- database now that resource_use, written every minute, stays in cache.db. Its rows are in the
-- cache.db the step's binary discards, so none is carried; a link is measured at the next play.

CREATE TABLE link_memory (                                  -- class C (D257 section 1); persistent.db again since step 0073 (D427)
  -- Who measured it, "device 3" or a session with no device's "session <key>", and on which
  -- network, the address the core saw its request come from, as clientnet.Of names it.
  player           TEXT NOT NULL,
  network          TEXT NOT NULL,
  -- Its speed, kbit/s, and the spread of the natural logs of the samples it rested on; NULL while
  -- it has said only its steadiness.
  speed_kbps       INTEGER CHECK (speed_kbps > 0),
  speed_log_spread REAL CHECK (speed_log_spread >= 0),
  speed_at         INTEGER,
  -- Its steadiness level (D352 section 3); NULL while it has said only its speed.
  steadiness       INTEGER CHECK (steadiness BETWEEN 0 AND 3),
  steadiness_at    INTEGER,
  -- When it last said anything of the link, the later of the two: what a stale row is found by.
  said_at          INTEGER NOT NULL,
  CHECK ((speed_kbps IS NULL) = (speed_at IS NULL)),
  CHECK ((steadiness IS NULL) = (steadiness_at IS NULL)),
  CHECK (speed_at IS NOT NULL OR steadiness_at IS NOT NULL),
  PRIMARY KEY (player, network)
) WITHOUT ROWID, STRICT;
CREATE INDEX link_memory_said ON link_memory(said_at);
