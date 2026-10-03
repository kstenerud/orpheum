-- SPDX-License-Identifier: GPL-3.0-or-later
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

-- name: UpdateNodeStatus :exec
UPDATE node SET last_status_json = sqlc.arg(last_status_json), last_status_at = sqlc.arg(last_status_at) WHERE id = sqlc.arg(id);

-- name: GetNodeStatus :one
SELECT id, name, last_status_json, last_status_at
FROM node
WHERE id = ?;

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
ON CONFLICT (node_id, device_key) DO UPDATE SET reader_limit = excluded.reader_limit, set_at = excluded.set_at;

-- name: ClearDeviceReaderLimit :exec
-- A device's limit given back to its kind's setting.
DELETE FROM device_reader_limit WHERE node_id = sqlc.arg(node_id) AND device_key = sqlc.arg(device_key);

-- name: ListDeviceReaderLimits :many
-- The reader limits an admin set for a node's devices.
SELECT device_key, reader_limit FROM device_reader_limit WHERE node_id = sqlc.arg(node_id) ORDER BY device_key;
