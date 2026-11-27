-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The fair runs of a quality each node made (start-quality-by-node-speed.md section 3), which refine
-- what the core expects of the node over its start-up trials (internal/core/chunks).

-- name: InsertNodeRunSpeed :exec
INSERT INTO node_run_speed (node_id, speed_key, codec, bit_depth, height, quality_kbps, quality_height,
  speed_percent, film_made_ms, decoded_on, made_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteNodeRunSpeedsOfOtherKeys :exec
-- A node's runs made under another FFmpeg, encoder or quality recipe: they start over.
DELETE FROM node_run_speed WHERE node_id = sqlc.arg(node_id) AND speed_key <> sqlc.arg(speed_key);

-- name: DeleteNodeRunSpeedsBeyondKept :exec
-- A node's runs of a kind and a quality but the last kept_count.
DELETE FROM node_run_speed AS gone WHERE gone.id IN (
  SELECT kept.id FROM node_run_speed AS kept
  WHERE kept.node_id = sqlc.arg(node_id) AND kept.codec = sqlc.arg(codec) AND kept.bit_depth = sqlc.arg(bit_depth)
    AND kept.height = sqlc.arg(height) AND kept.quality_kbps = sqlc.arg(quality_kbps)
    AND kept.quality_height = sqlc.arg(quality_height)
  ORDER BY kept.id DESC LIMIT -1 OFFSET sqlc.arg(kept_count)
);

-- name: ListNodeRunSpeedsOfKind :many
-- A node's runs of a kind of film under its FFmpeg, encoder and quality recipe, newest first.
SELECT quality_kbps, quality_height, speed_percent FROM node_run_speed
WHERE node_id = sqlc.arg(node_id) AND speed_key = sqlc.arg(speed_key) AND codec = sqlc.arg(codec)
  AND bit_depth = sqlc.arg(bit_depth) AND height = sqlc.arg(height)
ORDER BY id DESC;
