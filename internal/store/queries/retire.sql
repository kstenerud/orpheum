-- SPDX-License-Identifier: GPL-3.0-or-later
-- The retire actions (D231 section 7): a root, a node and a library each have one, a library's
-- being its delete. Each tombstones the files it covers, their history staying on their titles
-- (D173); the work those files and titles had is cancelled by the job queue.

-- name: RetireLibraryRoot :execrows
-- One root of a library, retired: it is scanned no more.
UPDATE library_root SET retired_at = sqlc.arg(retired_at)
WHERE library_id = sqlc.arg(library_id) AND node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path)
  AND retired_at IS NULL;

-- name: RetireRootsOfLibrary :many
-- A deleted library's roots, retired with it.
UPDATE library_root SET retired_at = sqlc.arg(retired_at)
WHERE library_id = sqlc.arg(library_id) AND retired_at IS NULL
RETURNING node_id, root_path;

-- name: ListLiveRootsOfNode :many
-- A node's roots in every live library, which retiring the node retires.
SELECT library_root.library_id, library_root.root_path
FROM library_root
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
WHERE library_root.node_id = sqlc.arg(node_id) AND library_root.retired_at IS NULL
ORDER BY library_root.library_id, library_root.root_path;

-- name: RemoveLibrary :execrows
-- A library deleted by an admin (D231 section 7).
UPDATE library SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: TombstoneLiveFilesOfLibrary :many
-- A deleted library's live files.
UPDATE file SET removed_at = sqlc.arg(removed_at)
WHERE library_id = sqlc.arg(library_id) AND removed_at IS NULL
RETURNING id;

-- name: ListStreamIDsOfFiles :many
-- The streams of files a retire tombstoned, whose extraction jobs it cancels; the ids are a JSON
-- array.
SELECT id FROM stream
WHERE video_file IN (SELECT value FROM json_each(CAST(sqlc.arg(file_id_list_json) AS TEXT)))
   OR bytes_file IN (SELECT value FROM json_each(CAST(sqlc.arg(file_id_list_json) AS TEXT)))
ORDER BY id;

-- name: ListFetchedSubtitleIDsOfFiles :many
-- The subtitles fetched for files a retire tombstoned, whose conversion jobs it cancels.
SELECT id FROM fetched_subtitle
WHERE video_file IN (SELECT value FROM json_each(CAST(sqlc.arg(file_id_list_json) AS TEXT)))
ORDER BY id;

-- name: ListRootMountPaths :many
-- The disks mounted inside a root, whose destinations a retire forgets.
SELECT mount_path FROM root_mount WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path) ORDER BY mount_path;

-- name: ListTitlesWithoutLiveFiles :many
-- A library's live titles with no live file left: after a retire, their work goes with their files.
SELECT title.id
FROM title
JOIN item ON item.id = title.id
WHERE item.library_id = sqlc.arg(library_id) AND item.removed_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM version CROSS JOIN file ON file.version_id = version.id
    WHERE version.title_id = title.id AND version.removed_at IS NULL AND file.removed_at IS NULL
  )
ORDER BY title.id;

-- name: TombstoneItemsOfLibrary :many
-- A deleted library's films, series and episodes, tombstoned and hidden with their history: a
-- film of it that reappears in another library brings its title back (D208 section 2, D231
-- section 7). A tombstone with no merged_into is one of these.
UPDATE item SET removed_at = sqlc.arg(removed_at)
WHERE library_id = sqlc.arg(library_id) AND removed_at IS NULL
RETURNING id;

-- name: RemoveNode :execrows
-- A node an admin retired (D231 section 7): it is gone, and its name, should it say hello again,
-- enrols a new node.
UPDATE node SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id) AND removed_at IS NULL;
