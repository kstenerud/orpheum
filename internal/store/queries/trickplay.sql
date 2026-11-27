-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The sweep for videos to make the scrub bar's pictures of (trickplay.md section 4). What is made is
-- kept in cache.db (internal/store/cache/queries/trickplay.sql), which this database cannot join, so
-- the sweep leaves out there the bytes already made or failed.

-- name: ListVideosForTrickplay :many
-- The live videos with a picture, among the ids after after_id and up to through_id, with where each
-- is and its bytes' payload hash. A picture is a video stream that is not a cover
-- (stream.is_attached_picture, migration 0097): a file of covers alone is never listed. A file no
-- library covers is listed by no sweep, as ListFilesToReadThrough says; whether one is under a root
-- or disk that is unavailable is the library's rule (library.Unavailability), which the sweep asks.
SELECT f.id, own.node_id, own.reported_path_raw, CAST(f.payload_hash AS BLOB) AS payload_hash
FROM file f
CROSS JOIN file_place ON file_place.file_id = f.id
CROSS JOIN place AS own ON own.id = file_place.place_id
WHERE f.id > sqlc.arg(after_id) AND f.id <= sqlc.arg(through_id) AND own.removed_at IS NULL
  AND own.filing = 'read' AND f.sidecar_kind IS NULL AND f.payload_hash IS NOT NULL
  AND EXISTS (SELECT 1 FROM file_probe WHERE file_probe.file_id = f.id)
  AND EXISTS (SELECT 1 FROM library_place WHERE library_place.place_id = own.id)
  AND EXISTS (SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'video' AND s.is_attached_picture = 0)
ORDER BY f.id;

-- name: ListPayloadHashesOfFiles :many
-- Every payload hash a file row still names, removed or not: what cache.db's trickplay is kept for.
-- A file's row goes at its purge (removedFilesKeptDays), and its pictures with it.
SELECT DISTINCT CAST(payload_hash AS BLOB) AS payload_hash FROM file WHERE payload_hash IS NOT NULL;
