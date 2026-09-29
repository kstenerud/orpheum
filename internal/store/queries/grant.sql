-- SPDX-License-Identifier: GPL-3.0-or-later
-- Credentials: one grant table, opaque and revocable, stored as SHA-256 hashes (D154).

-- name: InsertGrant :exec
INSERT INTO "grant" (secret_sha256, kind, user_id, scope, issued_at, expires_at, last_seen_at, device_name, device_id)
VALUES (sqlc.arg(secret_sha256), sqlc.arg(kind), sqlc.arg(user_id), sqlc.arg(scope), sqlc.arg(issued_at),
        sqlc.arg(expires_at), sqlc.arg(issued_at), sqlc.arg(device_name), sqlc.arg(device_id));

-- name: GetLiveGrant :one
SELECT secret_sha256, kind, user_id, scope, issued_at, expires_at, last_seen_at, device_id, device_name
FROM "grant"
WHERE secret_sha256 = sqlc.arg(secret_sha256)
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now));

-- name: UpdateGrantLastSeen :exec
UPDATE "grant" SET last_seen_at = sqlc.arg(last_seen_at)
WHERE secret_sha256 = sqlc.arg(secret_sha256);

-- name: MarkGrantRevoked :exec
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE secret_sha256 = sqlc.arg(secret_sha256) AND revoked_at IS NULL;

-- name: RevokeLiveGrantsOf :exec
-- A person's live grants of one kind and scope: an older registration link, when a new one is
-- made (D218).
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE user_id = sqlc.arg(user_id) AND kind = sqlc.arg(kind) AND scope = sqlc.arg(scope) AND revoked_at IS NULL;

-- name: GetLiveGrantOf :one
-- A person's live grant of one kind and scope: the device code still waiting (D221).
SELECT secret_sha256, device_name, device_id, expires_at FROM "grant"
WHERE user_id = sqlc.arg(user_id) AND kind = sqlc.arg(kind) AND scope = sqlc.arg(scope) AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now))
ORDER BY issued_at DESC
LIMIT 1;

-- name: GetGrantState :one
-- A grant whatever its state: why a session no longer works, when one does not.
SELECT kind, revoked_at, expires_at, last_seen_at, issued_at FROM "grant" WHERE secret_sha256 = sqlc.arg(secret_sha256);
