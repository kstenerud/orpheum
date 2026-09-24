-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each root's last scan and what it found (D208, D41, D42).

-- name: GetRootState :one
SELECT device, mount_point, outcome, withheld_since, withheld_reason
FROM root_state
WHERE node_id = ? AND path = ?;

-- name: RecordRootComplete :exec
-- A complete scan: its facts become the known-good ones later scans are compared against.
INSERT INTO root_state (node_id, path, device, mount_point, outcome, outcome_at)
VALUES (sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(device), sqlc.arg(mount_point), 'complete', sqlc.arg(at))
ON CONFLICT (node_id, path) DO UPDATE SET
  device = excluded.device, mount_point = excluded.mount_point, outcome = 'complete',
  outcome_at = excluded.outcome_at, withheld_since = NULL, withheld_reason = NULL;

-- name: RecordRootNotComplete :exec
-- A scan that could not read its root, or whose removals wait for the admin: the known-good
-- facts stay as they were.
INSERT INTO root_state (node_id, path, outcome, outcome_at, withheld_since, withheld_reason)
VALUES (sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(outcome), sqlc.arg(at), sqlc.arg(withheld_since),
  sqlc.arg(withheld_reason))
ON CONFLICT (node_id, path) DO UPDATE SET
  outcome = excluded.outcome, outcome_at = excluded.outcome_at,
  withheld_since = excluded.withheld_since, withheld_reason = excluded.withheld_reason;

-- name: ListRootMounts :many
SELECT path, device, present
FROM root_mount
WHERE node_id = ? AND root = ?
ORDER BY path;

-- name: RecordRootMountPresent :exec
INSERT INTO root_mount (node_id, root, path, device, present)
VALUES (sqlc.arg(node_id), sqlc.arg(root), sqlc.arg(path), sqlc.arg(device), 1)
ON CONFLICT (node_id, path) DO UPDATE SET root = excluded.root, device = excluded.device, present = 1;

-- name: RecordRootMountMissing :exec
UPDATE root_mount SET present = 0 WHERE node_id = ? AND path = ?;
