-- SPDX-License-Identifier: GPL-3.0-or-later
-- Matching a title to its provider (identification.md; D170, D172): the fetch job's reads and
-- writes in library.db. The kept search results are cache.db's.

-- name: GetTitleForMatch :one
SELECT title.id, title.kind, title.year, title.identification, title.followed_source, title.followed_source_id, item.library_id,
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
INSERT INTO provider_response (item_id, provider, part, fetched_at, body_gzip)
VALUES (sqlc.arg(item_id), sqlc.arg(provider), sqlc.arg(part), sqlc.arg(fetched_at), sqlc.arg(body_gzip))
ON CONFLICT (item_id, provider, part) DO UPDATE SET fetched_at = excluded.fetched_at, body_gzip = excluded.body_gzip;

-- name: GetProviderResponse :one
-- An item's stored response: part '' is the item itself, 'season/N' a series' season listing.
SELECT body_gzip
FROM provider_response
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider) AND part = sqlc.arg(part);

-- name: SetTitleMatch :exec
-- The outcome of a match: the source and its id, or ambiguous, or 'none' when the provider has
-- nothing. A confirmed match is a person's and stays (D172).
UPDATE title SET followed_source = sqlc.arg(followed_source), followed_source_id = sqlc.arg(followed_source_id),
  identification = CASE WHEN identification = 'confirmed' THEN identification ELSE sqlc.arg(identification) END
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
  AND title.followed_source IS NULL AND title.identification = 'unidentified'
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY title.id;

-- name: ListTitlesByIdent :many
-- The titles in one state of identification, for the chooser's list (cut-a section 5).
SELECT title.id, title.kind, title.year, title.identification, title.followed_source, title.followed_source_id, item.library_id,
  item_text.name
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.identification = sqlc.arg(identification) AND item.removed_at IS NULL
ORDER BY item_text.name, title.id;

-- name: DeleteItemProviderIDsOf :exec
-- A person's choice replaces the ids an earlier match recorded at that provider.
DELETE FROM item_provider_id
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider);

-- name: UpsertOverride :exec
-- A person's value for a field: the lock, kept over every source (D183).
INSERT INTO field_override (item_id, field, lang, override_value, origin, set_by_user_id, set_at)
VALUES (sqlc.arg(item_id), sqlc.arg(field), '', sqlc.arg(override_value), 'typed', sqlc.arg(set_by_user_id), sqlc.arg(set_at))
ON CONFLICT (item_id, field, lang) DO UPDATE SET
  override_value = excluded.override_value, origin = excluded.origin, set_by_user_id = excluded.set_by_user_id, set_at = excluded.set_at;

-- name: DeleteOverride :execrows
-- Removing an override is how a field returns to its source (D183). A person's correction is not
-- a tombstoned row (standards/sql.md section 6): its absence is the state.
DELETE FROM field_override
WHERE item_id = sqlc.arg(item_id) AND field = sqlc.arg(field) AND lang = '';
