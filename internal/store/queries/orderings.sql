-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A series' orderings beside aired, from its provider's episode groups (episode-orderings.md, D96,
-- D159): each derived from what is stored, its list and each group fetched (D170), and kept as
-- what is stored says, writing only what changed (D303, DL9).

-- name: UpsertProviderOrdering :exec
-- One of a series' orderings its provider lists, by its name, with its kind, its provider's id and
-- label for it and how many episodes the provider lists in it.
INSERT INTO ordering (sequence_id, name, provider, kind, source_id, label, listed_count)
VALUES (sqlc.arg(sequence_id), sqlc.arg(name), sqlc.arg(provider), sqlc.arg(kind), sqlc.arg(source_id),
  sqlc.arg(label), sqlc.arg(listed_count))
ON CONFLICT (sequence_id, name) DO UPDATE SET provider = excluded.provider, kind = excluded.kind,
  source_id = excluded.source_id, label = excluded.label, listed_count = excluded.listed_count
-- Only when it changes something (D303, DL9).
WHERE (ordering.provider, ordering.kind, ordering.source_id, ordering.label, ordering.listed_count)
  IS NOT (excluded.provider, excluded.kind, excluded.source_id, excluded.label, excluded.listed_count);

-- name: ListProviderOrderingsOfSeries :many
-- A series' orderings a provider listed, with what each says of itself.
SELECT id, name, kind, source_id, label, listed_count
FROM ordering
WHERE sequence_id = sqlc.arg(sequence_id) AND provider = sqlc.arg(provider) AND source_id IS NOT NULL
ORDER BY id;

-- name: ListSeriesEpisodeSourceIDs :many
-- A series' live episodes, each with its id at the provider it takes its values from: what places it
-- in a provider's ordering, by the episode that ordering lists.
SELECT work.id AS work_id, CAST(item.followed_source_id AS TEXT) AS followed_source_id
FROM series_member
JOIN work ON work.id = series_member.member_work
JOIN item ON item.id = work.id AND item.removed_at IS NULL
WHERE series_member.sequence_id = sqlc.arg(sequence_id) AND item.followed_source = sqlc.arg(provider)
  AND item.followed_source_id IS NOT NULL
ORDER BY work.id;

-- name: ListOrderingPositions :many
-- Every place an ordering holds.
SELECT position.work_id, grp.number AS group_number, position.number_in_group, position.sort_order
FROM position
LEFT JOIN grp ON grp.id = position.grp_id
WHERE position.ordering_id = sqlc.arg(ordering_id)
ORDER BY position.work_id;

-- name: DeletePosition :exec
-- A title an ordering no longer places: its provider's group no longer lists it, or it left the series.
DELETE FROM position WHERE ordering_id = sqlc.arg(ordering_id) AND work_id = sqlc.arg(work_id);

-- name: ListOrderingGroupTexts :many
-- The names an ordering gives its groups, in every language.
SELECT grp.number AS group_number, group_text.language, group_text.name
FROM grp
JOIN group_text ON group_text.grp_id = grp.id
WHERE grp.ordering_id = sqlc.arg(ordering_id)
ORDER BY grp.number, group_text.language;

-- name: InsertGroup :exec
-- A group of an ordering, a season or a disc (D426 section 5), made before the first position or name
-- in it; one there already is kept.
INSERT INTO grp (ordering_id, number) VALUES (sqlc.arg(ordering_id), sqlc.arg(number))
ON CONFLICT (ordering_id, number) DO NOTHING;

-- name: GetGroup :one
-- A group of an ordering by its number.
SELECT id FROM grp WHERE ordering_id = sqlc.arg(ordering_id) AND number = sqlc.arg(number);

-- name: UpsertGroupText :exec
-- A group's name in an ordering, in one language; InsertGroup makes the group first, and a group not
-- made has no id, which group_text refuses.
INSERT INTO group_text (grp_id, language, name)
VALUES ((SELECT grp.id FROM grp WHERE grp.ordering_id = sqlc.arg(ordering_id) AND grp.number = sqlc.arg(group_number)),
  sqlc.arg(language), sqlc.arg(name))
ON CONFLICT (grp_id, language) DO UPDATE SET name = excluded.name
-- Only when it changes something (D303, DL9).
WHERE group_text.name IS NOT excluded.name;

-- name: DeleteGroupText :exec
-- A group an ordering no longer has.
DELETE FROM group_text
WHERE grp_id = (SELECT grp.id FROM grp WHERE grp.ordering_id = sqlc.arg(ordering_id) AND grp.number = sqlc.arg(group_number))
  AND language = sqlc.arg(language);

-- name: ListLibrarySeriesWithFetchedGroups :many
-- A library's series with an episode group fetched, whose ordering a pass keeps placing its episodes
-- in as they come and go.
SELECT item.id AS sequence_id
FROM item
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.kind = 'series' AND item.removed_at IS NULL
  AND EXISTS (
    SELECT 1 FROM ordering
    JOIN provider_response ON provider_response.item_id = ordering.sequence_id
      AND provider_response.provider = ordering.provider
      AND provider_response.part = 'episode_group/' || ordering.source_id
    WHERE ordering.sequence_id = item.id AND ordering.source_id IS NOT NULL
  )
ORDER BY item.id;

-- name: ListChosenGroupsToFetch :many
-- The episode groups of a series that a person chose to watch it in and that are not fetched yet: a
-- first fetch, on their act (D170; episode-orderings.md section 3.4).
SELECT DISTINCT CAST(ordering.source_id AS TEXT) AS source_id
FROM ordering
JOIN preference ON preference.scope = 'item' AND preference.scope_id = ordering.sequence_id
  AND preference.axis = sqlc.arg(axis) AND CAST(json_extract(preference.value_json, '$') AS TEXT) = ordering.name
WHERE ordering.sequence_id = sqlc.arg(sequence_id) AND ordering.provider = sqlc.arg(provider)
  AND ordering.source_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM provider_response
    WHERE provider_response.item_id = ordering.sequence_id AND provider_response.provider = ordering.provider
      AND provider_response.part = 'episode_group/' || ordering.source_id
  )
ORDER BY 1;

-- name: ListSeriesOwingOrderingParts :many
-- Matched series owed a part of their orderings that should have been fetched and was not (D335 section
-- 1): a group a person chose, first; then the list of groups, for a series fetched before its record
-- carried it. A series with a failure of its own waits for a person (D231 section 4). A few at a time,
-- never a flood (D335 section 3).
SELECT sequence.id, CAST(EXISTS (
    SELECT 1 FROM ordering
    JOIN preference ON preference.scope = 'item' AND preference.scope_id = ordering.sequence_id
      AND preference.axis = sqlc.arg(axis) AND CAST(json_extract(preference.value_json, '$') AS TEXT) = ordering.name
    WHERE ordering.sequence_id = sequence.id AND ordering.source_id IS NOT NULL
      AND NOT EXISTS (
        SELECT 1 FROM provider_response
        WHERE provider_response.item_id = ordering.sequence_id AND provider_response.provider = ordering.provider
          AND provider_response.part = 'episode_group/' || ordering.source_id
      )
  ) AS INTEGER) AS is_chosen
FROM sequence
JOIN item ON item.id = sequence.id AND item.removed_at IS NULL
WHERE sequence.kind = 'series' AND item.followed_source = sqlc.arg(provider)
  AND item.followed_source_id IS NOT NULL
  AND item.identification_state IN ('identified', 'confirmed')
  AND NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = sequence.id)
  AND EXISTS (
    SELECT 1 FROM provider_response
    WHERE provider_response.item_id = sequence.id AND provider_response.provider = sqlc.arg(provider)
      AND provider_response.part = ''
  )
  AND (
    NOT EXISTS (
      SELECT 1 FROM provider_response
      WHERE provider_response.item_id = sequence.id AND provider_response.provider = sqlc.arg(provider)
        AND provider_response.part = 'episode_groups'
    )
    OR EXISTS (
      SELECT 1 FROM ordering
      JOIN preference ON preference.scope = 'item' AND preference.scope_id = ordering.sequence_id
        AND preference.axis = sqlc.arg(axis) AND CAST(json_extract(preference.value_json, '$') AS TEXT) = ordering.name
      WHERE ordering.sequence_id = sequence.id AND ordering.source_id IS NOT NULL
        AND NOT EXISTS (
          SELECT 1 FROM provider_response
          WHERE provider_response.item_id = ordering.sequence_id AND provider_response.provider = ordering.provider
            AND provider_response.part = 'episode_group/' || ordering.source_id
        )
    )
  )
ORDER BY is_chosen DESC, sequence.id
LIMIT sqlc.arg(max_rows);
