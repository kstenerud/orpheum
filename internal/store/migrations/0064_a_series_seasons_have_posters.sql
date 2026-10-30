-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D379 section 1: a series' page shows each season's own poster. A season is no item, so its poster
-- is an artwork row of its series, of kind 'poster', naming the season of the series' aired ordering
-- it is of; NULL is an image of the item itself, as every row was until now. TMDB names a season's
-- poster in the series' own stored record, which the next pass reads, so no row is written here.

ALTER TABLE artwork ADD COLUMN season_number INTEGER CHECK (season_number >= 0);
