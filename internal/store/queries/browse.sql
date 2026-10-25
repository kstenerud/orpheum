-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Browse (cut-a section 5, Browse): lists in a locale's order (D47), a title's detail and a
-- series' episodes. Search's three routes are in search.go: sqlc cannot parse FTS5's MATCH. Only what has a live file is
-- listed: a film whose files are all gone is kept, with its history, but not shown. The live-file
-- check starts from the title's live versions, in that order by CROSS JOIN: started from file, it
-- read every file for every title, seconds at the reference scale (audit-2026-09-25). Each list
-- holds only the libraries its caller reaches: all of them, or those in library_ids_json (D243
-- section 4).

-- name: ListFilmsByName :many
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, title.year, title.identification_state, item.library_id, item.added_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM sort_key
JOIN item ON item.id = sort_key.item_id
JOIN title ON title.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'movie' AND item.removed_at IS NULL
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
    OR item.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  AND (sort_key.collation_key > sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id > sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY sort_key.collation_key, sort_key.item_id
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsByAdded :many
SELECT item.id AS item_id, item_text.name, title.year, title.identification_state, item.library_id, item.added_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.kind = 'movie' AND item.removed_at IS NULL
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
    OR item.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  AND (item.added_at < sqlc.arg(before_at) OR (item.added_at = sqlc.arg(before_at) AND item.id < sqlc.arg(before_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY item.added_at DESC, item.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByName :many
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, collection.year, collection.identification_state, item.library_id,
  item.added_at, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM sort_key
JOIN item ON item.id = sort_key.item_id
JOIN collection ON collection.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'series' AND item.removed_at IS NULL
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
    OR item.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  AND (sort_key.collation_key > sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id > sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN file ON file.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND file.removed_at IS NULL
  )
ORDER BY sort_key.collation_key, sort_key.item_id
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByAdded :many
-- The series are read first, by CROSS JOIN, and sorted: they are few, and walking item_added
-- newest first would pass every film and episode added between them, about seventy for each
-- series at the reference scale.
SELECT item.id AS item_id, item_text.name, collection.year, collection.identification_state, item.library_id, item.added_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM collection
CROSS JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.kind = 'series' AND item.removed_at IS NULL
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
    OR item.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  AND (item.added_at < sqlc.arg(before_at) OR (item.added_at = sqlc.arg(before_at) AND item.id < sqlc.arg(before_id)))
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN file ON file.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND file.removed_at IS NULL
  )
ORDER BY item.added_at DESC, item.id DESC
LIMIT sqlc.arg(max_rows);

-- name: GetTitleDetail :one
-- A title, with a failure of its own to be fetched and a person's refresh waiting (D231 section 4).
SELECT title.id, title.kind, title.year, title.runtime_ms, title.identification_state, title.followed_source, title.followed_source_id,
  item.library_id, item.added_at, item_text.name, item_text.overview, item_text.tagline,
  fetch_failure.failure AS fetch_failure, fetch_failure.failed_at AS fetch_failed_at,
  refresh_request.requested_at AS refresh_requested_at, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
LEFT JOIN fetch_failure ON fetch_failure.item_id = title.id
LEFT JOIN refresh_request ON refresh_request.item_id = title.id
WHERE title.id = sqlc.arg(id) AND item.removed_at IS NULL;

-- name: ListTitleFiles :many
-- A title's versions and their live files, a version's parts in order.
SELECT version.id AS version_id, version.edition_id, file.id AS file_id, file.path, file.size_bytes, file.part_index
FROM version
JOIN file ON file.version_id = version.id
WHERE version.title_id = sqlc.arg(title_id) AND version.removed_at IS NULL AND file.removed_at IS NULL
ORDER BY version.id, file.part_index, file.id;

-- name: ListStreamsOfVideoFile :many
SELECT id, kind, codec, language, container_title, channel_count, disposition_flags
FROM stream
WHERE video_file = sqlc.arg(video_file)
ORDER BY kind, stream_index;

-- name: ListTitleExtras :many
SELECT extra_relation.extra_title, extra_relation.extra_type, item_text.name
FROM extra_relation
JOIN item ON item.id = extra_relation.extra_title
LEFT JOIN item_text ON item_text.item_id = extra_relation.extra_title AND item_text.is_original = 1
WHERE extra_relation.owner_title = sqlc.arg(owner_title) AND item.removed_at IS NULL
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = extra_relation.extra_title AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY extra_relation.position, extra_relation.extra_title;

-- name: GetEpisodePlace :one
-- An episode's series and its place in the aired order.
SELECT ordering.collection_id, position.group_number, position.number_in_group, item_text.name AS series_name
FROM position
JOIN ordering ON ordering.id = position.ordering_id
LEFT JOIN item_text ON item_text.item_id = ordering.collection_id AND item_text.is_original = 1
WHERE position.title_id = sqlc.arg(title_id) AND ordering.name = 'aired';

-- name: GetPlaceInOrdering :one
-- A member's place in one of its collection's named orderings (D96, D159): its group and number in
-- it, and how many groups the ordering has, one for an ordering that is a single run (absolute).
SELECT position.group_number, position.number_in_group,
  (SELECT count(DISTINCT coalesce(other.group_number, 0)) FROM position AS other WHERE other.ordering_id = ordering.id)
    AS group_count
FROM ordering
JOIN position ON position.ordering_id = ordering.id
WHERE ordering.collection_id = sqlc.arg(collection_id) AND ordering.name = sqlc.arg(ordering_name)
  AND position.title_id = sqlc.arg(title_id);

-- name: GetChosenOrdering :one
-- The ordering a person chose to watch a series in (D159 section 1), a preference at the series'
-- work scope, when the series still has it; none when they chose none, or one it no longer has.
SELECT ordering.name
FROM preference
JOIN ordering ON ordering.collection_id = preference.scope_id
  AND ordering.name = CAST(json_extract(preference.value_json, '$') AS TEXT)
WHERE preference.user_id = sqlc.arg(user_id) AND preference.scope = 'work'
  AND preference.scope_id = sqlc.arg(collection_id) AND preference.axis = sqlc.arg(axis);

-- name: GetSeriesDetail :one
-- A series, with a failure of its own to be fetched and a person's refresh waiting (D231 section 4).
SELECT collection.id, collection.year, collection.identification_state, collection.followed_source, collection.followed_source_id,
  item.library_id, item_text.name, item_text.overview,
  fetch_failure.failure AS fetch_failure, fetch_failure.failed_at AS fetch_failed_at,
  refresh_request.requested_at AS refresh_requested_at, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
LEFT JOIN fetch_failure ON fetch_failure.item_id = collection.id
LEFT JOIN refresh_request ON refresh_request.item_id = collection.id
WHERE collection.id = sqlc.arg(id) AND collection.kind = 'series' AND item.removed_at IS NULL;

-- name: ListSeriesEpisodeDetails :many
-- A series' episodes in one of its named orderings (D159; the one its person watches in), each
-- with whether it has a live file.
SELECT position.title_id, position.group_number, position.number_in_group, item_text.name, title.year, title.runtime_ms,
  title.identification_state,
  EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = position.title_id AND version.removed_at IS NULL AND file.removed_at IS NULL
  ) AS available
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN title ON title.id = position.title_id
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = position.title_id AND item_text.is_original = 1
WHERE ordering.collection_id = sqlc.arg(collection_id) AND ordering.name = sqlc.arg(ordering_name) AND item.removed_at IS NULL
ORDER BY position.sort_order;

-- name: ListSeasonEpisodeDetails :many
-- A page of one season's episodes, a group of one of the series' named orderings (D159; the one its
-- person watches in), after the episode at after_sort_order, each with whether it has a live file;
-- aired's season 0 holds the specials and the episodes with no season.
SELECT position.title_id, position.group_number, position.number_in_group, position.sort_order, item_text.name,
  title.year, title.runtime_ms, title.identification_state,
  EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = position.title_id AND version.removed_at IS NULL AND file.removed_at IS NULL
  ) AS available
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN title ON title.id = position.title_id
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = position.title_id AND item_text.is_original = 1
WHERE ordering.collection_id = sqlc.arg(collection_id) AND ordering.name = sqlc.arg(ordering_name) AND item.removed_at IS NULL
  AND coalesce(position.group_number, 0) = CAST(sqlc.arg(season_number) AS INTEGER)
  AND position.sort_order > sqlc.arg(after_sort_order)
ORDER BY position.sort_order
LIMIT sqlc.arg(max_rows);

-- name: ListLiveFileLocationsOfTitles :many
-- Where each of some titles' live playable files is: its node and the path it reported, for
-- whether any can be read now (D241 section 3).
SELECT version.title_id AS item_id, file.node_id, file.reported_path_raw,
  CAST(file.payload_hash IS NULL AS INTEGER) AS is_unread
FROM version
JOIN file ON file.version_id = version.id
WHERE version.title_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_id_list_json) AS TEXT)))
  AND version.removed_at IS NULL AND file.removed_at IS NULL AND file.filing IN ('listed', 'extra', 'unidentified')
ORDER BY version.title_id, file.id;

-- name: ListLiveFileLocationsOfSeries :many
-- Where each of some series' episodes' live playable files is, by the series.
SELECT ordering.collection_id AS item_id, file.node_id, file.reported_path_raw,
  CAST(file.payload_hash IS NULL AS INTEGER) AS is_unread
FROM ordering
JOIN position ON position.ordering_id = ordering.id
JOIN version ON version.title_id = position.title_id AND version.removed_at IS NULL
JOIN file ON file.version_id = version.id AND file.removed_at IS NULL AND file.filing IN ('listed', 'extra', 'unidentified')
WHERE ordering.collection_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_id_list_json) AS TEXT)))
  AND ordering.name = 'aired'
ORDER BY ordering.collection_id, file.id;

-- name: GetLibraryOfItem :one
-- An item's library, which says who may reach it (D243 section 4); NULL for one in none.
SELECT library_id FROM item WHERE id = sqlc.arg(id);

-- name: GetTitleOfFile :one
-- What a file is, for now playing: its title and name when one is filed, and its path.
SELECT file.path, title.id AS title_id, item_text.name
FROM file
LEFT JOIN version ON version.id = file.version_id
LEFT JOIN title ON title.id = version.title_id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE file.id = sqlc.arg(id);

-- name: ListUnreadFilesOfTitle :many
-- A title's live videos its listing stored that are not read yet, by where each is, for a person
-- who opened its page to have them read first (D304 section 4).
SELECT file.id, file.library_id, file.node_id, file.reported_path_raw
FROM version
JOIN file ON file.version_id = version.id
WHERE version.title_id = sqlc.arg(title_id) AND version.removed_at IS NULL AND file.removed_at IS NULL
  AND file.payload_hash IS NULL AND file.sidecar_kind IS NULL
ORDER BY file.id;

-- name: ListOrderingMemberFilesWithPositions :many
-- A collection's members in one of its named orderings (D159; 'aired' for a series today), each
-- live file of each with a person's position in it and when it was watched, and the file's length
-- where its probe knows it: where the person is in the sequence, for its Next Up (D160) and the
-- members a look at its page raises (D324 section 3). Each member's group, a series' season, 0 for
-- the specials and those with none, which a look at one season keeps to, and its number in it and
-- name, which its Next Up shows. A member's files come as its page lists them, so the first is the
-- one its page plays first.
SELECT position.title_id, CAST(coalesce(position.group_number, 0) AS INTEGER) AS group_number,
  CAST(coalesce(position.number_in_group, 0) AS INTEGER) AS number_in_group,
  CAST(coalesce(item_text.name, '') AS TEXT) AS name,
  file.id AS file_id,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = file.id), 0) AS INTEGER)
    AS duration_ms,
  play_position.position_ms, play_position.position_read_at
FROM ordering
JOIN position ON position.ordering_id = ordering.id
JOIN item ON item.id = position.title_id AND item.removed_at IS NULL
JOIN version ON version.title_id = position.title_id AND version.removed_at IS NULL
JOIN file ON file.version_id = version.id AND file.removed_at IS NULL AND file.filing IN ('listed', 'unidentified')
LEFT JOIN item_text ON item_text.item_id = position.title_id AND item_text.is_original = 1
LEFT JOIN play_position ON play_position.file_id = file.id AND play_position.user_id = sqlc.arg(user_id)
WHERE ordering.collection_id = sqlc.arg(collection_id) AND ordering.name = sqlc.arg(ordering_name)
ORDER BY position.sort_order, version.id, file.part_index, file.id;
