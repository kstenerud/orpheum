-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A series' files' order and an ordering no longer listed (D210 §1, D213 §4; plans/episode-orderings.md
-- §3.4, §6 steps 5 and 6). files_ordering_id is the ordering a library's files' season and episode
-- numbers are in, which an admin sets for the household; NULL, as every series is here, is aired. is_unlisted marks a
-- provider's ordering its provider's list no longer names: kept, with its last places, and said so.
ALTER TABLE "sequence" ADD COLUMN files_ordering_id INTEGER REFERENCES ordering(id) ON DELETE SET NULL;
ALTER TABLE ordering ADD COLUMN is_unlisted INTEGER NOT NULL DEFAULT 0 CHECK (is_unlisted IN (0, 1));
CREATE INDEX sequence_files_ordering ON "sequence"(files_ordering_id);
