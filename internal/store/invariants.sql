-- SPDX-License-Identifier: AGPL-3.0-or-later
-- persistent.db's invariants (D303, M7): each query names what must never be true and returns the
-- rows where it is, so a healthy database returns none. They run after every test that opens a
-- database through storetest; the doctor and a nightly check are to run them too (D303 phase D).
-- Each starts "-- invariant: <name>"; the text up to the next one is its query, which reads only.

-- invariant: no live version belongs to a removed title
SELECT version.id FROM version
JOIN item ON item.id = version.title_id
WHERE version.removed_at IS NULL AND item.removed_at IS NOT NULL AND EXISTS (
  SELECT 1 FROM file WHERE file.version_id = version.id AND file.removed_at IS NULL);
