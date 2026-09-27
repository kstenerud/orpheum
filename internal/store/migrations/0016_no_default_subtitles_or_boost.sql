-- SPDX-License-Identifier: GPL-3.0-or-later
-- Subtitles start off and the dialogue boost is a person's own choice, off until they turn it on
-- (D222 section 3, D223 section 3): the two server settings that were to say otherwise, which
-- nothing read, are gone from the registry, and their stored rows with them.

DELETE FROM setting WHERE key IN ('subtitles_on_by_default', 'dialogue_boost_on_by_default');
