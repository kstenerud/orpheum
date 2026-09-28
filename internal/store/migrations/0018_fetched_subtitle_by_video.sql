-- SPDX-License-Identifier: GPL-3.0-or-later
-- A playback lists the subtitles fetched for its video (D224), so they are found by the video, not
-- by reading every one (standards/sql.md section 11).

CREATE INDEX fetched_subtitle_video ON fetched_subtitle(video_file);
