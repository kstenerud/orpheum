-- SPDX-License-Identifier: GPL-3.0-or-later
-- The identification pass (identification.md): it reads stored observations and writes titles.

-- name: ListLibraryRoots :many
SELECT node_id, path FROM library_root WHERE library_id = ? ORDER BY path;

-- name: GetLibraryForIdentify :one
SELECT id, kind, metadata_source FROM library WHERE id = ? AND removed_at IS NULL;

-- name: ListIdentifyFiles :many
SELECT id, node_id, path, filing, filing_reason, payload_hash, version_id, attached_to, probe, identified_path
FROM file
WHERE library_id = ? AND removed_at IS NULL
ORDER BY path;

-- name: ListLibraryTitles :many
SELECT title.id, title.kind, title.year, title.ident, item.library_id, item_text.name
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE item.library_id = ? AND item.removed_at IS NULL
ORDER BY title.id;

-- name: ListTitlesByProviderID :many
SELECT title.id, item.library_id, title.ident, title.kind
FROM item_provider_id
JOIN title ON title.id = item_provider_id.item_id
JOIN item ON item.id = title.id
WHERE item_provider_id.provider = ? AND item_provider_id.provider_id = ? AND item.removed_at IS NULL
ORDER BY title.id;

-- name: CountLiveFilesOfTitle :one
SELECT count(*)
FROM file
JOIN version ON version.id = file.version_id
WHERE version.title_id = ? AND file.removed_at IS NULL;

-- name: GetVersionTitle :one
SELECT version.title_id, title.ident
FROM version
JOIN title ON title.id = version.title_id
WHERE version.id = ?;

-- name: InsertItem :one
INSERT INTO item (kind, library_id, created_at) VALUES (sqlc.arg(kind), sqlc.arg(library_id), sqlc.arg(created_at))
RETURNING id;

-- name: MoveItemToLibrary :exec
UPDATE item SET library_id = sqlc.arg(library_id) WHERE id = sqlc.arg(id);

-- name: InsertTitle :exec
INSERT INTO title (id, kind, year, runtime_ms, source, source_id, ident)
VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(year), sqlc.arg(runtime_ms), sqlc.arg(source), sqlc.arg(source_id),
  sqlc.arg(ident));

-- name: UpdateTitleResolved :exec
-- A confirmed match is a person's: the pass never lowers it (D172).
UPDATE title SET
  year = sqlc.arg(year), runtime_ms = sqlc.arg(runtime_ms), source = sqlc.arg(source),
  source_id = sqlc.arg(source_id),
  ident = CASE WHEN ident = 'confirmed' THEN ident ELSE sqlc.arg(ident) END
WHERE id = sqlc.arg(id);

-- name: UpsertOriginalText :exec
-- search_alt is the name as the one normaliser folds it (D48), which both search indexes take.
INSERT INTO item_text (item_id, lang, is_original, name, overview, tagline, search_alt)
VALUES (sqlc.arg(item_id), sqlc.arg(lang), 1, sqlc.arg(name), sqlc.arg(overview), sqlc.arg(tagline),
  sqlc.arg(search_alt))
ON CONFLICT (item_id, lang) DO UPDATE SET
  is_original = 1, name = excluded.name, overview = excluded.overview, tagline = excluded.tagline,
  search_alt = excluded.search_alt;

-- name: DeleteItemSortKeys :exec
-- An item's sort keys in one locale, replaced whenever its name is written (D47). sort_key is
-- a cache (class C).
DELETE FROM sort_key
WHERE item_id = sqlc.arg(item_id) AND locale = sqlc.arg(locale);

-- name: InsertSortKey :exec
INSERT INTO sort_key (item_id, locale, listing, key)
VALUES (sqlc.arg(item_id), sqlc.arg(locale), sqlc.arg(listing), sqlc.arg(key));

-- name: InsertItemProviderID :exec
INSERT INTO item_provider_id (item_id, provider, provider_id)
VALUES (sqlc.arg(item_id), sqlc.arg(provider), sqlc.arg(provider_id))
ON CONFLICT DO NOTHING;

-- name: InsertVersion :one
INSERT INTO version (title_id, created_at) VALUES (sqlc.arg(title_id), sqlc.arg(created_at))
RETURNING id;

-- name: MoveVersionToTitle :exec
UPDATE version SET title_id = sqlc.arg(title_id) WHERE id = sqlc.arg(id);

-- name: SetFileVersion :exec
UPDATE file SET version_id = sqlc.arg(version_id), identified_path = path WHERE id = sqlc.arg(id);

-- name: SetFileFiling :exec
UPDATE file SET filing = sqlc.arg(filing), filing_reason = sqlc.arg(filing_reason) WHERE id = sqlc.arg(id);

-- name: ListOverrides :many
SELECT field, value FROM field_override WHERE item_id = ? AND lang = '' ORDER BY field;

-- name: InsertOverrideIfAbsent :exec
-- A locked NFO field is an override (D182); one a person already set here stays.
INSERT INTO field_override (item_id, field, lang, value, origin, set_at)
VALUES (sqlc.arg(item_id), sqlc.arg(field), '', sqlc.arg(value), sqlc.arg(origin), sqlc.arg(set_at))
ON CONFLICT (item_id, field, lang) DO NOTHING;

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
INSERT INTO position (ordering_id, title_id, grp, number, sort)
VALUES (sqlc.arg(ordering_id), sqlc.arg(title_id), sqlc.arg(grp), sqlc.arg(number), sqlc.arg(sort))
ON CONFLICT (ordering_id, title_id) DO UPDATE SET grp = excluded.grp, number = excluded.number,
  sort = excluded.sort;

-- name: ListEpisodesOfSeries :many
SELECT position.title_id, position.grp, position.number
FROM position
JOIN ordering ON ordering.id = position.ordering_id
WHERE ordering.collection_id = ? AND ordering.name = 'aired'
ORDER BY position.sort;

-- name: UpsertScannerExtraRelation :exec
-- The scanner may assert an extra of a Title; a relation a person asserted stays theirs (D162).
INSERT INTO extra_relation (owner_title, extra_title, type, asserted_by)
VALUES (sqlc.arg(owner_title), sqlc.arg(extra_title), sqlc.arg(type), 'scanner')
ON CONFLICT (owner_title, extra_title) DO UPDATE SET type = excluded.type
WHERE extra_relation.asserted_by = 'scanner';

-- name: GetTitleIdent :one
SELECT ident FROM title WHERE id = ?;

-- name: ConfirmTitleMatch :exec
-- A person confirmed the match: permanent, and never moved by a later pass (D172).
UPDATE title SET ident = 'confirmed' WHERE id = ?;

-- name: ListFileTitles :many
SELECT file.id, file.path, file.filing, file.filing_reason, title.id AS title_id, title.kind, title.ident,
  title.year, title.source, item_text.name
FROM file
JOIN version ON version.id = file.version_id
JOIN title ON title.id = version.title_id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE file.library_id = ? AND file.removed_at IS NULL
ORDER BY file.path;
