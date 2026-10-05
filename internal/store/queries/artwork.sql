-- SPDX-License-Identifier: GPL-3.0-or-later
-- An item's images (schema.md, artwork; D212): which it has, where each came from, and the hash
-- its sizes are stored under in cache/art/. A person's choice is is_chosen = 1 and stays.

-- name: ListLiveArtwork :many
SELECT id, kind, source, source_ref, source_sha256, is_chosen, refused_at
FROM artwork
WHERE item_id = sqlc.arg(item_id) AND removed_at IS NULL
ORDER BY kind, is_chosen DESC, id DESC;

-- name: InsertArtwork :one
-- A stored image, with its hash; or one that could not be stored, with none, and when and why
-- (D231 section 4), and whether its source said it was not there (D213 section 1).
INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, refused_at, refusal, is_missing_at_source)
VALUES (sqlc.arg(item_id), sqlc.arg(kind), sqlc.arg(source), sqlc.arg(source_ref), sqlc.arg(source_sha256),
  sqlc.arg(refused_at), sqlc.arg(refusal), sqlc.arg(is_missing_at_source))
RETURNING id;

-- name: RetireReplacedArtwork :exec
-- A provider's image replaced by a newer one; a person's choice is never replaced this way.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id = sqlc.arg(item_id) AND kind = sqlc.arg(kind) AND source = sqlc.arg(source)
  AND is_chosen = 0 AND removed_at IS NULL AND id != sqlc.arg(keep_id);

-- name: GetArtworkSource :one
-- Where a stored image came from, to fetch it again for a size made on request, D212 section 3.
SELECT item_id, kind, source, source_ref
FROM artwork
WHERE source_sha256 = sqlc.arg(source_sha256) AND removed_at IS NULL AND source_ref IS NOT NULL
ORDER BY id DESC
LIMIT 1;

-- name: ListShownArtwork :many
-- The images a page of items shows, of one kind; the ids are a JSON array. Per item, a person's
-- choice first; then the source the item's library prefers, an image beside the media unless the
-- library takes TMDB only, D215; then the newest stored. The caller keeps each item's first row.
SELECT artwork.item_id, artwork.source_sha256
FROM artwork
JOIN item ON item.id = artwork.item_id
LEFT JOIN library ON library.id = item.library_id
WHERE artwork.item_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_id_list_json) AS TEXT)))
  AND artwork.kind = sqlc.arg(kind) AND artwork.removed_at IS NULL AND artwork.source_sha256 IS NOT NULL
ORDER BY artwork.item_id, artwork.is_chosen DESC,
  (artwork.source = 'local') = (COALESCE(library.metadata_source, '') != 'tmdb') DESC, artwork.id DESC;

-- name: CountEarlierMisses :one
-- How often this image was missing before and looked into since: a recheck removes the row it
-- left, so a removed row with no hash is a miss already rechecked, D213 section 1.
SELECT COUNT(*) AS misses
FROM artwork
WHERE item_id = sqlc.arg(item_id) AND kind = sqlc.arg(kind) AND source = sqlc.arg(source)
  AND source_ref = sqlc.arg(source_ref) AND source_sha256 IS NULL AND removed_at IS NOT NULL;

-- name: RetryItemArtwork :exec
-- A refresh that looks again: the item's images that were missing or refused are asked for once
-- more at its next pass, D213 sections 1 and 2.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id = sqlc.arg(item_id) AND source_sha256 IS NULL AND is_chosen = 0 AND removed_at IS NULL;

-- name: ListArtworkSourceSha256s :many
-- Every stored image, whether a live item shows it, and when it last stopped being shown: what
-- the artwork ceiling reads to find removed items' images, D212 section 6.
SELECT artwork.source_sha256,
  CAST(MAX(CASE WHEN artwork.removed_at IS NULL AND item.removed_at IS NULL THEN 1 ELSE 0 END) AS INTEGER) AS is_live,
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
WHERE item.library_id = sqlc.arg(library_id) AND artwork.source = 'local' AND artwork.removed_at IS NULL
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
    WHERE earlier.item_id = artwork.item_id AND earlier.kind = artwork.kind AND earlier.source = artwork.source
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
JOIN library ON library.id = item.library_id
WHERE item.id = sqlc.arg(item_id);

-- name: IsArtworkReached :one
-- Whether an image is held by a live item in a library the caller reaches (D243 section 4): an
-- image is served by its hash to whoever may see something that shows it.
SELECT EXISTS (
  SELECT 1 FROM artwork
  JOIN item ON item.id = artwork.item_id AND item.removed_at IS NULL
  WHERE artwork.source_sha256 = sqlc.arg(source_sha256)
    AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
      OR item.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
) AS is_reached;
