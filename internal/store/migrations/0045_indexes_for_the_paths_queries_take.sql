-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D303 (DL1, DL2, DL4; the access audit's A6): an index for each path a query takes that none
-- served, each of which read a whole table, several once per file of a pass.

-- The file a returning payload revives: removed files by library and payload, the newest first.
-- file_payload is partial on live files, so it cannot serve them (sql.md section 13, DL4).
CREATE INDEX file_removed_payload ON file(library_id, payload_hash, removed_at) WHERE removed_at IS NOT NULL;
-- A library's live files, by path: the scanner view, its counts, and a pass's files.
CREATE INDEX file_library ON file(library_id, path) WHERE removed_at IS NULL;
-- The files scans could not read, which the System page lists.
CREATE INDEX file_errored ON file(library_id, path) WHERE filing = 'errored' AND removed_at IS NULL;
-- The series a title is an episode of: a played file's work.
CREATE INDEX collection_member_title ON collection_member(member_title) WHERE member_title IS NOT NULL;
-- A device's sessions and secret, which removing it revokes.
CREATE INDEX grant_device ON "grant"(device_id) WHERE device_id IS NOT NULL;
-- A title's versions, removed ones too, which a merge moves: version_title holds the live alone.
DROP INDEX version_title;
CREATE INDEX version_title ON version(title_id);
