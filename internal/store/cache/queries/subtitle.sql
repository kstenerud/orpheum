-- SPDX-License-Identifier: GPL-3.0-or-later
-- Extracted subtitles (schema.md, cache.db; D217 section 6): a video's text track as ASS and
-- WebVTT, kept by the payload hash of the file its bytes are in, so a move or a rescan keeps it.

-- name: GetSubtitleText :one
SELECT bytes_payload_hash, stream_index, state, ass, vtt, failure, created_at, finished_at
FROM subtitle_text WHERE bytes_payload_hash = ? AND stream_index = ?;

-- name: RequestSubtitleText :exec
-- A track wanted: new, or asked for again after it failed.
INSERT INTO subtitle_text (bytes_payload_hash, stream_index, created_at) VALUES (?, ?, ?)
ON CONFLICT (bytes_payload_hash, stream_index) DO UPDATE
SET state = 'queued', failure = NULL, finished_at = NULL, created_at = excluded.created_at
WHERE subtitle_text.state = 'failed';

-- name: SetSubtitleTextComplete :exec
UPDATE subtitle_text SET state = 'complete', ass = ?, vtt = ?, failure = NULL, finished_at = ?
WHERE bytes_payload_hash = ? AND stream_index = ?;

-- name: SetSubtitleTextFailed :exec
UPDATE subtitle_text SET state = 'failed', failure = ?, finished_at = ?
WHERE bytes_payload_hash = ? AND stream_index = ?;
