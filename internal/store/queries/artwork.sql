-- SPDX-License-Identifier: AGPL-3.0-or-later
-- An item's images (schema.md, artwork; D212): which it has, where each came from, and the hash
-- its sizes are stored under in cache/art/. A person's choice is is_chosen = 1 and stays.

-- name: ListLiveArtwork :many
-- An item's images, stored or refused, a series' groups' among them (group_number, D379); a miss at
-- its source that is not yet looked at again, as ListItemsDueRecheck finds it, is one still to be
-- tried (D213 section 1).
SELECT artwork.id, artwork.kind, artwork.group_number, artwork.source, artwork.source_ref, artwork.source_sha256,
  artwork.is_chosen, artwork.refused_at, artwork.is_missing_at_source,
  CAST(artwork.is_missing_at_source = 1 AND artwork.source_sha256 IS NULL AND NOT EXISTS (
    SELECT 1 FROM artwork earlier
    WHERE earlier.item_id = artwork.item_id AND earlier.kind = artwork.kind
      AND earlier.group_number IS artwork.group_number AND earlier.source = artwork.source
      AND earlier.source_ref = artwork.source_ref AND earlier.source_sha256 IS NULL AND earlier.removed_at IS NOT NULL
  ) AS INTEGER) AS is_recheck_owed
FROM artwork
WHERE artwork.item_id = sqlc.arg(item_id) AND artwork.removed_at IS NULL
ORDER BY artwork.kind, artwork.is_chosen DESC, artwork.id DESC;

-- name: InsertArtwork :one
-- A stored image, with its hash; or one that could not be stored, with none, and when and why
-- (D231 section 4), and whether its source said it was not there (D213 section 1). A season's
-- poster names its season (D379); NULL is the item's own.
INSERT INTO artwork (item_id, kind, group_number, source, source_ref, source_sha256, refused_at, refusal,
  is_missing_at_source)
VALUES (sqlc.arg(item_id), sqlc.arg(kind), sqlc.narg(group_number), sqlc.arg(source), sqlc.arg(source_ref),
  sqlc.arg(source_sha256), sqlc.arg(refused_at), sqlc.arg(refusal), sqlc.arg(is_missing_at_source))
RETURNING id;

-- name: RetireReplacedArtwork :exec
-- A provider's image replaced by a newer one of the same season, or of none; a person's choice is
-- never replaced this way.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id = sqlc.arg(item_id) AND kind = sqlc.arg(kind) AND group_number IS sqlc.narg(group_number)
  AND source = sqlc.arg(source)
  AND is_chosen = 0 AND removed_at IS NULL AND id != sqlc.arg(keep_id);

-- name: GetArtworkSource :one
-- Where a stored image came from, to fetch it again for a size made on request, D212 section 3.
SELECT item_id, kind, source, source_ref
FROM artwork
WHERE source_sha256 = sqlc.arg(source_sha256) AND removed_at IS NULL AND source_ref IS NOT NULL
ORDER BY id DESC
LIMIT 1;

-- name: ListShownArtwork :many
-- The images a page of items shows, of one kind, each its own, never a season's; the ids are a JSON
-- array. Per item, any image but
-- a frame still before one, which stands in only where no other is, D362; then a person's choice;
-- then the source the item's library prefers, an image beside the media unless the library takes
-- TMDB only, D215; then the newest stored. The caller keeps each item's first row.
SELECT artwork.item_id, artwork.source_sha256, artwork.source
FROM artwork
JOIN item ON item.id = artwork.item_id
LEFT JOIN library ON library.id = item.filing_library_id
WHERE artwork.item_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_id_list_json) AS TEXT)))
  AND artwork.kind = sqlc.arg(kind) AND artwork.group_number IS NULL AND artwork.removed_at IS NULL
  AND artwork.source_sha256 IS NOT NULL
ORDER BY artwork.item_id, artwork.source = 'frame', artwork.is_chosen DESC,
  (artwork.source = 'local') = (COALESCE(library.metadata_source, '') != 'tmdb') DESC, artwork.id DESC;

-- name: ListSeasonPosters :many
-- The posters of a series' seasons, D379: per season, a person's choice first, then the source its
-- library prefers, then the newest stored, as ListShownArtwork orders an item's own. The caller keeps
-- each season's first row.
SELECT CAST(artwork.group_number AS INTEGER) AS group_number, artwork.source_sha256
FROM artwork
JOIN item ON item.id = artwork.item_id
LEFT JOIN library ON library.id = item.filing_library_id
WHERE artwork.item_id = sqlc.arg(series_id) AND artwork.kind = 'poster' AND artwork.group_number IS NOT NULL
  AND artwork.removed_at IS NULL AND artwork.source_sha256 IS NOT NULL
ORDER BY artwork.group_number, artwork.is_chosen DESC,
  (artwork.source = 'local') = (COALESCE(library.metadata_source, '') != 'tmdb') DESC, artwork.id DESC;

-- name: RetryItemArtwork :exec
-- A refresh that looks again: the item's images that were missing or refused are asked for once
-- more at its next pass, D213 sections 1 and 2.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id = sqlc.arg(item_id) AND source_sha256 IS NULL AND is_chosen = 0 AND removed_at IS NULL;

-- name: ListArtworkSourceSha256s :many
-- Every stored image, whether a live item shows it, whether one a library lists does, and when it last
-- stopped being shown: what the artwork ceiling reads to find removed items' images, D212 section 6,
-- and those of items no library covers any more, kept until the ceiling needs their room (D363
-- section 10). A person or another kind of item is always listed.
SELECT artwork.source_sha256,
  CAST(MAX(CASE WHEN artwork.removed_at IS NULL AND item.removed_at IS NULL THEN 1 ELSE 0 END) AS INTEGER) AS is_live,
  CAST(MAX(CASE WHEN artwork.removed_at IS NULL AND item.removed_at IS NULL AND (
    item.kind NOT IN ('movie', 'series', 'episode', 'extra', 'other')
    OR EXISTS (
      SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
      WHERE version.work_id = item.id AND version.removed_at IS NULL)
    OR EXISTS (
      SELECT 1 FROM sequence_member
      CROSS JOIN version ON version.work_id = sequence_member.member_work
      CROSS JOIN version_library ON version_library.version_id = version.id
      WHERE sequence_member.sequence_id = item.id AND version.removed_at IS NULL)
  ) THEN 1 ELSE 0 END) AS INTEGER) AS is_listed,
  CAST(MAX(COALESCE(artwork.removed_at, item.removed_at, 0)) AS INTEGER) AS gone_at
FROM artwork
JOIN item ON item.id = artwork.item_id
WHERE artwork.source_sha256 IS NOT NULL
GROUP BY artwork.source_sha256;

-- name: GetFileForArtwork :one
-- An image beside the media: the node that holds it, and its path, for the node to read.
SELECT node_id, path, reported_path_raw
FROM file
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: ListItemsWithLiveUnchosenLocalArtwork :many
-- A library's items that show an image found beside the media, D215: each pass checks they are
-- still there. An image a person chose is left out: a choice stays even when its file goes.
SELECT DISTINCT artwork.item_id
FROM item
JOIN artwork ON artwork.item_id = item.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND artwork.source = 'local' AND artwork.removed_at IS NULL
  AND artwork.is_chosen = 0
ORDER BY artwork.item_id;

-- name: RetireArtworkNotFound :exec
-- An item's images from one source that the source no longer offers: an image beside the media
-- that a pass no longer found (a deleted poster.jpg), or a provider's image the item's new work
-- lacks after a rematch, D223 section 9. A person's choice stays. The refs kept are a JSON array.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id = sqlc.arg(item_id) AND source = sqlc.arg(source) AND is_chosen = 0 AND removed_at IS NULL
  AND (source_ref IS NULL OR source_ref NOT IN (SELECT value FROM json_each(CAST(sqlc.arg(found_refs) AS TEXT))));

-- name: ListItemsDueRecheck :many
-- The live items with an image their source said was missing, not yet looked at again, and
-- refused before refused_before, a CDN's 404 lifetime ago: each is owed one refresh that asks for
-- its images again (D213 section 1), derived here rather than held as a job (D231 section 3). A
-- recheck removes the row it looks at again, so a removed miss of the same image is one rechecked.
SELECT DISTINCT artwork.item_id
FROM artwork
JOIN item ON item.id = artwork.item_id AND item.removed_at IS NULL
WHERE artwork.source = sqlc.arg(source) AND artwork.removed_at IS NULL AND artwork.source_sha256 IS NULL
  AND artwork.is_missing_at_source = 1 AND artwork.refused_at <= sqlc.arg(refused_before)
  AND NOT EXISTS (
    SELECT 1 FROM artwork earlier
    WHERE earlier.item_id = artwork.item_id AND earlier.kind = artwork.kind
      AND earlier.group_number IS artwork.group_number AND earlier.source = artwork.source
      AND earlier.source_ref = artwork.source_ref AND earlier.source_sha256 IS NULL AND earlier.removed_at IS NOT NULL
  )
ORDER BY artwork.item_id;

-- name: RetireArtworkRow :exec
-- An image beside the media that was refused, owed again once a scan has seen its file since
-- (a disk that was away and came back, G9).
UPDATE artwork SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: GetFileLastSeenAt :one
-- When a scan last saw a file: an image beside the media refused before then is owed again.
SELECT last_seen_at FROM file WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: GetItemLibraryMetadataSource :one
-- Where an item's library takes its data from: one that takes NFO files first shows an image
-- beside the media over a provider's (D215), so a provider's of the same kind is not fetched.
SELECT library.metadata_source
FROM item
JOIN library ON library.id = item.filing_library_id
WHERE item.id = sqlc.arg(item_id);

-- name: IsArtworkReached :one
-- Whether an image is held by a live item a library the caller reaches lists (D243 section 4, D363
-- section 8): an image is served by its hash to whoever may see something that shows it, a title or a
-- series one of whose episodes it lists.
SELECT CAST(EXISTS (
  SELECT 1 FROM artwork
  JOIN item ON item.id = artwork.item_id AND item.removed_at IS NULL
  WHERE artwork.source_sha256 = sqlc.arg(source_sha256)
    AND (EXISTS (
      SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
      WHERE version.work_id = item.id AND version.removed_at IS NULL
        AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
    ) OR EXISTS (
      SELECT 1 FROM sequence_member
      CROSS JOIN version ON version.work_id = sequence_member.member_work
      CROSS JOIN version_library ON version_library.version_id = version.id
      WHERE sequence_member.sequence_id = item.id AND version.removed_at IS NULL
        AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
    ))
) AS INTEGER) AS is_reached;

-- name: ListFrameStillCandidates :many
-- A library's films, episodes and other videos that show no picture of their kind, and whose every
-- other source has been tried, each with its first live file that has been read: what a frame
-- still may stand in for, D362. A film shows none with no poster and no backdrop, an episode with
-- no still, an other video with neither. A film, or an episode's series, has been tried when its
-- library reads NFO files alone, when it follows no source or its NFO, or when its source's answer
-- is stored; never while it awaits its match, is ambiguous or its fetch failed. An other video is
-- never matched. A frame still of the file's present bytes holds it: made, refused for having no
-- frame, or refused since a scan last saw the file; one refused before is owed again, its row named
-- for the caller to retire, as an image beside the media is (G9). The images its sources name are
-- the caller's to weigh.
SELECT work.id AS work_id, work.kind AS work_kind,
  CAST(COALESCE((
    SELECT member.sequence_id FROM sequence_member member
    JOIN sequence ON sequence.id = member.sequence_id AND sequence.kind = 'series'
    WHERE member.member_work = work.id LIMIT 1
  ), 0) AS INTEGER) AS series_id,
  file.id AS file_id, file.size_bytes, file.modified_at,
  CAST(COALESCE((
    SELECT stale.id FROM artwork stale
    WHERE stale.item_id = work.id AND stale.source = 'frame' AND stale.removed_at IS NULL
      AND stale.source_ref = printf('file:%d:%d:%d', file.id, file.size_bytes, file.modified_at)
      AND stale.source_sha256 IS NULL AND stale.is_missing_at_source = 0 AND stale.refused_at < file.last_seen_at
    LIMIT 1
  ), 0) AS INTEGER) AS stale_refusal_id
FROM item
JOIN library ON library.id = item.filing_library_id
CROSS JOIN work ON work.id = item.id
JOIN file ON file.id = (
  SELECT live.id FROM version CROSS JOIN file_filing ON file_filing.version_id = version.id
  CROSS JOIN file AS live ON live.id = file_filing.file_id
  WHERE version.work_id = work.id AND version.removed_at IS NULL AND live.removed_at IS NULL
    AND live.payload_hash IS NOT NULL
  ORDER BY live.path LIMIT 1
)
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL
  AND work.kind IN ('movie', 'episode', 'other')
  AND NOT EXISTS (
    SELECT 1 FROM artwork shown
    WHERE shown.item_id = work.id AND shown.removed_at IS NULL AND shown.source_sha256 IS NOT NULL
      AND shown.source != 'frame'
      AND ((work.kind = 'movie' AND shown.kind IN ('poster', 'backdrop'))
        OR (work.kind = 'episode' AND shown.kind = 'still')
        OR (work.kind = 'other' AND shown.kind IN ('still', 'backdrop')))
  )
  AND NOT EXISTS (
    SELECT 1 FROM artwork held
    WHERE held.item_id = work.id AND held.source = 'frame' AND held.removed_at IS NULL
      AND held.source_ref = printf('file:%d:%d:%d', file.id, file.size_bytes, file.modified_at)
      AND (held.source_sha256 IS NOT NULL OR held.is_missing_at_source = 1 OR held.refused_at >= file.last_seen_at)
  )
  AND (
    work.kind = 'other'
    OR (work.kind = 'movie' AND item.identification_state != 'ambiguous'
      AND NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = work.id)
      AND (library.metadata_source = 'nfo' OR item.followed_source IN ('none', 'nfo')
        OR EXISTS (
          SELECT 1 FROM provider_response
          WHERE provider_response.item_id = work.id AND provider_response.provider = item.followed_source
            AND provider_response.part = ''
        )))
    OR (work.kind = 'episode' AND EXISTS (
      SELECT 1 FROM sequence_member member
      JOIN item series ON series.id = member.sequence_id AND series.kind = 'series'
      WHERE member.member_work = work.id AND series.identification_state != 'ambiguous'
        AND NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = series.id)
        AND (library.metadata_source = 'nfo' OR series.followed_source IN ('none', 'nfo')
          OR EXISTS (
            SELECT 1 FROM provider_response
            WHERE provider_response.item_id = series.id AND provider_response.provider = series.followed_source
              AND provider_response.part = ''
          ))
    ))
  )
ORDER BY work.id;

-- name: ListFirstEpisodes :many
-- Each series' first episode a library lists, of the first regular season in its aired ordering,
-- else of the specials: the episode whose still a series with no poster shows, D362. The ids are a
-- JSON array.
SELECT ordering.sequence_id AS series_id, CAST((
  SELECT position.work_id FROM position
  JOIN work ON work.id = position.work_id
  WHERE position.ordering_id = ordering.id AND EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = work.id AND version.removed_at IS NULL
  )
  ORDER BY position.group_number IS NULL, position.group_number = 0, position.group_number, position.sort_order
  LIMIT 1
) AS INTEGER) AS episode_id
FROM ordering
WHERE ordering.sequence_id IN (SELECT value FROM json_each(CAST(sqlc.arg(series_id_list_json) AS TEXT)))
  AND ordering.name = 'aired';

-- name: GetWorkKind :one
-- A title's kind: which pictures of it a frame still stands in for, D362.
SELECT kind FROM work WHERE id = sqlc.arg(id);

-- name: RetireFrameStillsOfFile :execrows
-- The frame stills taken from a file, retired once the facts of its probe they were taken by changed:
-- the next pass takes one again where it is still wanted (D362, D392 section 3). A person's choice
-- stays.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id IN (
    SELECT version.work_id FROM file_filing JOIN version ON version.id = file_filing.version_id
    WHERE file_filing.file_id = sqlc.arg(file_id)
  )
  AND source = 'frame' AND removed_at IS NULL AND is_chosen = 0
  AND source_ref GLOB 'file:' || CAST(sqlc.arg(file_id) AS INTEGER) || ':*';
