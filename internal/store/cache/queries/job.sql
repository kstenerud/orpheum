-- SPDX-License-Identifier: GPL-3.0-or-later
-- The one work queue (cut-a section 5.3; schema.md, cache.db).

-- name: InsertJob :one
INSERT INTO job (kind, subject, args, priority, runnable_at, reason, created_at)
VALUES (sqlc.arg(kind), sqlc.arg(subject), sqlc.arg(args), sqlc.arg(priority), sqlc.arg(runnable_at),
        sqlc.arg(reason), sqlc.arg(created_at))
RETURNING id;

-- name: HasJob :one
-- Whether any job of this kind was ever queued for this subject: a first fetch happens once
-- (D170), and a later one is a person's request.
SELECT EXISTS (
  SELECT 1 FROM job WHERE kind = sqlc.arg(kind) AND subject IS sqlc.arg(subject)
) AS found;

-- name: FindOpenJob :one
-- A queued or running job of this kind for this subject, so a subject is queued once.
SELECT id, state
FROM job
WHERE kind = sqlc.arg(kind) AND subject IS sqlc.arg(subject) AND state IN ('queued', 'running')
ORDER BY id
LIMIT 1;

-- name: NextQueuedJob :one
-- The highest-priority queued job of a kind whose time has come; the oldest first among equals.
SELECT id, kind, subject, args, priority, runnable_at, attempts, reason
FROM job
WHERE state = 'queued' AND kind = sqlc.arg(kind) AND runnable_at <= sqlc.arg(now)
ORDER BY priority DESC, runnable_at, id
LIMIT 1;

-- name: StartJob :execrows
UPDATE job SET state = 'running', attempts = attempts + 1
WHERE id = sqlc.arg(id) AND state = 'queued';

-- name: FinishJob :exec
UPDATE job SET state = sqlc.arg(state), progress = sqlc.arg(progress), last_error = sqlc.arg(last_error),
               finished_at = sqlc.arg(finished_at)
WHERE id = sqlc.arg(id);

-- name: DeferJob :exec
-- Back to the queue until runnable_at: a rate limit, or a retry's back-off (D170). A wait that is
-- not the job's fault gives back the attempt StartJob counted.
UPDATE job SET state = 'queued', runnable_at = sqlc.arg(runnable_at), last_error = sqlc.arg(last_error),
               attempts = sqlc.arg(attempts)
WHERE id = sqlc.arg(id);

-- name: RequeueRunningJobs :execrows
-- After a crash, running jobs return to queued; their output is a cache, so they restart.
UPDATE job SET state = 'queued'
WHERE state = 'running';

-- name: GetJob :one
SELECT id, kind, subject, args, priority, state, node_id, runnable_at, attempts, progress, reason,
       last_error, created_at, finished_at
FROM job
WHERE id = sqlc.arg(id);

-- name: ListJobs :many
-- The job view, newest first.
SELECT id, kind, subject, args, priority, state, node_id, runnable_at, attempts, progress, reason,
       last_error, created_at, finished_at
FROM job
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);
