-- SPDX-License-Identifier: GPL-3.0-or-later
-- When each image was last served, to the day: what the artwork ceiling evicts first within a
-- class, D212 section 6. Written hourly in a batch, never on the request path.

-- name: MarkArtServed :exec
INSERT INTO art_served (content_hash_hex, last_served_epoch_day)
VALUES (sqlc.arg(content_hash_hex), sqlc.arg(last_served_epoch_day))
ON CONFLICT (content_hash_hex) DO UPDATE
SET last_served_epoch_day = MAX(art_served.last_served_epoch_day, excluded.last_served_epoch_day);

-- name: ListArtServed :many
SELECT content_hash_hex, last_served_epoch_day FROM art_served;

-- name: ForgetArtServed :exec
DELETE FROM art_served WHERE content_hash_hex = sqlc.arg(content_hash_hex);
