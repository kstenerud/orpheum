-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- A preference set on a film or a whole series has the scope 'item', not 'work' (D428): since step
-- 0074, a work is the playable thing (a movie, an episode, an extra, an other video), never a
-- series, so the one word would have meant two things. The CHECK names the new word, and every
-- stored row set at the old one moves to it, its value and time kept. An older binary writes
-- 'work', which the CHECK now refuses, so the compatible version rises.

CREATE TEMP TABLE preference_held AS SELECT * FROM preference;
DROP TABLE preference;
CREATE TABLE preference (                                  -- class A (D24, D57, D90, D103, D230, D428)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL CHECK (scope IN ('user','library','item','device','file')),
  scope_id    INTEGER NOT NULL DEFAULT 0,  -- 0 for 'user'; an item's is its film's or its series'
  axis        TEXT NOT NULL,               -- a file's 'subtitle_offset_ms', 'subtitle_used',
                                           -- 'audio_used'; an item's 'subtitle_language',
                                           -- 'audio_language', a series' 'episode_ordering';
                                           -- a user's own 'dialogue_boost', 'levelling'
  value_json  TEXT NOT NULL,               -- JSON; a switch is true or false, and unset is not false
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID, STRICT;
INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
SELECT user_id, CASE scope WHEN 'work' THEN 'item' ELSE scope END, scope_id, axis, value_json, set_at
FROM preference_held;
DROP TABLE preference_held;
CREATE INDEX preference_scope ON preference(scope, scope_id);
