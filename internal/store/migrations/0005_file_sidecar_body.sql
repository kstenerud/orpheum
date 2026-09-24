-- SPDX-License-Identifier: GPL-3.0-or-later
-- An NFO's bytes, as the node sent them (identification.md, Reading the sidecars). Identification
-- runs in the core, which never opens a file, and re-derives a title from stored input only
-- (D182), so the input is stored. Only NFO sidecars carry it; a subtitle sidecar's bytes are
-- read from the node when it is played.
-- Additive: an older binary ignores the column, so compatible_version stays where it is.

ALTER TABLE file ADD COLUMN sidecar_body BLOB;   -- class O: a rescan sends it again
