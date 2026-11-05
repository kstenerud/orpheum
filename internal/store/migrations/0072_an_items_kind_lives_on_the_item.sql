-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- An item's kind, and the columns every kind shares, live on the item (D426 section 2). item.kind
-- held only a class, title, collection or person, and the kind itself sat on the subtype, which
-- kept its own copy of the year, the release date, the followed source, the identification state
-- and when it was found: one list query and one index for each kind. Now item.kind is the API's
-- word (D416: movie, series, episode, extra, other, person) and the item carries the shared
-- columns. A title, a collection and a person keep a kind limited to their own, tied to the item's
-- by (id, kind), so the two cannot differ; a kind changed on the item follows to the subtype and
-- to the sort key, whose listing is the item's kind (audit 1).
--
-- Every row carries over, with the item's id sequence. The collections written were all series,
-- so a collection's kind is series alone; a box set returns with its feature. A series not yet
-- resolved held no identification state, which every resolution read as unidentified; it is
-- unidentified now, since every kind but a person is identified (D426 section 10). A release date
-- of another year than the item's, which released.go never writes, is dropped; the next pass
-- dates it again. The found_at triggers and their views become one view, item_found, and the
-- triggers that wrote a title or a series write the item; the two that ran on a new title or
-- series go, since a new one has no version or member to be found by (step 0058's machinery).

DROP TRIGGER file_filing_found_ad;
DROP TRIGGER file_filing_found_ai;
DROP TRIGGER file_filing_found_au;
DROP TRIGGER file_found_au;
DROP TRIGGER version_found_ai;
DROP TRIGGER version_found_au;
DROP TRIGGER collection_member_found_ad;
DROP TRIGGER collection_member_found_ai;
DROP TRIGGER collection_member_found_au;
DROP VIEW title_found;
DROP VIEW collection_found;

CREATE TEMP TABLE item_held AS SELECT * FROM item;
CREATE TEMP TABLE item_sequence_held AS SELECT seq FROM sqlite_sequence WHERE name = 'item';
CREATE TEMP TABLE title_held AS SELECT * FROM title;
CREATE TEMP TABLE collection_held AS SELECT * FROM collection;
CREATE TEMP TABLE person_held AS SELECT * FROM person;
CREATE TEMP TABLE sort_key_held AS SELECT * FROM sort_key;
DROP TABLE sort_key;
DROP TABLE person;
DROP TABLE collection;
DROP TABLE title;
DROP TABLE item;

CREATE TABLE item (                                        -- class I, and O/F/A by column
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  -- What it is, in the API's words (D416, D426 section 2); a title, a collection or a person names
  -- it again, tied here by (id, kind).
  kind        TEXT NOT NULL CHECK (kind IN ('movie','series','episode','extra','other','person')),
  filing_library_id  INTEGER REFERENCES library(id) ON DELETE RESTRICT,
  merged_into INTEGER REFERENCES item(id) ON DELETE RESTRICT,  -- D173
  added_at    INTEGER NOT NULL,
  removed_at  INTEGER,
  -- For shows, the series is what is identified (D209): it carries the year its search is keyed on
  -- and the match state a film carries.
  year        INTEGER,                                    -- O: resolved
  -- The day it was released as yyyymmdd, of its year (D366, released.go); NULL undated.
  released_on INTEGER CHECK (released_on IS NULL OR (released_on >= 10000 AND released_on / 10000 = year)),  -- F/A (D172)
  followed_source    TEXT CHECK (followed_source IN ('nfo','tmdb','tvdb','none')),  -- F/A
  followed_source_id TEXT,                                                          -- F/A
  -- Every kind but a person is identified.
  identification_state TEXT CHECK (identification_state IN
                   ('identified','ambiguous','unidentified','confirmed')),
  -- When it was found, derived by the triggers below as item_found defines it (step 0058), class C:
  -- a work's first-found live file's first_seen_at and that file's modified_at, a series' newest
  -- episode's; 0 with none. A new item has none, so each starts at 0 and the triggers move it.
  found_at               INTEGER NOT NULL DEFAULT 0,
  found_file_modified_at INTEGER NOT NULL DEFAULT 0,
  CHECK (followed_source_id IS NULL OR followed_source IS NOT NULL),  -- an id is an id at a source
  CHECK ((kind = 'person') = (identification_state IS NULL)),
  UNIQUE (id, kind)                      -- a subtype row names its item and the item's kind (D303, M5)
) STRICT;
INSERT INTO item (id, kind, filing_library_id, merged_into, added_at, removed_at, year, released_on,
  followed_source, followed_source_id, identification_state, found_at, found_file_modified_at)
SELECT item_held.id,
  CASE item_held.kind WHEN 'title' THEN title_held.kind WHEN 'collection' THEN collection_held.kind ELSE 'person' END,
  item_held.filing_library_id, item_held.merged_into, item_held.added_at, item_held.removed_at,
  coalesce(title_held.year, collection_held.year),
  CASE WHEN coalesce(title_held.released_on, collection_held.released_on) / 10000
    = coalesce(title_held.year, collection_held.year)
    THEN coalesce(title_held.released_on, collection_held.released_on) END,
  coalesce(title_held.followed_source, collection_held.followed_source),
  coalesce(title_held.followed_source_id, collection_held.followed_source_id),
  CASE WHEN item_held.kind = 'person' THEN NULL
    ELSE coalesce(title_held.identification_state, collection_held.identification_state, 'unidentified') END,
  coalesce(title_held.found_at, collection_held.found_at, 0),
  coalesce(title_held.found_file_modified_at, collection_held.found_file_modified_at, 0)
FROM item_held
LEFT JOIN title_held ON title_held.id = item_held.id
LEFT JOIN collection_held ON collection_held.id = item_held.id;
DELETE FROM sqlite_sequence WHERE name = 'item';
INSERT INTO sqlite_sequence (name, seq) SELECT 'item', seq FROM item_sequence_held;
CREATE INDEX item_filing_library ON item(filing_library_id, kind) WHERE removed_at IS NULL;
-- Newest first, of one kind (D366, step 0058): one index for every list, read from its top or its foot.
CREATE INDEX item_kind_found ON item(kind, found_at, found_file_modified_at, id);
-- By release date, of one kind (D366 section 1).
CREATE INDEX item_kind_released ON item(kind, released_on);
-- The chooser's list, of the one state it is given (audit 8): its state is a bound value, so no
-- partial index below can serve it.
CREATE INDEX item_identification ON item(identification_state, followed_source, kind);
-- The few the waiting count reads, of every kind it counts, as title_ambiguous and title_waiting
-- held a title's (D303, DL4): most items are identified, so a full index says nothing of them. Each
-- is keyed by the columns its count binds, as item_identification is, so the planner, weighing the
-- same terms, takes the index of the few rows over the full one.
CREATE INDEX item_ambiguous ON item(identification_state, kind) WHERE identification_state = 'ambiguous';
CREATE INDEX item_waiting ON item(identification_state, followed_source, kind)
  WHERE identification_state = 'unidentified' AND followed_source = 'none' AND kind != 'episode';

CREATE TABLE title (                                       -- mixed: see the columns
  id             INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  -- A title is a movie, an episode, an extra or an other video, as its item says (D426 section 2).
  kind           TEXT NOT NULL CHECK (kind IN ('movie','episode','extra','other')),
  runtime_ms     INTEGER,                                    -- O: resolved
  FOREIGN KEY (id, kind) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE RESTRICT
) STRICT;
INSERT INTO title (id, kind, runtime_ms) SELECT id, kind, runtime_ms FROM title_held;

CREATE TABLE collection (                                  -- class A or O
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  -- A collection is a series, as its item says (D426 section 2); a box set returns with its feature.
  kind        TEXT NOT NULL CHECK (kind = 'series'),
  created_by  TEXT NOT NULL CHECK (created_by IN ('scanner','provider','user')),
  FOREIGN KEY (id, kind) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE RESTRICT
) STRICT;
INSERT INTO collection (id, kind, created_by) SELECT id, kind, created_by FROM collection_held;

CREATE TABLE person (                                       -- class F, an item of kind 'person'
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  kind        TEXT NOT NULL CHECK (kind = 'person'),       -- the pair names the item and its kind
  -- Their id at TMDB, whose person ids are numbered apart from its films', so kept here and not in
  -- item_provider_id; NULL for one only an NFO names.
  tmdb_id     TEXT,
  -- Their name as the one normaliser folds it, which finds the person an NFO names again; the
  -- name itself is the item's text, which the full-text indexes read (D48).
  name_key    TEXT NOT NULL,
  -- Their picture as their source names it, a TMDB profile path or an NFO's thumb; not fetched.
  picture_ref TEXT,
  FOREIGN KEY (id, kind) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE RESTRICT
) STRICT;
INSERT INTO person (id, kind, tmdb_id, name_key, picture_ref)
SELECT id, 'person', tmdb_id, name_key, picture_ref FROM person_held;
CREATE INDEX person_name_key ON person(name_key);
CREATE UNIQUE INDEX person_tmdb ON person(tmdb_id) WHERE tmdb_id IS NOT NULL;

CREATE TABLE sort_key (                                     -- class C (D47)
  item_id       INTEGER NOT NULL,
  locale       TEXT NOT NULL,
  listing       TEXT NOT NULL,           -- the list it sorts in: its item's kind (audit 1)
  collation_key BLOB NOT NULL,           -- CLDR sort-key bytes, BINARY order
  PRIMARY KEY (item_id, locale),
  FOREIGN KEY (item_id, listing) REFERENCES item(id, kind) ON UPDATE CASCADE ON DELETE CASCADE
) WITHOUT ROWID, STRICT;
-- A key whose listing is not its item's kind was a stale cache; the next pass keys it again.
INSERT INTO sort_key (item_id, locale, listing, collation_key)
SELECT sort_key_held.item_id, sort_key_held.locale, sort_key_held.listing, sort_key_held.collation_key
FROM sort_key_held JOIN item ON item.id = sort_key_held.item_id AND item.kind = sort_key_held.listing;
CREATE INDEX sort_key_listing ON sort_key(locale, listing, collation_key, item_id);
-- Its key to the item, (item_id, listing), is looked up as the item goes or changes its kind.
CREATE INDEX sort_key_item ON sort_key(item_id, listing);

DROP TABLE sort_key_held;
DROP TABLE person_held;
DROP TABLE collection_held;
DROP TABLE title_held;
DROP TABLE item_sequence_held;
DROP TABLE item_held;

-- What an item's found_at is derived as: the one definition the triggers and the invariant read.
-- A series is as new as its newest episode; a work as the scan that first found its first live file.
CREATE VIEW item_found AS
SELECT item.id AS item_id,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_at FROM collection_member CROSS JOIN item AS member ON member.id = collection_member.member_title
    WHERE collection_member.collection_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT file.first_seen_at FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file ON file.id = file_filing.file_id
    WHERE version.title_id = item.id AND version.removed_at IS NULL AND file.removed_at IS NULL
    ORDER BY file.first_seen_at, coalesce(file.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_at,
  CASE WHEN item.kind = 'series' THEN coalesce((
    SELECT member.found_file_modified_at FROM collection_member
    CROSS JOIN item AS member ON member.id = collection_member.member_title
    WHERE collection_member.collection_id = item.id
    ORDER BY member.found_at DESC, member.found_file_modified_at DESC LIMIT 1
  ), 0) ELSE coalesce((
    SELECT coalesce(file.modified_at, 0) FROM version
    CROSS JOIN file_filing ON file_filing.version_id = version.id
    CROSS JOIN file ON file.id = file_filing.file_id
    WHERE version.title_id = item.id AND version.removed_at IS NULL AND file.removed_at IS NULL
    ORDER BY file.first_seen_at, coalesce(file.modified_at, 0) DESC LIMIT 1
  ), 0) END AS found_file_modified_at
FROM item;

CREATE TRIGGER file_filing_found_ad AFTER DELETE ON file_filing WHEN old.version_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT version.title_id FROM version WHERE version.id = old.version_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER file_filing_found_ai AFTER INSERT ON file_filing WHEN new.version_id IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = (SELECT version.title_id FROM version WHERE version.id = new.version_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER file_filing_found_au AFTER UPDATE OF version_id ON file_filing
WHEN old.version_id IS NOT new.version_id BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (SELECT version.title_id FROM version WHERE version.id IN (old.version_id, new.version_id))
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER file_found_au AFTER UPDATE OF removed_at, modified_at ON file
WHEN old.removed_at IS NOT new.removed_at OR old.modified_at IS NOT new.modified_at BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT version.title_id FROM file_filing CROSS JOIN version ON version.id = file_filing.version_id
      WHERE file_filing.file_id = new.id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER version_found_ai AFTER INSERT ON version BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = new.title_id
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER version_found_au AFTER UPDATE OF title_id, removed_at ON version
WHEN old.title_id IS NOT new.title_id OR old.removed_at IS NOT new.removed_at BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (old.title_id, new.title_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
-- A work found anew makes the series it is in as new as it (a series' members are works).
CREATE TRIGGER item_found_au AFTER UPDATE OF found_at, found_file_modified_at ON item
WHEN old.found_at IS NOT new.found_at OR old.found_file_modified_at IS NOT new.found_file_modified_at BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (
      SELECT collection_member.collection_id FROM collection_member WHERE collection_member.member_title = new.id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER collection_member_found_ad AFTER DELETE ON collection_member WHEN old.member_title IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = old.collection_id
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER collection_member_found_ai AFTER INSERT ON collection_member WHEN new.member_title IS NOT NULL BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id = new.collection_id
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
CREATE TRIGGER collection_member_found_au AFTER UPDATE OF collection_id, member_title ON collection_member
WHEN old.collection_id IS NOT new.collection_id OR old.member_title IS NOT new.member_title BEGIN
  UPDATE item SET
    found_at = (SELECT derived.found_at FROM item_found AS derived WHERE derived.item_id = item.id),
    found_file_modified_at = (
      SELECT derived.found_file_modified_at FROM item_found AS derived WHERE derived.item_id = item.id)
  WHERE item.id IN (old.collection_id, new.collection_id)
    AND EXISTS (
      SELECT 1 FROM item_found AS derived WHERE derived.item_id = item.id
        AND (item.found_at IS NOT derived.found_at
          OR item.found_file_modified_at IS NOT derived.found_file_modified_at));
END;
