-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The server's own videos live in one hidden system library, found by its kind (D435 sections 1-3).
-- system_library names it: one row at most, keyed by the kind 'system', so the server finds it by
-- what it is and never by its name. The library itself is an other library (D349), whose videos are
-- read from NFO files alone and never looked up, as D435 section 4 has them.
--
-- version_library, which every list of titles, the search, Next Up and a reach of an item read, leaves
-- the system library out (D435 section 2); library_place keeps it, so its folder is scanned and filed
-- as any library's is.

CREATE TABLE system_library (                              -- class A: the server's own (D435)
  kind       TEXT NOT NULL PRIMARY KEY CHECK (kind IN ('system')),
  library_id INTEGER NOT NULL UNIQUE REFERENCES library(id) ON DELETE RESTRICT
) WITHOUT ROWID, STRICT;

DROP VIEW version_library;
CREATE VIEW version_library AS
SELECT file_filing.version_id, file_filing.file_id, library_place.place_id, library_place.library_id
FROM file_filing
JOIN library_place ON library_place.file_id = file_filing.file_id
  AND library_place.library_kind = file_filing.library_kind
WHERE file_filing.version_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM system_library WHERE system_library.library_id = library_place.library_id);
