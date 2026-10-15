-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D303 (the access audit's A1): an item's sort key is found by the item. The key was ordered for
-- listing, (locale, listing, collation_key, item_id), with no index by item, so replacing one item's
-- key, and joining every name to its key, read the whole locale: quadratic in the library. The
-- table is keyed by (item_id, locale), one key per item in each locale, and the listing order is an
-- index, which carries the primary key and so covers every listing.

CREATE TABLE sort_key_by_item (                            -- class C (D47)
  item_id       INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
  locale        TEXT NOT NULL,
  listing       TEXT NOT NULL,           -- the list it sorts in: 'movie', 'series', 'album'…
  collation_key BLOB NOT NULL,           -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (item_id, locale)
) WITHOUT ROWID;
INSERT INTO sort_key_by_item (item_id, locale, listing, collation_key)
  SELECT item_id, locale, listing, collation_key FROM sort_key GROUP BY item_id, locale;
DROP TABLE sort_key;
ALTER TABLE sort_key_by_item RENAME TO sort_key;
CREATE INDEX sort_key_listing ON sort_key(locale, listing, collation_key, item_id);
