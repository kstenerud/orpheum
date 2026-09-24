-- SPDX-License-Identifier: GPL-3.0-or-later
-- What the scanner saw (schema.md, Files; D163, D173, D179): one row per observed path.

-- name: GetLibraryForRoot :one
SELECT library_root.library_id
FROM library_root
JOIN library ON library.id = library_root.library_id
WHERE library_root.node_id = ? AND library_root.path = ? AND library.removed_at IS NULL
ORDER BY library_root.library_id
LIMIT 1;

-- name: GetLiveFileByPath :one
SELECT id, payload_hash, last_seen
FROM file
WHERE node_id = ? AND path = ? AND removed_at IS NULL;

-- name: ListLiveFilesByHash :many
SELECT id, path, last_seen
FROM file
WHERE node_id = ? AND payload_hash = ? AND removed_at IS NULL
ORDER BY id;

-- name: InsertFile :one
INSERT INTO file (
  library_id, node_id, path, path_raw, size, mtime, payload_hash, payload_len,
  filing, filing_reason, error_detail, probe, first_seen, last_seen
) VALUES (
  sqlc.arg(library_id), sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(path_raw), sqlc.arg(size),
  sqlc.arg(mtime), sqlc.arg(payload_hash), sqlc.arg(payload_len), sqlc.arg(filing),
  sqlc.arg(filing_reason), sqlc.arg(error_detail), sqlc.arg(probe), sqlc.arg(seen), sqlc.arg(seen)
)
RETURNING id;

-- name: UpdateObservedFile :exec
-- A file observed again, or moved to path. A filing the core gave (listed, extra, attached,
-- unidentified) stays; one from a skip or an error becomes unidentified, now that it was read.
UPDATE file SET
  library_id = sqlc.arg(library_id),
  path = sqlc.arg(path),
  path_raw = sqlc.arg(path_raw),
  size = sqlc.arg(size),
  mtime = sqlc.arg(mtime),
  payload_hash = sqlc.arg(payload_hash),
  payload_len = sqlc.arg(payload_len),
  identity = 'confirmed',
  filing = CASE WHEN filing IN ('skipped', 'errored') THEN 'unidentified' ELSE filing END,
  filing_reason = CASE WHEN filing IN ('skipped', 'errored') THEN sqlc.arg(unidentified_reason)
                  ELSE filing_reason END,
  error_detail = NULL,
  probe = sqlc.arg(probe),
  last_seen = sqlc.arg(seen)
WHERE id = sqlc.arg(id);

-- name: UpdateFileFiling :exec
-- A path the node skipped or could not read: its filing says why, and it was seen.
UPDATE file SET
  filing = sqlc.arg(filing),
  filing_reason = sqlc.arg(filing_reason),
  error_detail = sqlc.arg(error_detail),
  last_seen = sqlc.arg(seen)
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesOfLibrary :many
SELECT id, path, last_seen
FROM file
WHERE node_id = ? AND library_id = ? AND removed_at IS NULL
ORDER BY id;

-- name: GetLatestTombstoneByHash :one
-- The file a returning payload revives (D208): the latest removed in the library.
SELECT id
FROM file
WHERE library_id = ? AND payload_hash = ? AND removed_at IS NOT NULL
ORDER BY removed_at DESC, id DESC
LIMIT 1;

-- name: ReviveFile :exec
UPDATE file SET removed_at = NULL WHERE id = ?;

-- name: TombstoneFile :exec
UPDATE file SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id);

-- name: UpsertRootScan :exec
INSERT INTO root_scan (node_id, path, completed_at)
VALUES (sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(completed_at))
ON CONFLICT (node_id, path) DO UPDATE SET completed_at = excluded.completed_at;

-- name: DeleteFileStreams :exec
DELETE FROM stream WHERE file_id = ?;

-- name: InsertStream :exec
INSERT INTO stream (file_id, media_file, stream_index, kind, codec, lang_raw, lang, title_raw, disposition, channels)
VALUES (
  sqlc.arg(file_id), sqlc.arg(file_id), sqlc.arg(stream_index), sqlc.arg(kind), sqlc.arg(codec),
  sqlc.arg(lang_raw), sqlc.arg(lang), sqlc.arg(title_raw), sqlc.arg(disposition), sqlc.arg(channels)
);

-- name: ListLiveFilesByNode :many
SELECT id, library_id, path, path_raw, size, payload_hash, payload_len, filing, filing_reason,
  error_detail, first_seen, last_seen
FROM file
WHERE node_id = ? AND removed_at IS NULL
ORDER BY path;

-- name: ListStreamsOfFile :many
SELECT stream_index, kind, codec, lang_raw, lang, title_raw, disposition, channels
FROM stream
WHERE file_id = ?
ORDER BY stream_index;

-- name: InsertSidecarFile :one
INSERT INTO file (
  library_id, node_id, path, path_raw, size, mtime, filing, filing_reason, sidecar_body, first_seen, last_seen
) VALUES (
  sqlc.arg(library_id), sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(path_raw), sqlc.arg(size),
  sqlc.arg(mtime), 'attached', sqlc.arg(kind), sqlc.arg(body), sqlc.arg(seen), sqlc.arg(seen)
)
RETURNING id;

-- name: UpdateSidecarFile :exec
UPDATE file SET
  library_id = sqlc.arg(library_id), path_raw = sqlc.arg(path_raw), size = sqlc.arg(size),
  mtime = sqlc.arg(mtime), filing = 'attached', filing_reason = sqlc.arg(kind),
  error_detail = NULL, sidecar_body = sqlc.arg(body), last_seen = sqlc.arg(seen)
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesForAttaching :many
SELECT id, path, filing, payload_hash, attached_to
FROM file
WHERE node_id = ? AND library_id = ? AND removed_at IS NULL
ORDER BY path;

-- name: SetFileAttachedTo :exec
UPDATE file SET attached_to = sqlc.arg(attached_to) WHERE id = sqlc.arg(id);

-- name: GetSidecarBody :one
SELECT sidecar_body FROM file WHERE id = ?;
