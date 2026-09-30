-- SPDX-License-Identifier: GPL-3.0-or-later
-- What the fetch sweeps derive their work from besides the stored responses (D231 sections 3 and
-- 4; schema.md, fetch_failure and refresh_request): a film's or series' own failure to be
-- fetched, and a person's request to fetch it again.

-- name: RecordFetchFailure :exec
-- A fetch that failed through the item's own fault, its last attempt included (D231 section 10):
-- the sweeps pass the item over until a person's refresh, or a fetch that succeeds, clears it.
INSERT INTO fetch_failure (item_id, failure, failed_at)
VALUES (sqlc.arg(item_id), sqlc.arg(failure), sqlc.arg(failed_at))
ON CONFLICT (item_id) DO UPDATE SET failure = excluded.failure, failed_at = excluded.failed_at;

-- name: ClearFetchFailure :exec
DELETE FROM fetch_failure WHERE item_id = sqlc.arg(item_id);

-- name: GetFetchFailure :one
SELECT item_id, failure, failed_at FROM fetch_failure WHERE item_id = sqlc.arg(item_id);

-- name: RequestRefresh :exec
-- A person's refresh (D213 section 2): the newest request's time, asking for the images again if
-- this one or one still waiting does.
INSERT INTO refresh_request (item_id, requested_at, should_fetch_images)
VALUES (sqlc.arg(item_id), sqlc.arg(requested_at), sqlc.arg(should_fetch_images))
ON CONFLICT (item_id) DO UPDATE SET requested_at = excluded.requested_at,
  should_fetch_images = max(refresh_request.should_fetch_images, excluded.should_fetch_images);

-- name: GetRefreshRequest :one
SELECT item_id, requested_at, should_fetch_images FROM refresh_request WHERE item_id = sqlc.arg(item_id);

-- name: SettleRefreshRequest :exec
-- A fetch that read the request has answered it; a request made since stays for the next.
DELETE FROM refresh_request WHERE item_id = sqlc.arg(item_id) AND requested_at <= sqlc.arg(read_at);

-- name: ListRefreshRequests :many
-- The live items a person asked to fetch again and whose fetch has not answered yet: the refresh
-- sweep's work, oldest request first.
SELECT refresh_request.item_id, refresh_request.should_fetch_images
FROM refresh_request
JOIN item ON item.id = refresh_request.item_id AND item.removed_at IS NULL
WHERE NOT EXISTS (SELECT 1 FROM fetch_failure WHERE fetch_failure.item_id = refresh_request.item_id)
ORDER BY refresh_request.requested_at, refresh_request.item_id;

-- name: ListFetchedLibraryIDs :many
-- The live libraries whose items are matched at a provider, which the first-match sweep looks
-- through (D170).
SELECT id FROM library WHERE removed_at IS NULL AND metadata_source <> 'nfo' ORDER BY id;
