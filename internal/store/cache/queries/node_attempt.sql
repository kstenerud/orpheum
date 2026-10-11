-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Work given to a node (schema.md, cache.db; D234 section 7): each attempt by its token, so the
-- node's word on it resolves its assigned job, after a restart too.

-- name: InsertNodeAttempt :exec
INSERT INTO node_attempt (attempt_token, job_id, job_kind, node_id, node_start_id, work_json, sent_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListNodeAttempts :many
SELECT attempt_token, job_id, job_kind, node_id, node_start_id, work_json, sent_at FROM node_attempt ORDER BY sent_at;

-- name: DeleteNodeAttempt :exec
DELETE FROM node_attempt WHERE attempt_token = ?;
