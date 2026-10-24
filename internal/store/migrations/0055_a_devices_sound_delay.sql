-- SPDX-License-Identifier: AGPL-3.0-or-later
-- How long a device's sound is delayed after its picture, in milliseconds, for a TV that shows its
-- picture late: the owner's TCL TV, in TV Bro, showed every film's picture half a second after its
-- sound (2026-10-24). A device's own, as its audio_output is; none until its person sets one.
ALTER TABLE device ADD COLUMN sound_delay_ms INTEGER NOT NULL DEFAULT 0
  CHECK (sound_delay_ms BETWEEN 0 AND 2000);
