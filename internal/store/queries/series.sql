-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A series is what is identified for shows (D209): its match state, its episodes' places, and
-- what the pass and the matcher read and write about it.

-- name: GetItemKind :one
SELECT kind FROM item WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: GetSeriesForMatch :one
SELECT sequence.id, item.year, item.followed_source, item.followed_source_id, item.identification_state,
  item.filing_library_id, library.metadata_source, item_text.name
FROM sequence
JOIN item ON item.id = sequence.id
JOIN library ON library.id = item.filing_library_id
LEFT JOIN item_text ON item_text.item_id = sequence.id AND item_text.is_original = 1
WHERE sequence.id = sqlc.arg(id) AND sequence.kind = 'series' AND item.removed_at IS NULL;

-- name: SetSeriesMatch :exec
-- The outcome of a series' match: the source with its id, together, as a title's match writes them
-- (an id is an id at a source, D303 M5), or no id, the source left as it is. A confirmed match is a
-- person's and stays (D172).
UPDATE item SET followed_source_id = sqlc.arg(followed_source_id),
  followed_source = coalesce(sqlc.narg(followed_source), followed_source),
  identification_state = CASE WHEN identification_state = 'confirmed' THEN identification_state ELSE sqlc.arg(identification_state) END
WHERE id = sqlc.arg(id) AND kind = 'series';

-- name: ConfirmSeriesMatch :exec
-- A person confirmed a series' match as it stands, as ConfirmWorkMatch a title's (D172).
UPDATE item SET identification_state = 'confirmed'
WHERE id = sqlc.arg(id) AND kind = 'series'
  -- Only when it changes something (D303, DL9).
  AND identification_state IS NOT 'confirmed';

-- name: MarkSeriesUnmatched :exec
-- The provider found nothing for a series whose own fields have no source yet.
UPDATE item SET followed_source = 'none'
WHERE id = sqlc.arg(id) AND kind = 'series' AND followed_source IS NULL;

-- name: UpdateSeriesResolved :exec
-- A pass's resolution of a series' own fields; its match is the matcher's and a person's.
UPDATE item SET year = sqlc.arg(year), released_on = sqlc.arg(released_on),
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
SELECT position.work_id, grp.number AS group_number, position.number_in_group, item.followed_source, item.added_at
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN work ON work.id = position.work_id
JOIN item ON item.id = work.id
LEFT JOIN grp ON grp.id = position.grp_id
WHERE ordering.sequence_id = sqlc.arg(sequence_id) AND ordering.name = 'aired' AND item.removed_at IS NULL
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
SELECT sequence.id, item.year, item.identification_state, item.followed_source, item.followed_source_id,
  item.filing_library_id, item_text.name, fetch_failure.failure AS fetch_failure, fetch_failure.failed_at AS fetch_failed_at,
  refresh_request.requested_at AS refresh_requested_at
FROM sequence
JOIN item ON item.id = sequence.id
LEFT JOIN item_text ON item_text.item_id = sequence.id AND item_text.is_original = 1
LEFT JOIN fetch_failure ON fetch_failure.item_id = sequence.id
LEFT JOIN refresh_request ON refresh_request.item_id = sequence.id
WHERE sequence.kind = 'series' AND item.identification_state = sqlc.arg(identification_state) AND item.removed_at IS NULL
  AND (item.identification_state != 'unidentified' OR item.followed_source = 'none'
    OR fetch_failure.item_id IS NOT NULL OR refresh_request.item_id IS NOT NULL)
  AND EXISTS (
    SELECT 1 FROM sequence_member
    CROSS JOIN version ON version.work_id = sequence_member.member_work
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE sequence_member.sequence_id = sequence.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  -- A page, as ListWorksByIdentification's (D425 section 7).
  AND (COALESCE(item_text.name, '') > sqlc.arg(after_name)
    OR (COALESCE(item_text.name, '') = sqlc.arg(after_name) AND sequence.id > sqlc.arg(after_id)))
ORDER BY COALESCE(item_text.name, ''), sequence.id
LIMIT sqlc.arg(row_limit);

-- name: ListSeriesAwaitingMatch :many
-- Series in a provider's library that are not matched, have an episode no NFO speaks for, and
-- have no failure of their own (D231 section 3), with an episode's live file a library lists: one
-- emptied by a new reading of its folders, or no library covers, is searched for no one.
SELECT sequence.id
FROM sequence
JOIN item ON item.id = sequence.id
JOIN library ON library.id = item.filing_library_id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND sequence.kind = 'series'
  AND library.metadata_source != 'nfo' AND item.identification_state = 'unidentified'
  AND item.followed_source IS NOT 'none'
  AND NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = sequence.id)
  AND EXISTS (
    SELECT 1 FROM ordering
    JOIN position ON position.ordering_id = ordering.id
    JOIN item AS episode ON episode.id = position.work_id
    WHERE ordering.sequence_id = sequence.id AND ordering.name = 'aired'
      AND (episode.followed_source IS NULL OR episode.followed_source != 'nfo')
  )
  AND EXISTS (
    SELECT 1 FROM sequence_member
    CROSS JOIN version ON version.work_id = sequence_member.member_work
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE sequence_member.sequence_id = sequence.id AND version.removed_at IS NULL
  )
ORDER BY sequence.id;

-- name: ReturnAmbiguousSeriesToItsFirstMatch :exec
-- A series the matcher left ambiguous that its kept results now settle: owed its first match again.
UPDATE item SET identification_state = 'unidentified'
WHERE id = sqlc.arg(id) AND kind = 'series' AND identification_state = 'ambiguous';
