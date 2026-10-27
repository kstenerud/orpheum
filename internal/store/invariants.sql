-- SPDX-License-Identifier: AGPL-3.0-or-later
-- persistent.db's invariants (D303, M7): each query names what must never be true and returns the
-- rows where it is, so a healthy database returns none. They run after every test that opens a
-- database through storetest, and on the running server at start and daily, which logs each one
-- broken as an error (cmd/orpheum's checkInvariants); the doctor is to run them too.
-- Each starts "-- invariant: <name>"; the text up to the next one is its query, which reads only.

-- invariant: no live version belongs to a removed title
SELECT version.id FROM version
JOIN item ON item.id = version.title_id
WHERE version.removed_at IS NULL AND item.removed_at IS NOT NULL AND EXISTS (
  SELECT 1 FROM file_filing CROSS JOIN file ON file.id = file_filing.file_id
  WHERE file_filing.version_id = version.id AND file.removed_at IS NULL);

-- invariant: every live title has a version
-- A pass makes a title for a file, and tombstones one it moves the last version out of or made and
-- did not use (identify's retireEmptied).
SELECT title.id FROM title
JOIN item ON item.id = title.id
WHERE item.removed_at IS NULL AND NOT EXISTS (SELECT 1 FROM version WHERE version.title_id = title.id);

-- invariant: every title and series is as new as its files say
-- found_at and found_file_modified_at, which newest first sorts by, are derived by step 0058's
-- triggers from the files; a write they missed leaves one out of date.
SELECT title.id FROM title
JOIN title_found ON title_found.title_id = title.id
WHERE title.found_at IS NOT title_found.found_at
  OR title.found_file_modified_at IS NOT title_found.found_file_modified_at
UNION ALL
SELECT collection.id FROM collection
JOIN collection_found ON collection_found.collection_id = collection.id
WHERE collection.found_at IS NOT collection_found.found_at
  OR collection.found_file_modified_at IS NOT collection_found.found_file_modified_at;
