-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Libraries and the node roots they cover (schema.md, Libraries; D184, D107).

-- name: InsertLibrary :one
INSERT INTO library (name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at)
VALUES (
  sqlc.arg(name), sqlc.arg(kind), sqlc.arg(metadata_source), sqlc.arg(change_detection),
  sqlc.arg(check_every_hours), sqlc.arg(is_writable), sqlc.arg(created_at)
)
RETURNING id;

-- name: InsertLibraryRoot :exec
-- A library's folder, or one it had and retired, live again: a library that covers it again finds
-- what it held as it was (D363 section 10).
INSERT INTO library_root (library_id, node_id, root_path)
VALUES (sqlc.arg(library_id), sqlc.arg(node_id), sqlc.arg(path))
ON CONFLICT (library_id, node_id, root_path) DO UPDATE SET retired_at = NULL
-- Only when it changes something (D303, DL9).
WHERE library_root.retired_at IS NOT NULL;

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
-- A library's roots with their node, the root of the scan that reads each, the outermost live library
-- folder on its node at or above it, of whichever library (D363 section 2), and that scan's last
-- outcome: whether the files are available, and a withheld scan's reason (D208).
SELECT library_root.root_path, library_root.node_id, node.name AS node_name,
  CAST(coalesce((
    SELECT outer_root.root_path FROM library_root AS outer_root
    JOIN library ON library.id = outer_root.library_id AND library.removed_at IS NULL
    WHERE outer_root.node_id = library_root.node_id AND outer_root.retired_at IS NULL
      AND (outer_root.root_path = library_root.root_path OR outer_root.root_path = '/'
        OR substr(library_root.root_path, 1, length(outer_root.root_path) + 1) = outer_root.root_path || '/')
    ORDER BY length(outer_root.root_path) LIMIT 1), library_root.root_path) AS TEXT) AS scan_root_path,
  root_state.last_scan_outcome, root_state.last_scan_at, root_state.withheld_reason
FROM library_root
JOIN node ON node.id = library_root.node_id
LEFT JOIN root_state ON root_state.node_id = library_root.node_id AND root_state.root_path = (
  SELECT outer_root.root_path FROM library_root AS outer_root
  JOIN library ON library.id = outer_root.library_id AND library.removed_at IS NULL
  WHERE outer_root.node_id = library_root.node_id AND outer_root.retired_at IS NULL
    AND (outer_root.root_path = library_root.root_path OR outer_root.root_path = '/'
      OR substr(library_root.root_path, 1, length(outer_root.root_path) + 1) = outer_root.root_path || '/')
  ORDER BY length(outer_root.root_path) LIMIT 1)
WHERE library_root.library_id = sqlc.arg(library_id) AND library_root.retired_at IS NULL
ORDER BY library_root.root_path;

-- name: SetLibraryChangeDetection :execrows
-- How a live library notices changes, chosen again by an admin: watching, checking every so many
-- hours, or only when asked (D184; the owner, 2026-10-09).
UPDATE library SET revision = revision + 1, change_detection = sqlc.arg(change_detection),
  check_every_hours = sqlc.narg(check_every_hours)
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: ListCheckedRoots :many
-- Every live root of a library that notices changes by itself, watching or checking every so many
-- hours, with when it was last scanned: what the scan schedule reads (D27, D272 section 1).
SELECT DISTINCT library_root.node_id, library_root.root_path, library.change_detection,
  library.check_every_hours, root_state.last_scan_at
FROM library_root
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
LEFT JOIN root_state ON root_state.node_id = library_root.node_id AND root_state.root_path = library_root.root_path
WHERE library_root.retired_at IS NULL AND library.change_detection IN ('watch', 'interval')
ORDER BY library_root.node_id, library_root.root_path;

-- name: CountLibraryFilings :many
-- How many live places a library's folders cover in each filing: the scanner view's summary (D163). A
-- video is filed as the library's kind filed it, and unidentified until it has (D363 section 5).
SELECT CAST(CASE WHEN library_place.filing <> 'read' THEN library_place.filing WHEN file.sidecar_kind IS NOT NULL THEN 'attached'
    WHEN library_place.place_id IS NOT library_place.file_id THEN 'copy' ELSE coalesce(file_filing.filing, 'unidentified') END AS TEXT) AS filing, count(*) AS files
FROM library_place
LEFT JOIN file ON file.id = library_place.file_id
LEFT JOIN file_filing ON file_filing.file_id = library_place.place_id
  AND file_filing.library_kind = library_place.library_kind
WHERE library_place.library_id = sqlc.arg(library_id)
GROUP BY 1
ORDER BY 1;

-- name: ListFilesPage :many
-- What the scanner saw (D163): live places by id after a cursor, each with the first live library that
-- covers it, optionally of one library and one filing. A video is filed as that library's kind filed
-- it, and unidentified until it has (D363 section 5); a copy is filed a copy.
SELECT place.id, CAST(coalesce(covering.id, 0) AS INTEGER) AS library_id, place.path,
  CAST(CASE WHEN place.filing <> 'read' THEN place.filing WHEN file.sidecar_kind IS NOT NULL THEN 'attached'
    WHEN place.id IS NOT place.file_id THEN 'copy' ELSE coalesce(file_filing.filing, 'unidentified') END AS TEXT) AS filing,
  coalesce(file_filing.filing_reason, place.filing_reason) AS filing_reason,
  place.error_detail_json, file.size_bytes, place.first_seen_at, place.last_seen_at
FROM place
LEFT JOIN file ON file.id = place.file_id
LEFT JOIN library AS covering ON covering.id = (
  SELECT first.library_id FROM library_place AS first
  WHERE first.place_id = place.id AND (first.library_id = sqlc.narg(library_id) OR sqlc.narg(library_id) IS NULL)
  ORDER BY first.library_id LIMIT 1)
LEFT JOIN file_filing ON file_filing.file_id = place.id AND file_filing.library_kind = covering.kind
WHERE place.removed_at IS NULL AND place.id > sqlc.arg(after_id)
  AND (covering.id IS NOT NULL OR sqlc.narg(library_id) IS NULL)
  AND (CAST(CASE WHEN place.filing <> 'read' THEN place.filing WHEN file.sidecar_kind IS NOT NULL THEN 'attached'
    WHEN place.id IS NOT place.file_id THEN 'copy' ELSE coalesce(file_filing.filing, 'unidentified') END AS TEXT)
    = sqlc.narg(filing) OR sqlc.narg(filing) IS NULL)
ORDER BY place.id
LIMIT sqlc.arg(max_rows);
