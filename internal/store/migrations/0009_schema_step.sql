-- SPDX-License-Identifier: GPL-3.0-or-later
-- The checksum of every step as it was applied. A step is never edited once committed, but one
-- written and applied, then edited before it was committed, leaves a database whose schema is
-- not the step's (it happened: plan-deviations P17). The runner records each step's checksum and
-- refuses a database where an applied step's text has since changed. Steps applied before this
-- one are recorded, when it runs, with the text this binary carries.
-- Additive: an older binary ignores the table, so compatible_version stays where it is.

CREATE TABLE schema_step (                                 -- class I: the runner's own record
  version INTEGER PRIMARY KEY,
  sha256  TEXT NOT NULL                  -- hex, of the step file's bytes
);
