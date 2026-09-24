-- SPDX-License-Identifier: GPL-3.0-or-later
-- Matching a title to its provider (identification.md; D170, D172): the fetch job's reads and
-- writes in library.db. The kept search results are cache.db's.

-- name: GetTitleForMatch :one
SELECT title.id, title.kind, title.year, title.ident, title.source, title.source_id, item.library_id,
  library.metadata_source, item_text.name
FROM title
JOIN item ON item.id = title.id
JOIN library ON library.id = item.library_id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.id = sqlc.arg(id) AND item.removed_at IS NULL;

-- name: ListItemProviderIDs :many
SELECT provider, provider_id
FROM item_provider_id
WHERE item_id = sqlc.arg(item_id)
ORDER BY provider, provider_id;

-- name: UpsertProviderResponse :exec
INSERT INTO provider_response (title_id, provider, fetched_at, body)
VALUES (sqlc.arg(title_id), sqlc.arg(provider), sqlc.arg(fetched_at), sqlc.arg(body))
ON CONFLICT (title_id, provider) DO UPDATE SET fetched_at = excluded.fetched_at, body = excluded.body;

-- name: GetProviderResponse :one
SELECT body
FROM provider_response
WHERE title_id = sqlc.arg(title_id) AND provider = sqlc.arg(provider);

-- name: SetTitleMatch :exec
-- The outcome of a match: the source and its id, or ambiguous, or 'none' when the provider has
-- nothing. A confirmed match is a person's and stays (D172).
UPDATE title SET source = sqlc.arg(source), source_id = sqlc.arg(source_id),
  ident = CASE WHEN ident = 'confirmed' THEN ident ELSE sqlc.arg(ident) END
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesOfTitle :many
SELECT file.id, file.path
FROM file
JOIN version ON version.id = file.version_id
WHERE version.title_id = sqlc.arg(title_id) AND file.removed_at IS NULL
ORDER BY file.path;

-- name: MoveVersionsBetweenTitles :exec
UPDATE version SET title_id = sqlc.arg(to_title)
WHERE title_id = sqlc.arg(from_title);

-- name: MergeItem :exec
-- A title found to be another already here: it leaves a tombstone pointing at the one that
-- stays (D173).
UPDATE item SET merged_into = sqlc.arg(merged_into), removed_at = sqlc.arg(removed_at)
WHERE id = sqlc.arg(id);

-- name: ListTitlesAwaitingMatch :many
-- Films with live files that no source has answered for yet (D170): the fetch queue's work.
SELECT title.id
FROM title
JOIN item ON item.id = title.id
WHERE item.library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND title.kind = 'movie'
  AND title.source IS NULL AND title.ident = 'unidentified'
  AND EXISTS (
    SELECT 1 FROM file JOIN version ON version.id = file.version_id
    WHERE version.title_id = title.id AND file.removed_at IS NULL
  )
ORDER BY title.id;
