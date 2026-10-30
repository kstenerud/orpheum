-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Every list's other direction (D366 section 2), and films and series by release date (section 1):
-- each the twin of its list in browse.sql, read by the same index the other way.

-- name: ListFilmsByNameDescending :many
-- Films Z to A, as ListFilmsByName reads them A to Z, after a place in this order.
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, title.year, title.identification_state, item.added_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM sort_key
JOIN item ON item.id = sort_key.item_id
JOIN title ON title.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'movie' AND item.removed_at IS NULL
  AND (sort_key.collation_key < sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id < sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY sort_key.collation_key DESC, sort_key.item_id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsByAddedAscending :many
-- Films oldest first, as ListFilmsByAdded reads them newest first, by title_film_found from its foot.
SELECT item.id AS item_id, item_text.name, title.year, title.identification_state, item.added_at,
  title.found_at, title.found_file_modified_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.kind = 'movie' AND item.removed_at IS NULL
  AND (title.found_at, title.found_file_modified_at, title.id)
    > (sqlc.arg(after_found_at), sqlc.arg(after_modified_at), sqlc.arg(after_id))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY title.found_at, title.found_file_modified_at, title.id
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsByReleased :many
-- Films newest released first (D366 section 1), by title_film_released from its top. A film with no
-- date is none of these, its row value NULL; ListFilmsUndated lists it after them.
SELECT item.id AS item_id, item_text.name, title.year, title.identification_state, item.added_at, title.released_on,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.kind = 'movie' AND item.removed_at IS NULL
  AND (title.released_on, title.id) < (CAST(sqlc.arg(before_released_on) AS INTEGER), CAST(sqlc.arg(before_id) AS INTEGER))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY title.released_on DESC, title.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsByReleasedAscending :many
-- Films oldest released first, as ListFilmsByReleased reads them newest first, by title_film_released.
SELECT item.id AS item_id, item_text.name, title.year, title.identification_state, item.added_at, title.released_on,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.kind = 'movie' AND item.removed_at IS NULL
  AND (title.released_on, title.id) > (CAST(sqlc.arg(after_released_on) AS INTEGER), CAST(sqlc.arg(after_id) AS INTEGER))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY title.released_on, title.id
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsUndated :many
-- The films with no release date, which a sort by it lists last whichever way it runs (D366 section
-- 3), by id, from title_film_released's NULLs.
SELECT item.id AS item_id, item_text.name, title.year, title.identification_state, item.added_at, title.released_on,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE title.kind = 'movie' AND title.released_on IS NULL AND item.removed_at IS NULL AND title.id > sqlc.arg(after_id)
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY title.id
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByNameDescending :many
-- Series Z to A, as ListSeriesByName reads them A to Z.
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, collection.year, collection.identification_state,
  item.added_at, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM sort_key
JOIN item ON item.id = sort_key.item_id
JOIN collection ON collection.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'series' AND item.removed_at IS NULL
  AND (sort_key.collation_key < sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id < sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY sort_key.collation_key DESC, sort_key.item_id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByAddedAscending :many
-- Series oldest first, as ListSeriesByAdded reads them newest first, by collection_series_found.
SELECT item.id AS item_id, item_text.name, collection.year, collection.identification_state, item.added_at,
  collection.found_at, collection.found_file_modified_at,
  EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.kind = 'series' AND item.removed_at IS NULL
  AND (collection.found_at, collection.found_file_modified_at, collection.id)
    > (sqlc.arg(after_found_at), sqlc.arg(after_modified_at), sqlc.arg(after_id))
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY collection.found_at, collection.found_file_modified_at, collection.id
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByReleased :many
-- Series newest first by their first air date (D366 section 1), by collection_series_released
-- from its top; ListSeriesUndated lists those with none after them.
SELECT item.id AS item_id, item_text.name, collection.year, collection.identification_state, item.added_at,
  collection.released_on, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.kind = 'series' AND item.removed_at IS NULL
  AND (collection.released_on, collection.id) < (CAST(sqlc.arg(before_released_on) AS INTEGER), CAST(sqlc.arg(before_id) AS INTEGER))
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY collection.released_on DESC, collection.id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesByReleasedAscending :many
-- Series oldest first by their first air date, by collection_series_released.
SELECT item.id AS item_id, item_text.name, collection.year, collection.identification_state, item.added_at,
  collection.released_on, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.kind = 'series' AND item.removed_at IS NULL
  AND (collection.released_on, collection.id) > (CAST(sqlc.arg(after_released_on) AS INTEGER), CAST(sqlc.arg(after_id) AS INTEGER))
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY collection.released_on, collection.id
LIMIT sqlc.arg(max_rows);

-- name: ListSeriesUndated :many
-- The series with no first air date, last in a sort by it either way (D366 section 3), by id.
SELECT item.id AS item_id, item_text.name, collection.year, collection.identification_state, item.added_at,
  collection.released_on, EXISTS (SELECT 1 FROM guessed_match WHERE guessed_match.item_id = item.id) AS is_guessed
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE collection.kind = 'series' AND collection.released_on IS NULL AND item.removed_at IS NULL
  AND collection.id > sqlc.arg(after_id)
  AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY collection.id
LIMIT sqlc.arg(max_rows);

-- name: ListOtherByNameDescending :many
-- Other videos Z to A, as ListOtherByName reads them A to Z.
SELECT sort_key.item_id, sort_key.collation_key, item_text.name, title.identification_state,
  item.added_at, other_video.recorded_at, other_video.recorded_from, other_video.file_name
FROM sort_key
JOIN item ON item.id = sort_key.item_id
JOIN title ON title.id = sort_key.item_id
LEFT JOIN item_text ON item_text.item_id = sort_key.item_id AND item_text.is_original = 1
LEFT JOIN other_video ON other_video.title_id = sort_key.item_id
WHERE sort_key.locale = sqlc.arg(locale) AND sort_key.listing = 'other' AND item.removed_at IS NULL
  AND (sort_key.collation_key < sqlc.arg(after_key) OR (sort_key.collation_key = sqlc.arg(after_key) AND sort_key.item_id < sqlc.arg(after_id)))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY sort_key.collation_key DESC, sort_key.item_id DESC
LIMIT sqlc.arg(max_rows);

-- name: ListOtherByAddedAscending :many
-- Other videos oldest added first, as ListOtherByAdded reads them newest first, by title_other_found.
SELECT item.id AS item_id, item_text.name, title.identification_state, item.added_at,
  title.found_at, title.found_file_modified_at, other_video.recorded_at, other_video.recorded_from,
  other_video.file_name
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
LEFT JOIN other_video ON other_video.title_id = title.id
WHERE title.kind = 'other' AND item.removed_at IS NULL
  AND (title.found_at, title.found_file_modified_at, title.id)
    > (sqlc.arg(after_found_at), sqlc.arg(after_modified_at), sqlc.arg(after_id))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY title.found_at, title.found_file_modified_at, title.id
LIMIT sqlc.arg(max_rows);

-- name: ListOtherByRecordedAscending :many
-- Other videos oldest recorded first, as ListOtherByRecorded reads them newest first, by
-- other_video_recorded from its foot.
SELECT item.id AS item_id, item_text.name, title.identification_state, item.added_at,
  other_video.recorded_at, other_video.recorded_from, other_video.file_name
FROM other_video
CROSS JOIN title ON title.id = other_video.title_id
CROSS JOIN item ON item.id = other_video.title_id
LEFT JOIN item_text ON item_text.item_id = other_video.title_id AND item_text.is_original = 1
WHERE item.removed_at IS NULL
  AND (other_video.recorded_at, other_video.title_id) > (sqlc.arg(after_recorded_at), sqlc.arg(after_id))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY other_video.recorded_at, other_video.title_id
LIMIT sqlc.arg(max_rows);

-- name: ListOtherByFileNameDescending :many
-- Other videos by their files' names Z to A, as ListOtherByFileName reads them A to Z.
SELECT item.id AS item_id, other_video.file_name_key, item_text.name, title.identification_state,
  item.added_at, other_video.recorded_at, other_video.recorded_from, other_video.file_name
FROM other_video
CROSS JOIN title ON title.id = other_video.title_id
CROSS JOIN item ON item.id = other_video.title_id
LEFT JOIN item_text ON item_text.item_id = other_video.title_id AND item_text.is_original = 1
WHERE item.removed_at IS NULL
  AND (other_video.file_name_key, other_video.title_id)
    < (CAST(sqlc.arg(after_key) AS BLOB), CAST(sqlc.arg(after_id) AS INTEGER))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  )
  AND (CAST(sqlc.narg(item_ids_json) AS TEXT) IS NULL
    OR item.id IN (SELECT value FROM json_each(CAST(sqlc.narg(item_ids_json) AS TEXT))))
ORDER BY other_video.file_name_key DESC, other_video.title_id DESC
LIMIT sqlc.arg(max_rows);
