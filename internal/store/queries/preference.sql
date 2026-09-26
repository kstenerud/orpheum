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
