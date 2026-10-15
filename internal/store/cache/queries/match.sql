-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A film's or series' search results, kept so choosing or correcting costs nothing (D172, D301).

-- name: DeleteMatchCandidates :exec
DELETE FROM match_candidate
WHERE item_id = sqlc.arg(item_id);

-- name: InsertMatchCandidate :exec
INSERT INTO match_candidate (item_id, rank, provider, provider_id, name, year, runtime_ms, provider_poster_path,
                             votes)
VALUES (sqlc.arg(item_id), sqlc.arg(rank), sqlc.arg(provider), sqlc.arg(provider_id), sqlc.arg(name),
        sqlc.arg(year), sqlc.arg(runtime_ms), sqlc.arg(provider_poster_path), sqlc.arg(votes));

-- name: ListMatchCandidates :many
SELECT item_id, rank, provider, provider_id, name, year, runtime_ms, provider_poster_path, votes
FROM match_candidate
WHERE item_id = sqlc.arg(item_id)
ORDER BY rank;
