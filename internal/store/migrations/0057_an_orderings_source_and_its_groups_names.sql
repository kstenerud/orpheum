-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A series' orderings beside aired (episode-orderings.md §3.5; D96, D159): each says what kind of
-- order it is, where it came from and what its provider calls it, and each names its own groups.
-- name stays the key a person's choice stores; the label is display text beside it.
--   kind:         'aired', or a TMDB episode group's type: 'aired_alternate', 'absolute', 'dvd',
--                 'digital', 'story_arc', 'production', 'tv'; 'authored' for a person's
--   source_id:    the provider's id for it (a TMDB episode group's), NULL for aired and a person's
--   label:        its provider's name for it ("DVD Order"), NULL for aired
--   listed_count: how many episodes its provider lists in it, before it is fetched; NULL when not said
ALTER TABLE ordering ADD COLUMN kind TEXT;
ALTER TABLE ordering ADD COLUMN source_id TEXT;
ALTER TABLE ordering ADD COLUMN label TEXT;
ALTER TABLE ordering ADD COLUMN listed_count INTEGER;
UPDATE ordering SET kind = 'aired' WHERE name = 'aired';
-- A group number is a different part in each ordering (aired's season 1, a DVD order's disc 1), so a
-- group's name is its ordering's, not its series'. Nothing wrote group_text before; any row there is
-- its series' aired ordering's.
CREATE TABLE group_text_new (                              -- a season's name, a disc's, a part's
  ordering_id   INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  group_number  INTEGER NOT NULL,
  language      TEXT NOT NULL,
  name          TEXT,
  overview      TEXT,
  PRIMARY KEY (ordering_id, group_number, language)
) STRICT;
INSERT INTO group_text_new (ordering_id, group_number, language, name, overview)
SELECT ordering.id, group_text.group_number, group_text.language, group_text.name, group_text.overview
FROM group_text
JOIN ordering ON ordering.collection_id = group_text.collection_id AND ordering.name = 'aired';
DROP TABLE group_text;
ALTER TABLE group_text_new RENAME TO group_text;
