-- SPDX-License-Identifier: GPL-3.0-or-later
-- The one work queue's storage (cut-a section 5.3; schema.md, cache.db): internal/store's
-- JobStorage, which internal/platform/jobqueue drives (D231 section 2).

-- name: InsertJob :one
INSERT INTO job (kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, runnable_at, reason,
  destination, created_at)
VALUES (sqlc.arg(kind), sqlc.arg(item_id), sqlc.arg(rendition_id), sqlc.arg(stream_id), sqlc.arg(fetched_subtitle_id),
        sqlc.arg(file_id), sqlc.arg(args_json), sqlc.arg(priority), sqlc.arg(runnable_at), sqlc.arg(reason),
        sqlc.arg(destination), sqlc.arg(created_at))
RETURNING id;

-- name: ListEveryOpenJob :many
-- Every queued and running job: what a retire action looks through for the work it cancels
-- (D231 section 7).
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, state, destination, runnable_at,
       is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure, created_at, finished_at
FROM job
WHERE state IN ('queued', 'running')
ORDER BY id;

-- name: ListOpenJobs :many
-- The queued and running jobs of this kind for this target: at most one of each (D231 section 8).
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, state, destination, runnable_at,
       is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure, created_at, finished_at
FROM job
WHERE kind = sqlc.arg(kind) AND item_id IS sqlc.arg(item_id) AND rendition_id IS sqlc.arg(rendition_id) AND stream_id IS sqlc.arg(stream_id)
  AND fetched_subtitle_id IS sqlc.arg(fetched_subtitle_id) AND file_id IS sqlc.arg(file_id)
  AND state IN ('queued', 'running')
ORDER BY id;

-- name: MergeQueuedJob :execrows
-- A repeat request folded into the queued job for its target (D231 section 8).
UPDATE job SET args_json = sqlc.arg(args_json), priority = sqlc.arg(priority), reason = sqlc.arg(reason),
               runnable_at = sqlc.arg(runnable_at), destination = sqlc.arg(destination)
WHERE id = sqlc.arg(id) AND state = 'queued';

-- name: NextQueuedJob :one
-- The highest-priority queued job of a kind whose time has come and whose destination is not
-- parked (D231 section 6; unclaimable_json is a JSON array of the parked destinations' names); the
-- earliest, then the oldest, among equals.
SELECT id
FROM job
WHERE state = 'queued' AND kind = sqlc.arg(kind) AND runnable_at <= sqlc.arg(now)
  AND (destination IS NULL
       OR destination NOT IN (SELECT value FROM json_each(CAST(sqlc.arg(unclaimable_json) AS TEXT))))
ORDER BY priority DESC, runnable_at, id
LIMIT 1;

-- name: StartJob :one
-- A claim: the job runs, and its attempt is counted.
UPDATE job SET state = 'running', attempt_count = attempt_count + 1
WHERE id = sqlc.arg(id) AND state = 'queued'
RETURNING id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, state, destination,
          runnable_at, is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure,
          created_at, finished_at;

-- name: FinishJob :exec
-- A cancelled job stays cancelled, whatever its handler answers after. A claim that refuses a
-- job at its attempt limit gives back the attempt it counted.
UPDATE job SET state = sqlc.arg(state), progress_fraction = sqlc.arg(progress_fraction), last_failure = sqlc.arg(last_failure),
               attempt_count = sqlc.arg(attempt_count), finished_at = sqlc.arg(finished_at)
WHERE id = sqlc.arg(id) AND state <> 'cancelled';

-- name: DeferJob :exec
-- Back to the queue until runnable_at: a snooze, a park, a retry's back-off (D170, D231). A wait
-- that is not the job's fault gives back the attempt StartJob counted.
UPDATE job SET state = 'queued', runnable_at = sqlc.arg(runnable_at), last_failure = sqlc.arg(last_failure),
               attempt_count = sqlc.arg(attempt_count), is_snoozed = sqlc.arg(is_snoozed),
               last_attempt_tool_failed = sqlc.arg(last_attempt_tool_failed), destination = sqlc.arg(destination)
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
-- After a stop, running jobs return to queued and restart; each keeps the attempt its claim
-- counted, so one that stops the server every time reaches its limit (S86).
UPDATE job SET state = 'queued'
WHERE state = 'running';

-- name: ListJobs :many
-- The job view, newest first.
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, args_json, priority, state, destination, runnable_at,
       is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure, created_at, finished_at
FROM job
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);
