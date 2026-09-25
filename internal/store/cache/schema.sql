-- SPDX-License-Identifier: GPL-3.0-or-later
-- cache.db, never backed up (schema.md, cache.db). Versioned by discard (migrations.md): when
-- the version below differs from the file's user_version, the file is deleted and built again
-- from this schema. Raise it on any change. Nothing here is referenced by library.db, and
-- nothing here references it with a foreign key: SQLite cannot enforce one across files.
-- prepared_rendition arrives with prepare in M3; the version rises then.
-- cache schema version 3

CREATE TABLE job (                                         -- the one work queue (D193)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  kind        TEXT NOT NULL CHECK (kind IN ('fetch','artwork','prepare','scan','backup')),
  subject     INTEGER,                   -- item_id, file_id, library_id, per kind
  args        TEXT NOT NULL DEFAULT '{}',  -- JSON: provider, profile, reason
  priority    INTEGER NOT NULL DEFAULT 0,  -- higher runs first; "play this now" is highest
  state       TEXT NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued','running','done','failed','cancelled')),
  node_id     INTEGER,                   -- where it runs, for node work
  not_before  INTEGER NOT NULL,          -- rate limits and retry back-off (D170)
  attempts    INTEGER NOT NULL DEFAULT 0,
  progress    REAL,                      -- 0..1 when known
  reason      TEXT,                      -- why it exists, shown to the user
  last_error  TEXT,                      -- FFmpeg's exit and stderr tail, the provider's reply
  created_at  INTEGER NOT NULL,
  finished_at INTEGER
);
CREATE INDEX job_next ON job(state, priority DESC, not_before) WHERE state = 'queued';

CREATE TABLE match_candidate (                             -- D172: kept so choosing costs nothing
  item_id     INTEGER NOT NULL,          -- a film, or a series (D209)
  rank        INTEGER NOT NULL,
  provider    TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  name        TEXT, year INTEGER, runtime_ms INTEGER, poster_ref TEXT,
  PRIMARY KEY (item_id, rank)
);
