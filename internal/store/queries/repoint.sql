-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Repointing a folder (D429): "folder X on node A is now folder Y on node B" rewrites every stored
-- path under X in one transaction. Only internal/core/library's repoint.go runs these.

-- name: ListPlacesUnder :many
-- Every place on a node at or under a folder, live and removed (the folder's own, a folder a scan
-- found, and library.PathRange's range of bytes; D211): what a repoint of the folder moves, or what is
-- already known at its target. The live ones are read through place_live_path and the removed ones
-- through place_removed_at: one query over both read the whole table (sql.md section 11).
SELECT live.id, live.file_id, live.path, live.reported_path_raw, live.removed_at
FROM place AS live
WHERE live.node_id = sqlc.arg(node_id) AND live.removed_at IS NULL
  AND (live.reported_path_raw = CAST(sqlc.arg(folder) AS BLOB)
    OR (live.reported_path_raw > CAST(sqlc.arg(after) AS BLOB) AND live.reported_path_raw < CAST(sqlc.arg(before) AS BLOB)))
UNION ALL
SELECT gone.id, gone.file_id, gone.path, gone.reported_path_raw, gone.removed_at
FROM place AS gone
WHERE gone.removed_at IS NOT NULL AND gone.node_id = sqlc.arg(node_id)
  AND (gone.reported_path_raw = CAST(sqlc.arg(folder) AS BLOB)
    OR (gone.reported_path_raw > CAST(sqlc.arg(after) AS BLOB) AND gone.reported_path_raw < CAST(sqlc.arg(before) AS BLOB)))
ORDER BY 4;

-- name: CountLookAlikePlacesUnder :one
-- The places on a node whose text (NFC) is under a folder but whose bytes are not: a folder spelled
-- in another normal form, a different folder on disk, which a repoint by bytes leaves where it is.
SELECT count(*)
FROM place
WHERE node_id = sqlc.arg(node_id)
  AND path > sqlc.arg(text_after) AND path < sqlc.arg(text_before)
  AND NOT (reported_path_raw > CAST(sqlc.arg(after) AS BLOB) AND reported_path_raw < CAST(sqlc.arg(before) AS BLOB));

-- name: ListRootsOfNodeWithRetired :many
-- Every folder on a node of every library, live and retired, with its library's name and whether the
-- library and the folder are live: what a repoint moves and what it checks for conflicts.
SELECT library_root.library_id, library.name AS library_name, library_root.root_path,
  CAST(library_root.retired_at IS NULL AND library.removed_at IS NULL AS INTEGER) AS is_live
FROM library_root
JOIN library ON library.id = library_root.library_id
WHERE library_root.node_id = sqlc.arg(node_id)
ORDER BY library_root.library_id, library_root.root_path;

-- name: MovePlace :exec
-- A place a repoint moves, first aside to a path no real one has and then to its new place: its node,
-- its bytes and their text. Moved for good (is_final), a live place is unconfirmed, so the next scan
-- reads it again and confirms it (D432 section 6), and moved to another node it forgets its device
-- and inode, which name nothing there.
UPDATE place SET node_id = sqlc.arg(node_id), path = sqlc.arg(path), reported_path_raw = sqlc.arg(reported_path_raw),
  is_unconfirmed = CASE WHEN CAST(sqlc.arg(is_final) AS INTEGER) = 1 AND removed_at IS NULL THEN 1 ELSE is_unconfirmed END,
  filesystem_device = CASE WHEN CAST(sqlc.arg(forget_disk) AS INTEGER) = 1 THEN NULL ELSE filesystem_device END,
  inode = CASE WHEN CAST(sqlc.arg(forget_disk) AS INTEGER) = 1 THEN NULL ELSE inode END
WHERE id = sqlc.arg(id);

-- name: MoveLibraryRoot :execrows
-- A library's folder, live or retired, that a repoint moves: first aside, then to its new place.
UPDATE library_root SET node_id = sqlc.arg(new_node_id), root_path = sqlc.arg(new_root_path)
WHERE library_id = sqlc.arg(library_id) AND node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: ForgetRootStatesAtOrUnder :exec
-- The scan state of every folder at or under a repointed folder: its facts, rebuilt by the scan of
-- its new place.
DELETE FROM root_state
WHERE node_id = sqlc.arg(node_id)
  AND (root_path = sqlc.arg(root_path) OR (root_path > sqlc.arg(after) AND root_path < sqlc.arg(before)));

-- name: ForgetRootScansAtOrUnder :exec
DELETE FROM root_scan
WHERE node_id = sqlc.arg(node_id)
  AND (root_path = sqlc.arg(root_path) OR (root_path > sqlc.arg(after) AND root_path < sqlc.arg(before)));

-- name: ListRootMountsAtOrUnder :many
-- The disks mounted at or under a repointed folder, whatever root they are inside: their parked work
-- goes and their facts are forgotten.
SELECT mount_path FROM root_mount
WHERE node_id = sqlc.arg(node_id)
  AND (mount_path = sqlc.arg(root_path) OR (mount_path > sqlc.arg(after) AND mount_path < sqlc.arg(before))
    OR root_path = sqlc.arg(root_path) OR (root_path > sqlc.arg(after) AND root_path < sqlc.arg(before)))
ORDER BY mount_path;

-- name: ForgetRootMountsAtOrUnder :exec
DELETE FROM root_mount
WHERE node_id = sqlc.arg(node_id)
  AND (mount_path = sqlc.arg(root_path) OR (mount_path > sqlc.arg(after) AND mount_path < sqlc.arg(before))
    OR root_path = sqlc.arg(root_path) OR (root_path > sqlc.arg(after) AND root_path < sqlc.arg(before)));

-- name: ForgetUnavailablePlacesAtOrUnder :exec
-- The availability of every place at or under a repointed folder, or of a root inside it: a scan of
-- the new place finds it again.
DELETE FROM unavailable_place
WHERE node_id = sqlc.arg(node_id)
  AND (path = sqlc.arg(root_path) OR (path > sqlc.arg(after) AND path < sqlc.arg(before))
    OR root_path = sqlc.arg(root_path) OR (root_path > sqlc.arg(after) AND root_path < sqlc.arg(before)));
