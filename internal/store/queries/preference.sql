-- SPDX-License-Identifier: GPL-3.0-or-later
-- A person's preferences (schema.md, preference; D24, D90, D230): one value per axis at a scope.
-- A value set equal to the starting one is still a choice, so it is stored, never deleted.

-- name: SetPreference :exec
INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
VALUES (sqlc.arg(user_id), sqlc.arg(scope), sqlc.arg(scope_id), sqlc.arg(axis), sqlc.arg(value_json), sqlc.arg(set_at))
ON CONFLICT (user_id, scope, scope_id, axis) DO UPDATE SET value_json = excluded.value_json, set_at = excluded.set_at;

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
  SELECT collection_member.collection_id FROM collection_member
  JOIN collection ON collection.id = collection_member.collection_id AND collection.kind = 'series'
  WHERE collection_member.member_title = version.title_id
  ORDER BY collection_member.collection_id LIMIT 1
), version.title_id) AS INTEGER) AS work_id
FROM file
JOIN version ON version.id = file.version_id AND version.removed_at IS NULL
WHERE file.id = sqlc.arg(file_id) AND file.removed_at IS NULL;
