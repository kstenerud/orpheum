-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D257 section 1, D352 Q2 and its revision of 2026-10-29: what a player last said of its link on a
-- network, its speed with the spread of its samples and its steadiness level, each with when, is
-- kept here rather than in the core's memory, so a restart, a deploy among them, no longer forgets
-- it and the next playback starts on it (bandwidth-sampling-2026-10-29.md P2). Losing a row costs
-- one start a measured link, so it is a cache: rows said nothing of within link_speed_fresh_hours
-- are deleted.

CREATE TABLE link_memory (                                  -- class C (D257 section 1)
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

-- The rows said nothing of within the time a link is fresh, deleted as a player says more.
CREATE INDEX link_memory_said ON link_memory(said_at);
