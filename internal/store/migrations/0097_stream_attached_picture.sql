-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Whether a stream is cover art in its container, FFmpeg's attached_pic disposition (bit 1024 of
-- disposition_flags, which library.DispositionBit names in Go): the one place the store's queries
-- read the rule from, which decide.MainVideo follows in Go (a file's picture is its first video
-- stream that is not a cover). It is computed from the flags as found, never written, so no writer
-- can disagree with them and no step backfills it; virtual, as SQLite adds a generated column only so.
ALTER TABLE stream ADD COLUMN is_attached_picture INTEGER NOT NULL
  GENERATED ALWAYS AS ((disposition_flags & 1024) <> 0) VIRTUAL;
