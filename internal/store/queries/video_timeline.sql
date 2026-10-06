-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each picture's timeline, as a read-through found it (D247 section 10): its keyframes and its bytes
-- in each second, by the bytes it was read from.

-- name: GetVideoTimeline :one
SELECT payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json, failure, made_at
FROM video_timeline WHERE payload_hash = ? AND video_stream_index = ?;

-- name: UpsertVideoTimeline :exec
INSERT INTO video_timeline (payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json, failure,
  made_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(video_stream_index), sqlc.arg(keyframes_ms_json),
  sqlc.arg(bytes_in_each_second_json), sqlc.arg(failure), sqlc.arg(made_at))
ON CONFLICT (payload_hash, video_stream_index) DO UPDATE SET
  keyframes_ms_json = excluded.keyframes_ms_json, bytes_in_each_second_json = excluded.bytes_in_each_second_json,
  failure = excluded.failure, made_at = excluded.made_at;

-- name: DeleteVideoTimelineFailedBefore :execrows
-- A person's play tries again what failed before they pressed play: the failure goes, and the
-- picture is read through again.
DELETE FROM video_timeline
WHERE payload_hash = sqlc.arg(payload_hash) AND video_stream_index = sqlc.arg(video_stream_index)
  AND failure IS NOT NULL AND made_at < sqlc.arg(before);
