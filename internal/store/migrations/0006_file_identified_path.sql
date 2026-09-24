-- SPDX-License-Identifier: GPL-3.0-or-later
-- The path a file's identification was derived from (D208 section 1). A file whose path has not
-- changed keeps its title, whatever a person has since done to that title's name; a renamed or
-- moved file is identified again from its new path, unless a person confirmed its match. A
-- re-interpretation pass over the whole library clears it.
-- Additive: an older binary ignores the column, so compatible_version stays where it is.

ALTER TABLE file ADD COLUMN identified_path TEXT;   -- class O
