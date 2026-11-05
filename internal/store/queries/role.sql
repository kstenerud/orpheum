-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Roles (D130) and the libraries each reaches (D243 section 4): a person has one role, which is
-- their whole access. The built-in roles are seeded by the baseline; the admin makes the rest.

-- name: ListRoles :many
-- Every role, the built-in ones first, with how many live people hold each.
SELECT role.id, role.name, role.is_builtin, role.capabilities,
  (SELECT count(*) FROM user WHERE user.role_id = role.id AND user.removed_at IS NULL) AS holder_count
FROM role
ORDER BY role.is_builtin DESC, role.name, role.id;

-- name: ListRoleLibraries :many
-- Every role's live libraries, and whether each may be rebroadcast.
SELECT role_library.role_id, role_library.library_id, role_library.may_rebroadcast
FROM role_library
JOIN library ON library.id = role_library.library_id AND library.removed_at IS NULL
ORDER BY role_library.role_id, role_library.library_id;

-- name: GetRole :one
SELECT id, name, is_builtin, capabilities, revision FROM role WHERE id = sqlc.arg(id);

-- name: InsertRole :one
INSERT INTO role (name, is_builtin, capabilities) VALUES (sqlc.arg(name), 0, sqlc.arg(capabilities))
RETURNING id;

-- name: UpdateRole :exec
-- A role the admin made: its name and capabilities. A built-in role keeps both.
UPDATE role SET revision = revision + 1, name = sqlc.arg(name), capabilities = sqlc.arg(capabilities)
WHERE id = sqlc.arg(id) AND is_builtin = 0;

-- name: DeleteRole :execrows
-- A role the admin made that no person holds, live or removed: a removed person's row still names
-- its role.
DELETE FROM role
WHERE role.id = sqlc.arg(role_id) AND role.is_builtin = 0
  AND NOT EXISTS (SELECT 1 FROM user WHERE user.role_id = role.id);

-- name: DeleteRoleLibraries :exec
DELETE FROM role_library WHERE role_id = sqlc.arg(role_id);

-- name: InsertRoleLibrary :exec
INSERT INTO role_library (role_id, library_id, may_rebroadcast)
VALUES (sqlc.arg(role_id), sqlc.arg(library_id), sqlc.arg(may_rebroadcast));

-- name: UpdateUserRole :exec
UPDATE user SET revision = revision + 1, role_id = sqlc.arg(role_id) WHERE id = sqlc.arg(id) AND removed_at IS NULL;
