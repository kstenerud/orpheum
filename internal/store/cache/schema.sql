-- SPDX-License-Identifier: GPL-3.0-or-later
-- cache.db, never backed up (schema.md, cache.db). Versioned by discard (migrations.md): when
-- the version below differs from the file's user_version, the file is deleted and built again
-- from this schema. Raise it on any change. Nothing here is referenced by library.db, and
-- nothing here references it with a foreign key: SQLite cannot enforce one across files.
-- cache schema version 10

CREATE TABLE job (                                         -- the one work queue (D193)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('fetch','artwork','prepare','extract_subtitle','scan','backup')),
  subject     INTEGER,                   -- item_id, file_id, library_id, stream_id, per kind
  args        TEXT NOT NULL DEFAULT '{}',  -- JSON: provider, profile, reason
  priority    INTEGER NOT NULL DEFAULT 0,  -- higher runs first; "play this now" is highest
  state       TEXT NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued','running','done','failed','cancelled')),
  node_id     INTEGER,                   -- where it runs, for node work
  runnable_at INTEGER NOT NULL,          -- not before this: rate limits and retry back-off (D170)
  attempts    INTEGER NOT NULL DEFAULT 0,
  progress    REAL,                      -- 0..1 when known
  reason      TEXT,                      -- why it exists, shown to the user
  last_error  TEXT,                      -- FFmpeg's exit and stderr tail, the provider's reply
  created_at  INTEGER NOT NULL,
  finished_at INTEGER
);
CREATE INDEX job_next ON job(state, priority DESC, runnable_at) WHERE state = 'queued';

CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  item_id     INTEGER NOT NULL,          -- a film, or a series (D209)
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, poster_ref TEXT,
  PRIMARY KEY (item_id, rank)
);

CREATE TABLE art_served (                                  -- D212 section 6: recency, to the day
  hash        TEXT PRIMARY KEY,          -- an image's directory under cache/art/
  last_served_epoch_day INTEGER NOT NULL  -- days since the Unix epoch it was last served
) WITHOUT ROWID;

CREATE TABLE prepared_rendition (                          -- D175, D180, D217 section 5
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  source_file  INTEGER NOT NULL,         -- library.db's file; play state records against it
  payload_hash BLOB NOT NULL,            -- the source as it was when prepared (D179)
  profile      TEXT NOT NULL CHECK (profile IN ('remux','h264-8m')),
  node_id      INTEGER NOT NULL,         -- the node that holds it, beside its source
  state        TEXT NOT NULL DEFAULT 'queued'
                 CHECK (state IN ('queued','running','complete','failed')),
  segment_length_s INTEGER NOT NULL,     -- seconds per segment; another setting remakes it
  run_id       TEXT NOT NULL,            -- names this making's directory on the node; new each time
  prepared_ms  INTEGER NOT NULL DEFAULT 0,  -- how much of the source is prepared
  size_bytes   INTEGER,                  -- its size once complete
  error        TEXT,                     -- why it failed: FFmpeg's exit and stderr tail
  created_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  UNIQUE (source_file, profile)
);

CREATE TABLE subtitle_text (                               -- D217 section 6: by the video's payload hash
  payload_hash BLOB NOT NULL,            -- the video the track is in, as it was when extracted
  stream_index INTEGER NOT NULL,         -- the track's stream index in that video
  state        TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','complete','failed')),
  ass          BLOB,                     -- once complete: for JASSUB
  vtt          BLOB,                     -- once complete: for the cast receiver
  error        TEXT,                     -- why it failed: FFmpeg's exit and stderr tail
  created_at   INTEGER NOT NULL,
  finished_at  INTEGER,
  PRIMARY KEY (payload_hash, stream_index)
);
