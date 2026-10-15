-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Play state (schema.md; play-state.md, D165, D217 section 7): the guarded position, the log of
-- state changes, and the watched spans.

-- name: GetPlayPosition :one
SELECT position_ms, report_seq, device_id, updated_at, position_read_at
FROM play_position
WHERE user_id = sqlc.arg(user_id) AND file_id = sqlc.arg(file_id);

-- name: MovePlayPosition :exec
-- The guard: a report moves the position only when the watching it describes is later than the
-- watching that set it, wherever each report came from and whenever it arrived.
INSERT INTO play_position (user_id, file_id, position_ms, report_seq, device_id, updated_at, position_read_at)
VALUES (sqlc.arg(user_id), sqlc.arg(file_id), sqlc.arg(position_ms), sqlc.arg(report_seq), sqlc.arg(device_id),
  sqlc.arg(updated_at), sqlc.arg(position_read_at))
ON CONFLICT (user_id, file_id) DO UPDATE SET
  position_ms = excluded.position_ms, report_seq = excluded.report_seq, device_id = excluded.device_id,
  updated_at = excluded.updated_at, position_read_at = excluded.position_read_at
WHERE excluded.position_read_at > play_position.position_read_at;

-- name: InsertPlayEvent :exec
INSERT INTO play_event (user_id, file_id, device_id, kind, position_ms, happened_at, happened_mono_ms)
VALUES (sqlc.arg(user_id), sqlc.arg(file_id), sqlc.arg(device_id), sqlc.arg(kind), sqlc.arg(position_ms),
  sqlc.arg(happened_at), sqlc.arg(happened_mono_ms));

-- name: InsertPlaySpan :exec
INSERT INTO play_span (user_id, file_id, device_id, from_position_ms, to_position_ms, started_at, ended_at, is_unattended)
VALUES (sqlc.arg(user_id), sqlc.arg(file_id), sqlc.arg(device_id), sqlc.arg(from_position_ms),
  sqlc.arg(to_position_ms), sqlc.arg(started_at), sqlc.arg(ended_at), sqlc.arg(is_unattended));

-- name: ListPlaySpansOfFile :many
-- A person's watching of one file, in the order it happened.
SELECT from_position_ms, to_position_ms, started_at, ended_at, device_id, is_unattended
FROM play_span
WHERE user_id = sqlc.arg(user_id) AND file_id = sqlc.arg(file_id)
ORDER BY started_at, id;

-- name: ListPlayEventsOfFile :many
-- A person's changes of play state in one file, in the order they happened.
SELECT kind, position_ms, happened_at, device_id
FROM play_event
WHERE user_id = sqlc.arg(user_id) AND file_id = sqlc.arg(file_id)
ORDER BY happened_at, id;

-- name: ListRecentPlayPositions :many
-- A person's positions in live files of live films and episodes, most recent watching first,
-- with each file's length where its probe knows it and when its work was last hidden: what
-- Continue Watching is chosen from. Extras are left out (D162).
SELECT play_position.file_id, play_position.position_ms, play_position.position_read_at,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = file.id), 0) AS INTEGER)
    AS duration_ms, file.path,
  title.id AS title_id, title.kind, title.year, item.library_id, item_text.name, progress_hide.hidden_at
FROM play_position
JOIN file ON file.id = play_position.file_id AND file.removed_at IS NULL
JOIN version ON version.id = file.version_id AND version.removed_at IS NULL
JOIN title ON title.id = version.title_id AND title.kind IN ('movie', 'episode')
JOIN item ON item.id = title.id AND item.removed_at IS NULL
LEFT JOIN item_text ON item_text.item_id = title.id AND item_text.is_original = 1
LEFT JOIN progress_hide ON progress_hide.user_id = play_position.user_id AND progress_hide.item_id = title.id
WHERE play_position.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
    OR item.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
ORDER BY play_position.position_read_at DESC
LIMIT sqlc.arg(max_rows);

-- name: HideProgress :exec
-- A work hidden from Continue Watching until it next has activity (D188): not a completion.
INSERT INTO progress_hide (user_id, item_id, hidden_at) VALUES (sqlc.arg(user_id), sqlc.arg(item_id), sqlc.arg(hidden_at))
ON CONFLICT (user_id, item_id) DO UPDATE SET hidden_at = excluded.hidden_at;

-- name: UnhideProgress :exec
DELETE FROM progress_hide WHERE user_id = sqlc.arg(user_id) AND item_id = sqlc.arg(item_id);

-- A removed person's own watching goes with them (D173 section 4.6).

-- name: DeletePlaySpansOfUser :exec
DELETE FROM play_span WHERE user_id = sqlc.arg(user_id);

-- name: DeletePlayEventsOfUser :exec
DELETE FROM play_event WHERE user_id = sqlc.arg(user_id);

-- name: DeletePlayPositionsOfUser :exec
DELETE FROM play_position WHERE user_id = sqlc.arg(user_id);

-- name: DeleteProgressHidesOfUser :exec
DELETE FROM progress_hide WHERE user_id = sqlc.arg(user_id);
