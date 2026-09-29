-- SPDX-License-Identifier: GPL-3.0-or-later
-- The one work queue (cut-a section 5.3; schema.md, cache.db).

-- name: InsertJob :one
INSERT INTO job (kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, runnable_at, reason,
  created_at)
VALUES (sqlc.arg(kind), sqlc.arg(item_id), sqlc.arg(rendition_id), sqlc.arg(stream_id), sqlc.arg(fetched_subtitle_id),
        sqlc.arg(file_id), sqlc.arg(args_json), sqlc.arg(priority), sqlc.arg(runnable_at), sqlc.arg(reason),
        sqlc.arg(created_at))
RETURNING id;

-- name: HasJob :one
-- Whether any job of this kind was ever queued for this target: a first fetch happens once
-- (D170), and a later one is a person's request.
SELECT EXISTS (
  SELECT 1 FROM job WHERE kind = sqlc.arg(kind) AND item_id IS sqlc.arg(item_id) AND rendition_id IS sqlc.arg(rendition_id) AND stream_id IS sqlc.arg(stream_id)
  AND fetched_subtitle_id IS sqlc.arg(fetched_subtitle_id) AND file_id IS sqlc.arg(file_id)
) AS found;

-- name: FindOpenJob :one
-- A queued or running job of this kind for this target, so a target is queued once.
SELECT id, state
FROM job
WHERE kind = sqlc.arg(kind) AND item_id IS sqlc.arg(item_id) AND rendition_id IS sqlc.arg(rendition_id) AND stream_id IS sqlc.arg(stream_id)
  AND fetched_subtitle_id IS sqlc.arg(fetched_subtitle_id) AND file_id IS sqlc.arg(file_id)
  AND state IN ('queued', 'running')
ORDER BY id
LIMIT 1;

-- name: NextQueuedJob :one
-- The highest-priority queued job of a kind whose time has come; the oldest first among equals.
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, runnable_at, attempts,
  reason
FROM job
WHERE state = 'queued' AND kind = sqlc.arg(kind) AND runnable_at <= sqlc.arg(now)
ORDER BY priority DESC, runnable_at, id
LIMIT 1;

-- name: StartJob :execrows
UPDATE job SET state = 'running', attempts = attempts + 1
WHERE id = sqlc.arg(id) AND state = 'queued';

-- name: FinishJob :exec
-- A cancelled job stays cancelled, whatever its handler answers after.
UPDATE job SET state = sqlc.arg(state), progress_fraction = sqlc.arg(progress_fraction), last_error = sqlc.arg(last_error),
               finished_at = sqlc.arg(finished_at)
WHERE id = sqlc.arg(id) AND state <> 'cancelled';

-- name: DeferJob :exec
-- Back to the queue until runnable_at: a rate limit, or a retry's back-off (D170). A wait that is
-- not the job's fault gives back the attempt StartJob counted.
UPDATE job SET state = 'queued', runnable_at = sqlc.arg(runnable_at), last_error = sqlc.arg(last_error),
               attempts = sqlc.arg(attempts)
WHERE id = sqlc.arg(id) AND state <> 'cancelled';

-- name: CancelJob :execrows
-- A person's cancel (cut-a section 5.3): only a queued or running job can be.
UPDATE job SET state = 'cancelled', finished_at = sqlc.arg(finished_at)
WHERE id = sqlc.arg(id) AND state IN ('queued', 'running');

-- name: SetJobProgress :exec
-- How far a running job is, 0 to 1, for the job view.
UPDATE job SET progress_fraction = sqlc.arg(progress_fraction)
WHERE id = sqlc.arg(id) AND state = 'running';

-- name: RequeueRunningJobs :execrows
-- After a crash, running jobs return to queued; their output is a cache, so they restart.
UPDATE job SET state = 'queued'
WHERE state = 'running';

-- name: GetJob :one
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, state, node_id, runnable_at,
       attempts, progress_fraction, reason, last_error, created_at, finished_at
FROM job
WHERE id = sqlc.arg(id);

-- name: ListJobs :many
-- The job view, newest first.
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, state, node_id, runnable_at,
       attempts, progress_fraction, reason, last_error, created_at, finished_at
FROM job
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);
