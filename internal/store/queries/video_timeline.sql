-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each picture's timeline, as a read-through found it (D247 section 10): its keyframes and its bytes
-- in each second, by the bytes it was read from.

-- name: GetVideoTimeline :one
SELECT payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json, failure, made_at
FROM video_timeline WHERE payload_hash = ? AND video_stream_index = ?;

-- name: UpsertVideoTimeline :exec
-- A timeline's keyframes, once known, are kept, so the grid a playback is cut on does not move: a
-- later read through adds its bytes in each second, and a failure of one changes nothing of it (D259
-- section 2).
INSERT INTO video_timeline (payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json, failure,
  made_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(video_stream_index), sqlc.arg(keyframes_ms_json),
  sqlc.arg(bytes_in_each_second_json), sqlc.arg(failure), sqlc.arg(made_at))
ON CONFLICT (payload_hash, video_stream_index) DO UPDATE SET
  keyframes_ms_json = CASE WHEN video_timeline.keyframes_ms_json = '[]' THEN excluded.keyframes_ms_json
    ELSE video_timeline.keyframes_ms_json END,
  bytes_in_each_second_json = CASE WHEN excluded.bytes_in_each_second_json = '[]'
    THEN video_timeline.bytes_in_each_second_json ELSE excluded.bytes_in_each_second_json END,
  failure = CASE WHEN video_timeline.keyframes_ms_json = '[]' THEN excluded.failure ELSE video_timeline.failure END,
  made_at = excluded.made_at;

-- name: DeleteVideoTimelineFailedBefore :execrows
-- A person's play tries again what failed before they pressed play: the failure goes, and the
-- picture is read through again.
DELETE FROM video_timeline
WHERE payload_hash = sqlc.arg(payload_hash) AND video_stream_index = sqlc.arg(video_stream_index)
  AND failure IS NOT NULL AND made_at < sqlc.arg(before);
