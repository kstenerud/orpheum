-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The failures of unknown cause, each kept with everything it said (D511 sections 2 to 6).

-- name: RecordUnknownFailure :exec
-- A failure of unknown cause, or one that failed again: its words and the start it failed on are
-- the newest, and its count is raised (D511 section 4). The count changes the row at every write, so
-- no write here changes nothing (D303).
-- @sqlc-vet-disable upsert-only-if-changed
INSERT INTO unknown_failure (kind, subject, error_text, output, version, first_failed_at, last_failed_at,
  failure_count, start_id)
VALUES (sqlc.arg(kind), sqlc.arg(subject), sqlc.arg(error_text), sqlc.arg(output), sqlc.arg(version),
  sqlc.arg(failed_at), sqlc.arg(failed_at), 1, sqlc.arg(start_id))
ON CONFLICT (kind, subject) DO UPDATE SET
  error_text = excluded.error_text, output = excluded.output, version = excluded.version,
  last_failed_at = max(unknown_failure.last_failed_at, excluded.last_failed_at),
  first_failed_at = min(unknown_failure.first_failed_at, excluded.first_failed_at),
  failure_count = unknown_failure.failure_count + 1, start_id = excluded.start_id;

-- name: ListUnknownFailures :many
-- Every one, a kind's together, its newest first: the Needs attention page's and the export's.
SELECT kind, subject, error_text, output, version, first_failed_at, last_failed_at, failure_count, start_id
FROM unknown_failure
ORDER BY kind, last_failed_at DESC, subject;

-- name: ListUnknownFailuresOfStartsBefore :many
-- Those an earlier start than this one failed on, which this start tries once more (D511 section 4).
SELECT kind, subject
FROM unknown_failure
WHERE start_id <> sqlc.arg(start_id)
ORDER BY kind, subject;

-- name: DeleteUnknownFailure :exec
-- One whose work has since succeeded.
DELETE FROM unknown_failure WHERE kind = sqlc.arg(kind) AND subject = sqlc.arg(subject);

-- name: DeleteUnknownFailuresOfSubjects :execrows
-- Those of every kind whose subject is one named, a JSON array of subjects: a rescan forgets those of
-- its films, series, episodes and files, so their work is tried again (D535 section 8).
DELETE FROM unknown_failure
WHERE subject IN (SELECT value FROM json_each(CAST(sqlc.arg(subjects_json) AS TEXT)));
