-- SPDX-License-Identifier: AGPL-3.0-or-later
-- What the core and each node used of their machines, a row a minute (toolkit.md, "Core and node
-- record their own CPU and memory use").

-- name: InsertResourceUse :exec
INSERT INTO resource_use (at, process, node_name, scope, cpu_micros, elapsed_ms, held_bytes, cache_bytes,
  peak_bytes, plays, running_json)
VALUES (sqlc.arg(at), sqlc.arg(process), sqlc.arg(node_name), sqlc.arg(scope), sqlc.narg(cpu_micros),
  sqlc.narg(elapsed_ms), sqlc.arg(held_bytes), sqlc.arg(cache_bytes), sqlc.narg(peak_bytes), sqlc.arg(plays),
  sqlc.arg(running_json))
ON CONFLICT (at, process, node_name) DO NOTHING;

-- name: DeleteResourceUseBefore :exec
-- Forgets the rows older than resourceuse keeps.
DELETE FROM resource_use WHERE at < ?;

-- name: ListResourceUseSince :many
-- Every row since a time, oldest first: the hours the System page shows are made of them.
SELECT at, process, node_name, scope, cpu_micros, elapsed_ms, held_bytes, cache_bytes, peak_bytes, plays,
  running_json
FROM resource_use
WHERE at >= ?
ORDER BY at, process, node_name;

-- name: SummarizeResourceUse :many
-- Each process's figures over every row kept: the first, its CPU time over its time, its busiest
-- minute's CPU time per second in microseconds, and the most memory it held, cached and peaked.
SELECT process, node_name,
  CAST(min(at) AS INTEGER) AS first_at,
  CAST(coalesce(sum(cpu_micros), 0) AS INTEGER) AS cpu_micros,
  CAST(coalesce(sum(elapsed_ms), 0) AS INTEGER) AS elapsed_ms,
  CAST(coalesce(max(cpu_micros * 1000 / elapsed_ms), 0) AS INTEGER) AS busiest_micros_per_second,
  CAST(max(held_bytes) AS INTEGER) AS held_bytes,
  CAST(max(cache_bytes) AS INTEGER) AS cache_bytes,
  CAST(coalesce(max(peak_bytes), 0) AS INTEGER) AS peak_bytes
FROM resource_use
GROUP BY process, node_name
ORDER BY process, node_name;
