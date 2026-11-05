-- SPDX-License-Identifier: AGPL-3.0-or-later
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
UPDATE library SET revision = revision + 1, removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id) AND removed_at IS NULL;

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

-- name: ListWorksNoLibraryLists :many
-- The live titles a library's pass files that no library lists any more: after a retire, their work
-- goes, and their data waits for the storage budgets to need its room (D363 section 10).
SELECT work.id
FROM work
JOIN item ON item.id = work.id
WHERE item.filing_library_id = sqlc.arg(library_id) AND item.removed_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM version CROSS JOIN version_library ON version_library.version_id = version.id
    WHERE version.work_id = work.id AND version.removed_at IS NULL
  )
ORDER BY work.id;

-- name: ListUncoveredFilesUnder :many
-- The live places under a folder on a node (ListLiveFilesUnder's range) that no live library's live
-- folder covers: what a retired folder or a deleted library leaves to no one, kept with its history,
-- whose work goes (D363 section 10).
SELECT place.id
FROM file AS place
WHERE place.node_id = sqlc.arg(node_id) AND place.removed_at IS NULL
  AND place.reported_path_raw > CAST(sqlc.arg(after) AS BLOB) AND place.reported_path_raw < CAST(sqlc.arg(before) AS BLOB)
  AND NOT EXISTS (SELECT 1 FROM library_place WHERE library_place.place_id = place.id)
ORDER BY place.id;

-- name: RemoveNode :execrows
-- A node an admin retired (D231 section 7): it is gone, and its name, should it say hello again,
-- enrols a new node.
UPDATE node SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: RepointLibraryRoot :execrows
-- A library root an admin says is now at another path, on the same or another node (D241 section 6).
UPDATE library_root SET node_id = sqlc.arg(new_node_id), root_path = sqlc.arg(new_root_path)
WHERE library_id = sqlc.arg(library_id) AND node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path)
  AND retired_at IS NULL;

-- name: RepointFile :exec
-- A live file of a repointed root, at its new place: its modified time is forgotten, so the next
-- scan reads it again and confirms it by its payload hash before it counts as there.
UPDATE file SET node_id = sqlc.arg(node_id), path = sqlc.arg(path), reported_path_raw = sqlc.arg(reported_path_raw),
  modified_at = NULL
WHERE id = sqlc.arg(id);

-- name: ForgetRootState :exec
-- A repointed root's state at its old place: its scans, its facts and the disks mounted inside it.
DELETE FROM root_state WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: ForgetRootMounts :exec
DELETE FROM root_mount WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: ForgetRootScan :exec
DELETE FROM root_scan WHERE node_id = sqlc.arg(node_id) AND root_path = sqlc.arg(root_path);

-- name: ListWorksOfFileNoLibraryLists :many
-- The titles a file is in, of each kind, that no library lists any more: after an admin said it is
-- gone, their work goes with its.
SELECT DISTINCT version.work_id
FROM file_filing
CROSS JOIN version ON version.id = file_filing.version_id
WHERE file_filing.file_id = sqlc.arg(file_id)
  AND NOT EXISTS (
    SELECT 1 FROM version AS listed CROSS JOIN version_library ON version_library.version_id = listed.id
    WHERE listed.work_id = version.work_id AND listed.removed_at IS NULL
  )
ORDER BY version.work_id;
