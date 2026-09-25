-- SPDX-License-Identifier: GPL-3.0-or-later
-- D48's alternates column, which migration 0001 left out: a name folded by the one normaliser
-- in Go (diacritics removed, and the atomic letters no Unicode option decomposes, Ø Æ Œ ß Ð Ł,
-- spelled out), so "Solvi" finds "Sølvi". The word index and the trigram index both take it;
-- they are rebuilt, which is safe because both are derivable (class C). A row written before
-- this step has no alternates until the identification pass next writes it.
-- Additive for an older binary: it writes item_text without the column, and the triggers index
-- a NULL, so compatible_version stays where it is.

ALTER TABLE item_text ADD COLUMN search_alt TEXT;   -- class C: the normaliser's output (D48)

DROP TRIGGER item_text_ai;
DROP TRIGGER item_text_ad;
DROP TRIGGER item_text_au;
DROP TABLE item_fts;
DROP TABLE item_fts_tri;

CREATE VIRTUAL TABLE item_fts USING fts5(                 -- class C (D48)
  name, search_alt, overview, content='item_text', content_rowid='rowid',
  tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE item_fts_tri USING fts5(
  name, search_alt, content='item_text', content_rowid='rowid', tokenize='trigram'
);

CREATE TRIGGER item_text_ai AFTER INSERT ON item_text BEGIN
  INSERT INTO item_fts(rowid, name, search_alt, overview)
    VALUES (new.rowid, new.name, new.search_alt, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alt) VALUES (new.rowid, new.name, new.search_alt);
END;
CREATE TRIGGER item_text_ad AFTER DELETE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alt, overview)
    VALUES ('delete', old.rowid, old.name, old.search_alt, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alt)
    VALUES ('delete', old.rowid, old.name, old.search_alt);
END;
CREATE TRIGGER item_text_au AFTER UPDATE ON item_text BEGIN
  INSERT INTO item_fts(item_fts, rowid, name, search_alt, overview)
    VALUES ('delete', old.rowid, old.name, old.search_alt, old.overview);
  INSERT INTO item_fts_tri(item_fts_tri, rowid, name, search_alt)
    VALUES ('delete', old.rowid, old.name, old.search_alt);
  INSERT INTO item_fts(rowid, name, search_alt, overview)
    VALUES (new.rowid, new.name, new.search_alt, new.overview);
  INSERT INTO item_fts_tri(rowid, name, search_alt) VALUES (new.rowid, new.name, new.search_alt);
END;

INSERT INTO item_fts(item_fts) VALUES ('rebuild');
INSERT INTO item_fts_tri(item_fts_tri) VALUES ('rebuild');
