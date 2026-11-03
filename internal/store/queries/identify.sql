-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The identification pass (identification.md): it reads stored observations and writes titles.

-- name: ListLibraryRoots :many
SELECT node_id, root_path FROM library_root WHERE library_id = sqlc.arg(library_id) AND retired_at IS NULL ORDER BY root_path;

-- name: ListLiveLibraryIDs :many
-- Every live library, which the identification sweep passes over (D231 section 3).
SELECT id FROM library WHERE removed_at IS NULL ORDER BY id;

-- name: GetLibraryForIdentify :one
SELECT id, kind, metadata_source FROM library WHERE id = ? AND removed_at IS NULL;

-- name: ListIdentifyFiles :many
-- What a library's pass files and reads (D363 sections 3, 5): each video it governs, at the first-found
-- of its places the library covers, as the library's kind filed it; and each sidecar and other file
-- at a place it covers, a sidecar of a copy attached to the copy's file, and whether it lies beside a
-- copy rather than the file's first place, whose NFO is read first (section 9). A video is governed by
-- the library of its kind with the outermost folder covering one of its places, the oldest of two.
SELECT library_place.file_id AS id, library_place.node_id, library_place.path, library_place.reported_path_raw,
  CAST(CASE WHEN file.filing = 'video' THEN coalesce(file_filing.filing, 'unidentified') ELSE file.filing END AS TEXT)
    AS filing,
  file_filing.filing_reason,
  file.sidecar_kind, file.payload_hash, file_filing.version_id,
  coalesce(owner.copy_of, file.attached_to_file) AS attached_to_file,
  CAST(owner.copy_of IS NOT NULL AS INTEGER) AS is_beside_a_copy,
  CAST(coalesce(file_probe.duration_ms, 0) AS INTEGER) AS duration_ms,
  file_filing.identified_from_path, file.size_bytes, file.modified_at
FROM library_place
CROSS JOIN file ON file.id = library_place.file_id
LEFT JOIN file_filing ON file_filing.file_id = file.id AND file_filing.library_kind = library_place.library_kind
LEFT JOIN file_probe ON file_probe.file_id = file.id
LEFT JOIN file AS owner ON owner.id = file.attached_to_file
WHERE library_place.library_id = sqlc.arg(library_id)
  AND (file.filing <> 'video' OR (
    (SELECT governing.library_id FROM place_library AS governing
     WHERE (governing.place_id = file.id OR governing.copy_of = file.id)
       AND governing.library_kind = library_place.library_kind
     ORDER BY length(governing.root_path), governing.library_id LIMIT 1) = library_place.library_id
    AND NOT EXISTS (
      SELECT 1 FROM place_library AS earlier
      WHERE (earlier.place_id = file.id OR earlier.copy_of = file.id) AND earlier.library_id = library_place.library_id
        AND (earlier.copy_of IS NOT NULL, earlier.first_seen_at, earlier.place_id)
          < (library_place.copy_of IS NOT NULL, library_place.first_seen_at, library_place.place_id))))
ORDER BY library_place.path;

-- name: ListTitlesToAdopt :many
-- The titles of the videos a library now governs, whose filing library governs none of their files
-- any more, deleted or out-covered (D363 sections 5, 10): its pass files them from now on, as it
-- found them, with their history.
SELECT DISTINCT version.title_id
FROM library_place
CROSS JOIN file_filing ON file_filing.file_id = library_place.file_id
  AND file_filing.library_kind = library_place.library_kind
CROSS JOIN version ON version.id = file_filing.version_id
CROSS JOIN item ON item.id = version.title_id
WHERE library_place.library_id = sqlc.arg(library_id)
  AND item.removed_at IS NULL AND item.filing_library_id IS NOT library_place.library_id
  AND (SELECT governing.library_id FROM place_library AS governing
       WHERE (governing.place_id = file_filing.file_id OR governing.copy_of = file_filing.file_id)
         AND governing.library_kind = file_filing.library_kind
       ORDER BY length(governing.root_path), governing.library_id LIMIT 1) = library_place.library_id
  AND NOT EXISTS (
    SELECT 1 FROM version AS held
    CROSS JOIN file_filing AS filed ON filed.version_id = held.id
    WHERE held.title_id = item.id
      AND (SELECT governing.library_id FROM place_library AS governing
           WHERE (governing.place_id = filed.file_id OR governing.copy_of = filed.file_id)
             AND governing.library_kind = filed.library_kind
           ORDER BY length(governing.root_path), governing.library_id LIMIT 1) = item.filing_library_id)
ORDER BY version.title_id;

-- name: ListSeriesToAdopt :many
-- The series some of whose episodes a library's pass files, whose filing library files none of them
-- any more: they follow their episodes (D363 sections 5, 10).
SELECT DISTINCT collection_member.collection_id
FROM item AS episode
CROSS JOIN collection_member ON collection_member.member_title = episode.id
CROSS JOIN item AS series ON series.id = collection_member.collection_id
WHERE episode.filing_library_id = sqlc.arg(library_id) AND episode.removed_at IS NULL AND episode.kind = 'title'
  AND series.removed_at IS NULL AND series.kind = 'collection' AND series.filing_library_id IS NOT sqlc.arg(library_id)
  AND NOT EXISTS (
    SELECT 1 FROM collection_member AS held
    CROSS JOIN item AS sibling ON sibling.id = held.member_title
    WHERE held.collection_id = series.id AND sibling.removed_at IS NULL
      AND sibling.filing_library_id = series.filing_library_id)
ORDER BY collection_member.collection_id;

-- name: SetItemsFilingLibrary :exec
-- Items a library's pass adopts (ListTitlesToAdopt, ListSeriesToAdopt): the ids are a JSON array.
UPDATE item SET filing_library_id = sqlc.arg(library_id)
WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_ids_json) AS TEXT)))
  AND filing_library_id IS NOT sqlc.arg(library_id);

-- name: ListLibraryTitles :many
SELECT title.id, title.kind, title.year, title.identification_state, item.filing_library_id, item_text.name
FROM title
JOIN item ON item.id = title.id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL
ORDER BY title.id;

-- name: ListTitlesByProviderID :many
-- The titles a provider's id names: the live ones, and those hidden with a deleted library before
-- D363, tombstoned with no merged_into, which a film that reappears brings back (D208 section 2, D231
-- section 7). Each says whether the library whose pass files it lists none of its files any more:
-- hidden, or its library deleted or no longer covering them, it follows its files where they are
-- found again (D363 section 10).
SELECT title.id, item.filing_library_id, title.identification_state, title.kind, item.removed_at,
  CAST(item.removed_at IS NOT NULL OR NOT EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL
      AND version_library.library_id = item.filing_library_id) AS INTEGER) AS is_filed_by_none
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
FROM version
CROSS JOIN file_filing ON file_filing.version_id = version.id
CROSS JOIN file ON file.id = file_filing.file_id
WHERE version.title_id = ? AND version.removed_at IS NULL AND file.removed_at IS NULL;

-- name: ListSeriesByProviderID :many
-- The series a provider's id names: the live ones, and those hidden with a deleted library before
-- D363, tombstoned with no merged_into, which a series whose episodes reappear brings back (D208
-- section 2, D231 section 7). Each says whether the library whose pass files it lists none of its
-- episodes' files any more (D363 section 10).
SELECT collection.id, item.filing_library_id, item.removed_at,
  CAST(item.removed_at IS NOT NULL OR NOT EXISTS (
    SELECT 1 FROM collection_member
    CROSS JOIN version ON version.title_id = collection_member.member_title
    CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE collection_member.collection_id = collection.id AND version.removed_at IS NULL
      AND version_library.library_id = item.filing_library_id) AS INTEGER) AS is_filed_by_none
FROM item_provider_id
JOIN collection ON collection.id = item_provider_id.item_id AND collection.kind = 'series'
JOIN item ON item.id = collection.id
WHERE item_provider_id.provider = ? AND item_provider_id.provider_id = ?
  AND (item.removed_at IS NULL OR item.merged_into IS NULL)
ORDER BY item.removed_at IS NOT NULL, collection.id;

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
-- An item, and the library whose pass files it (D363 section 5).
INSERT INTO item (kind, filing_library_id, added_at)
VALUES (sqlc.arg(kind), sqlc.arg(library_id), sqlc.arg(added_at))
RETURNING id;

-- name: MoveItemToLibrary :exec
-- A title follows its files to the library whose pass files them, one hidden before brought back with
-- them.
UPDATE item SET filing_library_id = sqlc.arg(library_id), removed_at = NULL WHERE id = sqlc.arg(id);

-- name: InsertTitle :exec
INSERT INTO title (id, kind, year, runtime_ms, followed_source, followed_source_id, identification_state)
VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(year), sqlc.arg(runtime_ms), sqlc.arg(followed_source), sqlc.arg(followed_source_id),
  sqlc.arg(identification_state));

-- name: UpdateTitleResolved :exec
-- A confirmed match is a person's: the pass never lowers it (D172). Its kind is how the pass files
-- it: an extra whose film is gone is filed as a film (D162), and its kind follows, which the sort
-- keys are listed by.
UPDATE title SET
  kind = sqlc.arg(kind),
  year = sqlc.arg(year), released_on = sqlc.arg(released_on), runtime_ms = sqlc.arg(runtime_ms),
  followed_source = sqlc.arg(followed_source), followed_source_id = sqlc.arg(followed_source_id),
  identification_state = CASE WHEN identification_state = 'confirmed' THEN identification_state ELSE sqlc.arg(identification_state) END
WHERE id = sqlc.arg(id)
  -- Only when it changes something (D303, DL9).
  AND (kind, year, released_on, runtime_ms, followed_source, followed_source_id, identification_state)
    IS NOT (sqlc.arg(kind), sqlc.arg(year), sqlc.arg(released_on), sqlc.arg(runtime_ms), sqlc.arg(followed_source),
      sqlc.arg(followed_source_id),
      CASE WHEN identification_state = 'confirmed' THEN identification_state ELSE sqlc.arg(identification_state) END);

-- name: UpsertOriginalText :exec
-- search_alternates is the name as the one normaliser folds it (D48), which both search indexes take.
-- sort_title is the NFO's own sort name, which wins over the name's key (D402).
INSERT INTO item_text (item_id, language, is_original, name, overview, tagline, search_alternates, sort_title)
VALUES (sqlc.arg(item_id), sqlc.arg(language), 1, sqlc.arg(name), sqlc.arg(overview), sqlc.arg(tagline),
  sqlc.arg(search_alternates), sqlc.arg(sort_title))
ON CONFLICT (item_id, language) DO UPDATE SET
  is_original = 1, name = excluded.name, overview = excluded.overview, tagline = excluded.tagline,
  search_alternates = excluded.search_alternates, sort_title = excluded.sort_title
-- A text already as derived is left alone (D303, DL9): nothing is written, and no index rewritten.
WHERE item_text.is_original IS NOT 1 OR item_text.name IS NOT excluded.name
  OR item_text.overview IS NOT excluded.overview OR item_text.tagline IS NOT excluded.tagline
  OR item_text.search_alternates IS NOT excluded.search_alternates OR item_text.sort_title IS NOT excluded.sort_title;

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

-- name: MergeTitleLeftWithoutVersions :execrows
-- A title a pass moved its last version out of, tombstoned pointing at the title the version went
-- to, as a match's join leaves one (D173): left live, it stayed in its series and its library with
-- no file, one more each time it happened. Merged, it is no deleted library's to bring back.
UPDATE item SET merged_into = sqlc.arg(merged_into), removed_at = sqlc.arg(removed_at)
WHERE item.id = sqlc.arg(title_id) AND item.removed_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM version WHERE version.title_id = sqlc.arg(title_id));

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
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND collection.kind = 'series'
ORDER BY collection.id;

-- name: InsertCollection :exec
INSERT INTO collection (id, kind, membership, created_by)
VALUES (sqlc.arg(id), sqlc.arg(kind), 'explicit', sqlc.arg(created_by));

-- name: UpsertCollectionMember :exec
INSERT INTO collection_member (collection_id, member_title, position)
VALUES (sqlc.arg(collection_id), sqlc.arg(member_title), sqlc.arg(position))
ON CONFLICT (collection_id, member_title) DO UPDATE SET position = excluded.position
-- Only when it changes something (D303, DL9).
WHERE (collection_member.position) IS NOT (excluded.position);

-- name: GetOrdering :one
SELECT id FROM ordering WHERE collection_id = ? AND name = ?;

-- name: InsertOrdering :one
-- A series' ordering its files' numbers place its episodes in, of its kind: aired.
INSERT INTO ordering (collection_id, name, provider, kind)
VALUES (sqlc.arg(collection_id), sqlc.arg(name), NULL, sqlc.arg(kind))
RETURNING id;

-- name: UpsertPosition :exec
INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order)
VALUES (sqlc.arg(ordering_id), sqlc.arg(title_id), sqlc.arg(group_number), sqlc.arg(number_in_group), sqlc.arg(sort_order))
ON CONFLICT (ordering_id, title_id) DO UPDATE SET group_number = excluded.group_number, number_in_group = excluded.number_in_group,
  sort_order = excluded.sort_order
-- Only when it changes something (D303, DL9).
WHERE (position.group_number, position.number_in_group, position.sort_order)
  IS NOT (excluded.group_number, excluded.number_in_group, excluded.sort_order);

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
WHERE extra_relation.asserted_by = 'scanner' AND extra_relation.extra_type IS NOT excluded.extra_type;

-- name: GetTitleIdentification :one
SELECT identification_state FROM title WHERE id = ?;

-- name: ConfirmTitleMatch :exec
-- A person confirmed the match: permanent, and never moved by a later pass (D172).
UPDATE title SET identification_state = 'confirmed' WHERE id = ?;

-- name: ListFileTitles :many
-- Each live file a library's folders cover, as the library's kind filed it, with its title: at each
-- place the library covers.
SELECT library_place.file_id AS id, library_place.path, file_filing.filing, file_filing.filing_reason,
  title.id AS title_id, title.kind, title.identification_state, title.year, title.followed_source, item_text.name
FROM library_place
CROSS JOIN file_filing ON file_filing.file_id = library_place.file_id
  AND file_filing.library_kind = library_place.library_kind
CROSS JOIN version ON version.id = file_filing.version_id
CROSS JOIN title ON title.id = version.title_id
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
WHERE library_place.library_id = sqlc.arg(library_id)
ORDER BY library_place.path;

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
SELECT item_text.item_id, item_text.name, item_text.search_alternates, item_text.sort_title, title.kind AS title_kind,
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

-- name: HasSortKeysOfOtherLocales :one
-- Whether any key is for a locale no longer in use: two short reads of sort_key_listing, so the
-- refresh deletes, and takes the writer, only when one is (D303, DL9).
SELECT EXISTS (
  SELECT 1 FROM sort_key WHERE sort_key.locale < sqlc.arg(locale) OR sort_key.locale > sqlc.arg(locale)
) AS has_other;

-- name: DeleteSortKeysOfOtherLocales :exec
-- Keys for a locale no longer in use (sort_key is a cache, class C).
DELETE FROM sort_key
WHERE locale != sqlc.arg(locale);

-- The rows a pass reads of its library at its start, a few statements whatever its size, which it
-- compares against before it writes, so that a pass over an unchanged library runs neither a
-- statement per file nor a write (D303, DL7, DL9). Each reads from the library's items outward:
-- CROSS JOIN keeps SQLite from scanning the other table instead, which it would when a library is
-- most of the database.

-- name: ListLibraryVersionTitles :many
-- The title each version of a live file at a place the library covers is in, as its kind filed it.
SELECT version.id, version.title_id, title.identification_state
FROM library_place
CROSS JOIN file_filing ON file_filing.file_id = library_place.file_id
  AND file_filing.library_kind = library_place.library_kind
CROSS JOIN version ON version.id = file_filing.version_id
CROSS JOIN title ON title.id = version.title_id
WHERE library_place.library_id = sqlc.arg(library_id);

-- name: ListLibraryNFOBodies :many
-- The bodies of the live NFO files at places the library covers.
SELECT file_nfo.file_id, file_nfo.body
FROM library_place
CROSS JOIN file_nfo ON file_nfo.file_id = library_place.place_id
WHERE library_place.library_id = sqlc.arg(library_id);

-- name: ListLibraryTitlesForPass :many
-- The library's live titles, with the values a pass resolves.
SELECT title.id, title.kind, title.year, title.released_on, title.runtime_ms, title.identification_state,
  title.followed_source, title.followed_source_id
FROM item
JOIN title ON title.id = item.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'title';

-- name: ListLibraryOverrides :many
-- The overrides of the library's live items.
SELECT field_override.item_id, field_override.field, field_override.override_value
FROM item
JOIN field_override ON field_override.item_id = item.id AND field_override.language = ''
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL
ORDER BY field_override.item_id, field_override.field;

-- name: ListLibraryProviderIDs :many
-- The provider ids of the library's live items.
SELECT item_provider_id.item_id, item_provider_id.provider, item_provider_id.provider_id
FROM item
CROSS JOIN item_provider_id ON item_provider_id.item_id = item.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL;

-- name: ListLibraryOriginalTexts :many
-- The text a pass writes of the library's live items, with each one's sort key in a locale.
SELECT item_text.item_id, item_text.language, item_text.name, item_text.overview, item_text.tagline,
  item_text.search_alternates, item_text.sort_title, sort_key.listing, sort_key.collation_key
FROM item
CROSS JOIN item_text ON item_text.item_id = item.id AND item_text.is_original = 1
LEFT JOIN sort_key ON sort_key.item_id = item.id AND sort_key.locale = sqlc.arg(locale)
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL;

-- name: ListLibraryAiredPositions :many
-- Each live title's place in a series' aired order, with the series' ordering.
SELECT position.title_id, position.ordering_id, ordering.collection_id, position.group_number,
  position.number_in_group, position.sort_order
FROM item
CROSS JOIN position ON position.title_id = item.id
CROSS JOIN ordering ON ordering.id = position.ordering_id AND ordering.name = 'aired'
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'title';

-- name: ListLibrarySeriesForPass :many
-- The library's live series, with their aired ordering when they have one and their match.
SELECT collection.id, collection.year, collection.released_on, collection.followed_source,
  collection.followed_source_id, collection.identification_state, item.filing_library_id, library.metadata_source,
  item_text.name, ordering.id AS aired_ordering_id
FROM item
CROSS JOIN collection ON collection.id = item.id AND collection.kind = 'series'
CROSS JOIN library ON library.id = item.filing_library_id
LEFT JOIN item_text ON item_text.item_id = collection.id AND item_text.is_original = 1
LEFT JOIN ordering ON ordering.collection_id = collection.id AND ordering.name = 'aired'
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'collection';

-- name: ListLibrarySeriesMembers :many
-- The titles in the library's live series, with their places.
SELECT collection_member.collection_id, collection_member.member_title, collection_member.position
FROM item
JOIN collection_member ON collection_member.collection_id = item.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'collection'
  AND collection_member.member_title IS NOT NULL;

-- name: ListLibrarySeriesEpisodePlaces :many
-- ListSeriesEpisodePlaces for every live series of the library.
SELECT ordering.collection_id, position.title_id, position.group_number, position.number_in_group, episode.removed_at
FROM item
CROSS JOIN ordering ON ordering.collection_id = item.id AND ordering.name = 'aired'
CROSS JOIN position ON position.ordering_id = ordering.id
CROSS JOIN item AS episode ON episode.id = position.title_id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'collection'
  AND (episode.removed_at IS NULL OR episode.merged_into IS NULL)
ORDER BY ordering.collection_id, position.sort_order;

-- name: ListLibraryProviderResponses :many
-- The stored responses of the library's live items, which a pass parses and never fetches (D170).
SELECT provider_response.item_id, provider_response.provider, provider_response.part,
  provider_response.fetched_at, provider_response.body_gzip
FROM item
CROSS JOIN provider_response ON provider_response.item_id = item.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL;

-- name: ListLibraryLiveEpisodeSources :many
-- ListLiveSeriesEpisodeSources for every live series of the library.
SELECT ordering.collection_id, position.title_id, position.group_number, position.number_in_group,
  title.followed_source, episode.added_at
FROM item
CROSS JOIN ordering ON ordering.collection_id = item.id AND ordering.name = 'aired'
CROSS JOIN position ON position.ordering_id = ordering.id
CROSS JOIN title ON title.id = position.title_id
CROSS JOIN item AS episode ON episode.id = title.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'collection' AND episode.removed_at IS NULL
ORDER BY ordering.collection_id, position.sort_order;

-- name: ListLibraryExtraRelations :many
-- The extras of the library's live titles, and who asserted each.
SELECT extra_relation.owner_title, extra_relation.extra_title, extra_relation.extra_type, extra_relation.asserted_by
FROM item
JOIN extra_relation ON extra_relation.owner_title = item.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'title';

-- name: CountRowsChangedByTheWriter :one
-- How many rows the writer has changed since it opened: a pass writing in batches reads it as it
-- goes, to know how much its batch has written.
SELECT CAST(total_changes() AS INTEGER) AS changed;

-- name: ListFileVersions :many
-- The versions a file is in, one for each kind of library that filed it, and their titles (D363
-- section 5).
SELECT file_filing.library_kind, file_filing.version_id, version.title_id
FROM file_filing
CROSS JOIN version ON version.id = file_filing.version_id
WHERE file_filing.file_id = sqlc.arg(id)
ORDER BY file_filing.library_kind;

-- name: DeleteFilingsOfFile :exec
-- A file that became a copy of another is that file's place, and files nothing of its own (D363
-- section 2).
DELETE FROM file_filing WHERE file_id = sqlc.arg(file_id);

-- name: RemoveVersion :exec
-- A version whose file was retired: the row a listing stored unread for a file stored before.
UPDATE version SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: MergeTitleWithNoLiveVersion :exec
-- A title the pass filed an unread file in, merged into the title of the file its read found it
-- was, once it has no live version left (D304 section 5).
UPDATE item SET merged_into = sqlc.arg(merged_into), removed_at = sqlc.arg(removed_at)
WHERE item.id = sqlc.arg(title_id) AND item.removed_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM version WHERE version.title_id = sqlc.arg(title_id) AND version.removed_at IS NULL);

-- name: ListUnnumberedEpisodeFiles :many
-- The live files of each series' live episodes that have no number in its aired order, with the
-- series' name, at most max_rows: those outside a specials or extras folder suggest names read
-- wrongly (D306 section 6).
SELECT ordering.collection_id AS series_id, CAST(coalesce(series_text.name, '') AS TEXT) AS series_name, file.path
FROM position
JOIN ordering ON ordering.id = position.ordering_id AND ordering.name = 'aired'
JOIN item AS episode ON episode.id = position.title_id AND episode.removed_at IS NULL
JOIN item AS series ON series.id = ordering.collection_id AND series.removed_at IS NULL
LEFT JOIN item_text AS series_text ON series_text.item_id = ordering.collection_id AND series_text.is_original = 1
JOIN version ON version.title_id = position.title_id AND version.removed_at IS NULL
JOIN file_filing ON file_filing.version_id = version.id
JOIN file ON file.id = file_filing.file_id AND file.removed_at IS NULL
WHERE position.number_in_group IS NULL
ORDER BY ordering.collection_id, file.path
LIMIT sqlc.arg(max_rows);

-- name: ListFilmsWithoutSound :many
-- The live films whose video file was read and holds a picture but no audio track, with the film's
-- name, at most max_rows: a video-only download whose sound was never merged in, which Needs
-- attention lists among the problem films (the owner, 2026-11-02).
SELECT title.id AS title_id, CAST(coalesce(title_text.name, '') AS TEXT) AS title_name, file.path
FROM title
JOIN item ON item.id = title.id AND item.removed_at IS NULL
LEFT JOIN item_text AS title_text ON title_text.item_id = title.id AND title_text.is_original = 1
JOIN version ON version.title_id = title.id AND version.removed_at IS NULL
JOIN file_filing ON file_filing.version_id = version.id
JOIN file ON file.id = file_filing.file_id AND file.removed_at IS NULL
WHERE title.kind = 'movie' AND title.media_kind = 'video'
  AND EXISTS (SELECT 1 FROM stream WHERE stream.video_file = file.id AND stream.kind = 'video'
    AND (stream.disposition_flags & 1024) = 0)
  AND NOT EXISTS (SELECT 1 FROM stream WHERE stream.video_file = file.id AND stream.kind = 'audio')
ORDER BY title.id, file.path
LIMIT sqlc.arg(max_rows);

-- name: ListFilesOfWork :many
-- The live videos a film, an episode or a series holds, a series' through its episodes, each with
-- how it was filed and its title: what a report of its filing lists (D306 section 3).
SELECT file.id, file.path, file_filing.filing, file_filing.filing_reason, file.size_bytes, file.node_id,
  title.id AS title_id, title.kind AS title_kind, title.identification_state,
  CAST(coalesce(title_text.name, '') AS TEXT) AS title_name, title.year,
  position.group_number, position.number_in_group
FROM title
JOIN version ON version.title_id = title.id AND version.removed_at IS NULL
JOIN file_filing ON file_filing.version_id = version.id
JOIN file ON file.id = file_filing.file_id AND file.removed_at IS NULL
LEFT JOIN item_text AS title_text ON title_text.item_id = title.id AND title_text.is_original = 1
LEFT JOIN position ON position.title_id = title.id
  AND position.ordering_id IN (SELECT ordering.id FROM ordering WHERE ordering.name = 'aired')
WHERE title.id = sqlc.arg(item_id)
  OR title.id IN (SELECT position.title_id FROM position JOIN ordering ON ordering.id = position.ordering_id
    WHERE ordering.collection_id = sqlc.arg(item_id) AND ordering.name = 'aired')
ORDER BY file.path
LIMIT sqlc.arg(max_rows);

-- name: ListUnnumberedEpisodePathsOfSeries :many
-- The live files of a series' live episodes that have no number in its aired order: those outside
-- a specials or extras folder have every season's listing fetched, to be placed by their names
-- (D307 section 2).
SELECT file.path
FROM ordering
JOIN position ON position.ordering_id = ordering.id AND position.number_in_group IS NULL
JOIN item AS episode ON episode.id = position.title_id AND episode.removed_at IS NULL
JOIN version ON version.title_id = position.title_id AND version.removed_at IS NULL
JOIN file_filing ON file_filing.version_id = version.id
JOIN file ON file.id = file_filing.file_id AND file.removed_at IS NULL
WHERE ordering.collection_id = sqlc.arg(series_id) AND ordering.name = 'aired'
ORDER BY file.path
LIMIT sqlc.arg(max_rows);

-- name: ListLibraryUnnumberedEpisodePaths :many
-- ListUnnumberedEpisodePathsOfSeries for every series of a library, in one statement a pass.
SELECT ordering.collection_id AS series_id, file.path
FROM item AS series
JOIN ordering ON ordering.collection_id = series.id AND ordering.name = 'aired'
JOIN position ON position.ordering_id = ordering.id AND position.number_in_group IS NULL
JOIN item AS episode ON episode.id = position.title_id AND episode.removed_at IS NULL
JOIN version ON version.title_id = position.title_id AND version.removed_at IS NULL
JOIN file_filing ON file_filing.version_id = version.id
JOIN file ON file.id = file_filing.file_id AND file.removed_at IS NULL
WHERE series.filing_library_id = sqlc.arg(library_id) AND series.removed_at IS NULL
ORDER BY ordering.collection_id, file.path;

-- name: ListLibraryOtherVideos :many
-- The other videos of a library's live titles, as the pass last derived them (D349 section 4).
SELECT other_video.title_id, other_video.recorded_at, other_video.recorded_from, other_video.file_name,
  other_video.file_name_key
FROM item
JOIN other_video ON other_video.title_id = item.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL AND item.kind = 'title';

-- name: GetOtherVideo :one
SELECT title_id, recorded_at, recorded_from, file_name, file_name_key
FROM other_video
WHERE title_id = ?;

-- name: UpsertOtherVideo :exec
-- An other video's date, where it came from, and its file's name and that name's sort key.
INSERT INTO other_video (title_id, recorded_at, recorded_from, file_name, file_name_key)
VALUES (sqlc.arg(title_id), sqlc.arg(recorded_at), sqlc.arg(recorded_from), sqlc.arg(file_name), sqlc.arg(file_name_key))
ON CONFLICT (title_id) DO UPDATE SET recorded_at = excluded.recorded_at, recorded_from = excluded.recorded_from,
  file_name = excluded.file_name, file_name_key = excluded.file_name_key
-- Only when it changes something (D303, DL9).
WHERE (other_video.recorded_at, other_video.recorded_from, other_video.file_name, other_video.file_name_key)
  IS NOT (excluded.recorded_at, excluded.recorded_from, excluded.file_name, excluded.file_name_key);

-- name: ListLibraryRecordedTimes :many
-- When each live file at a place a library covers says it was recorded, as its node's probe read it,
-- for those that say: an other video's date (D349 section 4).
SELECT DISTINCT library_place.file_id,
  CAST(json_extract(file_probe.probe_json, '$.format.recordedAtMs') AS INTEGER) AS recorded_at_ms
FROM library_place
CROSS JOIN file_probe ON file_probe.file_id = library_place.file_id
WHERE library_place.library_id = sqlc.arg(library_id)
  AND json_extract(file_probe.probe_json, '$.format.recordedAtMs') IS NOT NULL;

-- name: ListOtherVideoFileNames :many
-- Every other video's file name and its sort key in the locale, which a change of locale keys again.
SELECT title_id, file_name, file_name_key FROM other_video ORDER BY title_id;

-- name: SetOtherVideoFileNameKey :exec
UPDATE other_video SET file_name_key = sqlc.arg(file_name_key)
WHERE title_id = sqlc.arg(title_id) AND file_name_key IS NOT sqlc.arg(file_name_key);

-- name: CountFilingsOfVersion :one
-- How many files a version holds, live or not: a version a copy left holds others still.
SELECT count(*) FROM file_filing WHERE version_id = sqlc.arg(version_id);

-- name: GetFilingLibraryOfItem :one
-- The library whose pass files an item (D363 section 5); NULL for none.
SELECT filing_library_id FROM item WHERE id = sqlc.arg(id);
