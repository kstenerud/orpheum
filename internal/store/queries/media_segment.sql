-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Media segments: the typed time ranges inside a file, each with the provider that found it (D82,
-- plans/media-segments.md). A detector replaces only its own rows; a person's are no detector's.

-- name: DeleteDetectedSegmentsNotIn :exec
-- A detector's segments of a file that it no longer finds: found is a JSON array of
-- {"type", "startMs", "endMs"}. Its others, and every other provider's, stay.
DELETE FROM media_segment
WHERE media_segment.file_id = sqlc.arg(file_id) AND media_segment.provider = sqlc.arg(provider)
  AND NOT EXISTS (
    SELECT 1 FROM json_each(CAST(sqlc.arg(found) AS TEXT)) AS found
    WHERE json_extract(found.value, '$.type') = media_segment.type
      AND json_extract(found.value, '$.startMs') = media_segment.start_ms
      AND json_extract(found.value, '$.endMs') = media_segment.end_ms);

-- name: InsertDetectedSegmentsMissing :exec
-- What a detector finds of a file that it has not kept already, so a file read again unchanged
-- writes nothing (D303, DL9): found as DeleteDetectedSegmentsNotIn's.
INSERT INTO media_segment (file_id, type, start_ms, end_ms, provider, confidence)
SELECT sqlc.arg(file_id), json_extract(found.value, '$.type'), json_extract(found.value, '$.startMs'),
  json_extract(found.value, '$.endMs'), sqlc.arg(provider), sqlc.arg(confidence)
FROM json_each(CAST(sqlc.arg(found) AS TEXT)) AS found
WHERE NOT EXISTS (
  SELECT 1 FROM media_segment AS kept
  WHERE kept.file_id = sqlc.arg(file_id) AND kept.provider = sqlc.arg(provider)
    AND kept.type = json_extract(found.value, '$.type')
    AND kept.start_ms = json_extract(found.value, '$.startMs')
    AND kept.end_ms = json_extract(found.value, '$.endMs'));

-- name: ListSegmentsOfFile :many
-- A file's segments as a player takes them, in order: of each type, a person's where they set any,
-- which beat every detector's of it, else the detectors'.
SELECT media_segment.type, media_segment.start_ms, media_segment.end_ms, media_segment.provider
FROM media_segment
WHERE media_segment.file_id = sqlc.arg(file_id)
  AND (media_segment.provider = 'person' OR NOT EXISTS (
    SELECT 1 FROM media_segment AS own
    WHERE own.file_id = media_segment.file_id AND own.provider = 'person' AND own.type = media_segment.type))
ORDER BY media_segment.start_ms, media_segment.end_ms, media_segment.id;

-- name: DeleteSegmentsOfPurgedFiles :exec
-- The segments of files purged: in seconds of a file, they go with it (library-model.md, D173).
DELETE FROM media_segment WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: MovePersonSegmentsToFile :exec
-- A person's segments of a copy, its file's from now on: the copy's payload is its file's, and so is
-- its timeline (D363).
UPDATE media_segment SET file_id = sqlc.arg(to_file)
WHERE media_segment.file_id = sqlc.arg(from_file) AND media_segment.provider = 'person';

-- name: DeleteSegmentsOfFile :exec
-- A copy's detected segments, once its person's are moved: its file's detectors find the same.
DELETE FROM media_segment WHERE file_id = sqlc.arg(file_id);
