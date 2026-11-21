-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The fingerprint detector's seasons (D101, D115; plans/media-segments.md): which videos of each
-- season of an aired ordering its sound can be compared across, and which of their bytes were.

-- name: ListSeasonVideos :many
-- The live videos of each season, a group of a series' aired ordering, whose sound can be measured,
-- by season, node and file, with their bytes' measurement at version when there is one: a video
-- filed as an episode in a live version, read at its own place in a library, probed with a length,
-- with a sound track. group_id 0 lists every season's; another lists that season's alone.
SELECT DISTINCT CAST(position.grp_id AS INTEGER) AS group_id, own.node_id, version.work_id, f.id AS file_id, f.payload_hash,
  measured.outcome AS measured_outcome
FROM ordering
CROSS JOIN position ON position.ordering_id = ordering.id
CROSS JOIN item AS episode ON episode.id = position.work_id
CROSS JOIN version ON version.work_id = position.work_id
CROSS JOIN file_filing ON file_filing.version_id = version.id
CROSS JOIN file AS f ON f.id = file_filing.file_id
CROSS JOIN file_place ON file_place.file_id = f.id
CROSS JOIN place AS own ON own.id = file_place.place_id
CROSS JOIN file_probe ON file_probe.file_id = f.id
LEFT JOIN repeat_measurement AS measured
  ON measured.payload_hash = f.payload_hash AND measured.version = sqlc.arg(version)
WHERE ordering.name = 'aired' AND position.grp_id IS NOT NULL
  AND (position.grp_id = sqlc.arg(group_id) OR sqlc.arg(group_id) = 0)
  AND episode.kind = 'episode' AND episode.removed_at IS NULL AND version.removed_at IS NULL
  AND file_filing.filing = 'listed' AND f.sidecar_kind IS NULL AND f.payload_hash IS NOT NULL
  AND own.removed_at IS NULL AND own.filing = 'read' AND file_probe.duration_ms > 0
  AND EXISTS (SELECT 1 FROM stream WHERE stream.video_file = f.id AND stream.kind = 'audio')
  AND EXISTS (SELECT 1 FROM library_place WHERE library_place.place_id = own.id)
ORDER BY position.grp_id, own.node_id, f.id;

-- name: UpsertRepeatMeasurement :exec
-- A video's bytes measured at a version: what the comparison found, or FFmpeg's failure.
INSERT INTO repeat_measurement (payload_hash, version, outcome, failure, measured_at)
VALUES (sqlc.arg(payload_hash), sqlc.arg(version), sqlc.arg(outcome), sqlc.narg(failure), sqlc.arg(measured_at))
ON CONFLICT (payload_hash, version) DO UPDATE SET
  outcome = excluded.outcome, failure = excluded.failure, measured_at = excluded.measured_at
WHERE (repeat_measurement.outcome, repeat_measurement.failure) IS NOT (excluded.outcome, excluded.failure);
