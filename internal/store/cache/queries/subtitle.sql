-- SPDX-License-Identifier: GPL-3.0-or-later
-- Extracted subtitles (schema.md, cache.db; D217 section 6): a video's text track as ASS and
-- WebVTT, kept by the video's payload hash so a move or a rescan keeps it.

-- name: GetSubtitleText :one
SELECT payload_hash, stream_index, state, ass, vtt, error, created_at, finished_at
FROM subtitle_text WHERE payload_hash = ? AND stream_index = ?;

-- name: WantSubtitleText :exec
-- A track wanted: new, or asked for again after it failed.
INSERT INTO subtitle_text (payload_hash, stream_index, created_at) VALUES (?, ?, ?)
ON CONFLICT (payload_hash, stream_index) DO UPDATE
SET state = 'queued', error = NULL, finished_at = NULL, created_at = excluded.created_at
WHERE subtitle_text.state = 'failed';

-- name: SetSubtitleTextComplete :exec
UPDATE subtitle_text SET state = 'complete', ass = ?, vtt = ?, error = NULL, finished_at = ?
WHERE payload_hash = ? AND stream_index = ?;

-- name: SetSubtitleTextFailed :exec
UPDATE subtitle_text SET state = 'failed', error = ?, finished_at = ?
WHERE payload_hash = ? AND stream_index = ?;
