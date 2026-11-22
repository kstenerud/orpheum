-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A file's sound offset for every person (D537): how far its sound is moved from its picture, in
-- milliseconds, later when positive and earlier when negative, as a device's is. An admin sets it;
-- a file with no row plays in step. A person's own offset for a file is a preference at file scope
-- ('sound_offset_ms'), which overrides it for them. It is the file's, not its video's: a video's
-- other version starts at 0. It goes with the file.
CREATE TABLE file_sound_offset (                            -- class A (D537)
  file_id   INTEGER PRIMARY KEY REFERENCES file(id) ON DELETE CASCADE,
  offset_ms INTEGER NOT NULL CHECK (offset_ms BETWEEN -2000 AND 2000 AND offset_ms <> 0),
  set_at    INTEGER NOT NULL
) STRICT;
-- A problem report's suggestion that a file needs a sound offset (D537): the file the person played
-- and the offset they set for themselves, which the admin may make everyone's. NULL for a report
-- that suggests none; the file NULL once it is deleted.
ALTER TABLE problem_report ADD COLUMN sound_file_id INTEGER REFERENCES file(id) ON DELETE SET NULL;
ALTER TABLE problem_report ADD COLUMN sound_offset_ms INTEGER
  CHECK (sound_offset_ms BETWEEN -2000 AND 2000);
CREATE INDEX problem_report_sound_file ON problem_report(sound_file_id);
