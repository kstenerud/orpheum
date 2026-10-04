-- SPDX-License-Identifier: GPL-3.0-or-later
-- Each audio track's loudness, as the node measured it (D226 sections 1 and 2).

-- name: GetMaxFileID :one
-- The highest file id, where a sweep for files to measure stops.
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) AS last_id FROM file;

-- name: ListFilesToMeasure :many
-- The live videos with sound whose bytes have no loudness yet, among the ids after after_id and up to
-- through_id, with where each is: a sweep reads the files a range of ids at a time, by the primary
-- key, so no one query reads them all. Whether a file is under a root or disk that is unavailable
-- is the library's one rule (library.Unavailability), which the sweep asks of each.
SELECT f.id, f.node_id, f.reported_path_raw
FROM file f
WHERE f.id > sqlc.arg(after_id) AND f.id <= sqlc.arg(through_id) AND f.removed_at IS NULL
  AND f.filing IN ('listed', 'extra', 'unidentified') AND f.payload_hash IS NOT NULL AND f.probe_json IS NOT NULL
  AND (SELECT 1 FROM stream s WHERE s.video_file = f.id AND s.kind = 'audio' LIMIT 1) IS NOT NULL
  AND (SELECT 1 FROM loudness l WHERE l.payload_hash = f.payload_hash LIMIT 1) IS NULL
ORDER BY f.id;

-- name: ListLoudnessOfPayload :many
-- What was measured of a file's bytes: each track's sounds, or why they could not be measured.
SELECT payload_hash, stream_index, measured_mix, integrated_lufs, range_lu, true_peak_dbtp, failure, measured_at
FROM loudness
WHERE payload_hash = ?
ORDER BY stream_index, measured_mix;

-- name: UpsertLoudness :exec
INSERT INTO loudness (payload_hash, stream_index, measured_mix, integrated_lufs, range_lu, true_peak_dbtp, failure,
  measured_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(stream_index), sqlc.arg(measured_mix), sqlc.arg(integrated_lufs),
  sqlc.arg(range_lu), sqlc.arg(true_peak_dbtp), sqlc.arg(failure), sqlc.arg(measured_at))
ON CONFLICT (payload_hash, stream_index, measured_mix) DO UPDATE SET
  integrated_lufs = excluded.integrated_lufs, range_lu = excluded.range_lu,
  true_peak_dbtp = excluded.true_peak_dbtp, failure = excluded.failure, measured_at = excluded.measured_at;
