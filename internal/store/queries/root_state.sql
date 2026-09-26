-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each root's last scan and what it found (D208, D41, D42).

-- name: GetRootState :one
SELECT filesystem_device, is_mount_point, outcome, withheld_scan_started_at, withheld_reason, removals_approved
FROM root_state
WHERE node_id = ? AND path = ?;

-- name: RecordRootComplete :exec
-- A complete scan: its facts become the known-good ones later scans are compared against.
INSERT INTO root_state (node_id, path, filesystem_device, is_mount_point, outcome, outcome_at)
VALUES (sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(filesystem_device), sqlc.arg(is_mount_point), 'complete', sqlc.arg(outcome_at))
ON CONFLICT (node_id, path) DO UPDATE SET
  filesystem_device = excluded.filesystem_device, is_mount_point = excluded.is_mount_point, outcome = 'complete',
  outcome_at = excluded.outcome_at, withheld_scan_started_at = NULL, withheld_reason = NULL, removals_approved = 0;

-- name: RecordRootNotComplete :exec
-- A scan that could not read its root, or whose removals wait for the admin: the known-good
-- facts stay as they were.
INSERT INTO root_state (node_id, path, outcome, outcome_at, withheld_scan_started_at, withheld_reason)
VALUES (sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(outcome), sqlc.arg(outcome_at), sqlc.arg(withheld_scan_started_at),
  sqlc.arg(withheld_reason))
ON CONFLICT (node_id, path) DO UPDATE SET
  outcome = excluded.outcome, outcome_at = excluded.outcome_at,
  withheld_scan_started_at = excluded.withheld_scan_started_at, withheld_reason = excluded.withheld_reason;

-- name: ListRootMounts :many
SELECT path, filesystem_device, is_present
FROM root_mount
WHERE node_id = ? AND root_path = ?
ORDER BY path;

-- name: RecordRootMountPresent :exec
INSERT INTO root_mount (node_id, root_path, path, filesystem_device, is_present)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(path), sqlc.arg(filesystem_device), 1)
ON CONFLICT (node_id, path) DO UPDATE SET
  root_path = excluded.root_path, filesystem_device = excluded.filesystem_device, is_present = 1;

-- name: RecordRootMountMissing :exec
UPDATE root_mount SET is_present = 0 WHERE node_id = ? AND path = ?;

-- name: ApproveRootRemovals :execrows
-- An admin's approval: the root's next complete scan removes what it does not see without
-- waiting (D208 section 3). A complete scan clears it.
UPDATE root_state SET removals_approved = 1
WHERE node_id = sqlc.arg(node_id) AND path = sqlc.arg(path);

-- name: ForgetMissingRootMount :execrows
-- An admin says a disk missing from a mount point inside a root is gone for good: the boundary
-- is forgotten, so a scan reads what is under it as removed. Only a missing one is forgotten; a
-- boundary is scan state, not a person's record (standards/sql.md section 6).
DELETE FROM root_mount
WHERE node_id = sqlc.arg(node_id) AND path = sqlc.arg(path) AND is_present = 0;

-- name: GetRootMount :one
SELECT root_path, is_present
FROM root_mount
WHERE node_id = sqlc.arg(node_id) AND path = sqlc.arg(path);
