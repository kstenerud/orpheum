-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The sweep for videos to read through: whose loudness (D226 section 2) or picture's timeline (D247
-- section 10) a read of the whole file would learn.

-- name: GetMaxFileID :one
-- The highest file id, where a sweep for files to read through stops.
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) AS last_id FROM file;

-- name: ListFilesToReadThrough :many
-- The live videos, among the ids after after_id and up to through_id, whose bytes have no loudness
-- though they have sound, or no timeline though they have a picture, with where each is. A sweep
-- reads the files a range of ids at a time, by the primary key, so no one query reads them all.
-- Whether a file is under a root or disk that is unavailable is the library's one rule
-- (library.Unavailability), which the sweep asks of each. A picture is a video stream that is not
-- a cover: disposition bit 1024 is FFmpeg's AV_DISPOSITION_ATTACHED_PIC. A timeline is looked for
-- by payload hash alone; the read-through picks the main picture. A timeline read from the file's
-- index has its keyframes but not its bytes in each second, which only a read through finds (D259
-- section 2), so it is read through still, unless that read failed (read_failure, D270 section 1). has_timeline is 1 when the bytes have any timeline, so
-- one with none is read from its index first, the sweep's fast tier (D269 section 5). A file no
-- library covers, its folder retired or its library deleted, is read by no sweep: it waits to be
-- covered again or reaped (D363 section 10).
SELECT f.id, own.node_id, own.reported_path_raw,
  CAST((SELECT 1 FROM video_timeline v WHERE v.payload_hash = f.payload_hash LIMIT 1) IS NOT NULL AS INTEGER)
    AS has_timeline
FROM file f
CROSS JOIN place AS own ON own.id = f.id
WHERE f.id > sqlc.arg(after_id) AND f.id <= sqlc.arg(through_id) AND own.removed_at IS NULL
  AND own.filing = 'read' AND f.sidecar_kind IS NULL AND f.payload_hash IS NOT NULL
  AND EXISTS (SELECT 1 FROM file_probe WHERE file_probe.file_id = f.id)
  AND EXISTS (SELECT 1 FROM library_place WHERE library_place.place_id = f.id)
  AND (
    ((SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'audio' LIMIT 1) IS NOT NULL
      AND (SELECT 1 FROM loudness l WHERE l.payload_hash = f.payload_hash LIMIT 1) IS NULL)
    OR ((SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'video' AND (s.disposition_flags & 1024) = 0
        LIMIT 1) IS NOT NULL
      AND (SELECT 1 FROM video_timeline v WHERE v.payload_hash = f.payload_hash
        AND (v.bytes_in_each_second_json <> '[]' OR v.failure IS NOT NULL OR v.read_failure IS NOT NULL)
        LIMIT 1) IS NULL))
ORDER BY f.id;

-- name: ListFileSizesAndNodes :many
-- The node, the size and the length of each live file named, by its id: the bytes ahead of a
-- read-through on its node (D324 section 6). A length not probed is 0.
SELECT file.id, own.node_id, file.size_bytes,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = file.id), 0) AS INTEGER)
    AS duration_ms
FROM file
CROSS JOIN place AS own ON own.id = file.id AND own.removed_at IS NULL
WHERE file.id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));
