-- SPDX-License-Identifier: GPL-3.0-or-later
-- The rolling refresh of stored provider responses, D213: what is due, and what a refresh
-- touches.

-- name: ListResponsesFetchedBefore :many
-- Live items with a response from this provider fetched before a time: the oldest part decides.
SELECT provider_response.item_id, CAST(MIN(provider_response.fetched_at) AS INTEGER) AS oldest
FROM provider_response
JOIN item ON item.id = provider_response.item_id
WHERE provider_response.provider = sqlc.arg(provider) AND item.removed_at IS NULL
GROUP BY provider_response.item_id
HAVING MIN(provider_response.fetched_at) < sqlc.arg(before);

-- name: ListResponseParts :many
SELECT part
FROM provider_response
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider)
ORDER BY part;

-- name: MarkProviderResponseAsked :exec
-- A refresh that found the item gone upstream keeps our copy and notes that it asked, so it is
-- not asked again before its next period, D213 section 4.
UPDATE provider_response SET fetched_at = sqlc.arg(fetched_at)
WHERE item_id = sqlc.arg(item_id) AND provider = sqlc.arg(provider) AND part = sqlc.arg(part);
