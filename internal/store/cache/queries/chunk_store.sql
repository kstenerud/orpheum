-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A chunk store (D247 section 10): one stream of a film on one grid, which its make_chunks jobs
-- target, so the runs of a film's streams queue side by side (internal/core/chunks).

-- name: EnsureChunkStore :one
-- The store's row, made the first time it is named. Its DO UPDATE sets a column to itself so that
-- RETURNING gives the id of a row already there, which a guard would stop it returning.
-- @sqlc-vet-disable upsert-only-if-changed
INSERT INTO chunk_store (source_file, stream, grid_id)
VALUES (sqlc.arg(source_file), sqlc.arg(stream), sqlc.arg(grid_id))
ON CONFLICT (source_file, stream, grid_id) DO UPDATE SET stream = excluded.stream
RETURNING id;

-- name: ListChunkStoresOfFile :many
-- A film's stores, every stream on every grid, whose runs a player that left no longer wants.
SELECT id FROM chunk_store WHERE source_file = sqlc.arg(source_file) ORDER BY id;

-- name: GetChunkStore :one
SELECT id, source_file, stream, grid_id
FROM chunk_store
WHERE id = sqlc.arg(id);
