-- SPDX-License-Identifier: GPL-3.0-or-later
-- Media nodes (schema.md, node; D207): enrolled by name on their first hello.

-- name: GetLiveNodeByName :one
SELECT id, name, enrolled_at, report_seq
FROM node
WHERE name = ? AND removed_at IS NULL
ORDER BY id
LIMIT 1;

-- name: InsertNode :one
INSERT INTO node (name, enrolled_at)
VALUES (sqlc.arg(name), sqlc.arg(enrolled_at))
RETURNING id;

-- name: UpdateNodeReportSeq :exec
UPDATE node SET report_seq = sqlc.arg(report_seq) WHERE id = sqlc.arg(id);

-- name: UpdateNodeStatus :exec
UPDATE node SET status = sqlc.arg(status), status_at = sqlc.arg(status_at) WHERE id = sqlc.arg(id);

-- name: GetNodeStatus :one
SELECT id, name, status, status_at
FROM node
WHERE id = ?;

-- name: ListLiveNodes :many
-- Every enrolled node, for an admin choosing where a library's roots are.
SELECT id, name, enrolled_at
FROM node
WHERE removed_at IS NULL
ORDER BY name;
