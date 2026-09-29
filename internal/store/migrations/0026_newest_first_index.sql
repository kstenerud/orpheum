-- SPDX-License-Identifier: GPL-3.0-or-later
-- The newest-first listings walk the live items in the order they were added, instead of sorting
-- every live film for each page: at the reference scale, with the machine busy, the sort took the
-- films' first page past its 50 ms budget. The rowid rides in the index, so it serves the
-- listings' (added_at, id) order too.

CREATE INDEX item_added ON item(added_at) WHERE removed_at IS NULL;
