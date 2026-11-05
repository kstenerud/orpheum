-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Play state (schema.md; play-state.md, D165, D217 section 7): the guarded position, the log of
-- state changes, and the watched spans.

-- name: GetPlayPosition :one
-- A person's position in a file, and the latest of their marks by hand of the titles it is filed as,
-- 0 for none: a position watched at or before it is not where the file resumes (D370 section 4).
SELECT play_position.position_ms, play_position.report_seq, play_position.device_id, play_position.updated_at,
  play_position.position_read_at,
  CAST(coalesce((
    SELECT max(work_watched.marked_at) FROM file_filing
    CROSS JOIN version ON version.id = file_filing.version_id
    CROSS JOIN work_watched ON work_watched.user_id = play_position.user_id
      AND work_watched.work_id = version.work_id
    WHERE file_filing.file_id = play_position.file_id), 0) AS INTEGER) AS marked_at
FROM play_position
WHERE play_position.user_id = sqlc.arg(user_id) AND play_position.file_id = sqlc.arg(file_id);

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
-- A person's positions in live files of live films, episodes and other videos (D349), most recent
-- watching first, with each file's length where its probe knows it and when its work was last hidden:
-- what Continue Watching is chosen from, with when the person last marked its title by hand, before
-- which a position does not count (D370 section 4). Extras are left out (D162). A position is the file's, at
-- whichever place it was played (D363 section 10): it shows with each title a library the person
-- reaches lists the file in, named by the first of its places they reach (section 8).
SELECT play_position.file_id, play_position.position_ms, play_position.position_read_at,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = own.file_id), 0) AS INTEGER)
    AS duration_ms,
  CAST((
    SELECT place.path FROM version_library CROSS JOIN place ON place.id = version_library.place_id
    WHERE version_library.version_id = version.id AND version_library.file_id = own.file_id
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
    ORDER BY place.id <> place.file_id, place.first_seen_at, place.id LIMIT 1) AS TEXT) AS path,
  work.id AS work_id, work.kind, item.year, item_text.name, item_hidden.hidden_at, work_watched.marked_at
FROM play_position
JOIN file_place AS own ON own.file_id = play_position.file_id
JOIN file_filing ON file_filing.file_id = own.file_id
JOIN version ON version.id = file_filing.version_id AND version.removed_at IS NULL
JOIN work ON work.id = version.work_id AND work.kind IN ('movie', 'episode', 'other')
JOIN item ON item.id = work.id AND item.removed_at IS NULL
LEFT JOIN item_text ON item_text.item_id = work.id AND item_text.is_original = 1
LEFT JOIN item_hidden ON item_hidden.user_id = play_position.user_id AND item_hidden.item_id = work.id
LEFT JOIN work_watched ON work_watched.user_id = play_position.user_id AND work_watched.work_id = work.id
WHERE play_position.user_id = sqlc.arg(user_id)
  AND EXISTS (
    SELECT 1 FROM version_library WHERE version_library.version_id = version.id
      AND version_library.file_id = own.file_id
      AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
        OR version_library.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT)))))
ORDER BY play_position.position_read_at DESC
LIMIT sqlc.arg(max_rows);

-- name: ListWorksPlayPositions :many
-- A person's positions in the live files of some titles, most recent watching first, with each
-- file's length where its probe knows it and when its title was last hidden: as
-- ListRecentPlayPositions has them, for a list's films' progress; the page's titles drive, by CROSS JOIN.
SELECT version.work_id, play_position.position_ms, play_position.position_read_at,
  CAST(coalesce((SELECT file_probe.duration_ms FROM file_probe WHERE file_probe.file_id = own.file_id), 0) AS INTEGER)
    AS duration_ms,
  item_hidden.hidden_at, work_watched.marked_at
FROM json_each(CAST(sqlc.arg(work_ids_json) AS TEXT)) AS listed
CROSS JOIN version ON version.work_id = listed.value AND version.removed_at IS NULL
CROSS JOIN file_filing ON file_filing.version_id = version.id
CROSS JOIN file_place AS own ON own.file_id = file_filing.file_id
CROSS JOIN play_position ON play_position.user_id = sqlc.arg(user_id) AND play_position.file_id = own.file_id
LEFT JOIN item_hidden ON item_hidden.user_id = play_position.user_id AND item_hidden.item_id = version.work_id
LEFT JOIN work_watched ON work_watched.user_id = play_position.user_id AND work_watched.work_id = version.work_id
ORDER BY play_position.position_read_at DESC;

-- name: HideProgress :exec
-- A work hidden from Continue Watching until it next has activity (D188): not a completion.
INSERT INTO item_hidden (user_id, item_id, hidden_at) VALUES (sqlc.arg(user_id), sqlc.arg(item_id), sqlc.arg(hidden_at))
ON CONFLICT (user_id, item_id) DO UPDATE SET hidden_at = excluded.hidden_at
-- Only when it changes something (D303, DL9).
WHERE (item_hidden.hidden_at) IS NOT (excluded.hidden_at);

-- name: UnhideProgress :exec
DELETE FROM item_hidden WHERE user_id = sqlc.arg(user_id) AND item_id = sqlc.arg(item_id);

-- A removed person's own watching goes with them (D173 section 4.6).

-- name: DeletePlaySpansOfUser :exec
DELETE FROM play_span WHERE user_id = sqlc.arg(user_id);

-- name: DeletePlayEventsOfUser :exec
DELETE FROM play_event WHERE user_id = sqlc.arg(user_id);

-- name: DeletePlayPositionsOfUser :exec
DELETE FROM play_position WHERE user_id = sqlc.arg(user_id);

-- name: DeleteProgressHidesOfUser :exec
DELETE FROM item_hidden WHERE user_id = sqlc.arg(user_id);

-- name: DeleteWorksWatchedOfUser :exec
DELETE FROM work_watched WHERE user_id = sqlc.arg(user_id);

-- A removed file deleted past its keeping leaves what was watched of it on its version (D306
-- section 8), a film's or an episode's before an other video's (D363 section 5): how often each person
-- finished it and how long they watched, added to what earlier deleted files of the version left.

-- name: FoldWatchingOfPurgedFiles :exec
INSERT INTO purged_watching (user_id, version_id, completed_count, watched_ms, last_watched_at)
SELECT watching.user_id, primary_filing.version_id, sum(watching.completed), sum(watching.watched_ms),
  max(watching.happened_at)
FROM (
  SELECT play_span.user_id, play_span.file_id, 0 AS completed,
    play_span.ended_at - play_span.started_at AS watched_ms, play_span.ended_at AS happened_at
  FROM play_span
  WHERE play_span.file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)))
  UNION ALL
  SELECT play_event.user_id, play_event.file_id, 1, 0, play_event.happened_at
  FROM play_event
  WHERE play_event.kind = 'completed'
    AND play_event.file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)))
) AS watching
JOIN file_filing AS primary_filing ON primary_filing.file_id = watching.file_id AND primary_filing.version_id = (
    SELECT file_filing.version_id FROM file_filing WHERE file_filing.file_id = watching.file_id AND file_filing.version_id IS NOT NULL
    ORDER BY file_filing.library_kind = 'other', file_filing.library_kind LIMIT 1)
WHERE primary_filing.version_id IS NOT NULL
GROUP BY watching.user_id, primary_filing.version_id
ON CONFLICT (user_id, version_id) DO UPDATE SET
  completed_count = purged_watching.completed_count + excluded.completed_count,
  watched_ms = purged_watching.watched_ms + excluded.watched_ms,
  last_watched_at = max(purged_watching.last_watched_at, excluded.last_watched_at)
-- Only when it adds something (D303, DL9).
WHERE excluded.completed_count > 0 OR excluded.watched_ms > 0
  OR excluded.last_watched_at > purged_watching.last_watched_at;

-- name: DeletePlaySpansOfPurgedFiles :exec
DELETE FROM play_span WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeletePlayEventsOfPurgedFiles :exec
DELETE FROM play_event WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeletePlayPositionsOfPurgedFiles :exec
DELETE FROM play_position WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeletePurgedWatchingOfUser :exec
-- A removed person's watching of deleted files goes with them too (D305 section 1).
DELETE FROM purged_watching WHERE user_id = sqlc.arg(user_id);

-- name: ListWatchedLiveFiles :many
-- The live files of live films, episodes and other videos anyone has a position in, with their titles: what a
-- person has shown interest in by watching it, whose work keeps the rung above the background
-- (D324).
SELECT DISTINCT play_position.file_id, work.id AS work_id
FROM play_position
JOIN file_place AS own ON own.file_id = play_position.file_id
JOIN file_filing ON file_filing.file_id = own.file_id
JOIN version ON version.id = file_filing.version_id AND version.removed_at IS NULL
JOIN work ON work.id = version.work_id AND work.kind IN ('movie', 'episode', 'other')
JOIN item ON item.id = work.id AND item.removed_at IS NULL
ORDER BY play_position.file_id;

-- A file confirmed to be a copy of another (D363 section 2) is that file's place: what anyone watched
-- of it is the file's, a person's position the later of the two.

-- name: MovePlayPositionsToFile :exec
INSERT INTO play_position (user_id, file_id, position_ms, report_seq, device_id, updated_at, position_read_at)
SELECT user_id, sqlc.arg(to_file), position_ms, report_seq, device_id, updated_at, position_read_at
FROM play_position WHERE play_position.file_id = sqlc.arg(from_file)
ON CONFLICT (user_id, file_id) DO UPDATE SET position_ms = excluded.position_ms, report_seq = excluded.report_seq,
  device_id = excluded.device_id, updated_at = excluded.updated_at, position_read_at = excluded.position_read_at
-- Only when it changes something (D303, DL9): the copy's position was the later.
WHERE excluded.position_read_at > play_position.position_read_at;

-- name: DeletePlayPositionsOfFile :exec
DELETE FROM play_position WHERE file_id = sqlc.arg(file_id);

-- name: MovePlayEventsToFile :exec
UPDATE play_event SET file_id = sqlc.arg(to_file) WHERE file_id = sqlc.arg(from_file);

-- name: MovePlaySpansToFile :exec
UPDATE play_span SET file_id = sqlc.arg(to_file) WHERE file_id = sqlc.arg(from_file);

-- Whether a person has watched a title (D370): kept per person and per title, set by playing one of
-- its files to its finish and by hand, and a mark by hand a fresh start for where it resumes.

-- name: GetFileDuration :one
-- How long a file plays, where its probe knows it: whether a position finished it (D370 section 1).
SELECT duration_ms FROM file_probe WHERE file_id = sqlc.arg(file_id);

-- name: MarkWatchedByPlay :exec
-- A person played a file to its finish (D370 section 1): each film, episode or other video it is filed
-- as becomes watched, unless it already is, or a mark by hand came after the watching.
INSERT INTO work_watched (user_id, work_id, watched_at)
SELECT DISTINCT CAST(sqlc.arg(user_id) AS INTEGER), version.work_id, CAST(sqlc.arg(watched_at) AS INTEGER)
FROM file_filing
CROSS JOIN version ON version.id = file_filing.version_id AND version.removed_at IS NULL
CROSS JOIN work ON work.id = version.work_id AND work.kind IN ('movie', 'episode', 'other')
WHERE file_filing.file_id = sqlc.arg(file_id)
ON CONFLICT (user_id, work_id) DO UPDATE SET watched_at = excluded.watched_at
-- Only when it changes something (D303, DL9), and never over a later mark by hand.
WHERE work_watched.watched_at IS NULL
  AND (work_watched.marked_at IS NULL OR work_watched.marked_at < excluded.watched_at);

-- name: MarkWorksWatched :exec
-- A person marks titles watched by hand (D370 section 4): watched from now unless they were already,
-- and marked now, so the positions watched before it are no longer where they resume.
INSERT INTO work_watched (user_id, work_id, watched_at, marked_at)
SELECT CAST(sqlc.arg(user_id) AS INTEGER), work.id, CAST(sqlc.arg(marked_at) AS INTEGER),
  CAST(sqlc.arg(marked_at) AS INTEGER)
FROM json_each(CAST(sqlc.arg(work_ids_json) AS TEXT)) AS marked
CROSS JOIN work ON work.id = marked.value AND work.kind IN ('movie', 'episode', 'other')
WHERE true
ON CONFLICT (user_id, work_id) DO UPDATE SET
  watched_at = coalesce(work_watched.watched_at, excluded.watched_at), marked_at = excluded.marked_at
-- Only when it changes something (D303, DL9).
WHERE work_watched.watched_at IS NULL OR work_watched.marked_at IS NOT excluded.marked_at;

-- name: MarkWorksUnwatched :exec
-- A person marks titles unwatched by hand (D370 section 4): as never watched, the positions watched
-- before it no longer where they resume.
INSERT INTO work_watched (user_id, work_id, watched_at, marked_at)
SELECT CAST(sqlc.arg(user_id) AS INTEGER), work.id, NULL, CAST(sqlc.arg(marked_at) AS INTEGER)
FROM json_each(CAST(sqlc.arg(work_ids_json) AS TEXT)) AS marked
CROSS JOIN work ON work.id = marked.value AND work.kind IN ('movie', 'episode', 'other')
WHERE true
ON CONFLICT (user_id, work_id) DO UPDATE SET watched_at = NULL, marked_at = excluded.marked_at
-- Only when it changes something (D303, DL9).
WHERE work_watched.watched_at IS NOT NULL OR work_watched.marked_at IS NOT excluded.marked_at;

-- name: RestoreWorksWatched :exec
-- An undo puts titles' marks back as they were before a mark by hand (D370 section 4): each named
-- [title id, watched_at, marked_at], one of the two times at least.
INSERT INTO work_watched (user_id, work_id, watched_at, marked_at)
SELECT CAST(sqlc.arg(user_id) AS INTEGER), work.id, json_extract(restored.value, '$[1]'),
  json_extract(restored.value, '$[2]')
FROM json_each(CAST(sqlc.arg(states_json) AS TEXT)) AS restored
CROSS JOIN work ON work.id = json_extract(restored.value, '$[0]') AND work.kind IN ('movie', 'episode', 'other')
WHERE json_extract(restored.value, '$[1]') IS NOT NULL OR json_extract(restored.value, '$[2]') IS NOT NULL
ON CONFLICT (user_id, work_id) DO UPDATE SET watched_at = excluded.watched_at, marked_at = excluded.marked_at
-- Only when it changes something (D303, DL9).
WHERE work_watched.watched_at IS NOT excluded.watched_at OR work_watched.marked_at IS NOT excluded.marked_at;

-- name: DeleteWorksWatched :exec
-- An undo forgets the marks of titles that had none before a mark by hand.
DELETE FROM work_watched
WHERE user_id = sqlc.arg(user_id)
  AND work_id IN (SELECT value FROM json_each(CAST(sqlc.arg(work_ids_json) AS TEXT)));

-- name: ListWorksWatched :many
-- A person's marks of some titles, those that have one: when each became watched, NULL while it is
-- not, and when they last marked it by hand. The titles drive, by CROSS JOIN.
SELECT work_watched.work_id, work_watched.watched_at, work_watched.marked_at
FROM json_each(CAST(sqlc.arg(work_ids_json) AS TEXT)) AS listed
CROSS JOIN work_watched ON work_watched.user_id = sqlc.arg(user_id) AND work_watched.work_id = listed.value
ORDER BY work_watched.work_id;
