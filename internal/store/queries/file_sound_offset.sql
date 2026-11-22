-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A file's sound offset for every person, an admin's (D537): later when positive, earlier when
-- negative, in milliseconds. A file with no row plays in step; a person's own is a preference.

-- name: SetFileSoundOffset :exec
-- Written only when it changes something (D303, DL9).
INSERT INTO file_sound_offset (file_id, offset_ms, set_at)
VALUES (sqlc.arg(file_id), sqlc.arg(offset_ms), sqlc.arg(set_at))
ON CONFLICT (file_id) DO UPDATE SET offset_ms = excluded.offset_ms, set_at = excluded.set_at
WHERE file_sound_offset.offset_ms IS NOT excluded.offset_ms;

-- name: DeleteFileSoundOffset :exec
-- A file set back in step for every person.
DELETE FROM file_sound_offset WHERE file_id = sqlc.arg(file_id);

-- name: GetFileSoundOffset :one
-- A file's offset for every person, 0 for none.
SELECT CAST(coalesce((SELECT offset_ms FROM file_sound_offset WHERE file_id = sqlc.arg(file_id)), 0) AS INTEGER)
  AS offset_ms;

-- name: MoveFileSoundOffsetToFile :exec
-- A file confirmed to be a copy of another (D363 section 2) gives the other its offset, where it has
-- none: the bytes, and so the sound's shift, are the same.
INSERT INTO file_sound_offset (file_id, offset_ms, set_at)
SELECT CAST(sqlc.arg(to_file) AS INTEGER), moved.offset_ms, moved.set_at FROM file_sound_offset AS moved
WHERE moved.file_id = sqlc.arg(from_file)
ON CONFLICT DO NOTHING;
