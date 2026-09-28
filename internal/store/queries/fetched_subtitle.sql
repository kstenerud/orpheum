-- SPDX-License-Identifier: GPL-3.0-or-later
-- Subtitles fetched on request (schema.md, fetched_subtitle; D224): kept in library.db, gzipped,
-- because each cost one of the account's daily downloads.

-- name: InsertFetchedSubtitle :one
INSERT INTO fetched_subtitle (video_file, provider, provider_ref, lang, format, body, fetched_by, fetched_at,
  release_name)
VALUES (sqlc.arg(video_file), sqlc.arg(provider), sqlc.arg(provider_ref), sqlc.arg(lang), sqlc.arg(format),
  sqlc.arg(body), sqlc.arg(fetched_by), sqlc.arg(fetched_at), sqlc.arg(release_name))
RETURNING id;

-- name: ListFetchedSubtitlesOfVideo :many
-- A video's fetched subtitles, oldest first, with their bodies, which name where their text is kept.
SELECT id, lang, release_name, body
FROM fetched_subtitle
WHERE video_file = sqlc.arg(video_file)
ORDER BY id;

-- name: GetFetchedSubtitle :one
-- A fetched subtitle and the live video it is for, for the job that converts it.
SELECT s.id, s.video_file, s.body, f.node_id
FROM fetched_subtitle s
JOIN file f ON f.id = s.video_file AND f.removed_at IS NULL
WHERE s.id = sqlc.arg(id);
