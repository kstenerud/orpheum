-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Each film's and series' genres and people, as its source says them: the provider's stored
-- response or its NFO, whichever it follows (D183). People are items of kind 'person' with a
-- credit table keyed on item, so a film's cast and an album's artist are one table (D186). The
-- identification pass writes all four from what it already reads, with no new request (D170,
-- D182), and fills the titles identified before this step on its first sweep.

CREATE TABLE person (                                       -- class F, an item of kind 'person'
  id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE RESTRICT,
  -- A person is an item of kind 'person' (D303, M5): the pair names the item and its kind.
  item_kind   TEXT NOT NULL DEFAULT 'person' CHECK (item_kind = 'person'),
  -- Their id at TMDB, whose person ids are numbered apart from its films', so kept here and not in
  -- item_provider_id; NULL for one only an NFO names.
  tmdb_id     TEXT,
  -- Their name as the one normaliser folds it, which finds the person an NFO names again; the
  -- name itself is the item's text, which the full-text indexes read (D48).
  name_key    TEXT NOT NULL,
  -- Their picture as their source names it, a TMDB profile path or an NFO's thumb; not fetched.
  picture_ref TEXT,
  FOREIGN KEY (id, item_kind) REFERENCES item(id, kind) ON DELETE RESTRICT
) STRICT;
CREATE UNIQUE INDEX person_tmdb ON person(tmdb_id) WHERE tmdb_id IS NOT NULL;
CREATE INDEX person_name_key ON person(name_key);

CREATE TABLE genre (                                        -- class F
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,                 -- as its first source spelled it
  name_key    TEXT NOT NULL UNIQUE           -- folded (D48): "Sci-Fi" from two sources is one
) STRICT;

CREATE TABLE item_genre (                                   -- class C, derived by the pass
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  genre_id    INTEGER NOT NULL REFERENCES genre(id) ON DELETE RESTRICT,
  ordinal     INTEGER NOT NULL CHECK (ordinal >= 0),  -- its place in its source's list
  PRIMARY KEY (item_id, genre_id)
) WITHOUT ROWID, STRICT;
CREATE INDEX item_genre_genre ON item_genre(genre_id, item_id);

CREATE TABLE credit (                                       -- class C, derived by the pass
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  role        TEXT NOT NULL CHECK (role IN ('cast', 'director', 'writer')),
  ordinal     INTEGER NOT NULL CHECK (ordinal >= 0),  -- billing order within the role
  person_id   INTEGER NOT NULL REFERENCES person(id) ON DELETE RESTRICT,
  character   TEXT,                          -- a cast member's part; NULL for crew
  PRIMARY KEY (item_id, role, ordinal)
) WITHOUT ROWID, STRICT;
CREATE INDEX credit_person ON credit(person_id, item_id);
