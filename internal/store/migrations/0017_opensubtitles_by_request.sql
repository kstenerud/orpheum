-- SPDX-License-Identifier: GPL-3.0-or-later
-- Fetching a subtitle on request (D224):
--   - each video keeps OpenSubtitles' moviehash of its bytes, which the node reports at scan,
--     to find subtitles made for this exact file (section 3);
--   - the count of downloads left is the provider's own, so the table that counted them here
--     goes (section 4);
--   - a fetched subtitle keeps the provider's name for it, which is how a person tells two apart;
--   - the OpenSubtitles login is the server's and the API key the application's (sections 1, 2):
--     a user's login and any stored key are gone, and the admin enters the login once.

ALTER TABLE file ADD COLUMN opensubtitles_hash TEXT;  -- 16 hex digits; NULL until scanned again
DROP TABLE subtitle_quota;
ALTER TABLE fetched_subtitle ADD COLUMN release_name TEXT;  -- the provider's file name for it
DELETE FROM secret
WHERE name = 'opensubtitles_api_key'
   OR (scope = 'user' AND name IN ('opensubtitles_username', 'opensubtitles_password'));
