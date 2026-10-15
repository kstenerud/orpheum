-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D303 (the access audit's A2; the independent audit's S12): an item's text, and the two full-text
-- indexes over it, are written only when the text changed. The update trigger fired for every row
-- an upsert touched, changed or not, deleting and re-inserting it in both indexes, the trigram one
-- included: four index writes per item on every pass. Each index's trigger now fires only when a
-- column it indexes really changed (SQLite fires an UPDATE trigger whenever a row is written,
-- whatever its values). And the indexes follow item_text's own key, id, instead of an implicit
-- rowid, which SQLite allows VACUUM to renumber.

CREATE TABLE item_text_keyed (                             -- class O/F, resolved (D45)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,  -- the full-text indexes' key for the row
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  language    TEXT NOT NULL,
  is_original INTEGER NOT NULL DEFAULT 0 CHECK (is_original IN (0,1)),
  name        TEXT NOT NULL,
  overview    TEXT,
  tagline     TEXT,
  -- class C: the name folded by the one normaliser in Go (diacritics removed, and the atomic
  -- letters no Unicode option decomposes, such as Ø and ß, spelled out), so "Bjork" finds
  -- "Bjørk" (D48).
  search_alternates TEXT,
  UNIQUE (item_id, language)
);
INSERT INTO item_text_keyed (id, item_id, language, is_original, name, overview, tagline, search_alternates)
  SELECT rowid, item_id, language, is_original, name, overview, tagline, search_alternates FROM item_text;
DROP TRIGGER item_text_ai;
DROP TRIGGER item_text_ad;
DROP TRIGGER item_text_au;
DROP TABLE item_fts;
DROP TABLE item_fts_tri;
DROP TABLE item_text;
ALTER TABLE item_text_keyed RENAME TO item_text;
CREATE UNIQUE INDEX item_text_original ON item_text(item_id) WHERE is_original = 1;

CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alternates, overview, content='item_text', content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, search_alternates, content='item_text', content_rowid='id', tokenize='trigram'
);
INSERT INTO item_fts(item_fts) VALUES ('rebuild');
INSERT INTO item_fts_tri(item_fts_tri) VALUES ('rebuild');

CREATE TRIGGER item_text_ai AFTER INSERT ON item_text BEGIN
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.id, new.name, new.search_alternates, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.id, new.name, new.search_alternates);
END;
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.id, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.id, old.name, old.search_alternates);
END;
CREATE TRIGGER item_text_au AFTER UPDATE OF name, search_alternates, overview ON item_text
WHEN old.name IS NOT new.name OR old.search_alternates IS NOT new.search_alternates
  OR old.overview IS NOT new.overview BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alternates, overview)
    VALUES ('delete', old.id, old.name, old.search_alternates, old.overview);
  INSERT INTO item_fts(rowid, name, search_alternates, overview)
    VALUES (new.id, new.name, new.search_alternates, new.overview);
END;
CREATE TRIGGER item_text_au_trigram AFTER UPDATE OF name, search_alternates ON item_text
WHEN old.name IS NOT new.name OR old.search_alternates IS NOT new.search_alternates BEGIN
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alternates)
    VALUES ('delete', old.id, old.name, old.search_alternates);
  INSERT INTO item_fts_tri(rowid, name, search_alternates)
    VALUES (new.id, new.name, new.search_alternates);
END;
