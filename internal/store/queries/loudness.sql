-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Each audio track's loudness, as the node measured it (D226 sections 1 and 2).

-- name: ListLoudnessOfPayload :many
-- What was measured of a file's bytes by the recipe named: each track's sounds, or why they could not
-- be measured. A measurement of another recipe is none of this one's (D536 section 3).
SELECT payload_hash, stream_index, measured_mix, integrated_lufs, range_lu, true_peak_dbtp, failure, measured_at
FROM loudness
WHERE payload_hash = sqlc.arg(payload_hash) AND recipe = sqlc.arg(recipe)
ORDER BY stream_index, measured_mix;

-- name: UpsertLoudness :exec
-- A measurement, keyed by the recipe it was made with (D536 section 3).
INSERT INTO loudness (payload_hash, stream_index, measured_mix, recipe, integrated_lufs, range_lu, true_peak_dbtp,
  failure, measured_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(stream_index), sqlc.arg(measured_mix), sqlc.arg(recipe),
  sqlc.arg(integrated_lufs), sqlc.arg(range_lu), sqlc.arg(true_peak_dbtp), sqlc.arg(failure), sqlc.arg(measured_at))
ON CONFLICT (payload_hash, stream_index, measured_mix, recipe) DO UPDATE SET
  integrated_lufs = excluded.integrated_lufs, range_lu = excluded.range_lu,
  true_peak_dbtp = excluded.true_peak_dbtp, failure = excluded.failure, measured_at = excluded.measured_at
-- Only when it changes something (D303, DL9).
WHERE (loudness.integrated_lufs, loudness.range_lu, loudness.true_peak_dbtp, loudness.failure,
  loudness.measured_at)
  IS NOT (excluded.integrated_lufs, excluded.range_lu, excluded.true_peak_dbtp, excluded.failure,
    excluded.measured_at);

-- name: DeleteLoudnessOfSound :execrows
-- What was measured of one track of a file's bytes, both mixes, thrown out once the track's facts in
-- its probe changed: the sweep measures it again (D392 section 3).
DELETE FROM loudness WHERE payload_hash = sqlc.arg(payload_hash) AND stream_index = sqlc.arg(stream_index);

-- name: ForgetLoudnessFailuresOfFiles :execrows
-- The failed measurements of the bytes of the files named, a JSON array of ids: every rescan forgets
-- them, and the read-through sweep measures those bytes again; a measurement made stays (D535
-- section 8).
DELETE FROM loudness
WHERE failure IS NOT NULL AND payload_hash IN (SELECT file.payload_hash FROM file
  WHERE file.id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT))));
