-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Media nodes (schema.md, node; D207): enrolled by name on their first hello.

-- name: GetLiveNodeByName :one
SELECT id, name, enrolled_at
FROM node
WHERE name = ? AND removed_at IS NULL
ORDER BY id
LIMIT 1;

-- name: InsertNode :one
INSERT INTO node (name, enrolled_at)
VALUES (sqlc.arg(name), sqlc.arg(enrolled_at))
RETURNING id;

-- name: ListLiveNodes :many
-- Every enrolled node, for an admin choosing where a library's roots are.
SELECT id, name, enrolled_at
FROM node
WHERE removed_at IS NULL
ORDER BY name;

-- name: SetDeviceReaderLimit :exec
-- How many may read one of a node's devices at once, as an admin set it (D240 section 5).
INSERT INTO device_reader_limit (node_id, device_key, reader_limit, set_at)
VALUES (sqlc.arg(node_id), sqlc.arg(device_key), sqlc.arg(reader_limit), sqlc.arg(set_at))
ON CONFLICT (node_id, device_key) DO UPDATE SET reader_limit = excluded.reader_limit, set_at = excluded.set_at
-- Only when it changes something (D303, DL9).
WHERE (device_reader_limit.reader_limit, device_reader_limit.set_at)
  IS NOT (excluded.reader_limit, excluded.set_at);

-- name: ClearDeviceReaderLimit :exec
-- A device's limit given back to its kind's setting.
DELETE FROM device_reader_limit WHERE node_id = sqlc.arg(node_id) AND device_key = sqlc.arg(device_key);

-- name: ListDeviceReaderLimits :many
-- The reader limits an admin set for a node's devices.
SELECT device_key, reader_limit FROM device_reader_limit WHERE node_id = sqlc.arg(node_id) ORDER BY device_key;

-- name: AcceptUnconfinedNode :execrows
-- An admin lets a node run FFmpeg without a confinement mechanism its kernel lacks (D450 section 1);
-- one already accepted keeps who accepted it first, and when.
INSERT INTO node_unconfined_leave (node_id, accepted_by_user_id, accepted_at)
VALUES (sqlc.arg(node_id), sqlc.arg(accepted_by_user_id), sqlc.arg(accepted_at))
ON CONFLICT (node_id) DO NOTHING;

-- name: GetUnconfinedLeave :one
-- Who let a node run unconfined, and when, for its polls and the System page (D450 section 1).
SELECT node_unconfined_leave.accepted_at, user.name AS accepted_by
FROM node_unconfined_leave
JOIN user ON user.id = node_unconfined_leave.accepted_by_user_id
WHERE node_unconfined_leave.node_id = sqlc.arg(node_id);
