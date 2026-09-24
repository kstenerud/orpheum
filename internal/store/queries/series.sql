-- SPDX-License-Identifier: GPL-3.0-or-later
-- A series is what is identified for shows (D209): its match state, its episodes' places, and
-- what the pass and the matcher read and write about it.

-- name: GetItemKind :one
SELECT kind FROM item WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: GetSeriesForMatch :one
SELECT collection.id, collection.year, collection.source, collection.source_id, collection.ident,
  item.library_id, library.metadata_source, item_text.name
FROM collection
JOIN item ON item.id = collection.id
JOIN library ON library.id = item.library_id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.id = sqlc.arg(id) AND collection.kind = 'series' AND item.removed_at IS NULL;

-- name: SetSeriesMatch :exec
-- The outcome of a series' match. A confirmed match is a person's and stays (D172).
UPDATE collection SET source_id = sqlc.arg(source_id),
  ident = CASE WHEN ident = 'confirmed' THEN ident ELSE sqlc.arg(ident) END
WHERE id = sqlc.arg(id) AND kind = 'series';

-- name: MarkSeriesUnmatched :exec
-- The provider found nothing for a series whose own fields have no source yet.
UPDATE collection SET source = 'none'
WHERE id = sqlc.arg(id) AND kind = 'series' AND source IS NULL;

-- name: UpdateSeriesResolved :exec
-- A pass's resolution of a series' own fields; its match is the matcher's and a person's.
UPDATE collection SET year = sqlc.arg(year), source = sqlc.arg(source),
  ident = CASE WHEN ident IN ('identified', 'ambiguous', 'confirmed') THEN ident ELSE sqlc.arg(ident) END
WHERE id = sqlc.arg(id) AND kind = 'series';

-- name: ListSeriesEpisodes :many
-- A series' episodes in its aired order, with where each takes its values from and when it
-- first appeared: a new episode missing from a season listing fetched before it arrived is
-- what fetches that listing again (D209 section 3).
SELECT position.title_id, position.grp, position.number, title.source, item.created_at
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN title ON title.id = position.title_id
JOIN item ON item.id = title.id
WHERE ordering.collection_id = sqlc.arg(collection_id) AND ordering.name = 'aired' AND item.removed_at IS NULL
ORDER BY position.sort;

-- name: GetProviderResponseFetchedAt :one
SELECT fetched_at
FROM provider_response
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider) AND part = sqlc.arg(part);

-- name: GetSeriesOfTitle :one
-- The series an episode belongs to, found through its place rather than its name.
SELECT ordering.collection_id
FROM position
JOIN ordering ON ordering.id = position.ordering_id
WHERE position.title_id = sqlc.arg(title_id) AND ordering.name = 'aired';

-- name: ListSeriesByIdent :many
-- The series in one state of identification, for the chooser's list.
SELECT collection.id, collection.year, collection.ident, collection.source, collection.source_id,
  item.library_id, item_text.name
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.kind = 'series' AND collection.ident = sqlc.arg(ident) AND item.removed_at IS NULL
ORDER BY item_text.name, collection.id;

-- name: ListSeriesAwaitingMatch :many
-- Series in a provider's library that are not matched and have an episode no NFO speaks for.
SELECT collection.id
FROM collection
JOIN item ON item.id = collection.id
JOIN library ON library.id = item.library_id
WHERE item.library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND collection.kind = 'series'
  AND library.metadata_source != 'nfo' AND collection.ident = 'unidentified'
  AND collection.source IS NOT 'none'
  AND EXISTS (
    SELECT 1 FROM ordering
    JOIN position ON position.ordering_id = ordering.id
    JOIN title ON title.id = position.title_id
    WHERE ordering.collection_id = collection.id AND ordering.name = 'aired'
      AND (title.source IS NULL OR title.source != 'nfo')
  )
ORDER BY collection.id;
