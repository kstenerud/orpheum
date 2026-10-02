-- SPDX-License-Identifier: GPL-3.0-or-later
-- cache.db, never backed up (schema.md, cache.db). Versioned by discard (migrations.md): when
-- the version below differs from the file's user_version, the file is deleted and built again
-- from this schema. Raise it on any change. Nothing here is referenced by persistent.db, and
-- nothing here references it with a foreign key: SQLite cannot enforce one across files.
-- cache schema version 24

CREATE TABLE job (                                         -- the one work queue (D193)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('fetch_metadata','make_artwork_sizes','prepare','extract_subtitle','convert_fetched_subtitle','measure_loudness','scan','backup')),
  -- What it works on, one column by kind (D227 section 2): fetch_metadata and make_artwork_sizes
  -- an item, prepare a rendition, extract_subtitle a stream, convert_fetched_subtitle a fetched subtitle,
  -- measure_loudness a file. At most one is set; scan and backup, not yet jobs, set none.
  item_id             INTEGER,
  rendition_id        INTEGER,
  stream_id           INTEGER,
  fetched_subtitle_id INTEGER,
  file_id             INTEGER,
  -- A kind's own arguments: fetch_metadata's isRefresh and shouldRetryImages; make_artwork_sizes'
  -- artworkSource, refs, sourceSha256 and sizeNames. The other kinds store '{}'.
  args_json   TEXT NOT NULL DEFAULT '{}',
  priority    INTEGER NOT NULL DEFAULT 0,  -- higher runs first; "play this now" is highest
  -- 'assigned' is a job whose work a node does: no runner waits for it (D234 section 7).
  state       TEXT NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued','running','assigned','done','failed','cancelled')),
  -- What it needs that can be down (D231 section 5): 'tmdb-api', 'node:3', 'root:3:/media/films';
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
  CHECK ((item_id IS NOT NULL) + (rendition_id IS NOT NULL) + (stream_id IS NOT NULL) +
         (fetched_subtitle_id IS NOT NULL) + (file_id IS NOT NULL) <= 1)
);
CREATE INDEX job_next ON job(state, priority DESC, runnable_at) WHERE state = 'queued';

CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  item_id     INTEGER NOT NULL,          -- a film, or a series (D209)
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, provider_poster_path TEXT,
  PRIMARY KEY (item_id, rank)
);

CREATE TABLE art_served (                                  -- D212 section 6: recency, to the day
  source_sha256_hex TEXT PRIMARY KEY,    -- an image's directory under cache/art/
  last_served_epoch_day INTEGER NOT NULL  -- days since the Unix epoch it was last served
) WITHOUT ROWID;

CREATE TABLE prepared_rendition (                          -- D175, D180, D217 section 5
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  source_file  INTEGER NOT NULL,         -- persistent.db's file; play state records against it
  source_payload_hash BLOB NOT NULL,     -- the source as it was when prepared (D179)
  profile      TEXT NOT NULL CHECK (profile IN ('remux','h264-8m')),
  node_id      INTEGER NOT NULL,         -- the node that holds it, beside its source
  state        TEXT NOT NULL DEFAULT 'queued'
                 CHECK (state IN ('queued','running','complete','failed')),
  segment_length_seconds INTEGER NOT NULL,    -- seconds per segment; another setting remakes it
  run_id       TEXT NOT NULL,            -- names this making's directory on the node; new each time
  prepared_through_ms INTEGER NOT NULL DEFAULT 0,  -- how far into the source it is prepared
  size_bytes   INTEGER,                  -- its size once complete
  failure      TEXT,                     -- why it failed: FFmpeg's exit and stderr tail
  video_encoder TEXT,                    -- what made the video once complete: h264_qsv, h264_nvenc, x264, or copy (D225)
  realtime_factor REAL,                  -- seconds of the source per second of running, as FFmpeg last said (D225)
  carried_gains_db_json TEXT NOT NULL DEFAULT '[]',  -- JSON: the gain each audio track's sound carries, by its place (D226)
  audio_languages_json TEXT NOT NULL DEFAULT '[]',  -- JSON: the language each audio track is tagged with, by its place; another remakes it
  created_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  UNIQUE (source_file, profile)
);

CREATE TABLE node_attempt (                                -- D234 section 7: work given to a node
  attempt_token TEXT PRIMARY KEY,        -- made for this sending; every result names it (D238 section 2)
  job_id      INTEGER NOT NULL,          -- the assigned job it is an attempt at; resolving it forgets this
  job_kind    TEXT NOT NULL,             -- the job's kind, whose answers resolve it
  node_id     INTEGER NOT NULL,          -- persistent.db's node it was sent to
  work_json   TEXT NOT NULL,             -- JSON: the work as sent, without the bytes it carried
  sent_at     INTEGER NOT NULL
);
CREATE INDEX node_attempt_job ON node_attempt(job_id);
