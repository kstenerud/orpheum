-- SPDX-License-Identifier: GPL-3.0-or-later
-- The one work queue (cut-a section 5.3; schema.md, cache.db).

-- name: InsertJob :one
INSERT INTO job (kind, subject, args, priority, not_before, reason, created_at)
VALUES (sqlc.arg(kind), sqlc.arg(subject), sqlc.arg(args), sqlc.arg(priority), sqlc.arg(not_before),
        sqlc.arg(reason), sqlc.arg(created_at))
RETURNING id;

-- name: FindOpenJob :one
-- A queued or running job of this kind for this subject, so a subject is queued once.
SELECT id, state
FROM job
WHERE kind = sqlc.arg(kind) AND subject IS sqlc.arg(subject) AND state IN ('queued', 'running')
ORDER BY id
LIMIT 1;

-- name: NextQueuedJob :one
-- The highest-priority queued job of a kind whose time has come; the oldest first among equals.
SELECT id, kind, subject, args, priority, not_before, attempts, reason
FROM job
WHERE state = 'queued' AND kind = sqlc.arg(kind) AND not_before <= sqlc.arg(now)
ORDER BY priority DESC, not_before, id
LIMIT 1;

-- name: EarliestQueuedJob :one
-- When the next queued job of a kind becomes due, so a runner can sleep until then.
SELECT CAST(min(not_before) AS INTEGER) AS due
FROM job
WHERE state = 'queued' AND kind = sqlc.arg(kind);

-- name: StartJob :execrows
UPDATE job SET state = 'running', attempts = attempts + 1
WHERE id = sqlc.arg(id) AND state = 'queued';

-- name: FinishJob :exec
UPDATE job SET state = sqlc.arg(state), progress = sqlc.arg(progress), last_error = sqlc.arg(last_error),
               finished_at = sqlc.arg(finished_at)
WHERE id = sqlc.arg(id);

-- name: DeferJob :exec
-- Back to the queue until not_before: a rate limit, or a retry's back-off (D170).
UPDATE job SET state = 'queued', not_before = sqlc.arg(not_before), last_error = sqlc.arg(last_error)
WHERE id = sqlc.arg(id);

-- name: RequeueRunningJobs :execrows
-- After a crash, running jobs return to queued; their output is a cache, so they restart.
UPDATE job SET state = 'queued'
WHERE state = 'running';

-- name: GetJob :one
SELECT id, kind, subject, args, priority, state, node_id, not_before, attempts, progress, reason,
       last_error, created_at, finished_at
FROM job
WHERE id = sqlc.arg(id);

-- name: ListJobs :many
-- The job view, newest first.
SELECT id, kind, subject, args, priority, state, node_id, not_before, attempts, progress, reason,
       last_error, created_at, finished_at
FROM job
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);
