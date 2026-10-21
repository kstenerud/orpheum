-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The one work queue's storage (cut-a section 5.3; schema.md, cache.db): internal/store's
-- JobStorage, which internal/platform/jobqueue drives (D231 section 2).

-- name: InsertJob :one
INSERT INTO job (kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, scan_id, scan_batch_id, chunk_store_id, args_json, priority, runnable_at, reason,
  destination, created_at)
VALUES (sqlc.arg(kind), sqlc.arg(item_id), sqlc.arg(rendition_id), sqlc.arg(stream_id), sqlc.arg(fetched_subtitle_id),
        sqlc.arg(file_id), sqlc.arg(scan_id), sqlc.arg(scan_batch_id), sqlc.arg(chunk_store_id), sqlc.arg(args_json), sqlc.arg(priority), sqlc.arg(runnable_at), sqlc.arg(reason),
        sqlc.arg(destination), sqlc.arg(created_at))
RETURNING id;

-- name: ListEveryOpenJob :many
-- Every queued, running and assigned job: what a retire action looks through for the work it
-- cancels (D231 section 7).
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, scan_id, scan_batch_id, chunk_store_id, args_json, priority, state, destination, runnable_at,
       is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure, created_at, finished_at, assigned_until, wanted_until
FROM job
WHERE state IN ('queued', 'running', 'assigned')
ORDER BY id;

-- name: ListOpenJobsOfTarget :many
-- The queued and running jobs of this kind for this target, an assigned job counting as running: at
-- most one of each (D231 section 8).
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, scan_id, scan_batch_id, chunk_store_id, args_json, priority, state, destination, runnable_at,
       is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure, created_at, finished_at, assigned_until, wanted_until
FROM job
WHERE kind = sqlc.arg(kind) AND item_id IS sqlc.arg(item_id) AND rendition_id IS sqlc.arg(rendition_id) AND stream_id IS sqlc.arg(stream_id)
  AND fetched_subtitle_id IS sqlc.arg(fetched_subtitle_id) AND file_id IS sqlc.arg(file_id)
  AND scan_id IS sqlc.arg(scan_id) AND scan_batch_id IS sqlc.arg(scan_batch_id)
  AND chunk_store_id IS sqlc.arg(chunk_store_id)
  AND state IN ('queued', 'running', 'assigned')
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
RETURNING id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, scan_id, scan_batch_id, chunk_store_id, args_json, priority, state, destination,
          runnable_at, is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure,
          created_at, finished_at, assigned_until, wanted_until;

-- name: FinishJob :exec
-- A cancelled job stays cancelled, whatever its handler answers after. A claim that refuses a
-- job at its attempt limit gives back the attempt it counted.
UPDATE job SET state = sqlc.arg(state), progress_fraction = sqlc.arg(progress_fraction), last_failure = sqlc.arg(last_failure),
               attempt_count = sqlc.arg(attempt_count), finished_at = sqlc.arg(finished_at), assigned_until = NULL
WHERE id = sqlc.arg(id) AND state <> 'cancelled';

-- name: DeferJob :exec
-- Back to the queue until runnable_at: a snooze, a park, a retry's back-off (D170, D231). A wait
-- that is not the job's fault gives back the attempt StartJob counted.
UPDATE job SET state = 'queued', runnable_at = sqlc.arg(runnable_at), last_failure = sqlc.arg(last_failure),
               attempt_count = sqlc.arg(attempt_count), is_snoozed = sqlc.arg(is_snoozed),
               last_attempt_tool_failed = sqlc.arg(last_attempt_tool_failed), destination = sqlc.arg(destination),
               assigned_until = NULL
WHERE id = sqlc.arg(id) AND state <> 'cancelled';

-- name: AssignJob :exec
-- A running job whose work a node now does: no runner waits for it, until its outcome is taken or
-- assigned_until passes (D234 section 7).
UPDATE job SET state = 'assigned', assigned_until = sqlc.arg(assigned_until), attempt_count = sqlc.arg(attempt_count)
WHERE id = sqlc.arg(id) AND state = 'running';

-- name: TakeAssignedJob :one
-- An assigned job back to running, so its outcome can be written; only one of its outcome and its
-- time running out takes it.
UPDATE job SET state = 'running'
WHERE id = sqlc.arg(id) AND state = 'assigned'
RETURNING id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, scan_id, scan_batch_id, chunk_store_id, args_json, priority, state, destination,
          runnable_at, is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure,
          created_at, finished_at, assigned_until, wanted_until;

-- name: TakeOverdueAssignedJob :one
-- The assigned job of a kind whose time ran out first, back to running so its failure can be written.
UPDATE job SET state = 'running'
WHERE id = (SELECT overdue.id FROM job AS overdue
            WHERE overdue.state = 'assigned' AND overdue.kind = sqlc.arg(kind) AND overdue.assigned_until <= sqlc.arg(now)
            ORDER BY overdue.assigned_until, overdue.id LIMIT 1)
RETURNING id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, scan_id, scan_batch_id, chunk_store_id, args_json, priority, state, destination,
          runnable_at, is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure,
          created_at, finished_at, assigned_until, wanted_until;

-- name: CancelJob :execrows
-- A person's cancel (cut-a section 5.3): only a queued, running or assigned job can be.
UPDATE job SET state = 'cancelled', finished_at = sqlc.arg(finished_at), assigned_until = NULL
WHERE id = sqlc.arg(id) AND state IN ('queued', 'running', 'assigned');

-- name: SetJobProgress :exec
-- How far a running job is, 0 to 1, for the job view.
UPDATE job SET progress_fraction = sqlc.arg(progress_fraction)
WHERE id = sqlc.arg(id) AND state = 'running';

-- name: RequeueRunningJobs :execrows
-- After a stop, running jobs return to queued and restart; each keeps the attempt its claim
-- counted, so one that stops the server every time reaches its limit (S86). An assigned job stays
-- assigned: its node still has it.
UPDATE job SET state = 'queued'
WHERE state = 'running';

-- name: ListJobs :many
-- The job view, newest first.
SELECT id, kind, item_id, rendition_id, stream_id, fetched_subtitle_id, file_id, scan_id, scan_batch_id, chunk_store_id, args_json, priority, state, destination, runnable_at,
       is_snoozed, attempt_count, last_attempt_tool_failed, progress_fraction, reason, last_failure, created_at, finished_at, assigned_until, wanted_until
FROM job
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);

-- name: SetJobWanted :exec
-- When a person stops waiting for an open job of a kind on a target. Kept wanted (is_release 0),
-- until, unless a later play said later (D240 section 6). Released (is_release 1), as by one who left
-- the page that asked for it, until, a moment past, if anyone waited, so the next demotion gives it
-- background priority (D258 section 2); a player still watching asks again, and keeps it wanted.
UPDATE job SET wanted_until = CASE
    WHEN CAST(sqlc.arg(is_release) AS INTEGER) = 1 THEN
      CASE WHEN wanted_until IS NULL THEN NULL ELSE CAST(sqlc.arg(until) AS INTEGER) END
    ELSE MAX(COALESCE(wanted_until, 0), CAST(sqlc.arg(until) AS INTEGER))
  END
WHERE kind = sqlc.arg(kind) AND state IN ('queued', 'running', 'assigned')
  AND item_id IS sqlc.arg(item_id) AND rendition_id IS sqlc.arg(rendition_id) AND stream_id IS sqlc.arg(stream_id)
  AND fetched_subtitle_id IS sqlc.arg(fetched_subtitle_id) AND file_id IS sqlc.arg(file_id)
  AND scan_id IS sqlc.arg(scan_id) AND scan_batch_id IS sqlc.arg(scan_batch_id)
  AND chunk_store_id IS sqlc.arg(chunk_store_id);

-- name: DemoteUnwantedJobs :many
-- The open jobs no person waits for any more, fallen to background priority (D234 section 3).
UPDATE job SET priority = sqlc.arg(priority), wanted_until = NULL
WHERE wanted_until IS NOT NULL AND wanted_until < CAST(sqlc.arg(now) AS INTEGER) AND state IN ('queued', 'running', 'assigned')
RETURNING id, state;

-- name: DeleteFinishedJobs :execrows
-- The jobs that ended before a time, done, failed or cancelled: the job view keeps what ended
-- lately, and an item's own failure is a fact kept elsewhere (D231 section 4, D233 section 8).
DELETE FROM job
WHERE state IN ('done', 'failed', 'cancelled') AND finished_at < CAST(sqlc.arg(before) AS INTEGER);

-- name: CountJobsByKind :many
-- Each kind's queued jobs, when the oldest of them was queued (0 when none is), and how many failed
-- since a time: the queue's health, as the System page shows it (D233 section 8).
SELECT kind,
       CAST(SUM(state = 'queued') AS INTEGER) AS queued_count,
       CAST(COALESCE(MIN(CASE WHEN state = 'queued' THEN created_at END), 0) AS INTEGER) AS oldest_queued_at,
       CAST(SUM(state = 'failed' AND finished_at >= sqlc.arg(failed_since)) AS INTEGER) AS failed_count
FROM job
GROUP BY kind
ORDER BY kind;

-- name: ListOpenJobsOfItemsAndFiles :many
-- The queued, running and assigned jobs about any of some items or files: the work a person's
-- interest in them raises (D324 section 3).
SELECT id, priority, state FROM job
WHERE state IN ('queued', 'running', 'assigned')
  AND (item_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_ids_json) AS TEXT)))
    OR file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT))))
ORDER BY id;

-- name: RaiseJob :one
-- An open job raised to the priority a person's interest gives it, never lowered: kept wanted until
-- a time, after which the next demotion gives it background priority, when until is not zero, and
-- for good when it is (D324 section 2).
UPDATE job SET priority = MAX(priority, CAST(sqlc.arg(priority) AS INTEGER)),
  wanted_until = CASE WHEN CAST(sqlc.arg(until) AS INTEGER) = 0 THEN wanted_until
    ELSE MAX(COALESCE(wanted_until, 0), CAST(sqlc.arg(until) AS INTEGER)) END
WHERE id = sqlc.arg(id) AND state IN ('queued', 'running', 'assigned')
RETURNING id, state, priority;

-- name: ReleaseJobsOfItemsAndFiles :exec
-- The jobs a person's interest raised, below a play's priority, that no one is interested in any
-- more: their wanted time ends now, and the next demotion gives them background priority (D324
-- section 2).
UPDATE job SET wanted_until = CAST(sqlc.arg(now) AS INTEGER)
WHERE wanted_until IS NOT NULL AND priority < sqlc.arg(below) AND state IN ('queued', 'running', 'assigned')
  AND (item_id IN (SELECT value FROM json_each(CAST(sqlc.arg(item_ids_json) AS TEXT)))
    OR file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT))));
