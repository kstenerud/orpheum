-- SPDX-License-Identifier: GPL-3.0-or-later
-- Runtime settings (schema.md, setting; D181, D200). Every key the registry in
-- internal/core/settings declares has a row, written with its starting value on first start.

-- name: ListServerSettings :many
SELECT key, value, set_by, set_at
FROM setting
WHERE scope = 'server'
ORDER BY key;

-- name: ListUserSettings :many
SELECT key, value, set_by, set_at
FROM setting
WHERE scope = 'user' AND user_id = ?
ORDER BY key;

-- name: UpsertServerSetting :exec
INSERT INTO setting (scope, user_id, key, value, set_by, set_at)
VALUES ('server', NULL, sqlc.arg(key), sqlc.arg(value), sqlc.arg(set_by), sqlc.arg(set_at))
ON CONFLICT (scope, ifnull(user_id, 0), key)
DO UPDATE SET value = excluded.value, set_by = excluded.set_by, set_at = excluded.set_at;

-- name: UpsertUserSetting :exec
INSERT INTO setting (scope, user_id, key, value, set_by, set_at)
VALUES ('user', sqlc.arg(user_id), sqlc.arg(key), sqlc.arg(value), sqlc.arg(set_by), sqlc.arg(set_at))
ON CONFLICT (scope, ifnull(user_id, 0), key)
DO UPDATE SET value = excluded.value, set_by = excluded.set_by, set_at = excluded.set_at;

-- name: GetServerSetting :one
SELECT key, value, set_by, set_at
FROM setting
WHERE scope = 'server' AND key = ?;

-- name: InsertServerSettingIfMissing :exec
INSERT INTO setting (scope, user_id, key, value, set_by, set_at)
VALUES ('server', NULL, sqlc.arg(key), sqlc.arg(value), NULL, sqlc.arg(set_at))
ON CONFLICT (scope, ifnull(user_id, 0), key) DO NOTHING;
