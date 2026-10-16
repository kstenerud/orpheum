-- SPDX-License-Identifier: AGPL-3.0-or-later
-- People and their roles (schema.md, People; D125, D130). schema.md defines no live_user view,
-- so these read the table and name the tombstone condition themselves.

-- name: GetRoleByName :one
SELECT id, name, is_builtin, capabilities
FROM role
WHERE name = ?;

-- name: CountLiveUsers :one
SELECT count(*) FROM user WHERE removed_at IS NULL;

-- name: InsertUser :one
INSERT INTO user (name, role_id, password_hash, admin_note, created_at)
VALUES (sqlc.arg(name), sqlc.arg(role_id), sqlc.arg(password_hash), sqlc.arg(admin_note), sqlc.arg(created_at))
RETURNING id;

-- name: GetLiveUserByName :one
-- A name finds its person whatever the case of its ASCII letters (SQLite's NOCASE), and names are
-- unique in the same way, so no two people differ only by a capital (the owner, 2026-09-27).
SELECT user.id, user.name, user.password_hash, role.name AS role_name, role.capabilities
FROM user
JOIN role ON role.id = user.role_id
WHERE user.name = ? COLLATE NOCASE AND user.removed_at IS NULL;

-- name: GetLiveUserByID :one
SELECT user.id, user.name, user.password_hash, user.admin_note, user.role_id, role.name AS role_name,
  role.capabilities, user.admin_away
FROM user
JOIN role ON role.id = user.role_id
WHERE user.id = ? AND user.removed_at IS NULL;

-- name: SetAdminAway :exec
-- Whether an admin administers from away from home too (D302): 'never' or 'always'.
UPDATE user SET admin_away = sqlc.arg(admin_away) WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: UpdateUserPasswordHash :exec
UPDATE user SET password_hash = sqlc.arg(password_hash)
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: ListLiveUsers :many
-- Everyone, for the admin's list of people: whether each has set a password yet, and the admin's
-- note of who each is for (D246 section 2).
SELECT user.id, user.name, role.name AS role_name, user.admin_note, user.password_hash IS NOT NULL AS has_password,
  user.created_at
FROM user
JOIN role ON role.id = user.role_id
WHERE user.removed_at IS NULL
ORDER BY user.name COLLATE NOCASE, user.id;

-- name: SetUserAdminNote :exec
UPDATE user SET admin_note = sqlc.narg(admin_note) WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: RenameUser :exec
UPDATE user SET name = sqlc.arg(name) WHERE id = sqlc.arg(id);

-- name: GetWatchStats :one
-- What a person has watched, for their page: time in spans, the titles they watched in, and when
-- they last watched.
SELECT CAST(coalesce(sum(play_span.ended_at - play_span.started_at), 0) AS INTEGER) AS watched_ms,
  count(DISTINCT version.title_id) AS titles_watched_count,
  CAST(coalesce(max(play_span.ended_at), 0) AS INTEGER) AS last_watched_at
FROM play_span
LEFT JOIN file ON file.id = play_span.file_id
LEFT JOIN version ON version.id = file.version_id
WHERE play_span.user_id = sqlc.arg(user_id);

-- name: ListLibraryIDsOfRole :many
-- The live libraries a role lists, which its members may browse and play (D243 section 4).
SELECT role_library.library_id FROM role_library
JOIN library ON library.id = role_library.library_id AND library.removed_at IS NULL
WHERE role_library.role_id = sqlc.arg(role_id)
ORDER BY role_library.library_id;

-- A person removed (S40, D305 section 1): what was theirs is deleted, and their row stays only as
-- a nameless tombstone, so ids are never reused and their name is free for someone new.

-- name: RemoveUser :execrows
-- Their row, nameless: no name, password or note of theirs is kept.
UPDATE user SET removed_at = sqlc.arg(removed_at), name = '', password_hash = NULL, admin_note = NULL
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: DeleteLocatorsOfRemovedUser :exec
-- What each of their sessions was found by at a discovery (discovery-core.md K7).
DELETE FROM locator
WHERE session_secret_sha256 IN (SELECT owned.secret_sha256 FROM "grant" AS owned WHERE owned.user_id = sqlc.arg(user_id));

-- name: DeleteInvitationsOfRemovedUser :exec
-- The invitations their links are (D246 section 2).
DELETE FROM invitation
WHERE secret_sha256 IN (SELECT owned.secret_sha256 FROM "grant" AS owned WHERE owned.user_id = sqlc.arg(user_id));

-- name: DeleteGrantsDerivedFromRemovedUser :exec
-- The credentials derived from theirs (D177), which held nothing but what theirs gave.
DELETE FROM "grant"
WHERE parent_secret_sha256 IN (SELECT owned.secret_sha256 FROM "grant" AS owned WHERE owned.user_id = sqlc.arg(user_id));

-- name: DeleteGrantsOfRemovedUser :exec
-- Every credential of theirs, live or ended: sessions, links, device codes (D154).
DELETE FROM "grant" WHERE user_id = sqlc.arg(user_id);

-- name: DeleteDevicesOfRemovedUser :exec
-- Their devices, once their plays, which name them, are deleted.
DELETE FROM device WHERE user_id = sqlc.arg(user_id);

-- name: DeleteRealmKeyOfRemovedUser :exec
DELETE FROM realm_key WHERE user_id = sqlc.arg(user_id);


-- name: DemoteAdministrators :many
-- Everyone whose role administers, given the built-in user role: the first step of the operator's
-- reset of who administers (orpheum reset-admin). Returns who was demoted.
UPDATE user SET role_id = (SELECT id FROM role WHERE name = 'user' AND is_builtin = 1)
WHERE removed_at IS NULL
  AND role_id IN (SELECT id FROM role WHERE ',' || capabilities || ',' LIKE '%,administer,%')
RETURNING name;
