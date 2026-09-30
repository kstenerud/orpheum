-- SPDX-License-Identifier: GPL-3.0-or-later
-- Subtitle text (schema.md, subtitle_text; D217 section 6, D224 section 5): a text track as ASS and
-- WebVTT, or why it could not be made, kept by its source's hash: the payload hash of the file an
-- embedded track's bytes are in, so a move or a rescan keeps it, or the SHA-256 of a fetched
-- subtitle's stored body. It is kept in library.db, since making it again reads a whole video
-- (the architecture map's F1); a track being made has no row until the node answers.

-- name: GetSubtitleText :one
SELECT source_hash, stream_index, ass, vtt, failure, made_at
FROM subtitle_text WHERE source_hash = ? AND stream_index = ?;

-- name: KeepSubtitleText :exec
-- What the node answered: the text, or why none could be made. A later answer replaces it.
INSERT INTO subtitle_text (source_hash, stream_index, ass, vtt, failure, made_at)
VALUES (sqlc.arg(source_hash), sqlc.arg(stream_index), sqlc.arg(ass), sqlc.arg(vtt), sqlc.arg(failure), sqlc.arg(made_at))
ON CONFLICT (source_hash, stream_index) DO UPDATE SET
  ass = excluded.ass, vtt = excluded.vtt, failure = excluded.failure, made_at = excluded.made_at;

-- name: ForgetSubtitleTextFailure :exec
-- A play asks again for a track that could not be made: its failure goes while it is made again.
DELETE FROM subtitle_text WHERE source_hash = ? AND stream_index = ? AND failure IS NOT NULL;
