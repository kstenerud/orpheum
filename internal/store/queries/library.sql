-- SPDX-License-Identifier: GPL-3.0-or-later
-- Libraries and the node roots they cover (schema.md, Libraries; D184, D107).

-- name: InsertLibrary :one
INSERT INTO library (name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at)
VALUES (
  sqlc.arg(name), sqlc.arg(kind), sqlc.arg(metadata_source), sqlc.arg(change_detection),
  sqlc.arg(check_every_hours), sqlc.arg(is_writable), sqlc.arg(created_at)
)
RETURNING id;

-- name: InsertLibraryRoot :exec
INSERT INTO library_root (library_id, node_id, path)
VALUES (sqlc.arg(library_id), sqlc.arg(node_id), sqlc.arg(path));

-- name: GetLiveLibraryByName :one
SELECT id, name, kind
FROM library
WHERE name = ? AND removed_at IS NULL
ORDER BY id
LIMIT 1;

-- name: ListLibraries :many
SELECT id, name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at
FROM library
WHERE removed_at IS NULL
ORDER BY name, id;

-- name: GetLibrary :one
SELECT id, name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at
FROM library
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: ListLibraryRootStates :many
-- A library's roots with their node and the last scan's outcome: whether the files are
-- available, and a withheld scan's reason (D208).
SELECT library_root.path, library_root.node_id, node.name AS node_name, root_state.last_scan_outcome,
  root_state.last_scan_at, root_state.withheld_reason
FROM library_root
JOIN node ON node.id = library_root.node_id
LEFT JOIN root_state ON root_state.node_id = library_root.node_id AND root_state.root_path = library_root.path
WHERE library_root.library_id = sqlc.arg(library_id)
ORDER BY library_root.path;

-- name: CountLibraryFilings :many
-- How many live files a library has in each filing: the scanner view's summary (D163).
SELECT filing, count(*) AS files
FROM file
WHERE library_id = sqlc.arg(library_id) AND removed_at IS NULL
GROUP BY filing
ORDER BY filing;

-- name: ListFilesPage :many
-- What the scanner saw (D163): live files by id after a cursor, optionally of one library and
-- one filing.
SELECT id, library_id, path, filing, filing_reason, error_detail_json, size_bytes, first_seen_at, last_seen_at
FROM file
WHERE removed_at IS NULL AND id > sqlc.arg(after_id)
  AND (library_id = sqlc.narg(library_id) OR sqlc.narg(library_id) IS NULL)
  AND (filing = sqlc.narg(filing) OR sqlc.narg(filing) IS NULL)
ORDER BY id
LIMIT sqlc.arg(max_rows);
