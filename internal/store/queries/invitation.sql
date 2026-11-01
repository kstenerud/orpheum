-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Invitations (D246 section 2): the role the account will have and the admin's note of who it is
-- for, with one or more links, each a single-use grant; using any one makes the account and ends
-- the others (D318); the invitation then keeps the account it made, and the list drops it (D403).

-- name: InsertInvitation :one
INSERT INTO invitation (role_id, admin_note, issued_at)
VALUES (sqlc.arg(role_id), sqlc.arg(admin_note), sqlc.arg(issued_at))
RETURNING id;

-- name: InsertInvitationLink :exec
INSERT INTO invitation_link (secret_sha256, invitation_id)
VALUES (sqlc.arg(secret_sha256), sqlc.arg(invitation_id));

-- name: ListInvitations :many
-- Every invitation still waiting, newest first, for the admin's list: its links still live and when
-- the last of them ends. One used is no longer listed: its account shows among the accounts (D403).
-- CROSS JOIN keeps SQLite's order: an invitation's links by
-- their index, then each one's grant by its key, never every grant for each invitation.
SELECT invitation.id, invitation.admin_note, role.name AS role_name, invitation.issued_at,
  CAST((SELECT count(*) FROM invitation_link
        CROSS JOIN "grant" ON "grant".secret_sha256 = invitation_link.secret_sha256
        WHERE invitation_link.invitation_id = invitation.id AND "grant".kind = 'issued_secret'
          AND "grant".revoked_at IS NULL AND "grant".expires_at > sqlc.arg(now)) AS INTEGER) AS live_links,
  CAST((SELECT coalesce(max("grant".expires_at), 0) FROM invitation_link
        CROSS JOIN "grant" ON "grant".secret_sha256 = invitation_link.secret_sha256
        WHERE invitation_link.invitation_id = invitation.id AND "grant".kind = 'issued_secret'
          AND "grant".revoked_at IS NULL) AS INTEGER) AS last_expires_at
FROM invitation
JOIN role ON role.id = invitation.role_id
WHERE invitation.made_user_id IS NULL
ORDER BY invitation.issued_at DESC, invitation.id DESC;

-- name: ListLiveInvitationLinks :many
-- Each live link of every invitation still waiting, oldest first, with its secret sealed, for the
-- admin to pass on again until it is used, withdrawn or ends (D267 section 5, which the owner chose
-- for invitations, 2026-10-28). Invitations are few; each one's links by their index, then each
-- link's grant by its key.
SELECT invitation_link.invitation_id, "grant".secret_sha256, "grant".expires_at, "grant".link_sealed
FROM invitation
CROSS JOIN invitation_link ON invitation_link.invitation_id = invitation.id
CROSS JOIN "grant" ON "grant".secret_sha256 = invitation_link.secret_sha256
WHERE invitation.made_user_id IS NULL AND "grant".kind = 'issued_secret' AND "grant".revoked_at IS NULL
  AND "grant".expires_at > sqlc.arg(now)
ORDER BY invitation_link.invitation_id, "grant".issued_at, "grant".secret_sha256;

-- name: GetInvitationBySecret :one
-- The invitation one of whose links secret is.
SELECT invitation.id, invitation.role_id, invitation.admin_note, invitation.made_user_id
FROM invitation_link
JOIN invitation ON invitation.id = invitation_link.invitation_id
WHERE invitation_link.secret_sha256 = sqlc.arg(secret_sha256);

-- name: GetInvitation :one
SELECT invitation.id, invitation.made_user_id FROM invitation WHERE invitation.id = sqlc.arg(id);

-- name: RevokeInvitationLinks :exec
-- Every link of the invitation not yet used (D318 sections 2, 3), its sealed secret forgotten with
-- it; a used one is its person's own link by now, kind 'link', and stays, sealed (discovery-core.md
-- K7).
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at), link_sealed = NULL
WHERE revoked_at IS NULL AND kind = 'issued_secret'
  AND secret_sha256 IN (SELECT secret_sha256 FROM invitation_link WHERE invitation_id = sqlc.arg(invitation_id));

-- name: MarkInvitationMade :exec
UPDATE invitation SET made_user_id = sqlc.arg(made_user_id), made_at = sqlc.arg(made_at)
WHERE id = sqlc.arg(id) AND made_user_id IS NULL;

-- name: DeleteInvitationLinks :exec
DELETE FROM invitation_link WHERE invitation_id = sqlc.arg(invitation_id);

-- name: DeleteInvitation :exec
DELETE FROM invitation WHERE id = sqlc.arg(id);

-- name: RevokeInvitationGrantsOfRole :exec
-- A role deleted withdraws the invitations into it, their sealed secrets forgotten.
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at), link_sealed = NULL
WHERE revoked_at IS NULL AND kind = 'issued_secret'
  AND secret_sha256 IN (SELECT invitation_link.secret_sha256 FROM invitation_link
                        JOIN invitation ON invitation.id = invitation_link.invitation_id
                        WHERE invitation.role_id = sqlc.arg(role_id));

-- name: DeleteInvitationLinksOfRole :exec
DELETE FROM invitation_link
WHERE invitation_id IN (SELECT id FROM invitation WHERE role_id = sqlc.arg(role_id));

-- name: DeleteInvitationsOfRole :exec
DELETE FROM invitation WHERE role_id = sqlc.arg(role_id);
