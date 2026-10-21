-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Each root's last scan and what it found (D208, D41, D42).

-- name: GetRootState :one
SELECT filesystem_device, is_mount_point, last_scan_outcome, withheld_scan_started_at, withheld_reason, are_removals_approved,
  unavailable_since
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
  unavailable_since = NULL
-- Only when it changes something (D303, DL9).
WHERE (root_state.filesystem_device, root_state.is_mount_point, root_state.last_scan_outcome,
  root_state.last_scan_at, root_state.withheld_scan_started_at, root_state.withheld_reason,
  root_state.are_removals_approved, root_state.unavailable_since)
  IS NOT (excluded.filesystem_device, excluded.is_mount_point, 'complete', excluded.last_scan_at, NULL, NULL,
    0, NULL);

-- name: RecordRootNotComplete :exec
-- A scan that could not read its root, or whose removals wait for the admin: the known-good
-- facts stay as they were. A root the scan could not read, or whose held-back scan saw none of it,
-- is unavailable since the first such scan in a row (D41, D231 section 7); one whose scan read it
-- and only holds back removals is available (D277 section 1).
INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at, withheld_reason,
  unavailable_since)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(last_scan_outcome), sqlc.arg(last_scan_at), sqlc.arg(withheld_scan_started_at),
  sqlc.arg(withheld_reason), sqlc.narg(unavailable_since))
ON CONFLICT (node_id, root_path) DO UPDATE SET
  last_scan_outcome = excluded.last_scan_outcome, last_scan_at = excluded.last_scan_at,
  withheld_scan_started_at = excluded.withheld_scan_started_at, withheld_reason = excluded.withheld_reason,
  unavailable_since = CASE WHEN excluded.unavailable_since IS NULL THEN NULL
    ELSE coalesce(root_state.unavailable_since, excluded.unavailable_since) END
-- Only when it changes something (D303, DL9).
WHERE (root_state.last_scan_outcome, root_state.last_scan_at, root_state.withheld_scan_started_at,
  root_state.withheld_reason, root_state.unavailable_since)
  IS NOT (excluded.last_scan_outcome, excluded.last_scan_at, excluded.withheld_scan_started_at,
    excluded.withheld_reason,
    CASE WHEN excluded.unavailable_since IS NULL THEN NULL ELSE coalesce(root_state.unavailable_since, excluded.unavailable_since) END);

-- name: MarkRootUnavailable :one
-- Work stalled waiting on the root's disk: the root is unavailable from now, unless it already
-- was, until a scan reads it again (D233 section 10). A root no scan has read yet has no state, and
-- returns nothing.
UPDATE root_state SET unavailable_since = coalesce(unavailable_since, sqlc.arg(now))
WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path)
RETURNING CAST(unavailable_since AS INTEGER) AS unavailable_since;

-- name: BringBackRoot :execrows
-- The root's disk answers again with the facts of its last complete scan, its device and whether it
-- is a mount point: the root is available again before a scan reads it, the scan following for what
-- changed meanwhile. Other facts are the empty directory an unmounted disk leaves, which stays
-- unavailable until a scan reads it (D208, D233 section 10).
UPDATE root_state SET unavailable_since = NULL
WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path) AND unavailable_since IS NOT NULL
  AND filesystem_device = sqlc.arg(filesystem_device) AND is_mount_point = sqlc.arg(is_mount_point);

-- name: ListRootMounts :many
SELECT mount_path, filesystem_device, is_present
FROM root_mount
WHERE node_id = ? AND root_path = ?
ORDER BY mount_path;

-- name: RecordRootMountPresent :exec
INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(mount_path), sqlc.arg(filesystem_device), 1)
ON CONFLICT (node_id, mount_path) DO UPDATE SET
  root_path = excluded.root_path, filesystem_device = excluded.filesystem_device, is_present = 1, missing_since = NULL
-- Only when it changes something (D303, DL9).
WHERE (root_mount.root_path, root_mount.filesystem_device, root_mount.is_present, root_mount.missing_since)
  IS NOT (excluded.root_path, excluded.filesystem_device, 1, NULL);

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
-- A node's roots whose last scan could not read them, or held back its removals having seen none
-- of the root, or whose disk work stalled on since: their files are not there to read, since the
-- first of those in a row (D208, D231 section 5, D233 section 10, D277 section 1).
SELECT root_path, CAST(coalesce(unavailable_since, last_scan_at) AS INTEGER) AS unavailable_since
FROM root_state
WHERE node_id = sqlc.arg(node_id) AND (last_scan_outcome = 'unreadable' OR unavailable_since IS NOT NULL)
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

-- name: ListWithheldRoots :many
-- The roots of live libraries, on live nodes and not retired, whose last scan's removals wait for
-- the admin, with why and since when: what the System page says waits (D208 section 3, D277
-- section 2).
SELECT library.id AS library_id, library.name AS library_name, node.name AS node_name, library_root.node_id,
  library_root.root_path, root_state.withheld_reason,
  CAST(root_state.withheld_scan_started_at AS INTEGER) AS withheld_scan_started_at
FROM library_root
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
JOIN node ON node.id = library_root.node_id AND node.removed_at IS NULL
JOIN root_state ON root_state.node_id = library_root.node_id AND root_state.root_path = library_root.root_path
WHERE library_root.retired_at IS NULL AND root_state.last_scan_outcome = 'withheld'
ORDER BY root_state.withheld_scan_started_at, library.id, library_root.root_path;
