-- SPDX-License-Identifier: AGPL-3.0-or-later
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
JOIN place f ON f.id = s.video_file AND f.removed_at IS NULL
WHERE s.id = sqlc.arg(id);

-- name: DeleteFetchedSubtitle :exec
-- A fetched subtitle its node could not convert, or did not in time while its person waited: only
-- a subtitle its node converted is kept (D224 section 6). Nothing refers to it yet.
DELETE FROM fetched_subtitle WHERE id = sqlc.arg(id);

-- name: GetFetchedSubtitleOfProviderFile :one
-- The subtitle already fetched for a video from a provider's file, which a second ask is given
-- rather than spending another download (D244 section 4).
SELECT id FROM fetched_subtitle
WHERE video_file = sqlc.arg(video_file) AND provider = sqlc.arg(provider)
  AND provider_file_id = sqlc.arg(provider_file_id);

-- name: DeleteFetchedSubtitlesOfPurgedFiles :exec
-- A removed file deleted past its keeping takes the subtitles fetched for it: they were timed to it.
DELETE FROM fetched_subtitle
WHERE video_file IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: MoveFetchedSubtitlesToFile :exec
-- The subtitles fetched for a file confirmed to be a copy of another (D363 section 2): the other's,
-- one fetched for both from one provider's file kept once.
UPDATE fetched_subtitle SET video_file = sqlc.arg(to_file)
WHERE fetched_subtitle.video_file = sqlc.arg(from_file)
  AND NOT EXISTS (
    SELECT 1 FROM fetched_subtitle AS kept
    WHERE kept.video_file = sqlc.arg(to_file) AND kept.provider = fetched_subtitle.provider
      AND kept.provider_file_id = fetched_subtitle.provider_file_id);

-- name: DeleteFetchedSubtitlesOfFile :exec
-- What was fetched for a copy that its file had fetched too.
DELETE FROM fetched_subtitle WHERE video_file = sqlc.arg(file_id);
