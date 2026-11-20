-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The typed time ranges inside a video (D82, D159; plans/media-segments.md): an intro, a recap, a
-- preview, the credits, a commercial, one marked but of no known type, and where one episode ends and
-- the next begins inside one file. Each says which provider found it and how sure it is.
--
-- D82 attaches them at the Version because a timecode indexes an encoding, not a work. Here a version
-- is one or more files, the parts of a multi-part one in order (file_filing.part_index), and a file can
-- be filed in a version by each kind of library, so a timecode is right only for the one file whose
-- timeline it is on: a position is file-relative for the same reason (D157). So a segment attaches to
-- the file, where its timeline is, and the version's segments are its files'.
--
-- A detector's rows (class O) are re-derived from what it read: a file's probe, for the chapter
-- provider, which replaces only its own rows. A person's (provider 'person', class A) are no detector's,
-- survive every re-detection, and beat a detector's of the same type.
CREATE TABLE media_segment (                               -- class O; a person's rows class A
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE RESTRICT,
  type        TEXT NOT NULL CHECK (type IN ('intro','recap','preview','credits','commercial','unknown',
                'episode_boundary')),
  start_ms    INTEGER NOT NULL CHECK (start_ms >= 0),
  end_ms      INTEGER NOT NULL CHECK (end_ms > start_ms),
  provider    TEXT NOT NULL CHECK (provider IN ('chapters','fingerprint','person')),
  confidence  REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1)  -- 1 for a chapter's title, a person's
) STRICT;
CREATE INDEX media_segment_file ON media_segment(file_id, provider);
