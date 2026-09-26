-- SPDX-License-Identifier: GPL-3.0-or-later
-- Credentials: one grant table, opaque and revocable, stored as SHA-256 hashes (D154).

-- name: InsertGrant :exec
INSERT INTO "grant" (hash, kind, subject, scope, issued_at, expires_at, last_seen_at, label, device_id)
VALUES (sqlc.arg(hash), sqlc.arg(kind), sqlc.arg(subject), sqlc.arg(scope), sqlc.arg(issued_at),
        sqlc.arg(expires_at), sqlc.arg(issued_at), sqlc.arg(label), sqlc.arg(device_id));

-- name: GetLiveGrant :one
SELECT hash, kind, subject, scope, issued_at, expires_at, last_seen_at, device_id
FROM "grant"
WHERE hash = sqlc.arg(hash)
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now));

-- name: UpdateGrantLastSeen :exec
UPDATE "grant" SET last_seen_at = sqlc.arg(last_seen_at)
WHERE hash = sqlc.arg(hash);

-- name: MarkGrantRevoked :exec
UPDATE "grant" SET revoked_at = sqlc.arg(revoked_at)
WHERE hash = sqlc.arg(hash) AND revoked_at IS NULL;
