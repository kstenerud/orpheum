-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The places that cannot be read now: a root, or a disk mounted inside one, with since when and why
-- (D328). Only internal/core/library's availability.go writes them.

-- name: MarkPlaceUnavailable :exec
-- A place found unavailable for cause: from now, unless it already was, when it keeps its since and
-- takes the latest cause.
INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
VALUES (sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(root_path), sqlc.arg(now), sqlc.arg(cause))
ON CONFLICT (node_id, path) DO UPDATE SET cause = excluded.cause
-- Only when it changes something (D303, DL9).
WHERE unavailable_place.cause IS NOT excluded.cause;

-- name: GetUnavailablePlaceSince :one
SELECT since FROM unavailable_place WHERE node_id = sqlc.arg(node_id) AND path = sqlc.arg(path);

-- name: ClearUnavailablePlace :execrows
-- A place a scan read, or found its disk where it was: it is available again.
DELETE FROM unavailable_place WHERE node_id = sqlc.arg(node_id) AND path = sqlc.arg(path);

-- name: BringBackPlace :execrows
-- A place whose disk answers again with the facts its last complete scan recorded, its device and
-- whether it is a mount point: it is available again before a scan reads it. Other facts are the
-- empty directory an unmounted disk leaves, which stays unavailable until a scan reads it (D208,
-- D326, D328 section 6).
DELETE FROM unavailable_place
WHERE unavailable_place.node_id = sqlc.arg(node_id) AND unavailable_place.path = sqlc.arg(path)
  AND (EXISTS (SELECT 1 FROM root_state
      WHERE root_state.node_id = sqlc.arg(node_id) AND root_state.root_path = sqlc.arg(path)
        AND root_state.filesystem_device = sqlc.arg(filesystem_device)
        AND root_state.is_mount_point = sqlc.arg(is_mount_point))
    OR EXISTS (SELECT 1 FROM root_mount
      WHERE root_mount.node_id = sqlc.arg(node_id) AND root_mount.mount_path = sqlc.arg(path)
        AND root_mount.filesystem_device = sqlc.arg(filesystem_device) AND sqlc.arg(is_mount_point) = 1));

-- name: ForgetRootPlaces :exec
-- A retired or repointed root's places at its old place.
DELETE FROM unavailable_place WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: ListUnavailablePlacesOfNode :many
-- A node's places that cannot be read now, and since when: what a file under one is not there to read
-- for (D328 section 1).
SELECT path, root_path, since FROM unavailable_place WHERE node_id = sqlc.arg(node_id) ORDER BY path;

-- name: ListUnavailablePlaces :many
-- Every place of a live library's root that cannot be read now, with why and its root's last scan:
-- what the sweep asks its node of, and scans again until one reads it (D233 section 10, D327
-- section 3).
SELECT DISTINCT unavailable_place.node_id, unavailable_place.path, unavailable_place.root_path, unavailable_place.cause,
  CAST(coalesce(root_state.last_scan_at, 0) AS INTEGER) AS last_scan_at
FROM unavailable_place
JOIN library_root ON library_root.node_id = unavailable_place.node_id
  AND library_root.root_path = unavailable_place.root_path AND library_root.retired_at IS NULL
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
LEFT JOIN root_state ON root_state.node_id = unavailable_place.node_id
  AND root_state.root_path = unavailable_place.root_path
ORDER BY unavailable_place.node_id, unavailable_place.path;

-- name: ListOfflinePlaces :many
-- The places of live libraries' roots, on live nodes and not retired, that cannot be read now, and
-- since when: what the System page says is offline until an admin retires it or it comes back (D41,
-- D231 section 7). A place whose path is its root's is the root; any other, a disk inside it.
SELECT library.id AS library_id, library.name AS library_name, node.name AS node_name,
  unavailable_place.root_path, unavailable_place.path, unavailable_place.since
FROM unavailable_place
JOIN library_root ON library_root.node_id = unavailable_place.node_id
  AND library_root.root_path = unavailable_place.root_path AND library_root.retired_at IS NULL
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
JOIN node ON node.id = unavailable_place.node_id AND node.removed_at IS NULL
ORDER BY unavailable_place.since, library.id, unavailable_place.path;
