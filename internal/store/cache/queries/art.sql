-- SPDX-License-Identifier: AGPL-3.0-or-later
-- When each image was last served, to the day: what the artwork ceiling evicts first within a
-- class, D212 section 6. Written hourly in a batch, never on the request path.

-- name: MarkArtServed :exec
INSERT INTO art_served (source_sha256_hex, last_served_epoch_day)
VALUES (sqlc.arg(source_sha256_hex), sqlc.arg(last_served_epoch_day))
ON CONFLICT (source_sha256_hex) DO UPDATE
SET last_served_epoch_day = MAX(art_served.last_served_epoch_day, excluded.last_served_epoch_day)
-- Only when it changes something (D303, DL9).
WHERE (art_served.last_served_epoch_day)
  IS NOT (MAX(art_served.last_served_epoch_day, excluded.last_served_epoch_day));

-- name: ListArtServed :many
SELECT source_sha256_hex, last_served_epoch_day FROM art_served;

-- name: ForgetArtServed :exec
DELETE FROM art_served WHERE source_sha256_hex = sqlc.arg(source_sha256_hex);
