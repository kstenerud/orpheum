-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Matching a title to its provider (identification.md; D170, D172): the fetch_metadata job's reads and
-- writes in persistent.db. The kept search results are cache.db's.

-- name: GetTitleForMatch :one
SELECT title.id, title.kind, title.year, title.identification_state, title.followed_source, title.followed_source_id, item.library_id,
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
-- A body fetched is a body asked for: both times are the fetch's.
INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
VALUES (sqlc.arg(item_id), sqlc.arg(provider), sqlc.arg(part), sqlc.arg(fetched_at), sqlc.arg(fetched_at),
        sqlc.arg(body_gzip))
ON CONFLICT (item_id, provider, part) DO UPDATE
SET fetched_at = excluded.fetched_at, last_asked_at = excluded.last_asked_at, body_gzip = excluded.body_gzip
-- Only when it changes something (D303, DL9).
WHERE (provider_response.fetched_at, provider_response.last_asked_at, provider_response.body_gzip)
  IS NOT (excluded.fetched_at, excluded.last_asked_at, excluded.body_gzip);

-- name: DeleteSeasonListings :exec
-- A series' stored season listings, dropped as a person chooses its match, which may be another
-- series than the one they listed (D301 section 2): the next listings are the chosen one's.
DELETE FROM provider_response
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider) AND part LIKE 'season/%';

-- name: GetProviderResponse :one
-- An item's stored response: part '' is the item itself, 'season/N' a series' season listing.
SELECT body_gzip
FROM provider_response
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider) AND part = sqlc.arg(part);

-- name: SetTitleMatch :exec
-- The outcome of a match: the source and its id, or ambiguous, or 'none' when the provider has
-- nothing. A confirmed match is a person's and stays (D172).
UPDATE title SET followed_source = sqlc.arg(followed_source), followed_source_id = sqlc.arg(followed_source_id),
  identification_state = CASE WHEN identification_state = 'confirmed' THEN identification_state ELSE sqlc.arg(identification_state) END
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesOfTitle :many
-- A title's live files: live files of its live versions, as ListTitleFiles and
-- CountLiveFilesOfTitle count them; and how long each plays, 0 before it is probed, which a film's
-- candidates' runtimes are fitted against (D365 section 1).
SELECT file.id, file.path,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = file.id), 0) AS INTEGER)
    AS duration_ms
FROM file
JOIN version ON version.id = file.version_id
WHERE version.title_id = sqlc.arg(title_id) AND version.removed_at IS NULL AND file.removed_at IS NULL
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
-- Films with live files that no source has answered for yet, and that have no failure of their
-- own: owed their first match however often an earlier fetch was lost (D170, D231 section 3).
SELECT title.id
FROM title
JOIN item ON item.id = title.id
WHERE item.library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND title.kind = 'movie'
  AND title.followed_source IS NULL AND title.identification_state = 'unidentified'
  AND NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = title.id)
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY title.id;

-- name: ListTitlesByIdentification :many
-- The titles in one state of identification, for the chooser's list (cut-a section 5), with a
-- failure of their own to be fetched and a person's refresh waiting (D231 section 4). Only those
-- with a live file: a title whose files went elsewhere waits for no one (the owner's NAS,
-- 2026-10-13, where a series library's films became its episodes). An episode is its series'
-- (D209); an other video is never matched (D349). An unidentified one not searched yet waits for
-- no one, unless its fetch failed on its own, which waits for a person's look again, or a person
-- asked for it again, which the list says is coming. So the list is what CountWaitingForAPerson counts, and those asked for (the owner's
-- staging, 2026-10-21, where the count said 15 and the list was hundreds of episodes).
SELECT title.id, title.kind, title.year, title.identification_state, title.followed_source, title.followed_source_id, item.library_id,
  item_text.name, fetch_failure.failure AS fetch_failure, fetch_failure.failed_at AS fetch_failed_at,
  refresh_request.requested_at AS refresh_requested_at
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
LEFT JOIN fetch_failure ON fetch_failure.item_id = title.id
LEFT JOIN refresh_request ON refresh_request.item_id = title.id
WHERE title.identification_state = sqlc.arg(identification_state) AND item.removed_at IS NULL
  AND title.kind NOT IN ('episode', 'other')
  AND (title.identification_state != 'unidentified' OR title.followed_source = 'none'
    OR fetch_failure.item_id IS NOT NULL OR refresh_request.item_id IS NOT NULL)
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
    OR item.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
  AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY item_text.name, title.id;

-- name: DeleteItemProviderIDsAtProvider :exec
-- A person's choice replaces the ids an earlier match recorded at that provider.
DELETE FROM item_provider_id
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider);

-- name: UpsertOverride :exec
-- A person's value for a field: the lock, kept over every source (D183).
INSERT INTO field_override (item_id, field, language, override_value, origin, set_by_user_id, set_at)
VALUES (sqlc.arg(item_id), sqlc.arg(field), '', sqlc.arg(override_value), 'typed', sqlc.arg(set_by_user_id), sqlc.arg(set_at))
ON CONFLICT (item_id, field, language) DO UPDATE SET
  override_value = excluded.override_value, origin = excluded.origin, set_by_user_id = excluded.set_by_user_id, set_at = excluded.set_at
-- Only when it changes something (D303, DL9).
WHERE (field_override.override_value, field_override.origin, field_override.set_by_user_id,
  field_override.set_at)
  IS NOT (excluded.override_value, excluded.origin, excluded.set_by_user_id, excluded.set_at);

-- name: DeleteOverride :execrows
-- Removing an override is how a field returns to its source (D183). A person's correction is not
-- a tombstoned row (standards/sql.md section 6): its absence is the state.
DELETE FROM field_override
WHERE item_id = sqlc.arg(item_id) AND field = sqlc.arg(field) AND language = '';

-- name: ReturnAmbiguousTitleToItsFirstMatch :exec
-- A film the matcher left ambiguous that its kept results now settle, under rules that changed
-- since: owed its first match again, which the matcher takes (the owner's NAS, 2026-10-13). A
-- person's choice is never undone: only 'ambiguous' changes.
UPDATE title SET identification_state = 'unidentified'
WHERE id = sqlc.arg(id) AND identification_state = 'ambiguous';

-- name: CountWaitingForAPerson :one
-- The films and series whose match waits for a person, as "Needs a match" lists them: several
-- found, or none found after a search, or a fetch that failed on its own, with a live file; an
-- episode is its series' (D209). Each part repeats the index it is counted by, title_ambiguous or
-- title_waiting (D303, DL4), or the few fetch_failure rows.
SELECT CAST((
  SELECT count(*) FROM title CROSS JOIN item ON item.id = title.id
  WHERE title.identification_state = 'ambiguous' AND item.removed_at IS NULL AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL)
) + (
  SELECT count(*) FROM title CROSS JOIN item ON item.id = title.id
  WHERE title.identification_state = 'unidentified' AND title.followed_source = 'none' AND title.kind != 'episode'
    AND item.removed_at IS NULL AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL)
) + (
  SELECT count(*) FROM collection CROSS JOIN item ON item.id = collection.id
  WHERE collection.kind = 'series' AND item.removed_at IS NULL
    AND (collection.identification_state = 'ambiguous'
      OR (collection.identification_state = 'unidentified' AND collection.followed_source = 'none'))
    AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN file ON file.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND file.removed_at IS NULL)
) + (
  SELECT count(*) FROM fetch_failure CROSS JOIN title ON title.id = fetch_failure.item_id
  CROSS JOIN item ON item.id = title.id
  WHERE title.identification_state = 'unidentified' AND IFNULL(title.followed_source, '') != 'none'
    AND title.kind != 'episode' AND item.removed_at IS NULL AND EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL)
) + (
  SELECT count(*) FROM fetch_failure CROSS JOIN collection ON collection.id = fetch_failure.item_id
  CROSS JOIN item ON item.id = collection.id
  WHERE collection.kind = 'series' AND collection.identification_state = 'unidentified'
    AND IFNULL(collection.followed_source, '') != 'none' AND item.removed_at IS NULL AND EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN file ON file.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND file.removed_at IS NULL)
) AS INTEGER) AS waiting_count;

-- name: ClearOverridesSetByRemovedUser :exec
-- The overrides a removed person typed stay, for everyone; who typed them is forgotten (D305).
UPDATE field_override SET set_by_user_id = NULL WHERE set_by_user_id = sqlc.arg(user_id);

-- name: ForgetItemProviderIDsBut :exec
-- A rescan's answer replaces every id an earlier match recorded: only the ids the item's files and
-- NFOs give stay, as provider:id in the JSON array kept_json (D357 section 4).
DELETE FROM item_provider_id
WHERE item_id = sqlc.arg(item_id)
  AND (provider || ':' || provider_id) NOT IN (SELECT value FROM json_each(CAST(sqlc.arg(kept_json) AS TEXT)));

-- name: GetItemName :one
-- A film's or series' name as it is shown, what a rescan's log names it by (D357 section 6).
SELECT name FROM item_text WHERE item_id = sqlc.arg(item_id) AND is_original = 1;
