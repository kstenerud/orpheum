-- SPDX-License-Identifier: GPL-3.0-or-later
-- When each image was last served, to the day: what the artwork ceiling evicts first within a
-- class, D212 section 6. Written hourly in a batch, never on the request path.

-- name: MarkArtServed :exec
INSERT INTO art_served (hash, day) VALUES (sqlc.arg(hash), sqlc.arg(day))
ON CONFLICT (hash) DO UPDATE SET day = MAX(art_served.day, excluded.day);

-- name: ListArtServed :many
SELECT hash, day FROM art_served;

-- name: ForgetArtServed :exec
DELETE FROM art_served WHERE hash = sqlc.arg(hash);
