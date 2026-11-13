-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A person is named and pictured by the newest response that credits them (the owner, 2026-11-13):
-- the film or series whose stored provider response last set their name and picture, whose age
-- that response's fetched_at says, so a response older than it, another item's, leaves the person
-- alone and a pass over an unchanged library writes nothing (D303). NULL, as every person is
-- here, is a person no response has set so: the next response that credits them may.
ALTER TABLE person ADD COLUMN named_by_item_id INTEGER;
