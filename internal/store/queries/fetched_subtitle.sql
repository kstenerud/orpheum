-- SPDX-License-Identifier: GPL-3.0-or-later
-- Subtitles fetched on request (schema.md, fetched_subtitle; D224): kept in persistent.db, gzipped,
-- because each cost one of the account's daily downloads.

-- name: InsertFetchedSubtitle :one
INSERT INTO fetched_subtitle (video_file, provider, provider_file_id, language, format, body_gzip, fetched_by_user_id, fetched_at,
  release_name)
VALUES (sqlc.arg(video_file), sqlc.arg(provider), sqlc.arg(provider_file_id), sqlc.arg(language), sqlc.arg(format),
  sqlc.arg(body_gzip), sqlc.arg(fetched_by_user_id), sqlc.arg(fetched_at), sqlc.arg(release_name))
RETURNING id;

-- name: ListFetchedSubtitlesOfVideo :many
-- A video's fetched subtitles, oldest first, with their bodies, which name where their text is kept.
SELECT id, language, release_name, body_gzip
FROM fetched_subtitle
WHERE video_file = sqlc.arg(video_file)
ORDER BY id;

-- name: GetFetchedSubtitle :one
-- A fetched subtitle and the live video it is for, for the job that converts it.
SELECT s.id, s.video_file, s.body_gzip, f.node_id
FROM fetched_subtitle s
JOIN file f ON f.id = s.video_file AND f.removed_at IS NULL
WHERE s.id = sqlc.arg(id);

-- name: DeleteFetchedSubtitle :exec
-- A fetched subtitle its node could not convert, or did not in time while its person waited: only
-- a subtitle its node converted is kept (D224 section 6). Nothing refers to it yet.
DELETE FROM fetched_subtitle WHERE id = sqlc.arg(id);
