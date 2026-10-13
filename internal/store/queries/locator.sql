-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Device locators: what a signed-in device finds its core by at a discovery, one per session, kept
-- as their hashes (discovery-core.md K7; the discovery protocol's section 3.1).

-- name: SetLocator :exec
-- The session's locator, replacing the one it had: each visit gets a fresh one.
INSERT INTO locator (session_secret_sha256, hash, issued_at)
VALUES (sqlc.arg(session_secret_sha256), sqlc.arg(hash), sqlc.arg(issued_at))
ON CONFLICT (session_secret_sha256) DO UPDATE SET hash = excluded.hash, issued_at = excluded.issued_at;

-- name: ListLiveLocators :many
-- The locators of every live session: not revoked, and used since idle_since.
SELECT locator.hash FROM locator
JOIN "grant" ON "grant".secret_sha256 = locator.session_secret_sha256
WHERE "grant".kind = 'session' AND "grant".revoked_at IS NULL AND "grant".last_used_at >= sqlc.arg(idle_since)
ORDER BY locator.hash;

-- name: GetLiveSessionOfLocator :one
-- The live session a locator belongs to, by the locator's hash: what an offer made under it is
-- proven under is that session's hash.
SELECT locator.session_secret_sha256 FROM locator
JOIN "grant" ON "grant".secret_sha256 = locator.session_secret_sha256
WHERE locator.hash = sqlc.arg(hash) AND "grant".kind = 'session' AND "grant".revoked_at IS NULL
  AND "grant".last_used_at >= sqlc.arg(idle_since);
