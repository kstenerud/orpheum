-- SPDX-License-Identifier: GPL-3.0-or-later
-- An ambiguous title's search results, kept so choosing costs nothing (D172).

-- name: DeleteMatchCandidates :exec
DELETE FROM match_candidate
WHERE title_id = sqlc.arg(title_id);

-- name: InsertMatchCandidate :exec
INSERT INTO match_candidate (title_id, rank, provider, provider_id, name, year, runtime_ms, poster_ref)
VALUES (sqlc.arg(title_id), sqlc.arg(rank), sqlc.arg(provider), sqlc.arg(provider_id), sqlc.arg(name),
        sqlc.arg(year), sqlc.arg(runtime_ms), sqlc.arg(poster_ref));

-- name: ListMatchCandidates :many
SELECT title_id, rank, provider, provider_id, name, year, runtime_ms, poster_ref
FROM match_candidate
WHERE title_id = sqlc.arg(title_id)
ORDER BY rank;
