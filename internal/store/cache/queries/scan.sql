-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A root's scan in progress, and the batches of files it needs read (D240; schema.md, cache.db).

-- name: InsertScan :one
-- A scan of a root, unless one is in progress: then nothing, and no id.
INSERT INTO scan (node_id, root_path, priority, started_at, scope_path)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(priority), sqlc.arg(started_at), sqlc.arg(scope_path))
ON CONFLICT (node_id, root_path) DO NOTHING
RETURNING id;

-- name: SetScanAgain :execrows
-- A scan asked for while one of the root runs: one more follows it (D236 section 3), of the scope
-- that covers every request made meanwhile.
UPDATE scan SET is_another_scan_queued = 1, again_scope_path = sqlc.arg(again_scope_path)
WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: JoinScan :exec
-- A scan asked for of a root whose scan's listing is not yet with its node joins it: its scope
-- widened to cover both, and its priority the higher of the two.
UPDATE scan SET scope_path = sqlc.arg(scope_path), priority = sqlc.arg(priority)
WHERE id = sqlc.arg(id);

-- name: SendScanListing :one
-- A scan's listing handed to its node, unless it was taken: what it lists is read in the same write,
-- so no scan asked for joins it after its scope went to the node.
UPDATE scan SET listing_sent_at = sqlc.arg(listing_sent_at)
WHERE id = sqlc.arg(id) AND listed_at IS NULL
RETURNING id, node_id, root_path, priority, started_at, listed_at, listed_count, to_examine_count, examined_count,
          is_another_scan_queued, scope_path, again_scope_path, listing_sent_at;

-- name: GetScanOfRoot :one
SELECT id, node_id, root_path, priority, started_at, listed_at, listed_count, to_examine_count, examined_count,
       is_another_scan_queued, scope_path, again_scope_path, listing_sent_at
FROM scan WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: GetScan :one
SELECT id, node_id, root_path, priority, started_at, listed_at, listed_count, to_examine_count, examined_count,
       is_another_scan_queued, scope_path, again_scope_path, listing_sent_at
FROM scan WHERE id = sqlc.arg(id);

-- name: ListScans :many
-- Every scan in progress, oldest first.
SELECT id, node_id, root_path, priority, started_at, listed_at, listed_count, to_examine_count, examined_count,
       is_another_scan_queued, scope_path, again_scope_path, listing_sent_at
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

-- name: GetUndoneScanBatchHolding :one
-- The batch of a scan in progress that still has to read a file a person asked to play (D304
-- section 4): its files' JSON holds the file's path as the batch encoded it, path_fragment.
SELECT id FROM scan_batch
WHERE scan_id = sqlc.arg(scan_id) AND is_done = 0 AND instr(files_json, sqlc.arg(path_fragment)) > 0
LIMIT 1;
