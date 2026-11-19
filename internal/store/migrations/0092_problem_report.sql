-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A problem report as it was gathered (D524 sections 2 to 4): who made it, when, from which version,
-- their own words, and everything else gathered as one JSON document: the device's facts, the
-- playback's state, the core's and each node's recent log or why it gave none, and the report's own
-- salt. It is kept as gathered, nothing in it made safe, for the household's admin alone; each
-- download renders it shareable or full. A report deletes itself 60 days after it was made.
CREATE TABLE problem_report (                               -- class A (D524)
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  -- Who made it; NULL once their account is deleted, the report kept for its own 60 days.
  user_id   INTEGER REFERENCES user(id) ON DELETE SET NULL,
  made_at   INTEGER NOT NULL,
  version   TEXT NOT NULL,
  words     TEXT NOT NULL,
  gathered_json TEXT NOT NULL CHECK (json_valid(gathered_json))
) STRICT;
CREATE INDEX problem_report_made_at ON problem_report(made_at);
CREATE INDEX problem_report_user_made ON problem_report(user_id, made_at);
