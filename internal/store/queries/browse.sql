-- SPDX-License-Identifier: GPL-3.0-or-later
-- Browse (cut-a section 5, Browse): lists in a locale's order (D47), a title's detail and a
-- series' episodes. Search's three routes are in search.go: sqlc cannot parse FTS5's MATCH. Only what has a live file is
-- listed: a film whose files are all gone is kept, with its history, but not shown. The live-file
-- check starts from the title's live versions, in that order by CROSS JOIN: started from file, it
-- read every file for every title, seconds at the reference scale (audit-2026-09-25).

-- name: ListFilmsByName :many
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, title.year, title.identification_state, item.library_id, item.added_at
FROM sort_key
JOIN item ON item.id = sort_key.item_id
JOIN title ON title.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'movie' AND item.removed_at IS NULL
  AND (sort_key.collation_key > sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id > sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY sort_key.collation_key, sort_key.item_id
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsByAdded :many
SELECT item.id AS item_id, item_text.name, title.year, title.identification_state, item.library_id, item.added_at
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.kind = 'movie' AND item.removed_at IS NULL
  AND (item.added_at < sqlc.arg(before_at) OR (item.added_at = sqlc.arg(before_at) AND item.id < sqlc.arg(before_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY item.added_at DESC, item.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByName :many
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, collection.year, collection.identification_state, item.library_id,
  item.added_at
FROM sort_key
JOIN item ON item.id = sort_key.item_id
JOIN collection ON collection.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'series' AND item.removed_at IS NULL
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
SELECT item.id AS item_id, item_text.name, collection.year, collection.identification_state, item.library_id, item.added_at
FROM collection
CROSS JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.kind = 'series' AND item.removed_at IS NULL
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
SELECT title.id, title.kind, title.year, title.runtime_ms, title.identification_state, title.followed_source, title.followed_source_id,
  item.library_id, item.added_at, item_text.name, item_text.overview, item_text.tagline
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
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
ORDER BY extra_relation.position, extra_relation.extra_title;

-- name: GetEpisodePlace :one
-- An episode's series and its place in the aired order.
SELECT ordering.collection_id, position.group_number, position.number_in_group, item_text.name AS series_name
FROM position
JOIN ordering ON ordering.id = position.ordering_id
LEFT JOIN item_text ON item_text.item_id = ordering.collection_id AND item_text.is_original = 1
WHERE position.title_id = sqlc.arg(title_id) AND ordering.name = 'aired';

-- name: GetSeriesDetail :one
SELECT collection.id, collection.year, collection.identification_state, collection.followed_source, collection.followed_source_id,
  item.library_id, item_text.name, item_text.overview
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.id = sqlc.arg(id) AND collection.kind = 'series' AND item.removed_at IS NULL;

-- name: ListSeriesEpisodeDetails :many
-- A series' episodes in aired order, each with whether it has a live file.
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
WHERE ordering.collection_id = sqlc.arg(collection_id) AND ordering.name = 'aired' AND item.removed_at IS NULL
ORDER BY position.sort_order;
