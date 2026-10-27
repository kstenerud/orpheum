-- SPDX-License-Identifier: AGPL-3.0-or-later
-- An admin's rescan in progress, and the films and series it matches again (D357; schema.md,
-- cache.db).

-- name: InsertRescan :one
-- A rescan of a scope, unless one of that scope is in progress: then nothing, and no id.
INSERT INTO rescan (library_id, item_id, requested_at)
VALUES (sqlc.arg(library_id), sqlc.narg(item_id), sqlc.arg(requested_at))
ON CONFLICT (library_id, coalesce(item_id, 0)) DO NOTHING
RETURNING id;

-- name: ListRescans :many
-- Every rescan in progress, oldest first, with how many of its films and series wait to be matched.
SELECT rescan.id, rescan.library_id, rescan.item_id, rescan.requested_at, rescan.listed_at, rescan.item_count,
       rescan.changed_count, rescan.unchanged_count, rescan.left_alone_count, rescan.unsettled_count,
       rescan.failed_count,
       CAST((SELECT count(*) FROM rematch WHERE rematch.rescan_id = rescan.id) AS INTEGER) AS waiting_count
FROM rescan ORDER BY rescan.id;

-- name: CountRescansOfLibraryOrItem :one
-- The rescans in progress over a film or series: its own, or its library's.
SELECT count(*) FROM rescan
WHERE library_id = sqlc.arg(library_id) AND (item_id IS NULL OR item_id = sqlc.arg(item_id));

-- name: InsertRematch :execrows
-- A film or series a rescan matches again, unless another rescan already does.
INSERT INTO rematch (item_id, rescan_id, name, year, ids_json)
VALUES (sqlc.arg(item_id), sqlc.arg(rescan_id), sqlc.arg(name), sqlc.arg(year), sqlc.arg(ids_json))
ON CONFLICT (item_id) DO NOTHING;

-- name: SetRescanListed :exec
-- A rescan's scan over, and its films and series listed to match again.
UPDATE rescan SET listed_at = sqlc.arg(listed_at), item_count = sqlc.arg(item_count)
WHERE id = sqlc.arg(id);

-- name: GetRematch :one
SELECT item_id, rescan_id, name, year, ids_json FROM rematch WHERE item_id = sqlc.arg(item_id);

-- name: ListRematches :many
-- Every film and series a rescan waits to match again, with its rescan's scope.
SELECT rematch.item_id, rescan.item_id AS rescan_item_id
FROM rematch JOIN rescan ON rescan.id = rematch.rescan_id
ORDER BY rematch.item_id;

-- name: DeleteRematch :exec
DELETE FROM rematch WHERE item_id = sqlc.arg(item_id);

-- name: CountRematchesOfRescan :one
SELECT count(*) FROM rematch WHERE rescan_id = sqlc.arg(rescan_id);

-- name: CountRematchedInRescan :exec
-- One film or series matched again, counted by what became of it.
UPDATE rescan SET
  changed_count = changed_count + sqlc.arg(changed),
  unchanged_count = unchanged_count + sqlc.arg(unchanged),
  left_alone_count = left_alone_count + sqlc.arg(left_alone),
  unsettled_count = unsettled_count + sqlc.arg(unsettled),
  failed_count = failed_count + sqlc.arg(failed)
WHERE id = sqlc.arg(id);

-- name: GetRescan :one
SELECT id, library_id, item_id, requested_at, listed_at, item_count, changed_count, unchanged_count,
       left_alone_count, unsettled_count, failed_count
FROM rescan WHERE id = sqlc.arg(id);

-- name: DeleteRescan :exec
-- A rescan ended: every film and series it listed was matched again.
DELETE FROM rescan WHERE id = sqlc.arg(id);
