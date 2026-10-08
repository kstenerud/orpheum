-- SPDX-License-Identifier: GPL-3.0-or-later
-- D270 section 1: a failed full read of a picture whose keyframes came from its file's index is
-- kept in a column of its own, so the sweep stops reading the file through every hour; failure
-- would make the film uncuttable, its keyframes known all the same.

ALTER TABLE video_timeline ADD COLUMN read_failure TEXT;  -- FFmpeg's words when a full read failed after the index gave the keyframes; NULL otherwise
