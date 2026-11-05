-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- The schema's names follow the API's (D416, D425; D426 section 7): a credit's job, not its role,
-- which is the word for what a user may do; a group, not a season, for an artwork's group and a
-- provider's listing of one, as an ordering's position already says; 'user', not 'person', for the
-- user who corrected an other video's recorded time, since a person is someone in the credits; and
-- item_hidden for the items a user hid from their progress. The renames carry every row. The other
-- videos, the invitations and the preferences are rebuilt, held aside and created again under their
-- own names as step 0068 does: the first for its new check, the other two for comments that called
-- a user an account or a person.

ALTER TABLE credit RENAME COLUMN role TO job;
ALTER TABLE artwork RENAME COLUMN season_number TO group_number;
ALTER TABLE progress_hide RENAME TO item_hidden;
UPDATE provider_response SET part = 'group/' || substr(part, length('season/') + 1)
WHERE part LIKE 'season/%';

CREATE TEMP TABLE other_video_held AS SELECT * FROM other_video;
DROP TABLE other_video;
CREATE TABLE other_video (                                 -- class C (D349 section 4)
  title_id      INTEGER PRIMARY KEY REFERENCES title(id) ON DELETE RESTRICT,
  recorded_at   INTEGER NOT NULL,          -- Unix milliseconds
  recorded_from TEXT NOT NULL CHECK (recorded_from IN ('tag','name','file','user')),
  file_name     TEXT NOT NULL,             -- its file's name, the last part of its path
  file_name_key BLOB NOT NULL              -- file_name's CLDR sort key in the sort_locale, BINARY order
) STRICT;
INSERT INTO other_video (title_id, recorded_at, recorded_from, file_name, file_name_key)
SELECT title_id, recorded_at, CASE recorded_from WHEN 'person' THEN 'user' ELSE recorded_from END, file_name,
  file_name_key
FROM other_video_held;
DROP TABLE other_video_held;
CREATE INDEX other_video_file_name ON other_video(file_name_key, title_id);
CREATE INDEX other_video_recorded ON other_video(recorded_at, title_id);

CREATE TEMP TABLE invitation_held AS SELECT * FROM invitation;
CREATE TEMP TABLE invitation_sequence_held AS SELECT seq FROM sqlite_sequence WHERE name = 'invitation';
DROP TABLE invitation;
CREATE TABLE invitation (                                  -- class A (D246 section 2, D318)
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  role_id       INTEGER NOT NULL REFERENCES role(id) ON DELETE RESTRICT,
  admin_note    TEXT NOT NULL CHECK (admin_note <> ''),
  issued_at     INTEGER NOT NULL,
  made_user_id  INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- the user its use made
  made_at       INTEGER,
  CHECK ((made_user_id IS NULL) = (made_at IS NULL))
) STRICT;
INSERT INTO invitation (id, role_id, admin_note, issued_at, made_user_id, made_at)
SELECT id, role_id, admin_note, issued_at, made_user_id, made_at FROM invitation_held;
DELETE FROM sqlite_sequence WHERE name = 'invitation';
INSERT INTO sqlite_sequence (name, seq) SELECT 'invitation', seq FROM invitation_sequence_held;
DROP TABLE invitation_held;
DROP TABLE invitation_sequence_held;
CREATE INDEX invitation_made_user ON invitation(made_user_id) WHERE made_user_id IS NOT NULL;
CREATE INDEX invitation_role ON invitation(role_id);

CREATE TEMP TABLE preference_held AS SELECT * FROM preference;
DROP TABLE preference;
CREATE TABLE preference (                                  -- class A (D24, D57, D90, D103, D230)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL CHECK (scope IN ('user','library','work','device','file')),
  scope_id    INTEGER NOT NULL DEFAULT 0,  -- 0 for 'user'; a work's is its series' or its title's item
  axis        TEXT NOT NULL,               -- a file's 'subtitle_offset_ms', 'subtitle_used',
                                           -- 'audio_used'; a work's 'subtitle_language',
                                           -- 'audio_language'; a user's own 'dialogue_boost',
                                           -- 'levelling'
  value_json  TEXT NOT NULL,               -- JSON; a switch is true or false, and unset is not false
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID, STRICT;
INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
SELECT user_id, scope, scope_id, axis, value_json, set_at FROM preference_held;
DROP TABLE preference_held;
CREATE INDEX preference_scope ON preference(scope, scope_id);
