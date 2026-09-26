-- SPDX-License-Identifier: GPL-3.0-or-later
-- Prepared renditions (schema.md, cache.db; D217 section 5): what each node was asked to
-- prepare, and how far it is.

-- name: GetRendition :one
SELECT id, source_file, payload_hash, profile, node_id, state, segment_s, done_ms, bytes, error, created_at, finished_at
FROM prepared_rendition WHERE source_file = ? AND profile = ?;

-- name: GetRenditionByID :one
SELECT id, source_file, payload_hash, profile, node_id, state, segment_s, done_ms, bytes, error, created_at, finished_at
FROM prepared_rendition WHERE id = ?;

-- name: InsertRendition :one
INSERT INTO prepared_rendition (source_file, payload_hash, profile, node_id, segment_s, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: RestartRendition :exec
-- A rendition prepared again from the start: its source changed, or it failed or was lost.
UPDATE prepared_rendition
SET payload_hash = ?, node_id = ?, segment_s = ?, state = 'queued', done_ms = 0, bytes = NULL, error = NULL,
    finished_at = NULL
WHERE id = ?;

-- name: SetRenditionRunning :exec
UPDATE prepared_rendition SET state = 'running', done_ms = ? WHERE id = ?;

-- name: SetRenditionComplete :exec
UPDATE prepared_rendition SET state = 'complete', bytes = ?, finished_at = ? WHERE id = ?;

-- name: SetRenditionFailed :exec
UPDATE prepared_rendition SET state = 'failed', error = ?, finished_at = ? WHERE id = ?;
