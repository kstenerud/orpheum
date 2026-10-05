-- SPDX-License-Identifier: GPL-3.0-or-later
-- The sweep for videos to read through: whose loudness (D226 section 2) or picture's timeline (D247
-- section 10, kept in cache.db) a read of the whole file would learn.

-- name: GetMaxFileID :one
-- The highest file id, where a sweep for files to read through stops.
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) AS last_id FROM file;

-- name: ListFilesToReadThrough :many
-- The live videos, among the ids after after_id and up to through_id, whose bytes have no loudness
-- though they have sound, or that have a picture, whose timeline is in cache.db for the sweep to
-- look for; with where each is and its payload hash. A sweep reads the files a range of ids at a
-- time, by the primary key, so no one query reads them all. Whether a file is under a root or disk
-- that is unavailable is the library's one rule (library.Unavailability), which the sweep asks of
-- each. A picture is a video stream that is not a cover: disposition bit 1024 is FFmpeg's
-- AV_DISPOSITION_ATTACHED_PIC.
SELECT f.id, f.node_id, f.reported_path_raw, f.payload_hash,
  CAST((SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'audio' LIMIT 1) IS NOT NULL
    AND (SELECT 1 FROM loudness l WHERE l.payload_hash = f.payload_hash LIMIT 1) IS NULL AS INTEGER) AS lacks_loudness,
  CAST((SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'video' AND (s.disposition_flags & 1024) = 0
    LIMIT 1) IS NOT NULL AS INTEGER) AS has_picture
FROM file f
WHERE f.id > sqlc.arg(after_id) AND f.id <= sqlc.arg(through_id) AND f.removed_at IS NULL
  AND f.filing IN ('listed', 'extra', 'unidentified') AND f.payload_hash IS NOT NULL AND f.probe_json IS NOT NULL
  AND (
    ((SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'audio' LIMIT 1) IS NOT NULL
      AND (SELECT 1 FROM loudness l WHERE l.payload_hash = f.payload_hash LIMIT 1) IS NULL)
    OR (SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'video' AND (s.disposition_flags & 1024) = 0
      LIMIT 1) IS NOT NULL)
ORDER BY f.id;
