-- SPDX-License-Identifier: AGPL-3.0-or-later
-- What the core and each node used of their machines, once a minute, so that what a server needs
-- can be judged from what one used, beside what it was doing (the owner, 2026-11-03; toolkit.md,
-- "Core and node record their own CPU and memory use"). Losing a row loses only its minute's
-- figure: rows older than resourceuse's keeping are deleted as new ones are written.

CREATE TABLE resource_use (                                 -- class C (toolkit.md)
  -- When it was measured, by the core's clock, and whose: the core's own, or a node's by its name.
  at           INTEGER NOT NULL,
  process      TEXT NOT NULL CHECK (process IN ('core', 'node')),
  node_name    TEXT NOT NULL DEFAULT '' CHECK ((process = 'node') = (node_name <> '')),
  -- What was measured: 'cgroup', the process's group with what it started, a node's FFmpeg runs
  -- among them; or 'process', the process alone (selfuse.Scope).
  scope        TEXT NOT NULL CHECK (scope IN ('cgroup', 'process')),
  -- The CPU time it used since its row before, in microseconds, over elapsed_ms of its own clock;
  -- NULL for the first row after a start, or after a restart reset its counter.
  cpu_micros   INTEGER CHECK (cpu_micros >= 0),
  elapsed_ms   INTEGER CHECK (elapsed_ms > 0),
  -- The memory it held, the files' pages kept for it, and the most it had held since its start;
  -- peak_bytes NULL where the kernel does not say.
  held_bytes   INTEGER NOT NULL CHECK (held_bytes >= 0),
  cache_bytes  INTEGER NOT NULL CHECK (cache_bytes >= 0),
  peak_bytes   INTEGER CHECK (peak_bytes >= 0),
  -- What it was doing: on the core's row, the playbacks playing; on a node's, the work it was
  -- running, as a JSON object of each kind of job's count, {"make_chunks":2}.
  plays        INTEGER NOT NULL DEFAULT 0 CHECK (plays >= 0),
  running_json TEXT NOT NULL DEFAULT '{}',
  CHECK ((cpu_micros IS NULL) = (elapsed_ms IS NULL)),
  PRIMARY KEY (at, process, node_name)
) WITHOUT ROWID, STRICT;
