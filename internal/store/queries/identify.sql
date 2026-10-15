-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The identification pass (identification.md): it reads stored observations and writes titles.

-- name: ListLibraryRoots :many
SELECT node_id, root_path FROM library_root WHERE library_id = ? AND retired_at IS NULL ORDER BY root_path;

-- name: ListLiveLibraryIDs :many
-- Every live library, which the identification sweep passes over (D231 section 3).
SELECT id FROM library WHERE removed_at IS NULL ORDER BY id;

-- name: GetLibraryForIdentify :one
SELECT id, kind, metadata_source FROM library WHERE id = ? AND removed_at IS NULL;

-- name: ListIdentifyFiles :many
SELECT file.id, file.node_id, file.path, file.reported_path_raw, file.filing, file.filing_reason, file.sidecar_kind,
  file.payload_hash, file.version_id, file.attached_to_file, CAST(coalesce(file_probe.duration_ms, 0) AS INTEGER) AS duration_ms,
  file.identified_from_path, file.size_bytes, file.modified_at
FROM file
LEFT JOIN file_probe ON file_probe.file_id = file.id
WHERE file.library_id = ? AND file.removed_at IS NULL
ORDER BY file.path;

-- name: ListLibraryTitles :many
SELECT title.id, title.kind, title.year, title.identification_state, item.library_id, item_text.name
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE item.library_id = ? AND item.removed_at IS NULL
ORDER BY title.id;

-- name: ListTitlesByProviderID :many
-- The titles a provider's id names: the live ones, and those of a deleted library, tombstoned
-- with no merged_into, which a film that reappears brings back (D208 section 2, D231 section 7).
SELECT title.id, item.library_id, title.identification_state, title.kind, item.removed_at
FROM item_provider_id
JOIN title ON title.id = item_provider_id.item_id
JOIN item ON item.id = title.id
WHERE item_provider_id.provider = ? AND item_provider_id.provider_id = ?
  AND (item.removed_at IS NULL OR item.merged_into IS NULL)
ORDER BY item.removed_at IS NOT NULL, title.id;

-- name: CountLiveFilesOfTitle :one
-- A title's live files: live files of its live versions, as ListTitleFiles and
-- ListLiveFilesOfTitle list them.
SELECT count(*)
FROM file
JOIN version ON version.id = file.version_id
WHERE version.title_id = ? AND version.removed_at IS NULL AND file.removed_at IS NULL;

-- name: ListSeriesByProviderID :many
-- The series a provider's id names: the live ones, and those of a deleted library, tombstoned
-- with no merged_into, which a series whose episodes reappear brings back (D208 section 2,
-- D231 section 7).
SELECT collection.id, item.library_id, item.removed_at
FROM item_provider_id
JOIN collection ON collection.id = item_provider_id.item_id AND collection.kind = 'series'
JOIN item ON item.id = collection.id
WHERE item_provider_id.provider = ? AND item_provider_id.provider_id = ?
  AND (item.removed_at IS NULL OR item.merged_into IS NULL)
ORDER BY item.removed_at IS NOT NULL, collection.id;

-- name: CountLiveFilesOfSeries :one
-- A series' live files: live files of the live versions of its episodes.
SELECT count(*)
FROM file
JOIN version ON version.id = file.version_id
JOIN collection_member ON collection_member.member_title = version.title_id
WHERE collection_member.collection_id = ? AND version.removed_at IS NULL AND file.removed_at IS NULL;

-- name: MoveEpisodeToSeries :exec
-- An episode joins another series at the same place: its membership, and its place in the
-- aired order, which moves to the other series' own.
UPDATE collection_member SET collection_id = sqlc.arg(to_series)
WHERE collection_id = sqlc.arg(from_series) AND member_title = sqlc.arg(title_id);

-- name: MovePositionToOrdering :exec
UPDATE position SET ordering_id = sqlc.arg(to_ordering)
WHERE ordering_id = sqlc.arg(from_ordering) AND title_id = sqlc.arg(title_id);

-- name: GetVersionTitle :one
SELECT version.title_id, title.identification_state
FROM version
JOIN title ON title.id = version.title_id
WHERE version.id = ?;

-- name: InsertItem :one
INSERT INTO item (kind, library_id, added_at) VALUES (sqlc.arg(kind), sqlc.arg(library_id), sqlc.arg(added_at))
RETURNING id;

-- name: MoveItemToLibrary :exec
-- A title follows its files to another library, a deleted library's brought back with them.
UPDATE item SET library_id = sqlc.arg(library_id), removed_at = NULL WHERE id = sqlc.arg(id);

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
  search_alternates = excluded.search_alternates
-- A text already as derived is left alone (D303, DL9): nothing is written, and no index rewritten.
WHERE item_text.is_original IS NOT 1 OR item_text.name IS NOT excluded.name
  OR item_text.overview IS NOT excluded.overview OR item_text.tagline IS NOT excluded.tagline
  OR item_text.search_alternates IS NOT excluded.search_alternates;

-- name: UpsertSortKey :exec
-- An item's sort key in one locale, made whenever its name is written (D47), and left alone when
-- it is already what the name derives (D303, DL9). sort_key is a cache (class C).
INSERT INTO sort_key (item_id, locale, listing, collation_key)
VALUES (sqlc.arg(item_id), sqlc.arg(locale), sqlc.arg(listing), sqlc.arg(collation_key))
ON CONFLICT (item_id, locale) DO UPDATE SET listing = excluded.listing, collation_key = excluded.collation_key
WHERE sort_key.listing IS NOT excluded.listing OR sort_key.collation_key IS NOT excluded.collation_key;

-- name: InsertItemProviderID :exec
INSERT INTO item_provider_id (item_id, provider, provider_id)
VALUES (sqlc.arg(item_id), sqlc.arg(provider), sqlc.arg(provider_id))
ON CONFLICT DO NOTHING;

-- name: InsertVersion :one
INSERT INTO version (title_id, created_at) VALUES (sqlc.arg(title_id), sqlc.arg(created_at))
RETURNING id;

-- name: MoveVersionToTitle :exec
UPDATE version SET title_id = sqlc.arg(title_id) WHERE id = sqlc.arg(id);

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
-- A series' episodes in its aired order: the live ones, and those hidden with a deleted library,
-- which come back with their files (D231 section 7).
SELECT position.title_id, position.group_number, position.number_in_group, item.removed_at
FROM position
JOIN ordering ON ordering.id = position.ordering_id
JOIN item ON item.id = position.title_id
WHERE ordering.collection_id = ? AND ordering.name = 'aired'
  AND (item.removed_at IS NULL OR item.merged_into IS NULL)
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
WHERE item_id = sqlc.arg(item_id) AND is_original = 1 AND search_alternates IS NOT sqlc.arg(search_alternates);

-- name: DeleteSortKeysOfOtherLocales :exec
-- Keys for a locale no longer in use (sort_key is a cache, class C).
DELETE FROM sort_key
WHERE locale != sqlc.arg(locale);
