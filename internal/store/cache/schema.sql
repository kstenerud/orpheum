-- SPDX-License-Identifier: AGPL-3.0-or-later
-- cache.db, never backed up (schema.md, cache.db). Versioned by discard (migrations.md): when
-- the version below differs from the file's user_version, the file is deleted and built again
-- from this schema. Raise it on any change. Nothing here is referenced by persistent.db, and
-- nothing here references it with a foreign key: SQLite cannot enforce one across files.
-- cache schema version 51

CREATE TABLE job (                                         -- the one work queue (D193)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('fetch_metadata','make_artwork_sizes','extract_subtitle','convert_fetched_subtitle','read_through','make_chunks','scan','examine','backup','renew_certificate','hash_whole_file','make_system_videos')),
  -- What it works on, one column by kind (D227 section 2): fetch_metadata and make_artwork_sizes
  -- an item, extract_subtitle the file its tracks are in (D340), convert_fetched_subtitle a fetched subtitle,
  -- read_through and hash_whole_file a file, make_chunks a chunk store, scan a scan (its listing), examine a scan's
  -- batch. At most one is set;
  -- backup, renew_certificate and make_system_videos set none: they work on the whole server (D243
  -- sections 6, 11; D435).
  item_id             INTEGER,
  stream_id           INTEGER,
  fetched_subtitle_id INTEGER,
  file_id             INTEGER,
  scan_id             INTEGER,
  scan_batch_id       INTEGER,
  chunk_store_id      INTEGER,
  -- A kind's own arguments: fetch_metadata's isRefresh and shouldRetryImages; make_artwork_sizes'
  -- artworkSource, refs, sourceSha256 and sizeNames. The other kinds store '{}'.
  args_json   TEXT NOT NULL DEFAULT '{}',
  priority    INTEGER NOT NULL DEFAULT 0,  -- higher runs first; "play this now" is highest
  -- 'assigned' is a job whose work a node does: no runner waits for it (D234 section 7).
  state       TEXT NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued','running','assigned','done','failed','cancelled')),
  -- What it needs that can be down (D231 section 5): 'tmdbApi', 'node:3', 'root:3:/media/films';
  -- named when it is queued, or the one it was last parked on. A claim passes over the parked ones.
  destination TEXT,
  runnable_at INTEGER NOT NULL,          -- not before this: rate limits and retry back-off (D170)
  -- Whether a queued job waits for a time it was given (a rate limit, a hold), which a repeat
  -- request never brings forward; 0 when it waits on its start or its back-off (D231 section 8).
  is_snoozed  INTEGER NOT NULL DEFAULT 0 CHECK (is_snoozed IN (0, 1)),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  -- Whether its last attempt ended with the tool failing (a stall, a panic): a second in a row is
  -- the job's own failure (D233 section 6).
  last_attempt_tool_failed INTEGER NOT NULL DEFAULT 0 CHECK (last_attempt_tool_failed IN (0, 1)),
  progress_fraction REAL,                -- 0..1 when known
  reason      TEXT,                      -- why it exists, shown to the user
  last_failure TEXT,                     -- FFmpeg's exit and stderr tail, the provider's reply
  created_at  INTEGER NOT NULL,
  finished_at INTEGER,
  -- When an assigned job's time runs out, after which it is its own failure; kept while its
  -- outcome is taken, and NULL once that is written.
  assigned_until INTEGER,
  -- Until when a person waits for play-driven work (D234 section 3, D240 section 6): each play that
  -- asks for it moves it on; past it, the job falls to background priority. NULL for other work.
  wanted_until INTEGER,
  CHECK ((item_id IS NOT NULL) + (stream_id IS NOT NULL) +
         (fetched_subtitle_id IS NOT NULL) + (file_id IS NOT NULL) + (scan_id IS NOT NULL) +
         (scan_batch_id IS NOT NULL) + (chunk_store_id IS NOT NULL) <= 1)
) STRICT;
-- The paths the queue's statements take (D303; the independent audit's S4): each read the whole
-- table, an enqueue's once per job queued, so a sweep that queued N jobs read the queue N times.
-- A claim takes the queued jobs of its kind in order.
CREATE INDEX job_claim ON job(kind, priority DESC, runnable_at, id) WHERE state = 'queued';
-- An open job of a kind on a target: an enqueue's look for one to merge into, and a wait's mark.
CREATE INDEX job_open_target ON job(kind, item_id, stream_id, fetched_subtitle_id, file_id,
  scan_id, scan_batch_id, chunk_store_id) WHERE state IN ('queued', 'running', 'assigned');
-- The open jobs about an item, and about a file: the work a person's interest raises, looked up by
-- the items and files it is in (D324 section 3; the audit of 2026-10-25, C4).
CREATE INDEX job_open_item ON job(item_id) WHERE state IN ('queued', 'running', 'assigned');
CREATE INDEX job_open_file ON job(file_id) WHERE state IN ('queued', 'running', 'assigned');
-- The jobs in a state: the running ones a start requeues, the assigned ones whose time ran out.
CREATE INDEX job_state ON job(state, kind, assigned_until);
-- The jobs a person waits for, which fall to background priority once no one does.
CREATE INDEX job_wanted ON job(wanted_until) WHERE wanted_until IS NOT NULL;
-- The jobs that ended, which are pruned once old.
CREATE INDEX job_finished ON job(finished_at) WHERE finished_at IS NOT NULL;

CREATE TABLE scan (                                        -- D240: a root's scan in progress
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id     INTEGER NOT NULL,          -- persistent.db's node, whose root it is
  root_path   TEXT NOT NULL,
  priority    INTEGER NOT NULL,          -- its jobs': a person's request, or the queue's own
  started_at  INTEGER NOT NULL,
  -- When its listing was taken, NULL before: from then the ingest holds what it compared, which a
  -- restart loses, so such a scan starts again.
  listed_at   INTEGER,
  listed_count  INTEGER NOT NULL DEFAULT 0,  -- the files its listing found (D236 section 2)
  to_examine_count INTEGER NOT NULL DEFAULT 0,  -- of them, those it needs read
  examined_count INTEGER NOT NULL DEFAULT 0, -- and of those, read
  -- Whether a scan was asked for while this one ran: one more follows it (D236 section 3).
  is_another_scan_queued INTEGER NOT NULL DEFAULT 0 CHECK (is_another_scan_queued IN (0, 1)),
  -- What it lists, relative to the root: '.' for the whole root, or the folder a watcher saw change,
  -- whose scan removes nothing outside it (D272 section 9).
  scope_path  TEXT NOT NULL DEFAULT '.',
  -- The scope of the one that follows, widened to cover every request made while this one ran.
  again_scope_path TEXT,
  -- When its listing was last handed to its node, NULL before: until then a scan asked for of the root
  -- joins this one, whose listing will see what the ask is about.
  listing_sent_at INTEGER,
  -- JSON: what it reads again, changed on disk or not, relative to the root: folders and files, '.' for
  -- every file (a full rescan, D392 section 2); an empty list, or NULL, for none, a scan that reads only
  -- what is new or changed. Every NFO and subtitle file in its scope is read again too when it names any.
  read_again_json TEXT,
  -- What the one that follows it reads again, every request made while this one ran together.
  again_read_again_json TEXT,
  UNIQUE (node_id, root_path)
) STRICT;

CREATE TABLE scan_batch (                                  -- D240 section 8: a batch to examine
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  scan_id     INTEGER NOT NULL REFERENCES scan(id) ON DELETE CASCADE,
  files_json  TEXT NOT NULL,             -- the files, as the examine command names them
  file_count  INTEGER NOT NULL,
  is_done     INTEGER NOT NULL DEFAULT 0 CHECK (is_done IN (0, 1))
) STRICT;
CREATE INDEX scan_batch_of_scan ON scan_batch(scan_id);

CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  item_id     INTEGER NOT NULL,          -- a film, or a series (D209)
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, provider_poster_path TEXT,
  votes       INTEGER,                   -- D301: the most voted settles a year shared
  PRIMARY KEY (item_id, rank)
) STRICT;

-- D357: an admin's rescan in progress, of one library or of one film or series in it: its scope's
-- files scanned, then every film and series in it matched again from what its files say. Kept as a
-- scan is, from the request to its end; lost with cache.db, it is asked for again.
CREATE TABLE rescan (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  library_id  INTEGER NOT NULL,          -- persistent.db's library
  item_id     INTEGER,                   -- the one film or series it rescans; NULL for the whole library
  requested_at INTEGER NOT NULL,
  -- When its scan was over and its films and series were listed to match again; NULL while it scans.
  listed_at   INTEGER,
  item_count  INTEGER NOT NULL DEFAULT 0,  -- the films and series it listed
  -- Of them, those matched again: to another work, to the same, left alone (a person's choice,
  -- an NFO's), left unsettled (several, or none, to choose from), or failed.
  changed_count   INTEGER NOT NULL DEFAULT 0,
  unchanged_count INTEGER NOT NULL DEFAULT 0,
  left_alone_count INTEGER NOT NULL DEFAULT 0,
  unsettled_count INTEGER NOT NULL DEFAULT 0,
  failed_count    INTEGER NOT NULL DEFAULT 0,
  -- Whether it is a full rescan: every file in its scope read again, changed on disk or not, and what
  -- a difference touches thrown out (D392).
  is_full     INTEGER NOT NULL DEFAULT 0 CHECK (is_full IN (0, 1))
) STRICT;
-- One rescan of a scope at a time: a second press while one runs adds nothing (D357 section 5).
CREATE UNIQUE INDEX rescan_scope ON rescan(library_id, coalesce(item_id, 0));

-- D357: a film or series a rescan matches again, and what its files and NFOs say it is, read once
-- the scan was over: the fetch_metadata job that runs for it, whichever, matches it from these.
CREATE TABLE rematch (
  item_id     INTEGER PRIMARY KEY,       -- persistent.db's film or series
  rescan_id   INTEGER NOT NULL REFERENCES rescan(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,             -- the name and year to search for
  year        INTEGER NOT NULL,          -- 0 for none
  ids_json    TEXT NOT NULL              -- JSON: the provider ids they give, by provider
) STRICT;
CREATE INDEX rematch_of_rescan ON rematch(rescan_id);

CREATE TABLE art_served (                                  -- D212 section 6: recency, to the day
  source_sha256_hex TEXT PRIMARY KEY,    -- an image's directory under cache/art/
  last_served_epoch_day INTEGER NOT NULL  -- days since the Unix epoch it was last served
) WITHOUT ROWID, STRICT;

CREATE TABLE chunk_store (                                 -- D247 section 10: one stream of a film on one grid
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  source_file INTEGER NOT NULL,          -- persistent.db's file
  stream      TEXT NOT NULL,             -- the stream's name: picture, q1200k480p, sound0, sound0-boost
  -- The hash of the grid it is cut on, then of the probe's facts it is made from (D392 section 3)
  store_key   TEXT NOT NULL,
  UNIQUE (source_file, stream, store_key)
) STRICT;

CREATE TABLE node_attempt (                                -- D234 section 7: work given to a node
  attempt_token TEXT PRIMARY KEY,        -- made for this sending; every result names it (D238 section 2)
  job_id      INTEGER NOT NULL,          -- the assigned job it is an attempt at; resolving it forgets this
  job_kind    TEXT NOT NULL,             -- the job's kind, whose answers resolve it
  node_id     INTEGER NOT NULL,          -- persistent.db's node it was sent to
  -- The start of that node it was given to, as its hello named it: a hello naming another start
  -- is a node that started again and lost it, whether or not the core restarted too (D238 section 3).
  node_start_id TEXT NOT NULL,
  work_json   TEXT NOT NULL,             -- JSON: the work as sent, without the bytes it carried
  sent_at     INTEGER NOT NULL
) STRICT;
CREATE INDEX node_attempt_job ON node_attempt(job_id);

-- Telemetry, written every minute or at each player's word: here, not in persistent.db, so the
-- hourly backup can skip a persistent.db nothing changed (sql.md section 13, M8; D426 section 9).
-- A discarded cache forgets them: a link is measured again, and the System page's hours start over.
CREATE TABLE resource_use (                                 -- class C (toolkit.md); cache.db since step 0070 (D426 section 9)
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
