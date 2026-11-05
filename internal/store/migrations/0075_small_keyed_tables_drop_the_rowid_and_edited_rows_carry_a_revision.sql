-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D426 section 10's rest. The small tables looked up by their whole key alone are stored by it
-- (WITHOUT ROWID): a library's roots, each root's last scan, state and mounts, the fields a person
-- set by hand, and an extra's tie to its work. A row's key was a second B-tree beside a rowid
-- table, so each read went through two. The rows people edit carry a revision, raised by every
-- write to them (sql.md M1): a field set by hand, a library, a user, a role and a setting. Every
-- row is kept, at revision 0.

CREATE TEMP TABLE library_root_held AS SELECT * FROM library_root;
DROP TABLE library_root;
CREATE TABLE library_root (                                -- class O: as the node reports it (D107)
  library_id INTEGER NOT NULL REFERENCES library(id) ON DELETE RESTRICT,
  node_id    INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path  TEXT NOT NULL, retired_at INTEGER,
  PRIMARY KEY (library_id, node_id, root_path)
) WITHOUT ROWID, STRICT;
INSERT INTO library_root (library_id, node_id, root_path, retired_at)
SELECT library_id, node_id, root_path, retired_at FROM library_root_held;
DROP TABLE library_root_held;
CREATE INDEX library_root_node ON library_root(node_id, root_path) WHERE retired_at IS NULL;

CREATE TEMP TABLE root_scan_held AS SELECT * FROM root_scan;
DROP TABLE root_scan;
CREATE TABLE root_scan (                                   -- class O
  node_id      INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path    TEXT NOT NULL,
  completed_at INTEGER NOT NULL,         -- the last scan of this root that finished
  PRIMARY KEY (node_id, root_path)
) WITHOUT ROWID, STRICT;
INSERT INTO root_scan (node_id, root_path, completed_at)
SELECT node_id, root_path, completed_at FROM root_scan_held;
DROP TABLE root_scan_held;

CREATE TEMP TABLE root_state_held AS SELECT * FROM root_state;
DROP TABLE root_state;
CREATE TABLE root_state (                                  -- class O
  node_id           INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path         TEXT NOT NULL,
  filesystem_device INTEGER,             -- st_dev at the last complete scan
  is_mount_point    INTEGER CHECK (is_mount_point IN (0, 1)),
  last_scan_outcome TEXT NOT NULL CHECK (last_scan_outcome IN ('complete', 'unreadable', 'withheld')),
  last_scan_at      INTEGER NOT NULL,
  withheld_scan_started_at INTEGER,      -- the withheld scan's start: what it did not see goes
  withheld_reason   TEXT,                --   once the admin confirms
  are_removals_approved INTEGER NOT NULL DEFAULT 0
    CHECK (are_removals_approved IN (0, 1)),  -- class A: the admin said the files are gone
  PRIMARY KEY (node_id, root_path)
) WITHOUT ROWID, STRICT;
INSERT INTO root_state (node_id, root_path, filesystem_device, is_mount_point, last_scan_outcome, last_scan_at, withheld_scan_started_at, withheld_reason, are_removals_approved)
SELECT node_id, root_path, filesystem_device, is_mount_point, last_scan_outcome, last_scan_at, withheld_scan_started_at, withheld_reason, are_removals_approved FROM root_state_held;
DROP TABLE root_state_held;

CREATE TEMP TABLE root_mount_held AS SELECT * FROM root_mount;
DROP TABLE root_mount;
CREATE TABLE root_mount (                                  -- class O
  node_id           INTEGER NOT NULL REFERENCES node(id) ON DELETE RESTRICT,
  root_path         TEXT NOT NULL,
  mount_path        TEXT NOT NULL,       -- NFC, absolute on the node, like file.path
  filesystem_device INTEGER NOT NULL,    -- st_dev when it was last seen where expected
  is_present        INTEGER NOT NULL CHECK (is_present IN (0, 1)),
  PRIMARY KEY (node_id, mount_path)
) WITHOUT ROWID, STRICT;
INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
SELECT node_id, root_path, mount_path, filesystem_device, is_present FROM root_mount_held;
DROP TABLE root_mount_held;

CREATE TEMP TABLE field_override_held AS SELECT * FROM field_override;
DROP TABLE field_override;
CREATE TABLE field_override (                              -- class A (D183)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  field       TEXT NOT NULL,             -- a field-registry name
  language    TEXT NOT NULL DEFAULT '',  -- '' for language-invariant fields
  override_value TEXT NOT NULL,          -- a frozen copy
  origin      TEXT NOT NULL,             -- 'typed', 'nfo', 'tmdb', 'tvdb' — display only
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  set_at      INTEGER NOT NULL,
  revision    INTEGER NOT NULL DEFAULT 0,  -- raised by every write (sql.md M1)
  PRIMARY KEY (item_id, field, language)
) WITHOUT ROWID, STRICT;
INSERT INTO field_override (item_id, field, language, override_value, origin, set_by_user_id, set_at)
SELECT item_id, field, language, override_value, origin, set_by_user_id, set_at FROM field_override_held;
DROP TABLE field_override_held;

CREATE TEMP TABLE extra_relation_held AS SELECT * FROM extra_relation;
DROP TABLE extra_relation;
CREATE TABLE extra_relation (                              -- class O, or A when asserted
  owner_work    INTEGER NOT NULL REFERENCES work(id) ON DELETE RESTRICT,
  extra_work    INTEGER NOT NULL REFERENCES work(id) ON DELETE RESTRICT,
  extra_type    TEXT NOT NULL CHECK (extra_type IN ('trailer','teaser','clip','featurette',
                  'behind_the_scenes','deleted_scene','interview','short','scene',
                  'theme_song','theme_video','unknown')),       -- D97, D162: eleven + unknown
  asserted_by   TEXT NOT NULL CHECK (asserted_by IN ('scanner','user')),
  PRIMARY KEY (owner_work, extra_work)
) WITHOUT ROWID, STRICT;
INSERT INTO extra_relation (owner_work, extra_work, extra_type, asserted_by)
SELECT owner_work, extra_work, extra_type, asserted_by FROM extra_relation_held;
DROP TABLE extra_relation_held;

ALTER TABLE library ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE role ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE setting ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
