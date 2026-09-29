-- SPDX-License-Identifier: GPL-3.0-or-later
-- Prepared renditions (schema.md, cache.db; D217 section 5): what each node was asked to
-- prepare, and how far it is.

-- name: GetRendition :one
SELECT id, source_file, source_payload_hash, profile, node_id, state, segment_length_seconds, run_id, prepared_through_ms, size_bytes, failure, video_encoder, realtime_factor, carried_gains_db_json, created_at, finished_at
FROM prepared_rendition WHERE source_file = ? AND profile = ?;

-- name: GetRenditionByID :one
SELECT id, source_file, source_payload_hash, profile, node_id, state, segment_length_seconds, run_id, prepared_through_ms, size_bytes, failure, video_encoder, realtime_factor, carried_gains_db_json, created_at, finished_at
FROM prepared_rendition WHERE id = ?;

-- name: InsertRendition :one
INSERT INTO prepared_rendition (source_file, source_payload_hash, profile, node_id, segment_length_seconds, run_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: RestartRendition :exec
-- A rendition prepared again from the start: its source changed, or it failed or was lost.
UPDATE prepared_rendition
SET source_payload_hash = ?, node_id = ?, segment_length_seconds = ?, run_id = ?, state = 'queued', prepared_through_ms = 0, size_bytes = NULL, failure = NULL,
    finished_at = NULL, video_encoder = NULL, realtime_factor = NULL, carried_gains_db_json = '[]'
WHERE id = ?;

-- name: SetRenditionGains :exec
-- The gain a making puts in each audio track's sound, as its job starts (D226 section 4).
UPDATE prepared_rendition SET carried_gains_db_json = sqlc.arg(carried_gains_db_json) WHERE id = sqlc.arg(id);

-- name: SetRenditionRunning :exec
UPDATE prepared_rendition SET state = 'running', prepared_through_ms = ? WHERE id = ?;

-- name: SetRenditionComplete :exec
UPDATE prepared_rendition
SET state = 'complete', size_bytes = ?, finished_at = ?, video_encoder = ?, realtime_factor = ?
WHERE id = ?;

-- name: SetRenditionFailed :exec
UPDATE prepared_rendition SET state = 'failed', failure = ?, finished_at = ? WHERE id = ?;

-- name: ListFinishedRenditions :many
-- A node's latest finished prepares, newest first, for the System page (D225).
SELECT id, source_file, profile, state, video_encoder, realtime_factor, size_bytes, failure, finished_at
FROM prepared_rendition
WHERE node_id = ? AND finished_at IS NOT NULL
ORDER BY finished_at DESC, id DESC
LIMIT ?;
