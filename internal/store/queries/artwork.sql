-- SPDX-License-Identifier: GPL-3.0-or-later
-- An item's images (schema.md, artwork; D212): which it has, where each came from, and the hash
-- its sizes are stored under in cache/art/. A person's choice is chosen = 1 and stays.

-- name: ListLiveArtwork :many
SELECT id, kind, source, source_ref, content_hash, chosen
FROM artwork
WHERE item_id = sqlc.arg(item_id) AND removed_at IS NULL
ORDER BY kind, chosen DESC, id DESC;

-- name: InsertArtwork :one
INSERT INTO artwork (item_id, kind, source, source_ref, content_hash)
VALUES (sqlc.arg(item_id), sqlc.arg(kind), sqlc.arg(source), sqlc.arg(source_ref), sqlc.arg(content_hash))
RETURNING id;

-- name: MarkArtworkRemoved :exec
-- A provider's image replaced by a newer one; a person's choice is never replaced this way.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id = sqlc.arg(item_id) AND kind = sqlc.arg(kind) AND source = sqlc.arg(source)
  AND chosen = 0 AND removed_at IS NULL AND id != sqlc.arg(keep_id);

-- name: GetArtworkSource :one
-- Where a stored image came from, to fetch it again for a size made on request, D212 section 3.
SELECT item_id, kind, source, source_ref
FROM artwork
WHERE content_hash = sqlc.arg(content_hash) AND removed_at IS NULL AND source_ref IS NOT NULL
ORDER BY id DESC
LIMIT 1;

-- name: ListShownArtwork :many
-- The images a page of items shows, of one kind; the ids are a JSON array. Per item, a person's
-- choice first, then the newest stored; the caller keeps the first row of each item.
SELECT item_id, content_hash
FROM artwork
WHERE item_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_ids) AS TEXT))) AND kind = sqlc.arg(kind)
  AND removed_at IS NULL AND content_hash IS NOT NULL
ORDER BY item_id, chosen DESC, id DESC;

-- name: CountEarlierMisses :one
-- How often this image was missing before and looked into since: a recheck removes the row it
-- left, so a removed row with no hash is a miss already rechecked, D213 section 1.
SELECT COUNT(*) AS misses
FROM artwork
WHERE item_id = sqlc.arg(item_id) AND kind = sqlc.arg(kind) AND source = sqlc.arg(source)
  AND source_ref = sqlc.arg(source_ref) AND content_hash IS NULL AND removed_at IS NOT NULL;

-- name: RetryItemArtwork :exec
-- A refresh that looks again: the item's images that were missing or refused are asked for once
-- more at its next pass, D213 sections 1 and 2.
UPDATE artwork SET removed_at = sqlc.arg(removed_at)
WHERE item_id = sqlc.arg(item_id) AND content_hash IS NULL AND chosen = 0 AND removed_at IS NULL;

-- name: ListArtworkHashes :many
-- Every stored image, whether a live item shows it, and when it last stopped being shown: what
-- the artwork ceiling reads to find removed items' images, D212 section 6.
SELECT artwork.content_hash AS hash,
  CAST(MAX(CASE WHEN artwork.removed_at IS NULL AND item.removed_at IS NULL THEN 1 ELSE 0 END) AS INTEGER) AS live,
  CAST(MAX(COALESCE(artwork.removed_at, item.removed_at, 0)) AS INTEGER) AS gone_at
FROM artwork
JOIN item ON item.id = artwork.item_id
WHERE artwork.content_hash IS NOT NULL
GROUP BY artwork.content_hash;
