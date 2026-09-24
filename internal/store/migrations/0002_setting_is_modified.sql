-- SPDX-License-Identifier: GPL-3.0-or-later
-- A setting remembers whether a person set it (D200 §1, as amended). Any manual set marks it,
-- even to the value it starts with; putting it back to its starting value clears the mark; and a
-- release that changes a starting value moves every unmarked row to the new one.
-- Additive: an older binary ignores the column, so compatible_version stays where it is.

ALTER TABLE setting ADD COLUMN is_modified INTEGER NOT NULL DEFAULT 0 CHECK (is_modified IN (0, 1));

-- Rows from before this step: a row a person set names who set it; a row the seed wrote does not.
UPDATE setting SET is_modified = 1 WHERE set_by IS NOT NULL;
