-- SPDX-License-Identifier: AGPL-3.0-or-later
-- persistent.db's invariants (D303, M7): each query names what must never be true and returns the
-- rows where it is, so a healthy database returns none. They run after every test that opens a
-- database through storetest; the doctor and a nightly check are to run them too (D303 phase D).
-- Each starts "-- invariant: <name>"; the text up to the next one is its query, which reads only.

-- invariant: no item has two sort keys in one locale
SELECT item_id FROM sort_key GROUP BY item_id, locale HAVING count(*) > 1;

-- invariant: no live version belongs to a removed title
SELECT version.id FROM version
JOIN item ON item.id = version.title_id
WHERE version.removed_at IS NULL AND item.removed_at IS NOT NULL AND EXISTS (
  SELECT 1 FROM file WHERE file.version_id = version.id AND file.removed_at IS NULL);

-- invariant: a title's subtype row is a title item, and a collection's a collection item
SELECT title.id FROM title JOIN item ON item.id = title.id WHERE item.kind != 'title'
UNION ALL
SELECT collection.id FROM collection JOIN item ON item.id = collection.id WHERE item.kind != 'collection';
