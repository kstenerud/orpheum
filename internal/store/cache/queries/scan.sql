-- SPDX-License-Identifier: GPL-3.0-or-later
-- A root's scan in progress, and the batches of files it needs read (D240; schema.md, cache.db).

-- name: InsertScan :one
-- A scan of a root, unless one is in progress: then nothing, and no id.
INSERT INTO scan (node_id, root_path, priority, started_at)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(priority), sqlc.arg(started_at))
ON CONFLICT (node_id, root_path) DO NOTHING
RETURNING id;

-- name: SetScanAgain :execrows
-- A scan asked for while one of the root runs: one more follows it (D236 section 3).
UPDATE scan SET is_another_scan_queued = 1 WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: GetScan :one
SELECT id, node_id, root_path, priority, started_at, listed_at, listed_count, to_examine_count, examined_count,
       is_another_scan_queued
FROM scan WHERE id = sqlc.arg(id);

-- name: ListScans :many
-- Every scan in progress, oldest first.
SELECT id, node_id, root_path, priority, started_at, listed_at, listed_count, to_examine_count, examined_count,
       is_another_scan_queued
FROM scan ORDER BY id;

-- name: SetScanListed :exec
-- A scan's listing taken: how many files it found, and how many of them it needs read.
UPDATE scan SET listed_at = sqlc.arg(listed_at), listed_count = sqlc.arg(listed_count),
                to_examine_count = sqlc.arg(to_examine_count)
WHERE id = sqlc.arg(id);

-- name: DeleteScan :exec
-- A scan ended, with its batches.
DELETE FROM scan WHERE id = sqlc.arg(id);

-- name: InsertScanBatch :one
INSERT INTO scan_batch (scan_id, files_json, file_count)
VALUES (sqlc.arg(scan_id), sqlc.arg(files_json), sqlc.arg(file_count))
RETURNING id;

-- name: GetScanBatch :one
SELECT id, scan_id, files_json, file_count, is_done FROM scan_batch WHERE id = sqlc.arg(id);

-- name: SetScanBatchDone :execrows
-- A batch's files read, or given up on: counted once.
UPDATE scan_batch SET is_done = 1 WHERE id = sqlc.arg(id) AND is_done = 0;

-- name: AddScanExamined :exec
UPDATE scan SET examined_count = examined_count + sqlc.arg(file_count) WHERE id = sqlc.arg(id);

-- name: CountUndoneScanBatches :one
SELECT COUNT(*) FROM scan_batch WHERE scan_id = sqlc.arg(scan_id) AND is_done = 0;
