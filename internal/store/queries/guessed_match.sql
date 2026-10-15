-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A match the matcher guessed among several whose names match, marked until a person confirms or
-- corrects it (D301 section 4).

-- name: MarkGuessedMatch :exec
INSERT INTO guessed_match (item_id, guessed_at) VALUES (sqlc.arg(item_id), sqlc.arg(guessed_at))
ON CONFLICT (item_id) DO UPDATE SET guessed_at = excluded.guessed_at
-- Only when it changes something (D303, DL9).
WHERE (guessed_match.guessed_at) IS NOT (excluded.guessed_at);

-- name: ClearGuessedMatch :exec
DELETE FROM guessed_match WHERE item_id = sqlc.arg(item_id);

-- name: IsGuessedMatch :one
-- Whether an item's match is a guess no person has confirmed.
SELECT EXISTS (SELECT 1 FROM guessed_match WHERE item_id = sqlc.arg(item_id)) AS is_guessed;
