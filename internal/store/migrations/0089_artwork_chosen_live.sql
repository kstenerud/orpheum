-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A person's choice of image is one per item and kind among its live rows (D510 section 3): a row
-- retired because the cache lost its image, and never stored again, keeps its is_chosen, and the
-- index counted it, so it would refuse a new choice for that item and kind. Retired rows are left out.
DROP INDEX artwork_chosen;
CREATE UNIQUE INDEX artwork_chosen ON artwork(item_id, kind) WHERE is_chosen = 1 AND removed_at IS NULL;
