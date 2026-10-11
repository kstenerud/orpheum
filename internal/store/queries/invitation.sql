-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Invitations not yet used (D246 section 2): each a single-use link's grant, the role the account
-- will have, and the admin's note of who it is for. Registering through one makes the account.

-- name: InsertInvitation :one
INSERT INTO invitation (secret_sha256, role_id, admin_note)
VALUES (sqlc.arg(secret_sha256), sqlc.arg(role_id), sqlc.arg(admin_note))
RETURNING id;

-- name: ListLiveInvitations :many
-- The invitations whose link still works, newest first, for the admin's list of people.
SELECT invitation.id, invitation.admin_note, role.name AS role_name, "grant".issued_at, "grant".expires_at
FROM invitation
JOIN "grant" ON "grant".secret_sha256 = invitation.secret_sha256
JOIN role ON role.id = invitation.role_id
WHERE "grant".revoked_at IS NULL AND ("grant".expires_at IS NULL OR "grant".expires_at > sqlc.arg(now))
ORDER BY "grant".issued_at DESC, invitation.id DESC;

-- name: GetInvitationBySecret :one
SELECT invitation.id, invitation.role_id, invitation.admin_note
FROM invitation
WHERE invitation.secret_sha256 = sqlc.arg(secret_sha256);

-- name: GetInvitation :one
SELECT invitation.id, invitation.secret_sha256 FROM invitation WHERE invitation.id = sqlc.arg(id);

-- name: DeleteInvitation :exec
DELETE FROM invitation WHERE id = sqlc.arg(id);

-- name: RevokeInvitationGrantsOfRole :exec
-- A role deleted withdraws the invitations into it.
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE revoked_at IS NULL
  AND secret_sha256 IN (SELECT secret_sha256 FROM invitation WHERE role_id = sqlc.arg(role_id));

-- name: DeleteInvitationsOfRole :exec
DELETE FROM invitation WHERE role_id = sqlc.arg(role_id);
