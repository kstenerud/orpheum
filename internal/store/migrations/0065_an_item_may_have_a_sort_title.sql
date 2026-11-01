-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D402: an item's NFO may name how it sorts (<sorttitle>), and that wins over its name without its
-- leading article. It is kept beside the name so that a rekey of every name, at a locale change or
-- a build that keys otherwise, derives the same key a scan did. NULL is no sort title, as every row
-- is until now.

ALTER TABLE item_text ADD COLUMN sort_title TEXT;
