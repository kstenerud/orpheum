-- SPDX-License-Identifier: GPL-3.0-or-later
-- An ambiguous film's or series' search results, kept so choosing costs nothing (D172).

-- name: DeleteMatchCandidates :exec
DELETE FROM match_candidate
WHERE item_id = sqlc.arg(item_id);

-- name: InsertMatchCandidate :exec
INSERT INTO match_candidate (item_id, rank, provider, provider_id, name, year, runtime_ms, poster_ref)
VALUES (sqlc.arg(item_id), sqlc.arg(rank), sqlc.arg(provider), sqlc.arg(provider_id), sqlc.arg(name),
        sqlc.arg(year), sqlc.arg(runtime_ms), sqlc.arg(poster_ref));

-- name: ListMatchCandidates :many
SELECT item_id, rank, provider, provider_id, name, year, runtime_ms, poster_ref
FROM match_candidate
WHERE item_id = sqlc.arg(item_id)
ORDER BY rank;
