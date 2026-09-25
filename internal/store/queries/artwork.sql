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
