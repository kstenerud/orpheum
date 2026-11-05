-- SPDX-License-Identifier: AGPL-3.0-or-later
-- What the scanner saw (schema.md, Files; D163, D173, D179, D426 section 4): a file, its payload, and
-- each place it was seen at (D363 section 2). A file's id is the id of the place it was first found at,
-- its own place; a confirmed copy at another place is a place holding it. A folder's places are read
-- by a range of the bytes their node reported (library.PathRange), which place_live_path serves.

-- name: ListLibrariesAtOrUnderRoot :many
-- The live libraries with a live folder at or under a scan's root on its node: those whose files its
-- scan finds, handed on to identification. A root no library's folder is at is no scan's to store.
SELECT DISTINCT library_root.library_id,
  CAST(library_root.root_path = sqlc.arg(root_path) AS INTEGER) AS is_at_root
FROM library_root
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
WHERE library_root.node_id = sqlc.arg(node_id) AND library_root.retired_at IS NULL
  AND (library_root.root_path = sqlc.arg(root_path) OR sqlc.arg(root_path) = '/'
    OR substr(library_root.root_path, 1, length(sqlc.arg(root_path)) + 1) = sqlc.arg(root_path) || '/')
ORDER BY library_root.library_id;

-- name: GetLiveFileByReportedPathRaw :one
-- A live place by the bytes its node reported (D211), and the payload of the file there: two names
-- that are one NFC text are two places.
SELECT place.id, file.payload_hash, place.last_seen_at
FROM place
LEFT JOIN file ON file.id = place.file_id
WHERE place.node_id = sqlc.arg(node_id) AND place.reported_path_raw = sqlc.arg(reported_path_raw)
  AND place.removed_at IS NULL;

-- name: ListLiveFilesByPayloadHash :many
-- The live places on a node holding a payload: a file's own and its copies', and a duplicate not yet
-- confirmed one (D179, D363 section 2).
SELECT place.id, place.path, place.reported_path_raw, place.last_seen_at
FROM file
CROSS JOIN place ON place.file_id = file.id AND place.removed_at IS NULL
WHERE place.node_id = sqlc.arg(node_id) AND file.payload_hash = sqlc.arg(payload_hash)
ORDER BY place.id;

-- name: InsertFile :one
-- A file found, its payload as read so far; the place it was found at takes its id (InsertPlace).
INSERT INTO file (size_bytes, payload_hash, payload_bytes, opensubtitles_hash, sidecar_kind)
VALUES (
  sqlc.narg(size_bytes), sqlc.narg(payload_hash), sqlc.narg(payload_bytes), sqlc.narg(opensubtitles_hash),
  sqlc.narg(sidecar_kind)
)
RETURNING id;

-- name: InsertPlace :exec
-- The place a file was first found at, its own: it has the file's id (D426 section 4).
INSERT INTO place (
  id, file_id, node_id, path, reported_path_raw, modified_at, filing, filing_reason, error_detail_json,
  first_seen_at, last_seen_at, filesystem_device, inode
) VALUES (
  sqlc.arg(file_id), sqlc.arg(file_id), sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(reported_path_raw),
  sqlc.narg(modified_at), sqlc.arg(filing), sqlc.narg(filing_reason), sqlc.narg(error_detail_json),
  sqlc.arg(seen_at), sqlc.arg(seen_at), sqlc.narg(filesystem_device), sqlc.narg(inode)
);

-- name: UpdateObservedPlace :exec
-- A place observed again, or a file moved to it. One a skip or an error filed is read now, and its
-- file unidentified until identification files it.
UPDATE place SET
  path = sqlc.arg(path),
  reported_path_raw = sqlc.arg(reported_path_raw),
  modified_at = sqlc.arg(modified_at),
  filing = 'read',
  filing_reason = CASE WHEN filing IN ('skipped', 'errored') THEN sqlc.arg(unidentified_reason)
                  ELSE filing_reason END,
  error_detail_json = NULL,
  filesystem_device = sqlc.narg(filesystem_device),
  inode = sqlc.narg(inode),
  last_seen_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id);

-- name: UpdateObservedFile :one
-- The payload of the file at a place observed again, and that file's id.
UPDATE file SET
  size_bytes = sqlc.arg(size_bytes),
  payload_hash = sqlc.arg(payload_hash),
  payload_bytes = sqlc.arg(payload_bytes),
  opensubtitles_hash = sqlc.narg(opensubtitles_hash)
WHERE file.id = (SELECT place.file_id FROM place WHERE place.id = sqlc.arg(place_id))
RETURNING id;

-- name: MoveFileTo :exec
-- A file a listing found at a new path on the same device and inode, with its size and modified
-- time as stored: it moved, and its place and history follow it without its being read again (D304
-- section 5). A pass identifies it again from the new path, unless a person confirmed it.
UPDATE place SET
  path = sqlc.arg(path),
  reported_path_raw = sqlc.arg(reported_path_raw),
  last_seen_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: UpdateFileFilingAndLastSeen :exec
-- A path the node skipped or could not read: its place's filing says why, and it was seen.
UPDATE place SET
  filing = sqlc.arg(filing),
  filing_reason = sqlc.arg(filing_reason),
  error_detail_json = sqlc.arg(error_detail_json),
  last_seen_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesUnder :many
-- The live places on a node under a folder: those whose reported bytes lie after the folder's path and
-- a slash and before its path and the character after the slash (library.PathRange; D211).
SELECT id, path, reported_path_raw, last_seen_at
FROM place
WHERE node_id = sqlc.arg(node_id) AND removed_at IS NULL
  AND reported_path_raw > CAST(sqlc.arg(after) AS BLOB) AND reported_path_raw < CAST(sqlc.arg(before) AS BLOB)
ORDER BY id;

-- name: GetLatestTombstoneByPayloadHash :one
-- The file a returning payload revives (D208): the latest whose own place on the node was removed,
-- never a copy's place, whose identity is its file's.
SELECT place.id
FROM file
CROSS JOIN place ON place.id = file.id AND place.removed_at IS NOT NULL
WHERE place.node_id = sqlc.arg(node_id) AND file.payload_hash = sqlc.arg(payload_hash)
ORDER BY place.removed_at DESC, place.id DESC
LIMIT 1;

-- name: ReviveFile :exec
UPDATE place SET removed_at = NULL WHERE id = ?;

-- name: TombstoneFile :exec
-- A place removed: a file whose own place this is is removed with it (D34, D179).
UPDATE place SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id);
-- name: UpsertRootScan :exec
INSERT INTO root_scan (node_id, root_path, completed_at)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(completed_at))
ON CONFLICT (node_id, root_path) DO UPDATE SET completed_at = excluded.completed_at
-- Only when it changes something (D303, DL9).
WHERE (root_scan.completed_at) IS NOT (excluded.completed_at);

-- name: DeleteStreamsOfBytesFile :exec
DELETE FROM stream WHERE bytes_file = ?;

-- name: UpsertStream :exec
-- A video's own track, by its stream index. A row keeps its id for as long as its file has the
-- index, so an extraction job or a playback's subtitle address that names it holds across scans.
INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, container_language, language, container_title, disposition_flags, channel_count)
VALUES (
  sqlc.arg(file_id), sqlc.arg(file_id), sqlc.arg(stream_index), sqlc.arg(kind), sqlc.arg(codec),
  sqlc.arg(container_language), sqlc.arg(language), sqlc.arg(container_title), sqlc.arg(disposition_flags), sqlc.arg(channel_count)
)
ON CONFLICT (bytes_file, stream_index) DO UPDATE SET
  video_file = excluded.video_file, kind = excluded.kind, codec = excluded.codec, container_language = excluded.container_language,
  language = excluded.language, container_title = excluded.container_title, disposition_flags = excluded.disposition_flags,
  channel_count = excluded.channel_count
-- Only when it changes something (D303, DL9).
WHERE (stream.video_file, stream.kind, stream.codec, stream.container_language, stream.language,
  stream.container_title, stream.disposition_flags, stream.channel_count)
  IS NOT (excluded.video_file, excluded.kind, excluded.codec, excluded.container_language, excluded.language,
    excluded.container_title, excluded.disposition_flags, excluded.channel_count);

-- name: DeleteStreamsOfBytesFileNotIn :exec
-- A video's own tracks whose stream index its probe no longer has.
DELETE FROM stream
WHERE bytes_file = sqlc.arg(file_id) AND stream_index IS NOT NULL
  AND stream_index NOT IN (SELECT value FROM json_each(CAST(sqlc.arg(kept_indexes) AS TEXT)));


-- name: ListLiveFilesByNode :many
-- The live places on a node, each with its file and how the scan filed it: a place skipped or that
-- could not be read; else a sidecar, 'attached', a copy of a file found first elsewhere, 'copy', or a
-- video, as a library's files page words it.
SELECT place.id, CAST(place.file_id IS NOT place.id AS INTEGER) AS is_copy, place.path, place.reported_path_raw,
  file.size_bytes, file.payload_hash, file.payload_bytes,
  CAST(CASE WHEN place.filing <> 'read' THEN place.filing WHEN file.sidecar_kind IS NOT NULL THEN 'attached'
    WHEN place.file_id IS NOT place.id THEN 'copy' ELSE 'video' END AS TEXT) AS filing,
  place.filing_reason, place.error_detail_json, place.first_seen_at, place.last_seen_at
FROM place
LEFT JOIN file ON file.id = place.file_id
WHERE place.node_id = sqlc.arg(node_id) AND place.removed_at IS NULL
ORDER BY place.path;

-- name: ListStreamsOfBytesFile :many
SELECT id, stream_index, kind, codec, container_language, language, container_title, disposition_flags, channel_count
FROM stream
WHERE bytes_file = ?
ORDER BY stream_index;

-- name: UpdateSidecarFile :exec
-- A sidecar observed again, the payload of the file at its place: a subtitle carries its payload hash,
-- the file hashed whole; an NFO or an image has none. Only an NFO's bytes are kept, in file_nfo.
UPDATE file SET
  size_bytes = sqlc.arg(size_bytes), payload_hash = sqlc.arg(payload_hash), payload_bytes = sqlc.arg(payload_bytes),
  sidecar_kind = sqlc.arg(sidecar_kind)
WHERE file.id = (SELECT place.file_id FROM place WHERE place.id = sqlc.arg(place_id));

-- name: UpdateSidecarPlace :exec
-- A sidecar's place observed again: read, whatever a skip or an error once filed it.
UPDATE place SET
  modified_at = sqlc.arg(modified_at), filing = 'read', filing_reason = sqlc.arg(filing_reason),
  error_detail_json = NULL, last_seen_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesForAttaching :many
-- The live read places under a folder on a node (ListLiveFilesUnder's range), with the file at each,
-- for the sidecars beside each video's place, its own or a copy's: what lies beside a copy is its
-- place's (D363 section 3).
SELECT place.id, file.id AS file_id, place.path, place.reported_path_raw,
  CAST(CASE WHEN file.sidecar_kind IS NOT NULL THEN 'attached' WHEN place.id IS NOT place.file_id THEN 'copy'
    ELSE 'video' END AS TEXT) AS filing,
  file.sidecar_kind, file.payload_hash, place.attached_to_place
FROM place
CROSS JOIN file ON file.id = place.file_id
WHERE place.node_id = sqlc.arg(node_id) AND place.removed_at IS NULL AND place.filing = 'read'
  AND place.reported_path_raw > CAST(sqlc.arg(after) AS BLOB)
  AND place.reported_path_raw < CAST(sqlc.arg(before) AS BLOB)
ORDER BY place.path;

-- name: SetFileAttachedTo :exec
-- A sidecar's place attached to the place of the video beside it (D363 section 3).
UPDATE place SET attached_to_place = sqlc.arg(attached_to_place) WHERE id = sqlc.arg(id);

-- name: GetNFOBody :one
SELECT file_nfo.body AS nfo_body FROM file LEFT JOIN file_nfo ON file_nfo.file_id = file.id WHERE file.id = ?;

-- name: GetFileProbeJSON :one
-- A file's kept probe, which one read again is compared with (D392 section 2).
SELECT probe_json FROM file_probe WHERE file_id = sqlc.arg(file_id);

-- name: UpsertFileProbe :exec
-- A file's probe, and its duration, written only when they changed (D303, DL9).
INSERT INTO file_probe (file_id, probe_json, duration_ms)
VALUES (sqlc.arg(file_id), sqlc.arg(probe_json), sqlc.arg(duration_ms))
ON CONFLICT (file_id) DO UPDATE SET probe_json = excluded.probe_json, duration_ms = excluded.duration_ms
WHERE file_probe.probe_json IS NOT excluded.probe_json OR file_probe.duration_ms IS NOT excluded.duration_ms;

-- name: UpsertFileNFO :exec
-- An NFO's bytes, written only when they changed (D303, DL9).
INSERT INTO file_nfo (file_id, body) VALUES (sqlc.arg(file_id), sqlc.arg(body))
ON CONFLICT (file_id) DO UPDATE SET body = excluded.body WHERE file_nfo.body IS NOT excluded.body;

-- name: DeleteFileNFO :exec
-- A sidecar that is no NFO, or no longer one, keeps no bytes.
DELETE FROM file_nfo WHERE file_id = sqlc.arg(file_id);

-- name: GetPlayableFile :one
-- A live file a person may play: a video the scan read or listed, whatever it was identified as, at
-- its own place. A sidecar, or a file whose own place a scan skipped, could not read or removed, is
-- not played as a file of its own; a copy's place is its file's.
SELECT file.id, place.node_id, place.reported_path_raw, file.payload_hash, file_probe.probe_json,
  file.size_bytes
FROM file
CROSS JOIN place ON place.id = file.id AND place.removed_at IS NULL AND place.filing = 'read'
LEFT JOIN file_probe ON file_probe.file_id = file.id
WHERE file.id = ? AND file.sidecar_kind IS NULL;

-- name: ListPlacesOfFile :many
-- A live file's live places, its own and then its copies in the order they were found, each with
-- the live libraries that cover it, as a JSON array, empty where none does: where it may be read, and
-- by whom (D363 section 8).
SELECT place.id AS place_id, place.node_id, place.path, place.reported_path_raw, place.first_seen_at,
  CAST(coalesce((
    SELECT json_group_array(library_place.library_id) FROM library_place WHERE library_place.place_id = place.id
  ), '[]') AS TEXT) AS library_ids_json
FROM place
WHERE place.file_id = CAST(sqlc.arg(file_id) AS INTEGER) AND place.removed_at IS NULL
ORDER BY place.id <> place.file_id, place.first_seen_at, place.id;

-- name: ListLiveRootPathsOnNode :many
-- Every live library folder on a node, longest first, so the folder a path is under is the first
-- that is a prefix of it: where the node's byte socket names a file from (D363 section 2).
SELECT DISTINCT library_root.root_path
FROM library_root
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
WHERE library_root.node_id = sqlc.arg(node_id) AND library_root.retired_at IS NULL
ORDER BY length(library_root.root_path) DESC, library_root.root_path;

-- name: ListSubtitleTracksOfVideo :many
-- A video's subtitle tracks, its own and those of the live sidecar files attached to its place or to a
-- copy's, with the file each track's bytes are in, its payload hash and, for a person, whether a
-- library they reach covers the sidecar's place: what a playback offers (D363 section 3).
SELECT s.id, s.stream_index, s.codec, s.language, s.container_title, s.disposition_flags, s.bytes_file,
  file.payload_hash AS bytes_payload_hash
FROM stream s
JOIN place ON place.id = s.bytes_file AND place.removed_at IS NULL
JOIN file ON file.id = s.bytes_file
WHERE s.video_file = sqlc.arg(video_file) AND s.kind = 'subtitle'
  AND (s.bytes_file = s.video_file OR CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
    OR EXISTS (
      SELECT 1 FROM library_place WHERE library_place.place_id = s.bytes_file
        AND library_place.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT)))))
ORDER BY s.bytes_file <> s.video_file, s.stream_index, place.path;

-- name: ListAudioTracksOfVideo :many
-- A video's own audio tracks in the order of their stream indexes, which is their place among
-- them, as a run of chunks numbers them: what a person chooses a language from (D230).
SELECT id, stream_index, language, container_title, disposition_flags
FROM stream
WHERE video_file = sqlc.arg(video_file) AND bytes_file = video_file AND kind = 'audio'
ORDER BY stream_index;

-- name: GetSubtitleStream :one
-- A subtitle track, and the live file its bytes are in: where its text is kept, and the job that
-- extracts it.
SELECT s.id, s.video_file, s.stream_index, s.codec, s.bytes_file, file.payload_hash
FROM stream s
JOIN place ON place.id = s.bytes_file AND place.removed_at IS NULL
JOIN file ON file.id = s.bytes_file
WHERE s.id = sqlc.arg(id) AND s.kind = 'subtitle';

-- name: ListSubtitleStreamsInFile :many
-- The subtitle tracks whose bytes are in a live file, a video's own or a sidecar's one, and where
-- the file is, its own place: the job that extracts them, all from one read of it (D340).
SELECT s.id, s.stream_index, s.codec, place.node_id, place.reported_path_raw, file.payload_hash
FROM stream s
JOIN place ON place.id = s.bytes_file AND place.removed_at IS NULL
JOIN file ON file.id = s.bytes_file
WHERE s.bytes_file = sqlc.arg(bytes_file) AND s.kind = 'subtitle'
ORDER BY s.stream_index;

-- name: GetSidecarStream :one
-- A sidecar's one stream: a whole-file track has no stream index (schema.md, stream).
SELECT id, video_file, codec, container_language, language, disposition_flags
FROM stream
WHERE bytes_file = sqlc.arg(bytes_file) AND stream_index IS NULL;

-- name: InsertSidecarStream :exec
INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, container_language, language, disposition_flags)
VALUES (sqlc.arg(bytes_file), sqlc.arg(video_file), NULL, 'subtitle', sqlc.arg(codec), sqlc.arg(container_language),
  sqlc.arg(language), sqlc.arg(disposition_flags));

-- name: UpdateSidecarStream :exec
UPDATE stream SET video_file = sqlc.arg(video_file), codec = sqlc.arg(codec), container_language = sqlc.arg(container_language),
  language = sqlc.arg(language), disposition_flags = sqlc.arg(disposition_flags)
WHERE id = sqlc.arg(id);

-- name: GetFileForSubtitleSearch :one
-- A live video, what finds subtitles made for it (D224 section 3): its OpenSubtitles hash, and the
-- title it is a version of, if it is identified: a film's or an episode's before an other video's,
-- which no provider knows.
SELECT f.id, f.opensubtitles_hash, v.work_id, t.kind AS work_kind
FROM file f
CROSS JOIN place ON place.id = f.id AND place.removed_at IS NULL
LEFT JOIN version v ON v.id = (
  SELECT file_filing.version_id FROM file_filing
  CROSS JOIN version AS live ON live.id = file_filing.version_id AND live.removed_at IS NULL
  WHERE file_filing.file_id = f.id
  ORDER BY file_filing.library_kind = 'other', file_filing.library_kind LIMIT 1)
LEFT JOIN work t ON t.id = v.work_id
WHERE f.id = sqlc.arg(id);

-- name: SetFileVersionAndIdentifiedPath :exec
-- The version a kind of library's identification put a file in, and the path it was identified from
-- (D208 section 1, D363 section 5): its own place's. A file not filed yet is unidentified until
-- FileAs says otherwise.
INSERT INTO file_filing (file_id, library_kind, filing, version_id, identified_from_path)
SELECT place.id, sqlc.arg(library_kind), 'unidentified', sqlc.arg(version_id), place.path
FROM place WHERE place.id = sqlc.arg(file_id)
ON CONFLICT (file_id, library_kind) DO UPDATE SET version_id = excluded.version_id,
  identified_from_path = excluded.identified_from_path
-- Only when it changes something (D303, DL9).
WHERE (file_filing.version_id, file_filing.identified_from_path)
  IS NOT (excluded.version_id, excluded.identified_from_path);

-- name: UpsertFileFiling :exec
-- How a kind of library's identification filed a file, and why (D163, D363 section 5).
INSERT INTO file_filing (file_id, library_kind, filing, filing_reason)
VALUES (sqlc.arg(file_id), sqlc.arg(library_kind), sqlc.arg(filing), sqlc.arg(filing_reason))
ON CONFLICT (file_id, library_kind) DO UPDATE SET filing = excluded.filing, filing_reason = excluded.filing_reason
-- Only when it changes something (D303, DL9).
WHERE (file_filing.filing, file_filing.filing_reason) IS NOT (excluded.filing, excluded.filing_reason);

-- name: ListFilesToCompare :many
-- The live places under a scan's root on its node (ListLiveFilesUnder's range), with what a listing
-- is compared with (D240 section 3): a file whose size and modified time are as stored, and which was
-- read whole, needs nothing. The filing is ListLiveFilesByNode's word.
SELECT place.id, place.reported_path_raw, file.size_bytes, place.modified_at,
  CAST(CASE WHEN place.filing <> 'read' THEN place.filing WHEN file.sidecar_kind IS NOT NULL THEN 'attached'
    WHEN place.file_id IS NOT place.id THEN 'copy' ELSE 'video' END AS TEXT) AS filing,
  file.sidecar_kind, place.filesystem_device, place.inode,
  CAST(file.payload_hash IS NOT NULL AS INTEGER) AS has_payload_hash,
  CAST(EXISTS (SELECT 1 FROM file_nfo WHERE file_nfo.file_id = place.file_id) AS INTEGER) AS has_nfo_body
FROM place
LEFT JOIN file ON file.id = place.file_id
WHERE place.node_id = sqlc.arg(node_id) AND place.removed_at IS NULL
  AND place.reported_path_raw > CAST(sqlc.arg(after) AS BLOB)
  AND place.reported_path_raw < CAST(sqlc.arg(before) AS BLOB);

-- name: SetFilesSeen :exec
-- The places a listing found as stored: they were seen, and nothing else changed. One statement
-- for the listing's places, not one a place (D303, DL7).
UPDATE place SET last_seen_at = sqlc.arg(seen_at)
WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids_json) AS TEXT)));

-- name: IsFileReached :one
-- Whether a live library of those listed, or any when every library is reached, covers a live place
-- of a file: who may reach it (D243 section 4, D363 section 8).
SELECT CAST(EXISTS (
  SELECT 1 FROM library_place
  WHERE library_place.file_id = CAST(sqlc.arg(file_id) AS INTEGER)
    AND (CAST(sqlc.arg(is_every_library) AS INTEGER) = 1
      OR library_place.library_id IN (SELECT value FROM json_each(CAST(sqlc.arg(library_ids_json) AS TEXT))))
) AS INTEGER) AS is_reached;

-- name: ListFilesAHeldBackScanDidNotSee :many
-- The live places under a root (ListLiveFilesUnder's range) that its held-back scan, started at
-- scan_started_at, did not see: what it would remove (D277 section 2). What could not be read, and
-- the places under it, are not among them: a scan removes nothing it could not read.
SELECT place.path
FROM place
WHERE place.node_id = sqlc.arg(node_id) AND place.removed_at IS NULL
  AND place.reported_path_raw > CAST(sqlc.arg(after) AS BLOB)
  AND place.reported_path_raw < CAST(sqlc.arg(before) AS BLOB)
  AND place.last_seen_at < sqlc.arg(scan_started_at) AND place.filing <> 'errored'
  AND NOT EXISTS (
    SELECT 1 FROM place AS unread
    WHERE unread.filing = 'errored' AND unread.removed_at IS NULL AND unread.node_id = place.node_id
      AND substr(place.path, 1, length(unread.path) + 1) = unread.path || '/'
  )
ORDER BY place.path;

-- name: ListUnreadablePaths :many
-- What scans could not read in live libraries' folders, each under the first library that covers it,
-- with the platform's error, owner, group and mode (D163 section 8): what the System page lists with
-- the fix (D277 section 3).
SELECT library.id AS library_id, library.name AS library_name, node.name AS node_name, place.node_id,
  place.path, place.filing_reason, place.error_detail_json, place.first_seen_at
FROM place
JOIN node ON node.id = place.node_id AND node.removed_at IS NULL
JOIN library ON library.id = (
  SELECT library_place.library_id FROM library_place WHERE library_place.place_id = place.id
  ORDER BY library_place.library_id LIMIT 1)
WHERE place.filing = 'errored' AND place.removed_at IS NULL
-- In place_errored's order, which it then reads, never the whole table (D303).
ORDER BY place.node_id, place.path;

-- name: ListFilesToPurge :many
-- Removed places past their keeping (D304 section 6, D306 section 8): removed before the cut-off,
-- each with no sidecar beside it and, a file's own, no other place of its file that stays, oldest
-- first, a batch at a time. A file goes with its last place (DeletePurgedFiles).
SELECT place.id FROM place
WHERE place.removed_at IS NOT NULL AND place.removed_at < sqlc.arg(removed_before)
  AND NOT EXISTS (SELECT 1 FROM place AS sidecar WHERE sidecar.attached_to_place = place.id
    AND (sidecar.removed_at IS NULL OR sidecar.removed_at >= sqlc.arg(removed_before)))
  AND NOT EXISTS (SELECT 1 FROM place AS copy WHERE copy.file_id = place.id AND copy.id <> place.id
    AND (copy.removed_at IS NULL OR copy.removed_at >= sqlc.arg(removed_before)))
ORDER BY place.removed_at, place.id
LIMIT sqlc.arg(max_rows);

-- name: DeleteStreamsOfPurgedFiles :exec
DELETE FROM stream
WHERE video_file IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)))
  OR bytes_file IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeleteProbesOfPurgedFiles :exec
DELETE FROM file_probe WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeleteNFOsOfPurgedFiles :exec
DELETE FROM file_nfo WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeleteFilingsOfPurgedFiles :exec
DELETE FROM file_filing WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeleteWholeHashesOfPurgedFiles :exec
DELETE FROM whole_file_hash WHERE file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)));

-- name: DeletePurgedPlaces :execrows
DELETE FROM place WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(place_ids_json) AS TEXT)));

-- name: DeletePurgedFiles :exec
-- The files of purged places that no place holds any more: a file's id is its own place's.
DELETE FROM file WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)))
  AND NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = file.id);

-- A file found at a second place (D363 section 2): a video read at one place whose payload hash a
-- live video found earlier at another has is a candidate copy of it; D116's whole-file pass, run for
-- the two alone, confirms it, and its place holds the earlier one's file from then on. Until then, and
-- if the bytes differ, they are two files.

-- name: ListCopyCandidates :many
-- The live videos read at their own places, whose payload a live video found before them at its own
-- place has, each with the earliest such as the file it may be a copy of, and their whole-file hashes
-- as they stand: one missing, or hashed at another size or modified time than the file's now, is owed
-- a read. A pair both of whose reads are current and whose bytes differ, or one of which could not be
-- read, is settled as two files and listed no more until one changes. At most max_rows, in the order
-- they were found.
SELECT candidate.id, candidate.node_id, candidate.reported_path_raw, candidate_file.size_bytes,
  candidate.modified_at,
  first.id AS first_id, first.node_id AS first_node_id, first.reported_path_raw AS first_reported_path_raw,
  first_file.size_bytes AS first_size_bytes, first.modified_at AS first_modified_at,
  candidate_hash.sha256 AS candidate_sha256, candidate_hash.size_bytes AS candidate_hashed_size_bytes,
  candidate_hash.modified_at AS candidate_hashed_modified_at, candidate_hash.failure AS candidate_failure,
  first_hash.sha256 AS first_sha256, first_hash.size_bytes AS first_hashed_size_bytes,
  first_hash.modified_at AS first_hashed_modified_at, first_hash.failure AS first_failure
FROM file AS candidate_file
CROSS JOIN place AS candidate ON candidate.id = candidate_file.id
CROSS JOIN place AS first ON first.id = (
  SELECT earlier.id FROM file AS earlier_file
  CROSS JOIN place AS earlier ON earlier.id = earlier_file.id
  WHERE earlier_file.payload_hash = candidate_file.payload_hash AND earlier_file.sidecar_kind IS NULL
    AND earlier.removed_at IS NULL AND earlier.filing = 'read'
    AND (earlier.first_seen_at, earlier.id) < (candidate.first_seen_at, candidate.id)
  ORDER BY earlier.first_seen_at, earlier.id LIMIT 1)
CROSS JOIN file AS first_file ON first_file.id = first.id
LEFT JOIN whole_file_hash AS candidate_hash ON candidate_hash.file_id = candidate.id
LEFT JOIN whole_file_hash AS first_hash ON first_hash.file_id = first.id
WHERE candidate.removed_at IS NULL AND candidate.filing = 'read' AND candidate_file.sidecar_kind IS NULL
  AND candidate_file.payload_hash IS NOT NULL
  AND NOT coalesce(candidate_hash.file_id IS NOT NULL AND first_hash.file_id IS NOT NULL
    AND candidate_hash.size_bytes IS candidate_file.size_bytes
    AND candidate_hash.modified_at IS candidate.modified_at
    AND first_hash.size_bytes IS first_file.size_bytes AND first_hash.modified_at IS first.modified_at
    AND (candidate_hash.failure IS NOT NULL OR first_hash.failure IS NOT NULL
      OR candidate_hash.sha256 IS NOT first_hash.sha256), 0)
ORDER BY candidate.first_seen_at, candidate.id
LIMIT sqlc.arg(max_rows);

-- name: UpsertWholeFileHash :exec
-- A file's every byte hashed by its node, or why it could not be, at the size and modified time it
-- had (D116, D363 section 2); written only when it changed (D303, DL9).
INSERT INTO whole_file_hash (file_id, sha256, size_bytes, modified_at, failure, hashed_at)
VALUES (sqlc.arg(file_id), sqlc.arg(sha256), sqlc.arg(size_bytes), sqlc.arg(modified_at), sqlc.arg(failure),
  sqlc.arg(hashed_at))
ON CONFLICT (file_id) DO UPDATE SET sha256 = excluded.sha256, size_bytes = excluded.size_bytes,
  modified_at = excluded.modified_at, failure = excluded.failure, hashed_at = excluded.hashed_at
WHERE (whole_file_hash.sha256, whole_file_hash.size_bytes, whole_file_hash.modified_at, whole_file_hash.failure)
  IS NOT (excluded.sha256, excluded.size_bytes, excluded.modified_at, excluded.failure);

-- name: GetFileToHashWhole :one
-- A live video to read whole, at its own place and as it is now.
SELECT file.id, place.node_id, place.reported_path_raw, file.payload_hash, file.size_bytes, place.modified_at
FROM file
CROSS JOIN place ON place.id = file.id AND place.removed_at IS NULL AND place.filing = 'read'
WHERE file.id = sqlc.arg(id) AND file.sidecar_kind IS NULL;

-- name: MakeFileACopy :execrows
-- A video confirmed byte for byte the same as one found before it at another place: its place holds
-- that file from now on (D363 section 2), and the file it held, which no place holds then, goes
-- (DeleteFileOfACopy).
UPDATE place SET file_id = CAST(sqlc.arg(file_id) AS INTEGER), filing_reason = sqlc.arg(filing_reason)
WHERE place.id = sqlc.arg(id) AND place.removed_at IS NULL AND place.filing = 'read' AND place.file_id = place.id
  AND (SELECT file.sidecar_kind FROM file WHERE file.id = place.id) IS NULL;

-- name: DeleteFileOfACopy :exec
-- The file a copy's place held before it was confirmed one, once nothing is kept of it but its
-- whole-file hash, a cache, which goes with it.
DELETE FROM whole_file_hash WHERE whole_file_hash.file_id = sqlc.arg(file_id);

-- name: DeleteFileNoPlaceHolds :exec
DELETE FROM file WHERE file.id = sqlc.arg(file_id)
  AND NOT EXISTS (SELECT 1 FROM place WHERE place.file_id = sqlc.arg(file_id));

-- name: DeleteOwnStreamsOfFile :exec
-- A copy's own tracks: the file it is a copy of has the same, which a play offers.
DELETE FROM stream WHERE bytes_file = sqlc.arg(file_id) AND video_file = sqlc.arg(file_id);

-- name: MoveSidecarTracksToFile :exec
-- The tracks of the subtitles beside a copy, its file's tracks from now on: the sidecars stay at the
-- copy's place (D363 section 3).
UPDATE stream SET video_file = sqlc.arg(to_file) WHERE video_file = sqlc.arg(from_file) AND bytes_file <> sqlc.arg(from_file);

-- name: DeleteFileProbe :exec
-- A copy's probe: its file's is the same.
DELETE FROM file_probe WHERE file_id = sqlc.arg(file_id);

-- name: MoveFilingToFile :exec
-- A kind of library's filing of a copy, when it filed the file it is a copy of in no title of its
-- own: the file is in that title from now on (D363 section 5).
UPDATE file_filing SET file_id = sqlc.arg(to_file)
WHERE file_filing.file_id = sqlc.arg(from_file) AND file_filing.library_kind = sqlc.arg(library_kind)
  AND NOT EXISTS (
    SELECT 1 FROM file_filing AS kept WHERE kept.file_id = sqlc.arg(to_file) AND kept.library_kind = sqlc.arg(library_kind));

-- name: ListLiveCopiesOf :many
-- A file's live copies' places, the earliest found first: those that take its own place's when it goes.
SELECT id, node_id, path, reported_path_raw, modified_at, filesystem_device, inode, last_seen_at
FROM place
WHERE file_id = CAST(sqlc.arg(file_id) AS INTEGER) AND id <> CAST(sqlc.arg(file_id) AS INTEGER) AND removed_at IS NULL
ORDER BY first_seen_at, id;

-- name: MoveFileToItsCopysPlace :exec
-- A file whose own place went, at the place of a copy of it: its own place keeps its id, its history,
-- its filings and its tracks, at the copy's place (D363 section 2).
UPDATE place SET node_id = sqlc.arg(node_id), path = sqlc.arg(path), reported_path_raw = sqlc.arg(reported_path_raw),
  modified_at = sqlc.arg(modified_at),
  filesystem_device = sqlc.narg(filesystem_device), inode = sqlc.narg(inode), last_seen_at = sqlc.arg(last_seen_at)
WHERE id = sqlc.arg(id) AND removed_at IS NULL;

-- name: ReattachSidecars :exec
-- What lay beside a copy's place, beside the place that took its place.
UPDATE place SET attached_to_place = sqlc.arg(to_place) WHERE attached_to_place = sqlc.arg(from_place);

-- name: ListLivePlacesOfFiles :many
-- The live places of some files, their own and their copies': what retiring a file or a node
-- removes with it. The ids are a JSON array.
SELECT id, CAST(file_id IS NOT id AS INTEGER) AS is_copy FROM place
WHERE removed_at IS NULL AND file_id IN (SELECT value FROM json_each(CAST(sqlc.arg(file_ids_json) AS TEXT)))
ORDER BY id;

-- name: ListFilesAtSeveralPlaces :many
-- The live files found at more than one place, each with every place: what the doctor lists (D363
-- section 8), a page at a time after a file id.
SELECT own.id AS file_id, place.id AS place_id, place.node_id, place.path
FROM place AS own
CROSS JOIN place ON place.file_id = own.id AND place.removed_at IS NULL
WHERE own.removed_at IS NULL AND own.file_id = own.id AND own.id > sqlc.arg(after_id)
  AND EXISTS (SELECT 1 FROM place AS copy WHERE copy.file_id = own.id AND copy.id <> own.id
    AND copy.removed_at IS NULL)
ORDER BY own.id, place.id <> place.file_id, place.first_seen_at, place.id
LIMIT sqlc.arg(max_rows);

-- name: ListNFOsOfFilesAtSeveralPlaces :many
-- The NFO beside each place of each live file found at more than one place, with its bytes: whose
-- places' NFO files disagree, which the doctor lists (D363 section 9). Read from the copies' places.
SELECT own.id AS file_id, place.id AS place_id, place.path, nfo.id AS nfo_id, file_nfo.body
FROM place AS own
CROSS JOIN place ON place.file_id = own.id AND place.removed_at IS NULL
CROSS JOIN place AS nfo ON nfo.attached_to_place = place.id AND nfo.removed_at IS NULL
CROSS JOIN file AS nfo_file ON nfo_file.id = nfo.file_id AND nfo_file.sidecar_kind = 'nfo'
CROSS JOIN file_nfo ON file_nfo.file_id = nfo.file_id
WHERE own.removed_at IS NULL AND own.file_id = own.id
  AND own.id IN (SELECT copy.file_id FROM place AS copy WHERE copy.file_id <> copy.id AND copy.removed_at IS NULL)
ORDER BY own.id, place.id <> place.file_id, place.first_seen_at, place.id;
