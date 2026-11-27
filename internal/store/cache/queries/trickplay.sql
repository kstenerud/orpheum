-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A video's scrub-bar pictures (trickplay.md section 4), by its bytes' payload hash and the
-- trickplay version they were made at: the run's row, and each sheet the node posted
-- (internal/core/trickplay).

-- name: UpsertTrickplay :exec
-- The run's row: what it was made of and at, its end (done_at) or FFmpeg's failure.
INSERT INTO trickplay (payload_hash, version, video_stream_index, failure, done_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(version), sqlc.arg(video_stream_index), sqlc.narg(failure),
  sqlc.narg(done_at))
ON CONFLICT (payload_hash, version) DO UPDATE
SET video_stream_index = excluded.video_stream_index, failure = excluded.failure, done_at = excluded.done_at
-- Only when it changes something (D303, DL9).
WHERE (trickplay.video_stream_index, trickplay.failure, trickplay.done_at)
  IS NOT (excluded.video_stream_index, excluded.failure, excluded.done_at);

-- name: GetTrickplay :one
SELECT payload_hash, version, video_stream_index, failure, done_at
FROM trickplay
WHERE payload_hash = sqlc.arg(payload_hash) AND version = sqlc.arg(version);

-- name: ListTrickplaysEnded :many
-- The payload hashes whose pictures of the version are made or failed: the ones the sweep leaves out.
SELECT payload_hash FROM trickplay
WHERE version = sqlc.arg(version) AND (done_at IS NOT NULL OR failure IS NOT NULL);

-- name: ListTrickplayPayloadHashes :many
-- Every payload hash anything is kept of, at any version, a run's row or a sheet.
SELECT payload_hash FROM trickplay
UNION
SELECT payload_hash FROM trickplay_sheet;

-- name: UpsertTrickplaySheet :exec
-- One sheet the node posted: a repeat of it, as a node that sends again after a lost answer posts,
-- changes nothing, and one made again replaces it.
INSERT INTO trickplay_sheet (payload_hash, version, sheet_index, sheet_sha256_hex, size_bytes, picture_height,
  made_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(version), sqlc.arg(sheet_index), sqlc.arg(sheet_sha256_hex),
  sqlc.arg(size_bytes), sqlc.arg(picture_height), sqlc.arg(made_at))
ON CONFLICT (payload_hash, version, sheet_index) DO UPDATE
SET sheet_sha256_hex = excluded.sheet_sha256_hex, size_bytes = excluded.size_bytes,
  picture_height = excluded.picture_height, made_at = excluded.made_at
-- Only when it changes something (D303, DL9).
WHERE (trickplay_sheet.sheet_sha256_hex, trickplay_sheet.size_bytes, trickplay_sheet.picture_height)
  IS NOT (excluded.sheet_sha256_hex, excluded.size_bytes, excluded.picture_height);

-- name: ListTrickplaySheets :many
-- The sheets of a video's pictures at the version, in order.
SELECT sheet_index, sheet_sha256_hex, size_bytes, picture_height, made_at
FROM trickplay_sheet
WHERE payload_hash = sqlc.arg(payload_hash) AND version = sqlc.arg(version)
ORDER BY sheet_index;

-- name: ListTrickplaySheetFilesOf :many
-- The files of every sheet of the bytes, at any version: those a forgetting of them may delete.
SELECT DISTINCT sheet_sha256_hex FROM trickplay_sheet WHERE payload_hash = sqlc.arg(payload_hash);

-- name: CountTrickplaySheetsOfFile :one
-- How many sheets name a file: a sheet's image shared by two videos' bytes is kept while one does.
SELECT COUNT(*) FROM trickplay_sheet WHERE sheet_sha256_hex = sqlc.arg(sheet_sha256_hex);

-- name: DeleteTrickplaySheetsOf :execrows
DELETE FROM trickplay_sheet WHERE payload_hash = sqlc.arg(payload_hash);

-- name: DeleteTrickplayOf :execrows
DELETE FROM trickplay WHERE payload_hash = sqlc.arg(payload_hash);

-- name: HasTrickplaySheetFile :one
-- Whether a sheet of the bytes, at any version, is the file: what a file's sheet is served by.
SELECT EXISTS (
  SELECT 1 FROM trickplay_sheet
  WHERE payload_hash = sqlc.arg(payload_hash) AND sheet_sha256_hex = sqlc.arg(sheet_sha256_hex)
);
