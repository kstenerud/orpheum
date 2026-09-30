-- SPDX-License-Identifier: GPL-3.0-or-later
-- Subtitle text (schema.md, cache.db; D217 section 6, D224 section 5): a text track as ASS and
-- WebVTT, kept by its source's hash: the payload hash of the file an embedded track's bytes are
-- in, so a move or a rescan keeps it, or the SHA-256 of a fetched subtitle's stored body.

-- name: GetSubtitleText :one
SELECT source_hash, stream_index, state, ass, vtt, failure, created_at, finished_at
FROM subtitle_text WHERE source_hash = ? AND stream_index = ?;

-- name: RequestSubtitleText :exec
-- A track wanted: new, or asked for again after it failed.
INSERT INTO subtitle_text (source_hash, stream_index, created_at) VALUES (?, ?, ?)
ON CONFLICT (source_hash, stream_index) DO UPDATE
SET state = 'queued', failure = NULL, finished_at = NULL, created_at = excluded.created_at
WHERE subtitle_text.state = 'failed';

-- name: SetSubtitleTextComplete :exec
UPDATE subtitle_text SET state = 'complete', ass = ?, vtt = ?, failure = NULL, finished_at = ?
WHERE source_hash = ? AND stream_index = ?;

-- name: SetSubtitleTextFailed :exec
UPDATE subtitle_text SET state = 'failed', failure = ?, finished_at = ?
WHERE source_hash = ? AND stream_index = ?;
