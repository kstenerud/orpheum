-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D301 section 4: a film or series the matcher settled by a guess among several whose names match,
-- marked as one on its poster until a person confirms or corrects it, which removes the mark.

CREATE TABLE guessed_match (                               -- class F (D301)
  item_id     INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE CASCADE,
  guessed_at  INTEGER NOT NULL
);
