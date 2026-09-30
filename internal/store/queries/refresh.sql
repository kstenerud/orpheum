-- SPDX-License-Identifier: GPL-3.0-or-later
-- The rolling refresh of stored provider responses, D213: what is due, and what a refresh
-- touches.

-- name: ListResponsesAskedBefore :many
-- Live items whose provider was last asked for a response before a time: the part asked longest
-- ago decides. An item with a failure of its own waits for a person (D231 section 4).
SELECT provider_response.item_id, CAST(MIN(provider_response.last_asked_at) AS INTEGER) AS oldest_asked_at
FROM provider_response
JOIN item ON item.id = provider_response.item_id
WHERE provider_response.provider = sqlc.arg(provider) AND item.removed_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = provider_response.item_id)
GROUP BY provider_response.item_id
HAVING MIN(provider_response.last_asked_at) < sqlc.arg(before);

-- name: ListResponseParts :many
SELECT part
FROM provider_response
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider)
ORDER BY part;

-- name: MarkProviderResponseAsked :exec
-- A refresh that found the item gone upstream keeps our copy and notes that it asked, so it is
-- not asked again before its next period, D213 section 4. The copy's fetched_at stays its age.
UPDATE provider_response SET last_asked_at = sqlc.arg(last_asked_at)
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider) AND part = sqlc.arg(part);
