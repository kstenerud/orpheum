-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A failure of unknown cause, kept with everything it said (D511 section 2): the kind of work that
-- failed and what it was for, its error and its output in full, the server's version and when it
-- last failed, and how many times. Its thing is unavailable on account of it, and not tried again
-- while the server runs (section 3); at each start, the failures of an earlier start than the one
-- they were last tried on are tried once more (section 4). They are what D509 counts, and what the
-- Needs attention page and its export give out (D511 section 6).
CREATE TABLE unknown_failure (                              -- class O (D511)
  kind            TEXT NOT NULL CHECK (kind <> ''),
  -- What the failure was for, as its kind names it: a job's target, a file, an item.
  subject         TEXT NOT NULL CHECK (subject <> ''),
  error_text      TEXT NOT NULL,
  -- What the tool said besides its error, FFmpeg's standard error among it; '' when it said none.
  output          TEXT NOT NULL DEFAULT '',
  version         TEXT NOT NULL,
  first_failed_at INTEGER NOT NULL,
  last_failed_at  INTEGER NOT NULL CHECK (last_failed_at >= first_failed_at),
  failure_count   INTEGER NOT NULL CHECK (failure_count > 0),
  -- The start of the server it last failed on (a random name, D511 section 4): one an earlier start
  -- left is tried once more at the next.
  start_id        TEXT NOT NULL,
  PRIMARY KEY (kind, subject)
) WITHOUT ROWID, STRICT;
