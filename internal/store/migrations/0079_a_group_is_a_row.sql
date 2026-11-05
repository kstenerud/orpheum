-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- A group is a row (D426 section 5). A season or a disc was a number repeated on each position and
-- each name of it, and a season's poster named it by number on the series' artwork. grp holds it once,
-- in its ordering, and what is a group's references it: the positions in it, its names, and its
-- artwork, which is a series' aired season's (D379). A position with no group (no season) names none.
-- The API still addresses a group as its item and its number.
--
-- Every group a position, a name or a season's poster used becomes a row, and every other id is kept.
-- A season's poster is its series' aired order's group; a series with season posters and no aired
-- order (one whose every episode left it) is given its aired order, as the scanner would make it. A
-- season's poster on an item that is no series, which no writer makes, has no group to name and goes.
--
-- An older binary writes the numbers, so the compatible version rises.

CREATE TABLE grp (                                         -- a season, or a disc (D426 section 5)
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  number      INTEGER NOT NULL,          -- the season's number, or the disc's
  UNIQUE (ordering_id, number)
) STRICT;

INSERT INTO ordering (sequence_id, name, kind)
SELECT DISTINCT artwork.item_id, 'aired', 'aired' FROM artwork
JOIN "sequence" ON "sequence".id = artwork.item_id
WHERE artwork.group_number IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM ordering WHERE ordering.sequence_id = artwork.item_id AND ordering.name = 'aired');
DELETE FROM artwork
WHERE group_number IS NOT NULL AND NOT EXISTS (SELECT 1 FROM "sequence" WHERE "sequence".id = artwork.item_id);

INSERT INTO grp (ordering_id, number)
SELECT ordering_id, group_number FROM position WHERE group_number IS NOT NULL
UNION SELECT ordering_id, group_number FROM group_text
UNION SELECT ordering.id, artwork.group_number FROM artwork
  JOIN ordering ON ordering.sequence_id = artwork.item_id AND ordering.name = 'aired'
  WHERE artwork.group_number IS NOT NULL
ORDER BY 1, 2;

CREATE TABLE position_new (                                -- season/episode, or disc/track
  ordering_id INTEGER NOT NULL REFERENCES ordering(id) ON DELETE RESTRICT,
  work_id    INTEGER NOT NULL REFERENCES "work"(id) ON DELETE RESTRICT,
  grp_id      INTEGER REFERENCES grp(id) ON DELETE RESTRICT,  -- its season, or disc; NULL: none
  number_in_group INTEGER,               -- episode, or track
  sort_order  REAL NOT NULL,             -- a total order within the ordering
  PRIMARY KEY (ordering_id, work_id)
) STRICT;
INSERT INTO position_new (ordering_id, work_id, grp_id, number_in_group, sort_order)
SELECT position.ordering_id, position.work_id, grp.id, position.number_in_group, position.sort_order
FROM position
LEFT JOIN grp ON grp.ordering_id = position.ordering_id AND grp.number = position.group_number;
DROP TABLE position;
ALTER TABLE position_new RENAME TO position;
CREATE INDEX position_order ON position(ordering_id, sort_order);
CREATE INDEX position_work ON position(work_id);

CREATE TABLE group_text_new (                              -- a season's name, a disc's, a part's
  grp_id      INTEGER NOT NULL REFERENCES grp(id) ON DELETE RESTRICT,
  language    TEXT NOT NULL,
  name        TEXT,
  PRIMARY KEY (grp_id, language)
) WITHOUT ROWID, STRICT;
INSERT INTO group_text_new (grp_id, language, name)
SELECT grp.id, group_text.language, group_text.name
FROM group_text
JOIN grp ON grp.ordering_id = group_text.ordering_id AND grp.number = group_text.group_number;
DROP TABLE group_text;
ALTER TABLE group_text_new RENAME TO group_text;

-- artwork keeps its ids and its sequence: its season moves to a column of its own.
ALTER TABLE artwork ADD COLUMN grp_id INTEGER REFERENCES grp(id) ON DELETE RESTRICT;  -- a season's poster
UPDATE artwork SET grp_id = (
  SELECT grp.id FROM grp JOIN ordering ON ordering.id = grp.ordering_id
  WHERE ordering.sequence_id = artwork.item_id AND ordering.name = 'aired' AND grp.number = artwork.group_number)
WHERE group_number IS NOT NULL;
ALTER TABLE artwork DROP COLUMN group_number;
