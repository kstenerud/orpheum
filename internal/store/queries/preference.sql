-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A person's preferences (schema.md, preference; D24, D90, D230): one value per axis at a scope.
-- A value set equal to the starting one is still a choice, so it is stored, never deleted.

-- name: SetPreference :exec
INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
VALUES (sqlc.arg(user_id), sqlc.arg(scope), sqlc.arg(scope_id), sqlc.arg(axis), sqlc.arg(value_json), sqlc.arg(set_at))
ON CONFLICT (user_id, scope, scope_id, axis) DO UPDATE SET value_json = excluded.value_json, set_at = excluded.set_at
-- Only when it changes something (D303, DL9).
WHERE (preference.value_json, preference.set_at) IS NOT (excluded.value_json, excluded.set_at);

-- name: KeepPreferenceUnlessSet :exec
-- A value kept only where the axis has none at the scope: what a started file used, which a choice
-- made in the file has already set (D230).
INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
VALUES (sqlc.arg(user_id), sqlc.arg(scope), sqlc.arg(scope_id), sqlc.arg(axis), sqlc.arg(value_json), sqlc.arg(set_at))
ON CONFLICT (user_id, scope, scope_id, axis) DO NOTHING;

-- name: ListPreferencesAtScope :many
-- A person's preferences at one scope: a file's sync offset, say.
SELECT axis, value_json
FROM preference
WHERE user_id = ? AND scope = ? AND scope_id = ?
ORDER BY axis;

-- name: GetWorkOfFile :one
-- The work a live file is of (D230): the series its title is an episode of, or the title itself,
-- a film or an episode in no series, as the item that names it.
SELECT CAST(coalesce((
  SELECT series_member.sequence_id FROM series_member
  JOIN sequence ON sequence.id = series_member.sequence_id AND sequence.kind = 'series'
  WHERE series_member.member_work = version.work_id
  ORDER BY series_member.sequence_id LIMIT 1
), version.work_id) AS INTEGER) AS work_id
FROM file_place AS own
JOIN version ON version.id = (
  SELECT file_filing.version_id FROM file_filing WHERE file_filing.file_id = own.file_id AND file_filing.version_id IS NOT NULL
  ORDER BY file_filing.library_kind = 'other', file_filing.library_kind LIMIT 1) AND version.removed_at IS NULL
WHERE own.file_id = CAST(sqlc.arg(file_id) AS INTEGER);

-- name: DeletePreferencesOfUser :exec
-- A removed person's preferences go with them (D173 section 4.6).
DELETE FROM preference WHERE user_id = sqlc.arg(user_id);

-- name: DeletePreferencesOfPurgedFiles :exec
-- A removed file deleted past its keeping takes what each person chose in it.
DELETE FROM preference
WHERE scope = 'file' AND scope_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: MoveFilePreferencesToFile :exec
-- What each person chose in a file confirmed to be a copy of another (D363 section 2): the other's,
-- where they chose nothing of it there.
UPDATE preference SET scope_id = sqlc.arg(to_file)
WHERE preference.scope = 'file' AND preference.scope_id = sqlc.arg(from_file)
  AND NOT EXISTS (
    SELECT 1 FROM preference AS kept
    WHERE kept.user_id = preference.user_id AND kept.scope = 'file' AND kept.scope_id = sqlc.arg(to_file)
      AND kept.axis = preference.axis);

-- name: DeleteFilePreferencesOfFile :exec
DELETE FROM preference WHERE scope = 'file' AND scope_id = sqlc.arg(file_id);
