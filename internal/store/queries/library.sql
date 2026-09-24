-- SPDX-License-Identifier: GPL-3.0-or-later
-- Libraries and the node roots they cover (schema.md, Libraries; D184, D107).

-- name: InsertLibrary :one
INSERT INTO library (name, kind, metadata_source, change_detection, check_every_h, writable, created_at)
VALUES (
  sqlc.arg(name), sqlc.arg(kind), sqlc.arg(metadata_source), sqlc.arg(change_detection),
  sqlc.arg(check_every_h), sqlc.arg(writable), sqlc.arg(created_at)
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
