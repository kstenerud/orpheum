-- SPDX-License-Identifier: GPL-3.0-or-later
-- D230: the last subtitle and audio language a person chose in a series, or among a film's
-- copies, follows its unwatched episodes, worked out at play time. A work's choice is kept at
-- the new scope 'work', whose id is its series' item or, for a film or an episode in no series,
-- its title's; a file keeps only what was last used in it. 'series', which nothing wrote, goes:
-- the work is the series for an episode. A file's subtitle_choice becomes subtitle_used, a choice
-- carried to it from another file (D219, replaced) is kept only where the person started the
-- file, since an unwatched one now follows the work, and the kept track's "title" is named
-- containerTitle, what it holds, beside the track's kind. A work's subtitle_language is the most
-- recent choice a person made in one of its files. A started file with no stored choice is left
-- to follow the work until it is next played, rather than kept at the subtitles it lacked.
-- The setting table loses its scope and user_id: no setting is a person's (their choices are
-- preferences, the architecture map's F6), and a setting key spells out its unit, as D227
-- section 6 asks (the owner, 2026-09-30), its stored row moving with it. An older binary names
-- the old keys and axes, so the step raises compatible_version.
-- orpheum:raises-compatible-version

CREATE TABLE preference_new (                              -- class A (D24, D57, D90, D103, D230)
  user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE RESTRICT,
  scope       TEXT NOT NULL CHECK (scope IN ('user','library','work','device','file')),
  scope_id    INTEGER NOT NULL DEFAULT 0,  -- 0 for 'user'; a work's is its series' or its title's item
  axis        TEXT NOT NULL,               -- a file's 'subtitle_offset_ms', 'subtitle_used',
                                           -- 'audio_used'; a work's 'subtitle_language',
                                           -- 'audio_language'; a person's own 'dialogue_boost',
                                           -- 'levelling'
  value_json  TEXT NOT NULL,               -- JSON; a switch is true or false, and unset is not false
  set_at      INTEGER NOT NULL,
  PRIMARY KEY (user_id, scope, scope_id, axis)
) WITHOUT ROWID;

INSERT INTO preference_new (user_id, scope, scope_id, axis, value_json, set_at)
SELECT user_id, scope, scope_id, axis, value_json, set_at FROM preference
WHERE axis <> 'subtitle_choice' AND scope IN ('user', 'library', 'device', 'file');

-- A work's language: the most recent choice made in one of its files, off or the chosen track's
-- language, kind and container title.
WITH made AS (
  SELECT preference.user_id, preference.value_json, preference.set_at, preference.scope_id AS file_id,
    coalesce((
      SELECT collection_member.collection_id FROM collection_member
      JOIN collection ON collection.id = collection_member.collection_id AND collection.kind = 'series'
      WHERE collection_member.member_title = version.title_id
      ORDER BY collection_member.collection_id LIMIT 1
    ), version.title_id) AS work_id
  FROM preference
  JOIN file ON file.id = preference.scope_id
  JOIN version ON version.id = file.version_id
  WHERE preference.scope = 'file' AND preference.axis = 'subtitle_choice'
    AND NOT ifnull(json_extract(preference.value_json, '$.isCarriedFromWork'), 0)
    AND (ifnull(json_extract(preference.value_json, '$.isOff'), 0)
      OR json_type(preference.value_json, '$.track') IS NOT NULL)
),
ranked AS (
  SELECT made.*, row_number() OVER (
    PARTITION BY made.user_id, made.work_id ORDER BY made.set_at DESC, made.file_id DESC
  ) AS recency
  FROM made
)
INSERT INTO preference_new (user_id, scope, scope_id, axis, value_json, set_at)
SELECT user_id, 'work', work_id, 'subtitle_language',
  CASE WHEN ifnull(json_extract(value_json, '$.isOff'), 0) THEN json_object('isOff', json('true'))
  ELSE json_object(
    'language', ifnull(json_extract(value_json, '$.track.language'), ''),
    'containerTitle', ifnull(json_extract(value_json, '$.track.title'), ''),
    'kind', CASE
      WHEN ifnull(json_extract(value_json, '$.track.isForced'), 0) THEN 'forced'
      WHEN ifnull(json_extract(value_json, '$.track.isHearingImpaired'), 0) THEN 'hearing_impaired'
      ELSE 'plain' END)
  END,
  set_at
FROM ranked WHERE recency = 1;

-- What was last used in a file: its own choice, or one carried to it that the person played with.
INSERT INTO preference_new (user_id, scope, scope_id, axis, value_json, set_at)
SELECT user_id, 'file', scope_id, 'subtitle_used',
  CASE WHEN json_type(value_json, '$.track') IS NULL THEN json_remove(value_json, '$.isCarriedFromWork')
  ELSE json_set(json_remove(value_json, '$.isCarriedFromWork'), '$.track', json_object(
    'language', ifnull(json_extract(value_json, '$.track.language'), ''),
    'containerTitle', ifnull(json_extract(value_json, '$.track.title'), ''),
    'kind', CASE
      WHEN ifnull(json_extract(value_json, '$.track.isForced'), 0) THEN 'forced'
      WHEN ifnull(json_extract(value_json, '$.track.isHearingImpaired'), 0) THEN 'hearing_impaired'
      ELSE 'plain' END))
  END,
  set_at
FROM preference
WHERE scope = 'file' AND axis = 'subtitle_choice'
  AND (NOT ifnull(json_extract(value_json, '$.isCarriedFromWork'), 0) OR EXISTS (
    SELECT 1 FROM play_position
    WHERE play_position.user_id = preference.user_id AND play_position.file_id = preference.scope_id));

DROP TABLE preference;
ALTER TABLE preference_new RENAME TO preference;

CREATE TABLE setting_new (                                 -- class A: the server's (D200)
  key         TEXT NOT NULL PRIMARY KEY, -- a name from the settings registry in code
  value_json  TEXT NOT NULL,             -- JSON
  set_by_user_id INTEGER REFERENCES user(id) ON DELETE RESTRICT,  -- NULL: the setup wizard
  set_at      INTEGER NOT NULL,
  -- Whether a person set it (D201 section 1): any manual set marks it, even to its starting
  -- value; "set to default" clears it; a release that changes a starting value moves every
  -- unmarked row to the new one.
  is_modified INTEGER NOT NULL DEFAULT 0 CHECK (is_modified IN (0, 1))
);
INSERT INTO setting_new (key, value_json, set_by_user_id, set_at, is_modified)
SELECT CASE key
    WHEN 'signin_failure_window_s' THEN 'signin_failure_window_seconds'
    WHEN 'session_idle_expiry_h' THEN 'session_idle_expiry_hours'
    WHEN 'session_last_used_write_interval_s' THEN 'session_last_used_write_interval_seconds'
    WHEN 'backup_age_warning_h' THEN 'backup_age_warning_hours'
    WHEN 'node_silent_after_s' THEN 'node_silent_after_seconds'
    WHEN 'node_restart_after_silent_s' THEN 'node_restart_after_silent_seconds'
    WHEN 'node_restart_after_exit_s' THEN 'node_restart_after_exit_seconds'
    WHEN 'job_first_retry_delay_s' THEN 'job_first_retry_delay_seconds'
    WHEN 'provider_timeout_s' THEN 'provider_timeout_seconds'
    WHEN 'tmdb_refresh_after_d' THEN 'tmdb_refresh_after_days'
    WHEN 'metadata_refresh_check_every_h' THEN 'metadata_refresh_check_every_hours'
    WHEN 'removed_item_artwork_kept_d' THEN 'removed_item_artwork_kept_days'
    WHEN 'media_token_idle_expiry_h' THEN 'media_token_idle_expiry_hours'
    WHEN 'hls_segment_length_s' THEN 'hls_segment_length_seconds'
    ELSE key END,
  value_json, set_by_user_id, set_at, is_modified
FROM setting WHERE scope = 'server';
DROP TABLE setting;
ALTER TABLE setting_new RENAME TO setting;
