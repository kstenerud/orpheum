-- SPDX-License-Identifier: GPL-3.0-or-later
-- A chunk store (D247 section 10): one stream of a film on one grid, which its make_chunks jobs
-- target, so the runs of a film's streams queue side by side (internal/core/chunks).

-- name: EnsureChunkStore :one
-- The store's row, made the first time it is named.
INSERT INTO chunk_store (source_file, stream, grid_id)
VALUES (sqlc.arg(source_file), sqlc.arg(stream), sqlc.arg(grid_id))
ON CONFLICT (source_file, stream, grid_id) DO UPDATE SET stream = excluded.stream
RETURNING id;

-- name: GetChunkStore :one
SELECT id, source_file, stream, grid_id
FROM chunk_store
WHERE id = sqlc.arg(id);
