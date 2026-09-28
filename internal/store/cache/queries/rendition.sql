-- SPDX-License-Identifier: GPL-3.0-or-later
-- Prepared renditions (schema.md, cache.db; D217 section 5): what each node was asked to
-- prepare, and how far it is.

-- name: GetRendition :one
SELECT id, source_file, payload_hash, profile, node_id, state, segment_length_s, run_id, prepared_ms, size_bytes, error, video_encoder, speed, created_at, finished_at
FROM prepared_rendition WHERE source_file = ? AND profile = ?;

-- name: GetRenditionByID :one
SELECT id, source_file, payload_hash, profile, node_id, state, segment_length_s, run_id, prepared_ms, size_bytes, error, video_encoder, speed, created_at, finished_at
FROM prepared_rendition WHERE id = ?;

-- name: InsertRendition :one
INSERT INTO prepared_rendition (source_file, payload_hash, profile, node_id, segment_length_s, run_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: RestartRendition :exec
-- A rendition prepared again from the start: its source changed, or it failed or was lost.
UPDATE prepared_rendition
SET payload_hash = ?, node_id = ?, segment_length_s = ?, run_id = ?, state = 'queued', prepared_ms = 0, size_bytes = NULL, error = NULL,
    finished_at = NULL, video_encoder = NULL, speed = NULL
WHERE id = ?;

-- name: SetRenditionRunning :exec
UPDATE prepared_rendition SET state = 'running', prepared_ms = ? WHERE id = ?;

-- name: SetRenditionComplete :exec
UPDATE prepared_rendition
SET state = 'complete', size_bytes = ?, finished_at = ?, video_encoder = ?, speed = ?
WHERE id = ?;

-- name: SetRenditionFailed :exec
UPDATE prepared_rendition SET state = 'failed', error = ?, finished_at = ? WHERE id = ?;

-- name: ListFinishedRenditions :many
-- A node's latest finished prepares, newest first, for the System page (D225).
SELECT id, source_file, profile, state, video_encoder, speed, size_bytes, error, finished_at
FROM prepared_rendition
WHERE node_id = ? AND finished_at IS NOT NULL
ORDER BY finished_at DESC, id DESC
LIMIT ?;
