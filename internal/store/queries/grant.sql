-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Credentials: one grant table, opaque and revocable, stored as SHA-256 hashes (D154).

-- name: InsertGrant :exec
-- A link's grant also keeps its locate hash and offer key, and its secret sealed for the person it
-- becomes the own link of (discovery-core.md K7); NULL for the rest.
INSERT INTO "grant" (secret_sha256, kind, user_id, scope, issued_at, expires_at, last_used_at, device_name, device_id,
                     locate_hash, offer_key, link_sealed)
VALUES (sqlc.arg(secret_sha256), sqlc.arg(kind), sqlc.arg(user_id), sqlc.arg(scope), sqlc.arg(issued_at),
        sqlc.arg(expires_at), sqlc.arg(issued_at), sqlc.arg(device_name), sqlc.arg(device_id),
        sqlc.narg(locate_hash), sqlc.narg(offer_key), sqlc.narg(link_sealed));

-- name: GetLiveGrant :one
SELECT secret_sha256, kind, user_id, scope, issued_at, expires_at, last_used_at, device_id, device_name, locate_hash
FROM "grant"
WHERE secret_sha256 = sqlc.arg(secret_sha256)
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now));

-- name: UpdateGrantLastUsed :exec
-- A later use only: a use written late never moves the stored time back (D303 section 4).
UPDATE "grant" SET last_used_at = sqlc.arg(last_used_at)
WHERE secret_sha256 = sqlc.arg(secret_sha256) AND (last_used_at IS NULL OR last_used_at < sqlc.arg(last_used_at));

-- name: MarkGrantRevoked :exec
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE secret_sha256 = sqlc.arg(secret_sha256) AND revoked_at IS NULL;

-- name: RevokeLiveGrantsOfUser :exec
-- A person's live grants of one kind and scope: their other registration links, when one is used
-- (D318 section 2), cancelled (section 3) or, a reset link, replaced by a new one (D466 section 2).
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE user_id = sqlc.arg(user_id) AND kind = sqlc.arg(kind) AND scope = sqlc.arg(scope) AND revoked_at IS NULL;

-- name: GetLiveRegistrationLinkOfUser :one
-- When the person's outstanding registration link that lives longest stops working: a registered
-- person's reset link, whose page says until when it is live and never shows it again (D466 section 4).
SELECT expires_at FROM "grant"
WHERE user_id = sqlc.arg(user_id) AND kind = 'issued_secret' AND scope = 'register' AND revoked_at IS NULL
  AND expires_at > sqlc.arg(now)
ORDER BY expires_at DESC
LIMIT 1;

-- name: GetGrantState :one
-- A grant whatever its state: why a session no longer works, when one does not.
SELECT kind, revoked_at, expires_at, last_used_at, issued_at FROM "grant" WHERE secret_sha256 = sqlc.arg(secret_sha256);


-- name: RevokeSessionsOfUserExceptSession :exec
-- Every live session of the person's but one, the one asking: signing out everywhere else, and a
-- changed password.
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE user_id = sqlc.arg(user_id) AND kind = 'session' AND revoked_at IS NULL
  AND secret_sha256 IS NOT sqlc.arg(keep_secret_sha256);

-- name: BecomeOwnLink :exec
-- A used registration link, or invitation, becomes its person's own link: it locates the core for
-- them at a discovery, grants no sign-in, and lives until expires_at, renewed by their visits
-- (discovery-core.md K7; the discovery protocol's section 3.1).
UPDATE "grant" SET kind = 'link', scope = 'own', user_id = sqlc.arg(user_id), revoked_at = NULL,
  expires_at = sqlc.arg(expires_at), last_used_at = sqlc.arg(now)
WHERE secret_sha256 = sqlc.arg(secret_sha256) AND locate_hash IS NOT NULL;

-- name: GetOwnLinkOfUser :one
-- The person's own link while they have one: its grant, when it stops locating the core, and its
-- secret sealed, NULL for a link made before the core kept it.
SELECT secret_sha256, expires_at, link_sealed FROM "grant"
WHERE user_id = sqlc.arg(user_id) AND kind = 'link' AND scope = 'own' AND revoked_at IS NULL
  AND expires_at > sqlc.arg(now)
ORDER BY expires_at DESC
LIMIT 1;

-- name: RenewOwnLinksOfUser :exec
-- A signed-in visit renews the person's own link's life, which rolls.
UPDATE "grant" SET expires_at = sqlc.arg(expires_at)
WHERE user_id = sqlc.arg(user_id) AND kind = 'link' AND scope = 'own' AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now));

-- name: ListLiveLinkHashes :many
-- Every live link a discovery may find the core by: its locate hash and when it ends.
SELECT locate_hash, expires_at FROM "grant"
WHERE locate_hash IS NOT NULL AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > sqlc.arg(now))
ORDER BY locate_hash;

-- name: GetLiveLinkOfferKey :one
-- What an offer through a live link is proven under, by the link's locate hash.
SELECT offer_key FROM "grant"
WHERE locate_hash = sqlc.arg(locate_hash) AND offer_key IS NOT NULL AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now));

-- A credential ended, revoked or expired, before ended_before is deleted with what names it (D304
-- section 6): every sign-in ever made was kept.

-- name: DeleteLocatorsOfEndedGrants :exec
DELETE FROM locator WHERE session_secret_sha256 IN (
  SELECT ended.secret_sha256 FROM "grant" AS ended
  WHERE ended.revoked_at < sqlc.arg(ended_before) OR ended.expires_at < sqlc.arg(ended_before));

-- name: DeleteInvitationLinksOfEndedGrants :exec
DELETE FROM invitation_link WHERE secret_sha256 IN (
  SELECT ended.secret_sha256 FROM "grant" AS ended
  WHERE ended.revoked_at < sqlc.arg(ended_before) OR ended.expires_at < sqlc.arg(ended_before));

-- name: DeleteWaitingInvitationsWithNoLinks :exec
-- An invitation still waiting whose every link has gone; a used one stays, naming its account.
DELETE FROM invitation
WHERE made_user_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM invitation_link WHERE invitation_link.invitation_id = invitation.id);

-- name: DeleteEndedGrants :execrows
-- Those no credential derived from them still names (D177); a derived one goes first, and its
-- parent at the next sweep.
DELETE FROM "grant"
WHERE ("grant".revoked_at < sqlc.arg(ended_before) OR "grant".expires_at < sqlc.arg(ended_before))
  AND NOT EXISTS (SELECT 1 FROM "grant" AS derived WHERE derived.parent_secret_sha256 = "grant".secret_sha256);
