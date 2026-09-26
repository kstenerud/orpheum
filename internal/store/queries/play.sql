-- SPDX-License-Identifier: GPL-3.0-or-later
-- Play state (schema.md; play-state.md, D165, D217 section 7): the guarded position, the log of
-- state changes, and the watched spans.

-- name: GetPlayPosition :one
SELECT position_ms, seq, device_id, updated_at, watched_at
FROM play_position
WHERE user_id = sqlc.arg(user_id) AND file_id = sqlc.arg(file_id);

-- name: MovePlayPosition :exec
-- The guard: a report moves the position only when the watching it describes is later than the
-- watching that set it, wherever each report came from and whenever it arrived.
INSERT INTO play_position (user_id, file_id, position_ms, seq, device_id, updated_at, watched_at)
VALUES (sqlc.arg(user_id), sqlc.arg(file_id), sqlc.arg(position_ms), sqlc.arg(seq), sqlc.arg(device_id),
  sqlc.arg(updated_at), sqlc.arg(watched_at))
ON CONFLICT (user_id, file_id) DO UPDATE SET
  position_ms = excluded.position_ms, seq = excluded.seq, device_id = excluded.device_id,
  updated_at = excluded.updated_at, watched_at = excluded.watched_at
WHERE excluded.watched_at > play_position.watched_at;

-- name: InsertPlayEvent :exec
INSERT INTO play_event (user_id, file_id, device_id, kind, position_ms, server_at, client_mono_ms)
VALUES (sqlc.arg(user_id), sqlc.arg(file_id), sqlc.arg(device_id), sqlc.arg(kind), sqlc.arg(position_ms),
  sqlc.arg(server_at), sqlc.arg(client_mono_ms));

-- name: InsertPlaySpan :exec
INSERT INTO play_span (user_id, file_id, device_id, from_position_ms, to_position_ms, started_at, ended_at, unattended)
VALUES (sqlc.arg(user_id), sqlc.arg(file_id), sqlc.arg(device_id), sqlc.arg(from_position_ms),
  sqlc.arg(to_position_ms), sqlc.arg(started_at), sqlc.arg(ended_at), sqlc.arg(unattended));

-- name: ListPlaySpansOfFile :many
-- A person's watching of one file, in the order it happened.
SELECT from_position_ms, to_position_ms, started_at, ended_at, device_id, unattended
FROM play_span
WHERE user_id = sqlc.arg(user_id) AND file_id = sqlc.arg(file_id)
ORDER BY started_at, id;

-- name: ListPlayEventsOfFile :many
-- A person's changes of play state in one file, in the order they happened.
SELECT kind, position_ms, server_at, device_id
FROM play_event
WHERE user_id = sqlc.arg(user_id) AND file_id = sqlc.arg(file_id)
ORDER BY server_at, id;
