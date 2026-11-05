-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Genres, people and credits (step 0067; D186): what the identification pass writes of each film's
-- and series' source, and what browse reads of them. A person's name is their item's text, so the
-- full-text indexes find it as they find a title's (D48); search.go holds those statements. What a
-- caller reaches is the libraries listing a credited title or series, as browse.sql says it: a
-- live file at a place a live library of theirs covers, every library when is_every_library is 1
-- (D243 section 4, D363 section 6).

-- name: ListLibraryCredits :many
-- The credits of the library's live items, each with its person, for the pass's snapshot.
SELECT credit.item_id, credit.job, credit.ordinal, credit.character, credit.person_id, person.tmdb_id,
  person.picture_ref, item_text.name
FROM item
CROSS JOIN credit ON credit.item_id = item.id
CROSS JOIN person ON person.id = credit.person_id
CROSS JOIN item_text ON item_text.item_id = person.id AND item_text.is_original = 1
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL
ORDER BY credit.item_id, credit.job, credit.ordinal;

-- name: ListItemCredits :many
-- One item's credits, each with its person, in billing order within each job.
SELECT credit.item_id, credit.job, credit.ordinal, credit.character, credit.person_id, person.tmdb_id,
  person.picture_ref, item_text.name
FROM credit
CROSS JOIN person ON person.id = credit.person_id
CROSS JOIN item_text ON item_text.item_id = person.id AND item_text.is_original = 1
WHERE credit.item_id = sqlc.arg(item_id)
ORDER BY credit.job, credit.ordinal;

-- name: ListLibraryItemGenres :many
-- The genres of the library's live items, for the pass's snapshot.
SELECT item_genre.item_id, item_genre.ordinal, genre.id AS genre_id, genre.name
FROM item
CROSS JOIN item_genre ON item_genre.item_id = item.id
CROSS JOIN genre ON genre.id = item_genre.genre_id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL
ORDER BY item_genre.item_id, item_genre.ordinal;

-- name: ListItemGenres :many
-- One item's genres, in its source's order.
SELECT item_genre.item_id, item_genre.ordinal, genre.id AS genre_id, genre.name
FROM item_genre
CROSS JOIN genre ON genre.id = item_genre.genre_id
WHERE item_genre.item_id = sqlc.arg(item_id)
ORDER BY item_genre.ordinal;

-- name: DeleteItemCredits :exec
-- Credits are derived (class C): an item's whose source changed are written again whole.
DELETE FROM credit WHERE item_id = sqlc.arg(item_id);

-- name: InsertCredit :exec
INSERT INTO credit (item_id, job, ordinal, person_id, character)
VALUES (sqlc.arg(item_id), sqlc.arg(job), sqlc.arg(ordinal), sqlc.arg(person_id), sqlc.arg(character));

-- name: DeleteItemGenres :exec
-- The genres an item is in are derived (class C): an item's whose source changed are written again
-- whole.
DELETE FROM item_genre WHERE item_id = sqlc.arg(item_id);

-- name: InsertItemGenre :exec
INSERT INTO item_genre (item_id, genre_id, ordinal)
VALUES (sqlc.arg(item_id), sqlc.arg(genre_id), sqlc.arg(ordinal));

-- name: InsertGenreIfAbsent :exec
-- A genre is one by its folded name, spelled as its first source spelled it.
INSERT INTO genre (name, name_key) VALUES (sqlc.arg(name), sqlc.arg(name_key))
ON CONFLICT (name_key) DO NOTHING;

-- name: GetGenreIDByKey :one
SELECT id FROM genre WHERE name_key = sqlc.arg(name_key);

-- name: GetPersonByTMDBID :one
SELECT person.id, person.name_key, person.picture_ref FROM person WHERE person.tmdb_id = sqlc.arg(tmdb_id);

-- name: GetPersonIDByNameKey :one
-- The person an NFO names with no id: the first by that folded name, from any source.
SELECT person.id FROM person WHERE person.name_key = sqlc.arg(name_key) ORDER BY person.id LIMIT 1;

-- name: InsertPerson :exec
INSERT INTO person (id, kind, tmdb_id, name_key, picture_ref)
VALUES (sqlc.arg(id), 'person', sqlc.arg(tmdb_id), sqlc.arg(name_key), sqlc.arg(picture_ref));

-- name: UpdatePerson :exec
-- A person their source now names or pictures otherwise; unchanged, nothing is written.
UPDATE person SET name_key = sqlc.arg(name_key), picture_ref = sqlc.arg(picture_ref)
WHERE id = sqlc.arg(id) AND (name_key IS NOT sqlc.arg(name_key) OR picture_ref IS NOT sqlc.arg(picture_ref));

-- name: GetPersonReached :one
-- A person with a credit on a film or series the caller reaches, with their name and picture.
SELECT person.id, item_text.name, person.picture_ref
FROM person
CROSS JOIN item_text ON item_text.item_id = person.id AND item_text.is_original = 1
WHERE person.id = sqlc.arg(person_id) AND EXISTS (
  SELECT 1 FROM credit
  CROSS JOIN item ON item.id = credit.item_id
  LEFT JOIN work ON work.id = credit.item_id
  LEFT JOIN sequence ON sequence.id = credit.item_id
  WHERE credit.person_id = person.id AND item.removed_at IS NULL AND (
    (work.kind IN ('movie', 'other') AND EXISTS (
      SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
      WHERE version.work_id = work.id AND version.removed_at IS NULL
        AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
          OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))))
    OR (sequence.kind = 'series' AND EXISTS (
      SELECT 1 FROM sequence_member
      CROSS JOIN version ON version.work_id = sequence_member.member_work
      CROSS JOIN version_library ON version_library.version_id = version.id
      WHERE sequence_member.sequence_id = sequence.id AND version.removed_at IS NULL
        AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
          OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))))));

-- name: ListPersonCreditsOn :many
-- A person's credits on the items a list of them shows (GET /items?person=, D425 section 7): the
-- list read what the caller reaches, so this reads only the credits of the items it names.
SELECT credit.item_id, credit.job, credit.character
FROM credit
WHERE credit.person_id = sqlc.arg(person_id)
  AND credit.item_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_ids_json) AS TEXT)))
ORDER BY credit.item_id, credit.job, credit.ordinal;

-- name: ListGenresWithCounts :many
-- Each genre with how many films, series or other videos of a kind the caller reaches are in it;
-- an empty kind counts every kind. A genre none of them is in is left out.
SELECT genre.id, genre.name, count(*) AS work_count
FROM genre
CROSS JOIN item_genre ON item_genre.genre_id = genre.id
CROSS JOIN item ON item.id = item_genre.item_id
LEFT JOIN work ON work.id = item_genre.item_id
LEFT JOIN sequence ON sequence.id = item_genre.item_id
WHERE item.removed_at IS NULL
  AND (CAST(sqlc.arg(kind) AS TEXT) = '' OR work.kind = CAST(sqlc.arg(kind) AS TEXT)
    OR (CAST(sqlc.arg(kind) AS TEXT) = 'series' AND sequence.kind = 'series'))
  AND (
  (work.kind IN ('movie', 'other') AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = work.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))))
  OR (sequence.kind = 'series' AND EXISTS (
    SELECT 1 FROM sequence_member
    CROSS JOIN version ON version.work_id = sequence_member.member_work
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE sequence_member.sequence_id = sequence.id AND version.removed_at IS NULL
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT)))))))
GROUP BY genre.id
ORDER BY genre.name, genre.id;

-- name: ListFilteredItemIDs :many
-- The live films, other videos and series a list's filters keep: in a genre, crediting a person,
-- of a decade (its first year), and not watched by a person. A filter that is NULL, or
-- is_unwatched 0, keeps every one. A series is unwatched while one of its episodes with a live
-- version is. The list's own query applies the caller's reach (browse.sql).
SELECT item.id
FROM item
LEFT JOIN work ON work.id = item.id
LEFT JOIN sequence ON sequence.id = item.id
WHERE item.removed_at IS NULL AND (work.kind IN ('movie', 'other') OR sequence.kind = 'series')
  AND (CAST(sqlc.narg(genre_id) AS INTEGER) IS NULL OR EXISTS (
    SELECT 1 FROM item_genre
    WHERE item_genre.item_id = item.id AND item_genre.genre_id = CAST(sqlc.narg(genre_id) AS INTEGER)))
  AND (CAST(sqlc.narg(person_id) AS INTEGER) IS NULL OR EXISTS (
    SELECT 1 FROM credit
    WHERE credit.item_id = item.id AND credit.person_id = CAST(sqlc.narg(person_id) AS INTEGER)))
  AND (CAST(sqlc.narg(decade) AS INTEGER) IS NULL
    OR coalesce(item.year, item.year) BETWEEN CAST(sqlc.narg(decade) AS INTEGER)
      AND CAST(sqlc.narg(decade) AS INTEGER) + 9)
  AND (CAST(sqlc.arg(is_unwatched) AS INTEGER) = 0
    OR (work.id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM work_watched
      WHERE work_watched.user_id = sqlc.arg(user_id) AND work_watched.work_id = work.id
        AND work_watched.watched_at IS NOT NULL))
    OR (sequence.id IS NOT NULL AND EXISTS (
      SELECT 1 FROM sequence_member
      CROSS JOIN item AS episode ON episode.id = sequence_member.member_work
      WHERE sequence_member.sequence_id = sequence.id AND episode.removed_at IS NULL
        AND EXISTS (SELECT 1 FROM version WHERE version.work_id = episode.id AND version.removed_at IS NULL)
        AND NOT EXISTS (
          SELECT 1 FROM work_watched
          WHERE work_watched.user_id = sqlc.arg(user_id) AND work_watched.work_id = episode.id
            AND work_watched.watched_at IS NOT NULL))))
ORDER BY item.id;
