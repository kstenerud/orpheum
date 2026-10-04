-- SPDX-License-Identifier: GPL-3.0-or-later
-- D244 section 4 (S90): a subtitle is fetched for a file once, since each fetch spends one of the
-- account's daily downloads; a second ask for it is given the one kept. A file that already holds
-- the same subtitle twice keeps the first.

DELETE FROM fetched_subtitle
WHERE id NOT IN (
  SELECT MIN(id) FROM fetched_subtitle GROUP BY video_file, provider, provider_file_id
);
CREATE UNIQUE INDEX fetched_subtitle_once ON fetched_subtitle(video_file, provider, provider_file_id);
