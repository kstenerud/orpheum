-- SPDX-License-Identifier: AGPL-3.0-or-later
-- One view says which library covers a place (D426 section 4). Step 0060 wrote the same coverage
-- three times: place_library read it from the place outward, library_place from the folder inward,
-- and version_library from a version's files, each with its own CROSS JOIN order and its own test of
-- the path, a prefix in two and a range in one. library_place is now the one: each live place and
-- each live library whose live folder covers it, by the range of the bytes the place's node reported
-- that file_live_path serves (D211): after the folder's path and a slash, before its path and the
-- character after the slash; a folder of "/" holds every place on its node. Its joins are plain, so
-- the planner reads it from whichever end a query binds: a place, by its id, or a library, by its
-- folders. place_library goes, its queries read library_place, and version_library is what
-- library_place says of a version's files, the coverage no longer written out again.

DROP VIEW version_library;
DROP VIEW place_library;
DROP VIEW library_place;

CREATE VIEW library_place AS
SELECT library_root.library_id, library.kind AS library_kind, library_root.node_id, library_root.root_path,
  place.id AS place_id, place.copy_of, coalesce(place.copy_of, place.id) AS file_id, place.path,
  place.reported_path_raw, place.filing, place.first_seen_at
FROM file AS place
JOIN library_root ON library_root.node_id = place.node_id AND library_root.retired_at IS NULL
JOIN library ON library.id = library_root.library_id AND library.removed_at IS NULL
WHERE place.removed_at IS NULL
  AND place.reported_path_raw > CAST(CASE WHEN library_root.root_path = '/' THEN '/'
    ELSE library_root.root_path || '/' END AS BLOB)
  AND place.reported_path_raw < CAST(CASE WHEN library_root.root_path = '/' THEN '0'
    ELSE library_root.root_path || '0' END AS BLOB);

-- Each version and each library that lists it: one of its kind covering a live place of a live file
-- it holds (D363 sections 4, 5), as library_place says. An item two folders, two libraries or two
-- copies reach is one row per library, and a list asks only whether one exists (section 6).
CREATE VIEW version_library AS
SELECT file_filing.version_id, file_filing.file_id, library_place.place_id, library_place.library_id
FROM file_filing
JOIN library_place ON (library_place.place_id = file_filing.file_id OR library_place.copy_of = file_filing.file_id)
  AND library_place.library_kind = file_filing.library_kind
WHERE file_filing.version_id IS NOT NULL;
