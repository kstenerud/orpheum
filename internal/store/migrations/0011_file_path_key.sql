-- SPDX-License-Identifier: GPL-3.0-or-later
-- A file is known by the bytes its node reported, not by its NFC text (D211 section 1). Two
-- files whose names were different invalid UTF-8 both became the same text with U+FFFD in it,
-- as did two names differing only in normalisation, and the unique index on the text let the
-- library keep one of each pair. path_key is the reported path's bytes, always set; path stays
-- the NFC text for display and search, and path_raw the bytes where they differ from it.
-- The live-file index moves from the text to the key, which an older binary's lookups by text
-- would no longer be unique under, so the step raises compatible_version. Nothing is released.
-- orpheum:raises-compatible-version

ALTER TABLE file ADD COLUMN path_key BLOB;   -- class I: the reported bytes (D211)
UPDATE file SET path_key = coalesce(path_raw, CAST(path AS BLOB)) WHERE path_key IS NULL;

DROP INDEX file_live_path;
CREATE UNIQUE INDEX file_live_path ON file(node_id, path_key) WHERE removed_at IS NULL;
