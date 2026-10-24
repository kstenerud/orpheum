-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A device's sound may need to come earlier as well as later than its picture: the owner's TCL TV,
-- in TV Bro, plays its sound half a second late, not its picture (2026-10-24, a clip of a beep every
-- four seconds; the first, every second, could not tell). sound_delay_ms, 0 to 2000, becomes
-- sound_offset_ms, -2000 to 2000: positive holds the sound back, negative brings it forward.
ALTER TABLE device ADD COLUMN sound_offset_ms INTEGER NOT NULL DEFAULT 0
  CHECK (sound_offset_ms BETWEEN -2000 AND 2000);
UPDATE device SET sound_offset_ms = sound_delay_ms;
ALTER TABLE device DROP COLUMN sound_delay_ms;
