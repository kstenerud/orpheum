-- SPDX-License-Identifier: AGPL-3.0-or-later
-- D304 §5 (D240 §3's move by inode, which §8 deferred): a file keeps the filesystem device and
-- the inode its node last saw it at, so a listing tells a file moved on its disk at once, by the
-- same device, inode, size and modified time at a new path, and keeps its row and history without
-- reading it again. Unknown until a file is read again; a move is then told by its payload hash, as
-- before.
ALTER TABLE file ADD COLUMN filesystem_device INTEGER;  -- class I
ALTER TABLE file ADD COLUMN inode INTEGER;              -- class I
