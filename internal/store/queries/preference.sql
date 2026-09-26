-- SPDX-License-Identifier: GPL-3.0-or-later
-- A person's preferences (schema.md, preference; D24, D90): one value per axis at a scope. A
-- value set equal to the starting one is still a choice, so it is stored, never deleted.

-- name: SetPreference :exec
INSERT INTO preference (user_id, scope, scope_id, axis, value, set_at)
VALUES (sqlc.arg(user_id), sqlc.arg(scope), sqlc.arg(scope_id), sqlc.arg(axis), sqlc.arg(value), sqlc.arg(set_at))
ON CONFLICT (user_id, scope, scope_id, axis) DO UPDATE SET value = excluded.value, set_at = excluded.set_at;

-- name: ListPreferencesAt :many
-- A person's preferences at one scope: a file's sync offset, say.
SELECT axis, value
FROM preference
WHERE user_id = ? AND scope = ? AND scope_id = ?
ORDER BY axis;

-- name: DeletePreference :exec
DELETE FROM preference
WHERE user_id = sqlc.arg(user_id) AND scope = sqlc.arg(scope) AND scope_id = sqlc.arg(scope_id) AND axis = sqlc.arg(axis);

-- name: GetTitleOfFile :one
-- The title a live file is a version of.
SELECT version.title_id
FROM file
JOIN version ON version.id = file.version_id AND version.removed_at IS NULL
WHERE file.id = sqlc.arg(file_id) AND file.removed_at IS NULL;

-- name: ListFilesOfWork :many
-- The live files of a work, for carrying a choice across it (D219): of the title, or when the
-- title is an episode in a series, of every episode of that series.
SELECT DISTINCT file.id
FROM file
JOIN version ON version.id = file.version_id AND version.removed_at IS NULL
WHERE file.removed_at IS NULL AND (
  version.title_id = sqlc.arg(title_id)
  OR version.title_id IN (
    SELECT sibling.member_title FROM collection_member AS own
    JOIN collection_member AS sibling ON sibling.collection_id = own.collection_id
    JOIN collection ON collection.id = own.collection_id AND collection.kind = 'series'
    WHERE own.member_title = sqlc.arg(title_id)
  )
)
ORDER BY file.id;
