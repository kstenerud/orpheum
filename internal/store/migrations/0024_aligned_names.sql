-- SPDX-License-Identifier: GPL-3.0-or-later
-- One thing, one name, in every layer (D227's addendum): the columns the API had already renamed
-- follow it, a column holding JSON says so, and a stored subtitle choice's keys are the API's.

ALTER TABLE root_state RENAME COLUMN outcome TO last_scan_outcome;
ALTER TABLE root_state RENAME COLUMN outcome_at TO last_scan_at;
ALTER TABLE library RENAME COLUMN check_every_h TO check_every_hours;
ALTER TABLE file RENAME COLUMN error_detail TO error_detail_json;
ALTER TABLE preference RENAME COLUMN value TO value_json;

UPDATE preference
SET value_json = json_remove(
  json_set(value_json, '$.isCarriedFromWork',
    json(CASE json_type(value_json, '$.isCarried') WHEN 'true' THEN 'true' ELSE 'false' END)),
  '$.isCarried')
WHERE axis = 'subtitle_choice' AND json_type(value_json, '$.isCarried') IS NOT NULL;

UPDATE preference
SET value_json = json_remove(
  json_set(value_json, '$.fetchedSubtitleId', json_extract(value_json, '$.fetchedId')),
  '$.fetchedId')
WHERE axis = 'subtitle_choice' AND json_type(value_json, '$.fetchedId') IS NOT NULL;
