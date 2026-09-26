-- SPDX-License-Identifier: GPL-3.0-or-later
-- A play position moves only for a report about a later moment of watching (D217 section 7):
-- the moment is the device's monotonic time anchored to the server's clock when the report
-- arrives (D165). watched_at is that moment for the stored position, in Unix milliseconds, and
-- the guard compares against it. seq keeps the report's number within its playback, which only
-- tells a repeated report from a new one. No position was ever written before this step, so the
-- starting value stands for none.

ALTER TABLE play_position ADD COLUMN watched_at INTEGER NOT NULL DEFAULT 0;
