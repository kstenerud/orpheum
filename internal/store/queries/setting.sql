-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The server's runtime settings (schema.md, setting; D181, D200). Every key the registry in
-- internal/core/settings declares has a row, written with its starting value on first start.

-- name: ListSettings :many
SELECT key, value_json, set_by_user_id, set_at, is_modified
FROM setting
ORDER BY key;

-- name: UpsertSetting :exec
INSERT INTO setting (key, value_json, set_by_user_id, set_at, is_modified)
VALUES (sqlc.arg(key), sqlc.arg(value_json), sqlc.arg(set_by_user_id), sqlc.arg(set_at), sqlc.arg(is_modified))
ON CONFLICT (key)
DO UPDATE SET value_json = excluded.value_json, set_by_user_id = excluded.set_by_user_id, set_at = excluded.set_at,
              is_modified = excluded.is_modified
-- Only when it changes something (D303, DL9).
WHERE (setting.value_json, setting.set_by_user_id, setting.set_at, setting.is_modified)
  IS NOT (excluded.value_json, excluded.set_by_user_id, excluded.set_at, excluded.is_modified);

-- name: GetSetting :one
SELECT key, value_json, set_by_user_id, set_at, is_modified
FROM setting
WHERE key = ?;

-- name: InsertSettingIfMissing :exec
INSERT INTO setting (key, value_json, set_by_user_id, set_at)
VALUES (sqlc.arg(key), sqlc.arg(value_json), NULL, sqlc.arg(set_at))
ON CONFLICT (key) DO NOTHING;

-- name: DeleteSetting :exec
-- Only for a row no setting reads: a known setting is reset, never deleted (D202).
DELETE FROM setting
WHERE key = sqlc.arg(key);

-- name: UpdateUnmodifiedSetting :execrows
-- Moves a setting nobody changed to a release's new starting value; a changed one is left alone.
UPDATE setting SET value_json = sqlc.arg(value_json), set_at = sqlc.arg(set_at)
WHERE key = sqlc.arg(key) AND is_modified = 0 AND value_json != sqlc.arg(value_json);

-- name: ClearSettingsSetByRemovedUser :exec
-- The settings a removed person changed stay; who changed them is forgotten (D305).
UPDATE setting SET set_by_user_id = NULL WHERE set_by_user_id = sqlc.arg(user_id);
