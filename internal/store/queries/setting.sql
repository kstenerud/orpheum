-- SPDX-License-Identifier: GPL-3.0-or-later
-- Runtime settings (schema.md, setting; D181, D200). Every key the registry in
-- internal/core/settings declares has a row, written with its starting value on first start.

-- name: ListServerSettings :many
SELECT key, value, set_by, set_at, is_modified
FROM setting
WHERE scope = 'server'
ORDER BY key;

-- name: ListUserSettings :many
SELECT key, value, set_by, set_at, is_modified
FROM setting
WHERE scope = 'user' AND user_id = ?
ORDER BY key;

-- name: UpsertServerSetting :exec
INSERT INTO setting (scope, user_id, key, value, set_by, set_at, is_modified)
VALUES ('server', NULL, sqlc.arg(key), sqlc.arg(value), sqlc.arg(set_by), sqlc.arg(set_at), sqlc.arg(is_modified))
ON CONFLICT (scope, ifnull(user_id, 0), key)
DO UPDATE SET value = excluded.value, set_by = excluded.set_by, set_at = excluded.set_at,
              is_modified = excluded.is_modified;

-- name: UpsertUserSetting :exec
INSERT INTO setting (scope, user_id, key, value, set_by, set_at, is_modified)
VALUES ('user', sqlc.arg(user_id), sqlc.arg(key), sqlc.arg(value), sqlc.arg(set_by), sqlc.arg(set_at),
        sqlc.arg(is_modified))
ON CONFLICT (scope, ifnull(user_id, 0), key)
DO UPDATE SET value = excluded.value, set_by = excluded.set_by, set_at = excluded.set_at,
              is_modified = excluded.is_modified;

-- name: GetServerSetting :one
SELECT key, value, set_by, set_at, is_modified
FROM setting
WHERE scope = 'server' AND key = ?;

-- name: InsertServerSettingIfMissing :exec
INSERT INTO setting (scope, user_id, key, value, set_by, set_at)
VALUES ('server', NULL, sqlc.arg(key), sqlc.arg(value), NULL, sqlc.arg(set_at))
ON CONFLICT (scope, ifnull(user_id, 0), key) DO NOTHING;

-- name: ListSettingKeys :many
SELECT scope, user_id, key
FROM setting
ORDER BY scope, user_id, key;

-- name: UpdateUnmodifiedServerSetting :execrows
-- Moves a setting nobody changed to a release's new starting value; a changed one is left alone.
UPDATE setting SET value = sqlc.arg(value), set_at = sqlc.arg(set_at)
WHERE scope = 'server' AND key = sqlc.arg(key) AND is_modified = 0 AND value != sqlc.arg(value);
