-- SPDX-License-Identifier: GPL-3.0-or-later
-- People and their roles (schema.md, People; D125, D130). schema.md defines no live_user view,
-- so these read the table and name the tombstone condition themselves.

-- name: GetRoleByName :one
SELECT id, name, builtin, capabilities
FROM role
WHERE name = ?;

-- name: CountLiveUsers :one
SELECT count(*) FROM user WHERE removed_at IS NULL;

-- name: InsertUser :one
INSERT INTO user (name, role_id, password_hash, outsider, created_at)
VALUES (sqlc.arg(name), sqlc.arg(role_id), sqlc.arg(password_hash), sqlc.arg(outsider), sqlc.arg(created_at))
RETURNING id;

-- name: GetLiveUserByName :one
SELECT user.id, user.name, user.password_hash, user.outsider, role.name AS role_name, role.capabilities
FROM user
JOIN role ON role.id = user.role_id
WHERE user.name = ? AND user.removed_at IS NULL;

-- name: GetLiveUserByID :one
SELECT user.id, user.name, user.password_hash, user.outsider, role.name AS role_name, role.capabilities
FROM user
JOIN role ON role.id = user.role_id
WHERE user.id = ? AND user.removed_at IS NULL;

-- name: UpdateUserPasswordHash :exec
UPDATE user SET password_hash = sqlc.arg(password_hash)
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: ListLiveUsers :many
-- Everyone, for the admin's list of people: whether each has set a password yet.
SELECT user.id, user.name, role.name AS role_name, user.outsider, user.password_hash IS NOT NULL AS has_password,
  user.created_at
FROM user
JOIN role ON role.id = user.role_id
WHERE user.removed_at IS NULL
ORDER BY user.name COLLATE NOCASE, user.id;
