-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Prepared renditions (schema.md, cache.db; D217 section 5): what each node was asked to
-- prepare, and how far it is.

-- name: GetRenditionOfFileAndProfile :one
SELECT id, source_file, source_payload_hash, profile, node_id, state, segment_length_seconds, run_id, prepared_through_ms, size_bytes, failure, video_encoder, realtime_factor, carried_gains_db_json, audio_languages_json, created_at, finished_at
FROM prepared_rendition WHERE source_file = ? AND profile = ?;

-- name: GetRenditionByID :one
SELECT id, source_file, source_payload_hash, profile, node_id, state, segment_length_seconds, run_id, prepared_through_ms, size_bytes, failure, video_encoder, realtime_factor, carried_gains_db_json, audio_languages_json, created_at, finished_at
FROM prepared_rendition WHERE id = ?;

-- name: InsertRendition :one
INSERT INTO prepared_rendition (source_file, source_payload_hash, profile, node_id, segment_length_seconds, run_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: RestartRendition :exec
-- A rendition prepared again from the start: its source changed, or it failed or was lost.
UPDATE prepared_rendition
SET source_payload_hash = ?, node_id = ?, segment_length_seconds = ?, run_id = ?, state = 'queued', prepared_through_ms = 0, size_bytes = NULL, failure = NULL,
    finished_at = NULL, video_encoder = NULL, realtime_factor = NULL, carried_gains_db_json = '[]',
    audio_languages_json = '[]'
WHERE id = ?;

-- name: SetRenditionAudio :exec
-- What a making puts in each audio track, as its job starts: the gain in its sound (D226 section 4)
-- and the language it is tagged with.
UPDATE prepared_rendition
SET carried_gains_db_json = sqlc.arg(carried_gains_db_json), audio_languages_json = sqlc.arg(audio_languages_json)
WHERE id = sqlc.arg(id);

-- name: SetRenditionRunning :exec
-- How far a making is, by its run, while it has not ended: its end, recorded as it arrived, is never
-- undone by its progress heard after it.
UPDATE prepared_rendition SET state = 'running', prepared_through_ms = ?
WHERE source_file = ? AND profile = ? AND run_id = ? AND state IN ('queued', 'running');

-- name: SetRenditionComplete :execrows
-- A making that ended, by its run: a rendition made again since has another run, and is not touched
-- (G13).
UPDATE prepared_rendition
SET state = 'complete', size_bytes = ?, finished_at = ?, video_encoder = ?, realtime_factor = ?
WHERE source_file = ? AND profile = ? AND run_id = ?;

-- name: SetRenditionFailed :execrows
-- A making that failed, by its run, as SetRenditionComplete, unless the node said it was done: a
-- job that gave up on a making the node finished does not undo it.
UPDATE prepared_rendition SET state = 'failed', failure = ?, finished_at = ?
WHERE source_file = ? AND profile = ? AND run_id = ? AND state <> 'complete';

-- name: ListFinishedRenditions :many
-- A node's latest finished prepares, newest first, for the System page (D225).
SELECT id, source_file, profile, state, video_encoder, realtime_factor, size_bytes, failure, finished_at
FROM prepared_rendition
WHERE node_id = ? AND finished_at IS NOT NULL
ORDER BY finished_at DESC, id DESC
LIMIT ?;

-- name: InsertAdoptedRendition :execrows
-- A rendition a node holds that the index does not know, as after a cache.db discard: adopted,
-- complete, from the manifest beside it (D238 section 2). One the index has meanwhile is left.
INSERT INTO prepared_rendition (source_file, source_payload_hash, profile, node_id, state, segment_length_seconds,
  run_id, size_bytes, video_encoder, realtime_factor, carried_gains_db_json, audio_languages_json, created_at,
  finished_at)
VALUES (sqlc.arg(source_file), sqlc.arg(source_payload_hash), sqlc.arg(profile), sqlc.arg(node_id), 'complete',
  sqlc.arg(segment_length_seconds), sqlc.arg(run_id), sqlc.arg(size_bytes), sqlc.arg(video_encoder),
  sqlc.arg(realtime_factor), sqlc.arg(carried_gains_db_json), sqlc.arg(audio_languages_json), sqlc.arg(made_at),
  sqlc.arg(made_at))
ON CONFLICT (source_file, profile) DO NOTHING;
