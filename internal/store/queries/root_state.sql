-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each root's last scan and what it found (D208, D41, D42).

-- name: GetRootState :one
SELECT filesystem_device, is_mount_point, last_scan_outcome, withheld_scan_started_at, withheld_reason, are_removals_approved
FROM root_state
WHERE node_id = ? AND root_path = ?;

-- name: RecordRootComplete :exec
-- A complete scan: its facts become the known-good ones later scans are compared against, and the
-- root is no longer unavailable.
INSERT INTO root_state (node_id, root_path, filesystem_device, is_mount_point, last_scan_outcome, last_scan_at)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(filesystem_device), sqlc.arg(is_mount_point), 'complete', sqlc.arg(last_scan_at))
ON CONFLICT (node_id, root_path) DO UPDATE SET
  filesystem_device = excluded.filesystem_device, is_mount_point = excluded.is_mount_point, last_scan_outcome = 'complete',
  last_scan_at = excluded.last_scan_at, withheld_scan_started_at = NULL, withheld_reason = NULL, are_removals_approved = 0,
  unavailable_since = NULL;

-- name: RecordRootNotComplete :exec
-- A scan that could not read its root, or whose removals wait for the admin: the known-good
-- facts stay as they were, and the root is unavailable since the first such scan in a row (D41,
-- D231 section 7).
INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at, withheld_reason,
  unavailable_since)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(last_scan_outcome), sqlc.arg(last_scan_at), sqlc.arg(withheld_scan_started_at),
  sqlc.arg(withheld_reason), sqlc.arg(last_scan_at))
ON CONFLICT (node_id, root_path) DO UPDATE SET
  last_scan_outcome = excluded.last_scan_outcome, last_scan_at = excluded.last_scan_at,
  withheld_scan_started_at = excluded.withheld_scan_started_at, withheld_reason = excluded.withheld_reason,
  unavailable_since = coalesce(root_state.unavailable_since, excluded.last_scan_at);

-- name: MarkRootUnavailable :one
-- Work stalled waiting on the root's disk: the root is unavailable from now, unless it already
-- was, until a scan reads it again (D233 section 10). A root no scan has read yet has no state, and
-- returns nothing.
UPDATE root_state SET unavailable_since = coalesce(unavailable_since, sqlc.arg(now))
WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path)
RETURNING CAST(unavailable_since AS INTEGER) AS unavailable_since;

-- name: ListRootMounts :many
SELECT mount_path, filesystem_device, is_present
FROM root_mount
WHERE node_id = ? AND root_path = ?
ORDER BY mount_path;

-- name: RecordRootMountPresent :exec
INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(mount_path), sqlc.arg(filesystem_device), 1)
ON CONFLICT (node_id, mount_path) DO UPDATE SET
  root_path = excluded.root_path, filesystem_device = excluded.filesystem_device, is_present = 1, missing_since = NULL;

-- name: RecordRootMountMissing :exec
-- A disk a scan did not find where it was: missing since the first such scan in a row.
UPDATE root_mount SET is_present = 0, missing_since = coalesce(missing_since, sqlc.arg(missing_since))
WHERE node_id = sqlc.arg(node_id) AND mount_path = sqlc.arg(mount_path);

-- name: ApproveRootRemovals :execrows
-- An admin's approval: the root's next complete scan removes what it does not see without
-- waiting (D208 section 3). A complete scan clears it.
UPDATE root_state SET are_removals_approved = 1
WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: ForgetMissingRootMount :execrows
-- An admin says a disk missing from a mount point inside a root is gone for good: the boundary
-- is forgotten, so a scan reads what is under it as removed. Only a missing one is forgotten; a
-- boundary is scan state, not a person's record (standards/sql.md section 6).
DELETE FROM root_mount
WHERE node_id = sqlc.arg(node_id) AND mount_path = sqlc.arg(mount_path) AND is_present = 0;

-- name: GetRootMount :one
SELECT root_path, is_present
FROM root_mount
WHERE node_id = sqlc.arg(node_id) AND mount_path = sqlc.arg(mount_path);

-- name: ListUnavailableRootsOfNode :many
-- A node's roots whose last scan could not read them, or withheld its removals as a lost disk, or
-- whose disk work stalled on since: their files are not there to read, since the first of those
-- in a row (D208, D231 section 5, D233 section 10).
SELECT root_path, CAST(coalesce(unavailable_since, last_scan_at) AS INTEGER) AS unavailable_since
FROM root_state
WHERE node_id = sqlc.arg(node_id) AND (last_scan_outcome <> 'complete' OR unavailable_since IS NOT NULL)
ORDER BY root_path;

-- name: ListMissingMountsOfNode :many
-- The disks mounted inside a node's roots that the last complete scan did not find, with when
-- their root was last scanned (D208, D231 section 5).
SELECT root_mount.mount_path, root_state.last_scan_at
FROM root_mount
LEFT JOIN root_state ON root_state.node_id = root_mount.node_id AND root_state.root_path = root_mount.root_path
WHERE root_mount.node_id = sqlc.arg(node_id) AND root_mount.is_present = 0
ORDER BY root_mount.mount_path;

-- name: ListOfflineRoots :many
-- The roots of live libraries, on live nodes and not retired, that scans have found unavailable,
-- and since when: what the System page says is offline until an admin retires it or a scan finds
-- it (D41, D231 section 7).
SELECT library.id AS library_id, library.name AS library_name, node.name AS node_name, library_root.root_path,
  CAST(root_state.unavailable_since AS INTEGER) AS unavailable_since
FROM library_root
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
JOIN node ON node.id = library_root.node_id AND node.removed_at IS NULL
JOIN root_state ON root_state.node_id = library_root.node_id AND root_state.root_path = library_root.root_path
WHERE library_root.retired_at IS NULL AND root_state.unavailable_since IS NOT NULL
ORDER BY root_state.unavailable_since, library.id, library_root.root_path;

-- name: ListOfflineDisks :many
-- The disks mounted inside such roots that scans have found missing, and since when.
SELECT library.id AS library_id, library.name AS library_name, node.name AS node_name, root_mount.root_path,
  root_mount.mount_path, CAST(root_mount.missing_since AS INTEGER) AS missing_since
FROM root_mount
JOIN library_root ON library_root.node_id = root_mount.node_id AND library_root.root_path = root_mount.root_path
  AND library_root.retired_at IS NULL
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
JOIN node ON node.id = root_mount.node_id AND node.removed_at IS NULL
WHERE root_mount.is_present = 0 AND root_mount.missing_since IS NOT NULL
ORDER BY root_mount.missing_since, library.id, root_mount.mount_path;
