-- SPDX-License-Identifier: GPL-3.0-or-later
-- The identification pass (identification.md): it reads stored observations and writes titles.

-- name: ListLibraryRoots :many
SELECT node_id, root_path FROM library_root WHERE library_id = ? ORDER BY root_path;

-- name: GetLibraryForIdentify :one
SELECT id, kind, metadata_source FROM library WHERE id = ? AND removed_at IS NULL;

-- name: ListIdentifyFiles :many
SELECT id, node_id, path, reported_path_bytes, filing, filing_reason, sidecar_kind, payload_hash, version_id, attached_to_file, probe_json,
  identified_from_path, size_bytes, modified_at
FROM file
WHERE library_id = ? AND removed_at IS NULL
ORDER BY path;

-- name: ListLibraryTitles :many
SELECT title.id, title.kind, title.year, title.identification_state, item.library_id, item_text.name
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE item.library_id = ? AND item.removed_at IS NULL
ORDER BY title.id;

-- name: ListTitlesByProviderID :many
SELECT title.id, item.library_id, title.identification_state, title.kind
FROM item_provider_id
JOIN title ON title.id = item_provider_id.item_id
JOIN item ON item.id = title.id
WHERE item_provider_id.provider = ? AND item_provider_id.provider_id = ? AND item.removed_at IS NULL
ORDER BY title.id;

-- name: CountLiveFilesOfTitle :one
-- A title's live files: live files of its live versions, as ListTitleFiles and
-- ListLiveFilesOfTitle list them.
SELECT count(*)
FROM file
JOIN version ON version.id = file.version_id
WHERE version.title_id = ? AND version.removed_at IS NULL AND file.removed_at IS NULL;

-- name: GetVersionTitle :one
SELECT version.title_id, title.identification_state
FROM version
JOIN title ON title.id = version.title_id
WHERE version.id = ?;

-- name: InsertItem :one
INSERT INTO item (kind, library_id, added_at) VALUES (sqlc.arg(kind), sqlc.arg(library_id), sqlc.arg(added_at))
RETURNING id;

-- name: MoveItemToLibrary :exec
UPDATE item SET library_id = sqlc.arg(library_id) WHERE id = sqlc.arg(id);

-- name: InsertTitle :exec
INSERT INTO title (id, kind, year, runtime_ms, followed_source, followed_source_id, identification_state)
VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(year), sqlc.arg(runtime_ms), sqlc.arg(followed_source), sqlc.arg(followed_source_id),
  sqlc.arg(identification_state));

-- name: UpdateTitleResolved :exec
-- A confirmed match is a person's: the pass never lowers it (D172).
UPDATE title SET
  year = sqlc.arg(year), runtime_ms = sqlc.arg(runtime_ms), followed_source = sqlc.arg(followed_source),
  followed_source_id = sqlc.arg(followed_source_id),
  identification_state = CASE WHEN identification_state = 'confirmed' THEN identification_state ELSE sqlc.arg(identification_state) END
WHERE id = sqlc.arg(id);

-- name: UpsertOriginalText :exec
-- search_alternates is the name as the one normaliser folds it (D48), which both search indexes take.
INSERT INTO item_text (item_id, language, is_original, name, overview, tagline, search_alternates)
VALUES (sqlc.arg(item_id), sqlc.arg(language), 1, sqlc.arg(name), sqlc.arg(overview), sqlc.arg(tagline),
  sqlc.arg(search_alternates))
ON CONFLICT (item_id, language) DO UPDATE SET
  is_original = 1, name = excluded.name, overview = excluded.overview, tagline = excluded.tagline,
  search_alternates = excluded.search_alternates;

-- name: DeleteItemSortKeys :exec
-- An item's sort keys in one locale, replaced whenever its name is written (D47). sort_key is
-- a cache (class C).
DELETE FROM sort_key
WHERE item_id = sqlc.arg(item_id) AND locale = sqlc.arg(locale);

-- name: InsertSortKey :exec
INSERT INTO sort_key (item_id, locale, listing, collation_key)
VALUES (sqlc.arg(item_id), sqlc.arg(locale), sqlc.arg(listing), sqlc.arg(collation_key));

-- name: InsertItemProviderID :exec
INSERT INTO item_provider_id (item_id, provider, provider_id)
VALUES (sqlc.arg(item_id), sqlc.arg(provider), sqlc.arg(provider_id))
ON CONFLICT DO NOTHING;

-- name: InsertVersion :one
INSERT INTO version (title_id, created_at) VALUES (sqlc.arg(title_id), sqlc.arg(created_at))
RETURNING id;

-- name: MoveVersionToTitle :exec
UPDATE version SET title_id = sqlc.arg(title_id) WHERE id = sqlc.arg(id);

-- name: SetFileVersionAndIdentifiedPath :exec
UPDATE file SET version_id = sqlc.arg(version_id), identified_from_path = path WHERE id = sqlc.arg(id);

-- name: SetFileFiling :exec
UPDATE file SET filing = sqlc.arg(filing), filing_reason = sqlc.arg(filing_reason) WHERE id = sqlc.arg(id);

-- name: ListOverrides :many
SELECT field, override_value FROM field_override WHERE item_id = ? AND language = '' ORDER BY field;

-- name: InsertOverrideIfAbsent :exec
-- A locked NFO field is an override (D182); one a person already set here stays.
INSERT INTO field_override (item_id, field, language, override_value, origin, set_at)
VALUES (sqlc.arg(item_id), sqlc.arg(field), '', sqlc.arg(override_value), sqlc.arg(origin), sqlc.arg(set_at))
ON CONFLICT (item_id, field, language) DO NOTHING;

-- name: ListLibrarySeries :many
SELECT collection.id, item_text.name
FROM collection
JOIN item ON item.id = collection.id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
WHERE item.library_id = ? AND item.removed_at IS NULL AND collection.kind = 'series'
ORDER BY collection.id;

-- name: InsertCollection :exec
INSERT INTO collection (id, kind, membership, created_by)
VALUES (sqlc.arg(id), sqlc.arg(kind), 'explicit', sqlc.arg(created_by));

-- name: UpsertCollectionMember :exec
INSERT INTO collection_member (collection_id, member_title, position)
VALUES (sqlc.arg(collection_id), sqlc.arg(member_title), sqlc.arg(position))
ON CONFLICT (collection_id, member_title) DO UPDATE SET position = excluded.position;

-- name: GetOrdering :one
SELECT id FROM ordering WHERE collection_id = ? AND name = ?;

-- name: InsertOrdering :one
INSERT INTO ordering (collection_id, name, provider) VALUES (sqlc.arg(collection_id), sqlc.arg(name), NULL)
RETURNING id;

-- name: UpsertPosition :exec
INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order)
VALUES (sqlc.arg(ordering_id), sqlc.arg(title_id), sqlc.arg(group_number), sqlc.arg(number_in_group), sqlc.arg(sort_order))
ON CONFLICT (ordering_id, title_id) DO UPDATE SET group_number = excluded.group_number, number_in_group = excluded.number_in_group,
  sort_order = excluded.sort_order;

-- name: ListSeriesEpisodePlaces :many
SELECT position.title_id, position.group_number, position.number_in_group
FROM position
JOIN ordering ON ordering.id = position.ordering_id
WHERE ordering.collection_id = ? AND ordering.name = 'aired'
ORDER BY position.sort_order;

-- name: UpsertScannerExtraRelation :exec
-- The scanner may assert an extra of a Title; a relation a person asserted stays theirs (D162).
INSERT INTO extra_relation (owner_title, extra_title, extra_type, asserted_by)
VALUES (sqlc.arg(owner_title), sqlc.arg(extra_title), sqlc.arg(extra_type), 'scanner')
ON CONFLICT (owner_title, extra_title) DO UPDATE SET extra_type = excluded.extra_type
WHERE extra_relation.asserted_by = 'scanner';

-- name: GetTitleIdentification :one
SELECT identification_state FROM title WHERE id = ?;

-- name: ConfirmTitleMatch :exec
-- A person confirmed the match: permanent, and never moved by a later pass (D172).
UPDATE title SET identification_state = 'confirmed' WHERE id = ?;

-- name: ListFileTitles :many
SELECT file.id, file.path, file.filing, file.filing_reason, title.id AS title_id, title.kind, title.identification_state,
  title.year, title.followed_source, item_text.name
FROM file
JOIN version ON version.id = file.version_id
JOIN title ON title.id = version.title_id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE file.library_id = ? AND file.removed_at IS NULL
ORDER BY file.path;

-- name: CountItemsMissingSortKey :one
-- Live named items with no sort key in a locale: after the locale changed, or a pass that began
-- before it did (D47).
SELECT count(*)
FROM item_text
JOIN item ON item.id = item_text.item_id
WHERE item_text.is_original = 1 AND item.removed_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM sort_key WHERE sort_key.item_id = item_text.item_id AND sort_key.locale = sqlc.arg(locale)
  );

-- name: ListNamesWithSortAndSearchKeys :many
-- Every live named item: its name, its stored search fold, its kind as a title or a collection,
-- and its stored sort keys in a locale, one row per key and one with none when it has none. What
-- a refresh compares with what this build derives from the name (D47, D48).
SELECT item_text.item_id, item_text.name, item_text.search_alternates, title.kind AS title_kind,
  collection.kind AS collection_kind, sort_key.listing AS keyed_listing, sort_key.collation_key
FROM item_text
JOIN item ON item.id = item_text.item_id
LEFT JOIN title ON title.id = item_text.item_id
LEFT JOIN collection ON collection.id = item_text.item_id
LEFT JOIN sort_key ON sort_key.item_id = item_text.item_id AND sort_key.locale = sqlc.arg(locale)
WHERE item_text.is_original = 1 AND item.removed_at IS NULL
ORDER BY item_text.item_id;

-- name: SetSearchAlternates :exec
-- An item's name folded again, when the one normaliser now folds it otherwise (D48).
UPDATE item_text SET search_alternates = sqlc.arg(search_alternates)
WHERE item_id = sqlc.arg(item_id) AND is_original = 1;

-- name: DeleteSortKeysOfOtherLocales :exec
-- Keys for a locale no longer in use (sort_key is a cache, class C).
DELETE FROM sort_key
WHERE locale != sqlc.arg(locale);
