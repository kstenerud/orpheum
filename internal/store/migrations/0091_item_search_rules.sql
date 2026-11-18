-- SPDX-License-Identifier: AGPL-3.0-or-later
-- The version of the search rules a film or series was found to match nothing under (D513 section
-- 5): one found so under an older version than the matcher's, or under none, as every one found so
-- before this step, is returned to its first match by a sweep a few at a time and searched under the
-- rules of now. NULL on every item no search found nothing for.
ALTER TABLE item ADD COLUMN search_rules_version INTEGER CHECK (search_rules_version > 0);
