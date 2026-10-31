-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A series is what is identified for shows (D209): its match state, its episodes' places, and
-- what the pass and the matcher read and write about it.

-- name: GetItemKind :one
SELECT kind FROM item WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: GetSeriesForMatch :one
SELECT collection.id, collection.year, collection.followed_source, collection.followed_source_id, collection.identification_state,
  item.filing_library_id, library.metadata_source, item_text.name
FROM collection
JOIN item ON item.id = collection.id
JOIN library ON library.id = item.filing_library_id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.id = sqlc.arg(id) AND collection.kind = 'series' AND item.removed_at IS NULL;

-- name: SetSeriesMatch :exec
-- The outcome of a series' match: the source with its id, together, as a title's match writes them
-- (an id is an id at a source, D303 M5), or no id, the source left as it is. A confirmed match is a
-- person's and stays (D172).
UPDATE collection SET followed_source_id = sqlc.arg(followed_source_id),
  followed_source = coalesce(sqlc.narg(followed_source), followed_source),
  identification_state = CASE WHEN identification_state = 'confirmed' THEN identification_state ELSE sqlc.arg(identification_state) END
WHERE id = sqlc.arg(id) AND kind = 'series';

-- name: ConfirmSeriesMatch :exec
-- A person confirmed a series' match as it stands, as ConfirmTitleMatch a title's (D172).
UPDATE collection SET identification_state = 'confirmed'
WHERE id = sqlc.arg(id) AND kind = 'series'
  -- Only when it changes something (D303, DL9).
  AND identification_state IS NOT 'confirmed';

-- name: MarkSeriesUnmatched :exec
-- The provider found nothing for a series whose own fields have no source yet.
UPDATE collection SET followed_source = 'none'
WHERE id = sqlc.arg(id) AND kind = 'series' AND followed_source IS NULL;

-- name: UpdateSeriesResolved :exec
-- A pass's resolution of a series' own fields; its match is the matcher's and a person's.
UPDATE collection SET year = sqlc.arg(year), released_on = sqlc.arg(released_on),
  followed_source = sqlc.arg(followed_source),
  identification_state = CASE WHEN identification_state IN ('identified', 'ambiguous', 'confirmed') THEN identification_state ELSE sqlc.arg(identification_state) END
WHERE id = sqlc.arg(id) AND kind = 'series'
  -- Only when it changes something (D303, DL9).
  AND (year, released_on, followed_source, identification_state) IS NOT (sqlc.arg(year), sqlc.arg(released_on),
    sqlc.arg(followed_source),
    CASE WHEN identification_state IN ('identified', 'ambiguous', 'confirmed') THEN identification_state ELSE sqlc.arg(identification_state) END);

-- name: ListLiveSeriesEpisodeSources :many
-- A series' episodes in its aired order, with where each takes its values from and when it
-- first appeared: a new episode missing from a season listing fetched before it arrived is
-- what fetches that listing again (D209 section 3).
SELECT position.title_id, position.group_number, position.number_in_group, title.followed_source, item.added_at
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN title ON title.id = position.title_id
JOIN item ON item.id = title.id
WHERE ordering.collection_id = sqlc.arg(collection_id) AND ordering.name = 'aired' AND item.removed_at IS NULL
ORDER BY position.sort_order;

-- name: GetProviderResponseFetchedAt :one
SELECT fetched_at
FROM provider_response
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider) AND part = sqlc.arg(part);

-- name: ListSeriesByIdentification :many
-- The series in one state of identification, for the chooser's list, with a failure of their own
-- to be fetched and a person's refresh waiting (D231 section 4). Only those a library its caller
-- reaches lists by an episode's live file: a series an older reading of the folders made, its episodes filed again elsewhere, waits
-- for no one (the owner's NAS, 2026-10-13), as does an unidentified one not searched yet unless its
-- fetch failed on its own or a person asked for it again: the list is what CountWaitingForAPerson
-- counts, and those asked for.
SELECT collection.id, collection.year, collection.identification_state, collection.followed_source, collection.followed_source_id,
  item.filing_library_id, item_text.name, fetch_failure.failure AS fetch_failure, fetch_failure.failed_at AS fetch_failed_at,
  refresh_request.requested_at AS refresh_requested_at
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
LEFT JOIN fetch_failure ON fetch_failure.item_id = collection.id
LEFT JOIN refresh_request ON refresh_request.item_id = collection.id
WHERE collection.kind = 'series' AND collection.identification_state = sqlc.arg(identification_state) AND item.removed_at IS NULL
  AND (collection.identification_state != 'unidentified' OR collection.followed_source = 'none'
    OR fetch_failure.item_id IS NOT NULL OR refresh_request.item_id IS NOT NULL)
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
ORDER BY item_text.name, collection.id;

-- name: ListSeriesAwaitingMatch :many
-- Series in a provider's library that are not matched, have an episode no NFO speaks for, and
-- have no failure of their own (D231 section 3), with an episode's live file a library lists: one
-- emptied by a new reading of its folders, or no library covers, is searched for no one.
SELECT collection.id
FROM collection
JOIN item ON item.id = collection.id
JOIN library ON library.id = item.filing_library_id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND collection.kind = 'series'
  AND library.metadata_source != 'nfo' AND collection.identification_state = 'unidentified'
  AND collection.followed_source IS NOT 'none'
  AND NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = collection.id)
  AND EXISTS (
    SELECT 1 FROM ordering
    JOIN position ON position.ordering_id = ordering.id
    JOIN title ON title.id = position.title_id
    WHERE ordering.collection_id = collection.id AND ordering.name = 'aired'
      AND (title.followed_source IS NULL OR title.followed_source != 'nfo')
  )
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
  )
ORDER BY collection.id;

-- name: ReturnAmbiguousSeriesToItsFirstMatch :exec
-- A series the matcher left ambiguous that its kept results now settle: owed its first match again.
UPDATE collection SET identification_state = 'unidentified'
WHERE id = sqlc.arg(id) AND kind = 'series' AND identification_state = 'ambiguous';
