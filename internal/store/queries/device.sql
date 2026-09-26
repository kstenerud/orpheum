-- SPDX-License-Identifier: GPL-3.0-or-later
-- A person's devices (schema.md, device; D217 section 8): made at sign-in, named by the client.

-- name: InsertDevice :one
INSERT INTO device (user_id, name, platform, first_seen_at, last_seen_at)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(platform), sqlc.arg(seen_at), sqlc.arg(seen_at))
RETURNING id;
