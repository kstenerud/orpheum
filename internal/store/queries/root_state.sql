-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each root's last scan and what it found (D208, D41, D42).

-- name: GetRootState :one
SELECT filesystem_device, is_mount_point, last_scan_outcome, withheld_scan_started_at, withheld_reason, are_removals_approved
FROM root_state
WHERE node_id = ? AND root_path = ?;

-- name: RecordRootComplete :exec
-- A complete scan: its facts become the known-good ones later scans are compared against.
INSERT INTO root_state (node_id, root_path, filesystem_device, is_mount_point, last_scan_outcome, last_scan_at)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(filesystem_device), sqlc.arg(is_mount_point), 'complete', sqlc.arg(last_scan_at))
ON CONFLICT (node_id, root_path) DO UPDATE SET
  filesystem_device = excluded.filesystem_device, is_mount_point = excluded.is_mount_point, last_scan_outcome = 'complete',
  last_scan_at = excluded.last_scan_at, withheld_scan_started_at = NULL, withheld_reason = NULL, are_removals_approved = 0;

-- name: RecordRootNotComplete :exec
-- A scan that could not read its root, or whose removals wait for the admin: the known-good
-- facts stay as they were.
INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at, withheld_reason)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(last_scan_outcome), sqlc.arg(last_scan_at), sqlc.arg(withheld_scan_started_at),
  sqlc.arg(withheld_reason))
ON CONFLICT (node_id, root_path) DO UPDATE SET
  last_scan_outcome = excluded.last_scan_outcome, last_scan_at = excluded.last_scan_at,
  withheld_scan_started_at = excluded.withheld_scan_started_at, withheld_reason = excluded.withheld_reason;

-- name: ListRootMounts :many
SELECT mount_path, filesystem_device, is_present
FROM root_mount
WHERE node_id = ? AND root_path = ?
ORDER BY mount_path;

-- name: RecordRootMountPresent :exec
INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(mount_path), sqlc.arg(filesystem_device), 1)
ON CONFLICT (node_id, mount_path) DO UPDATE SET
  root_path = excluded.root_path, filesystem_device = excluded.filesystem_device, is_present = 1;

-- name: RecordRootMountMissing :exec
UPDATE root_mount SET is_present = 0 WHERE node_id = ? AND mount_path = ?;

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
