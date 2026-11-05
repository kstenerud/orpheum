-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A repointed place is unconfirmed until a scan reads it again (D432 section 6, the relocation
-- audit's check 1). A repoint used to forget the place's modified time to force that read, which
-- fired the found-time trigger for every moved place and left its items' found_file_modified_at 0
-- until the scan: "recently added" reordered during a move. The mark says the same without
-- touching the modified time, and a scan's read of the place clears it.
ALTER TABLE place ADD COLUMN is_unconfirmed INTEGER NOT NULL DEFAULT 0 CHECK (is_unconfirmed IN (0, 1));
