-- SPDX-License-Identifier: GPL-3.0-or-later
-- What the scanner saw (schema.md, Files; D163, D173, D179): one row per observed path.

-- name: GetLibraryForRoot :one
SELECT library_root.library_id
FROM library_root
JOIN library ON library.id = library_root.library_id
WHERE library_root.node_id = ? AND library_root.path = ? AND library.removed_at IS NULL
ORDER BY library_root.library_id
LIMIT 1;

-- name: GetLiveFileByKey :one
-- A live file by the bytes its node reported (D211): two names that are one NFC text are two
-- files.
SELECT id, payload_hash, last_seen_at
FROM file
WHERE node_id = sqlc.arg(node_id) AND reported_path_bytes = sqlc.arg(reported_path_bytes) AND removed_at IS NULL;

-- name: ListLiveFilesByHash :many
SELECT id, path, last_seen_at
FROM file
WHERE node_id = ? AND payload_hash = ? AND removed_at IS NULL
ORDER BY id;

-- name: InsertFile :one
INSERT INTO file (
  library_id, node_id, path, reported_path_bytes, size_bytes, mtime_ms, payload_hash, payload_bytes,
  filing, filing_reason, error_detail_json, probe_json, first_seen_at, last_seen_at, opensubtitles_hash
) VALUES (
  sqlc.arg(library_id), sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(reported_path_bytes), sqlc.arg(size_bytes),
  sqlc.arg(mtime_ms), sqlc.arg(payload_hash), sqlc.arg(payload_bytes), sqlc.arg(filing),
  sqlc.arg(filing_reason), sqlc.arg(error_detail_json), sqlc.arg(probe_json), sqlc.arg(seen_at), sqlc.arg(seen_at),
  sqlc.narg(opensubtitles_hash)
)
RETURNING id;

-- name: UpdateObservedFile :exec
-- A file observed again, or moved to path. A filing the core gave (listed, extra, attached,
-- unidentified) stays; one from a skip or an error becomes unidentified, now that it was read.
UPDATE file SET
  library_id = sqlc.arg(library_id),
  path = sqlc.arg(path),
  reported_path_bytes = sqlc.arg(reported_path_bytes),
  size_bytes = sqlc.arg(size_bytes),
  mtime_ms = sqlc.arg(mtime_ms),
  payload_hash = sqlc.arg(payload_hash),
  payload_bytes = sqlc.arg(payload_bytes),
  is_identity_confirmed = 1,
  filing = CASE WHEN filing IN ('skipped', 'errored') THEN 'unidentified' ELSE filing END,
  filing_reason = CASE WHEN filing IN ('skipped', 'errored') THEN sqlc.arg(unidentified_reason)
                  ELSE filing_reason END,
  error_detail_json = NULL,
  probe_json = sqlc.arg(probe_json),
  opensubtitles_hash = sqlc.narg(opensubtitles_hash),
  last_seen_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id);

-- name: UpdateFileFiling :exec
-- A path the node skipped or could not read: its filing says why, and it was seen.
UPDATE file SET
  filing = sqlc.arg(filing),
  filing_reason = sqlc.arg(filing_reason),
  error_detail_json = sqlc.arg(error_detail_json),
  last_seen_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesOfLibrary :many
SELECT id, path, last_seen_at
FROM file
WHERE node_id = ? AND library_id = ? AND removed_at IS NULL
ORDER BY id;

-- name: GetLatestTombstoneByHash :one
-- The file a returning payload revives (D208): the latest removed in the library.
SELECT id
FROM file
WHERE library_id = ? AND payload_hash = ? AND removed_at IS NOT NULL
ORDER BY removed_at DESC, id DESC
LIMIT 1;

-- name: ReviveFile :exec
UPDATE file SET removed_at = NULL WHERE id = ?;

-- name: TombstoneFile :exec
UPDATE file SET removed_at = sqlc.arg(removed_at) WHERE id = sqlc.arg(id);

-- name: UpsertRootScan :exec
INSERT INTO root_scan (node_id, root_path, completed_at)
VALUES (sqlc.arg(node_id), sqlc.arg(root_path), sqlc.arg(completed_at))
ON CONFLICT (node_id, root_path) DO UPDATE SET completed_at = excluded.completed_at;

-- name: DeleteFileStreams :exec
DELETE FROM stream WHERE bytes_file = ?;

-- name: UpsertStream :exec
-- A video's own track, by its stream index. A row keeps its id for as long as its file has the
-- index, so an extraction job or a playback's subtitle address that names it holds across scans.
INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, lang_raw, lang, title_raw, disposition_flags, channel_count)
VALUES (
  sqlc.arg(file_id), sqlc.arg(file_id), sqlc.arg(stream_index), sqlc.arg(kind), sqlc.arg(codec),
  sqlc.arg(lang_raw), sqlc.arg(lang), sqlc.arg(title_raw), sqlc.arg(disposition_flags), sqlc.arg(channel_count)
)
ON CONFLICT (bytes_file, stream_index) DO UPDATE SET
  video_file = excluded.video_file, kind = excluded.kind, codec = excluded.codec, lang_raw = excluded.lang_raw,
  lang = excluded.lang, title_raw = excluded.title_raw, disposition_flags = excluded.disposition_flags,
  channel_count = excluded.channel_count;

-- name: DeleteFileStreamsNotIn :exec
-- A video's own tracks whose stream index its probe no longer has.
DELETE FROM stream
WHERE bytes_file = sqlc.arg(file_id) AND stream_index IS NOT NULL
  AND stream_index NOT IN (SELECT value FROM json_each(CAST(sqlc.arg(kept_indexes) AS TEXT)));

-- name: ListLiveFilesByNode :many
SELECT id, library_id, path, reported_path_bytes, size_bytes, payload_hash, payload_bytes, filing, filing_reason,
  error_detail_json, first_seen_at, last_seen_at
FROM file
WHERE node_id = ? AND removed_at IS NULL
ORDER BY path;

-- name: ListStreamsOfFile :many
SELECT id, stream_index, kind, codec, lang_raw, lang, title_raw, disposition_flags, channel_count
FROM stream
WHERE bytes_file = ?
ORDER BY stream_index;

-- name: InsertSidecarFile :one
-- A subtitle carries its payload hash, the file hashed whole; an NFO or an image has none. Only
-- an NFO's bytes are kept, in nfo_body.
INSERT INTO file (
  library_id, node_id, path, reported_path_bytes, size_bytes, mtime_ms, payload_hash, payload_bytes, filing,
  filing_reason, sidecar_kind, nfo_body, first_seen_at, last_seen_at
) VALUES (
  sqlc.arg(library_id), sqlc.arg(node_id), sqlc.arg(path), sqlc.arg(reported_path_bytes), sqlc.arg(size_bytes),
  sqlc.arg(mtime_ms), sqlc.arg(payload_hash), sqlc.arg(payload_bytes), 'attached', sqlc.arg(filing_reason),
  sqlc.arg(sidecar_kind), sqlc.arg(nfo_body), sqlc.arg(seen_at), sqlc.arg(seen_at)
)
RETURNING id;

-- name: UpdateSidecarFile :exec
UPDATE file SET
  library_id = sqlc.arg(library_id), size_bytes = sqlc.arg(size_bytes),
  mtime_ms = sqlc.arg(mtime_ms), payload_hash = sqlc.arg(payload_hash), payload_bytes = sqlc.arg(payload_bytes),
  filing = 'attached', sidecar_kind = sqlc.arg(sidecar_kind), filing_reason = sqlc.arg(filing_reason),
  error_detail_json = NULL,
  nfo_body = sqlc.arg(nfo_body),
  last_seen_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id);

-- name: ListLiveFilesForAttaching :many
SELECT id, path, filing, sidecar_kind, payload_hash, attached_to_file
FROM file
WHERE node_id = ? AND library_id = ? AND removed_at IS NULL
ORDER BY path;

-- name: SetFileAttachedTo :exec
UPDATE file SET attached_to_file = sqlc.arg(attached_to_file) WHERE id = sqlc.arg(id);

-- name: GetNFOBody :one
SELECT nfo_body FROM file WHERE id = ?;

-- name: GetPlayableFile :one
-- A live file a person may play: a video the scan filed, whatever it was identified as. What a
-- scan skipped, could not read or attached to another is not played.
SELECT id, node_id, library_id, reported_path_bytes, payload_hash, probe_json, size_bytes
FROM file
WHERE id = ? AND removed_at IS NULL AND filing IN ('listed', 'extra', 'unidentified');

-- name: ListLibraryRootsOnNode :many
-- The roots a library has on a node, longest first, so the root a path is under is the first
-- that is a prefix of it.
SELECT path
FROM library_root
WHERE library_id = ? AND node_id = ?
ORDER BY length(path) DESC, path;

-- name: ListSubtitleTracksOfVideo :many
-- A video's subtitle tracks, its own and those of the live sidecar files attached to it, with the
-- payload hash of the file each track's bytes are in: what a playback offers.
SELECT s.id, s.stream_index, s.codec, s.lang, s.title_raw, s.disposition_flags, f.payload_hash AS bytes_payload_hash
FROM stream s
JOIN file f ON f.id = s.bytes_file AND f.removed_at IS NULL
WHERE s.video_file = sqlc.arg(video_file) AND s.kind = 'subtitle'
ORDER BY s.bytes_file <> s.video_file, s.stream_index, f.path;

-- name: GetSubtitleStream :one
-- A subtitle track, and the live file its bytes are in, for the job that extracts it.
SELECT s.id, s.video_file, s.stream_index, s.codec, f.library_id, f.node_id, f.reported_path_bytes, f.payload_hash
FROM stream s
JOIN file f ON f.id = s.bytes_file AND f.removed_at IS NULL
WHERE s.id = sqlc.arg(id) AND s.kind = 'subtitle';

-- name: GetSidecarStream :one
-- A sidecar's one stream: a whole-file track has no stream index (schema.md, stream).
SELECT id, video_file, codec, lang_raw, lang, disposition_flags
FROM stream
WHERE bytes_file = sqlc.arg(bytes_file) AND stream_index IS NULL;

-- name: InsertSidecarStream :exec
INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, lang_raw, lang, disposition_flags)
VALUES (sqlc.arg(bytes_file), sqlc.arg(video_file), NULL, 'subtitle', sqlc.arg(codec), sqlc.arg(lang_raw),
  sqlc.arg(lang), sqlc.arg(disposition_flags));

-- name: UpdateSidecarStream :exec
UPDATE stream SET video_file = sqlc.arg(video_file), codec = sqlc.arg(codec), lang_raw = sqlc.arg(lang_raw),
  lang = sqlc.arg(lang), disposition_flags = sqlc.arg(disposition_flags)
WHERE id = sqlc.arg(id);

-- name: GetFilePath :one
-- A file's path, removed or not, to name it where it was worked on (the System page, D225).
SELECT path FROM file WHERE id = ?;

-- name: GetFileForSubtitleSearch :one
-- A live video, what finds subtitles made for it (D224 section 3): its OpenSubtitles hash, and the
-- title it is a version of, if it is identified.
SELECT f.id, f.opensubtitles_hash, v.title_id, t.kind AS title_kind
FROM file f
LEFT JOIN version v ON v.id = f.version_id AND v.removed_at IS NULL
LEFT JOIN title t ON t.id = v.title_id
WHERE f.id = sqlc.arg(id) AND f.removed_at IS NULL;
