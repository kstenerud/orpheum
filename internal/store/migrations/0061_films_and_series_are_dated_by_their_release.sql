-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D366 section 1: films and series sort by release date too, a film's release date and a series'
-- first air date, as the source gives them, the year alone where no day is known. The day is kept as
-- a number that sorts as the date does, YYYYMMDD, a month or day unknown written 00: 1995-12-15 is
-- 19951215, 1995 alone 19950000. NULL is a title or series with no date, which a sort by date lists
-- last whichever way it runs (D366 section 3).

ALTER TABLE title ADD COLUMN released_on INTEGER CHECK (released_on IS NULL OR released_on >= 10000);
ALTER TABLE collection ADD COLUMN released_on INTEGER CHECK (released_on IS NULL OR released_on >= 10000);

-- Until the pass reads each one's day from its source, a year it already has dates it.
UPDATE title SET released_on = year * 10000 WHERE year > 0;
UPDATE collection SET released_on = year * 10000 WHERE year > 0;

-- A page of films or series by release date reads its kind's dated titles in either direction, and
-- those with none by id (released_on IS NULL), from these.
CREATE INDEX title_film_released ON title(released_on) WHERE kind = 'movie';
CREATE INDEX collection_series_released ON collection(released_on) WHERE kind = 'series';
