-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each picture's timeline, as a read-through found it (schema.md, cache.db; D247 section 10): its
-- keyframes and its bytes in each second, by the bytes it was read from.

-- name: GetVideoTimeline :one
SELECT payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json, failure, made_at
FROM video_timeline WHERE payload_hash = ? AND video_stream_index = ?;

-- name: CountVideoTimelinesOfPayload :one
-- How many of a file's bytes' pictures have a timeline, or the word that they cannot be read: a
-- sweep reads files through whose bytes have none.
SELECT COUNT(*) AS timeline_count FROM video_timeline WHERE payload_hash = ?;

-- name: UpsertVideoTimeline :exec
INSERT INTO video_timeline (payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json, failure,
  made_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(video_stream_index), sqlc.arg(keyframes_ms_json),
  sqlc.arg(bytes_in_each_second_json), sqlc.arg(failure), sqlc.arg(made_at))
ON CONFLICT (payload_hash, video_stream_index) DO UPDATE SET
  keyframes_ms_json = excluded.keyframes_ms_json, bytes_in_each_second_json = excluded.bytes_in_each_second_json,
  failure = excluded.failure, made_at = excluded.made_at;
