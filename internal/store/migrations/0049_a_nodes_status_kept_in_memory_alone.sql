-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D303 (M8; the independent audit's S6): a node's last status is kept in memory alone. Every
-- status report, a few seconds apart, wrote it into persistent.db, taking the one writer, filling
-- the write-ahead log and landing in every backup, and nothing read it back: the System page and
-- the doctor read the copy the core keeps in memory.
ALTER TABLE node DROP COLUMN last_status_json;
ALTER TABLE node DROP COLUMN last_status_at;
