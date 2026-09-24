-- SPDX-License-Identifier: GPL-3.0-or-later
-- For shows, the series is what is identified (D209). A series carries the year its search is
-- keyed on and the match state a title carries, and a stored response belongs to any item, not
-- only a title, as D186 says of every table that describes a thing. A series' response has
-- parts: '' the show, and 'season/1' and on its season listings, from which its episodes take
-- their values. A film's response is its '' part.
-- A rebuild renames provider_response.title_id to item_id, which an older binary's queries name,
-- so the step raises compatible_version. Nothing is released yet, so the floor costs no one.
-- orpheum:raises-compatible-version

ALTER TABLE collection ADD COLUMN year INTEGER;                                                 -- O: resolved
ALTER TABLE collection ADD COLUMN source TEXT CHECK (source IN ('nfo','tmdb','tvdb','none'));   -- F/A
ALTER TABLE collection ADD COLUMN source_id TEXT;                                               -- F/A
ALTER TABLE collection ADD COLUMN ident TEXT CHECK (ident IN
  ('identified','ambiguous','unidentified','confirmed'));                                       -- F/A (D172)

CREATE TABLE provider_response_new (                       -- class F: the stored input (D170)
  item_id     INTEGER NOT NULL REFERENCES item(id) ON DELETE RESTRICT,
  provider    TEXT NOT NULL,
  part        TEXT NOT NULL DEFAULT '',  -- '' the item itself; 'season/N' a series' season listing
  fetched_at  INTEGER NOT NULL,
  body        BLOB NOT NULL,             -- compressed; cast and crew are read from here in Cut A
  PRIMARY KEY (item_id, provider, part)
);
INSERT INTO provider_response_new (item_id, provider, part, fetched_at, body)
SELECT title_id, provider, '', fetched_at, body FROM provider_response;
DROP TABLE provider_response;
ALTER TABLE provider_response_new RENAME TO provider_response;
