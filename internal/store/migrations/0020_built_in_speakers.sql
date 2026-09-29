-- SPDX-License-Identifier: GPL-3.0-or-later
-- A device's own speakers are 'built_in_speakers', not 'tv_speakers' (the owner, 2026-09-29): the
-- answer covers a laptop's and a phone's as much as a television's. A CHECK cannot be changed in
-- place, so device is rebuilt, keeping its ids and its id sequence. An older binary would write
-- the old value, which the new CHECK refuses, so the step raises compatible_version.
-- orpheum:raises-compatible-version

CREATE TABLE device_new (                                  -- class A
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER REFERENCES user(id) ON DELETE RESTRICT,
  name          TEXT NOT NULL,
  platform      TEXT NOT NULL,
  audio_output  TEXT CHECK (audio_output IN ('built_in_speakers','receiver','headphones')),
                                         -- NULL until asked on first playback (D184)
  first_seen_at    INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL,
  removed_at    INTEGER
);
INSERT INTO device_new (id, user_id, name, platform, audio_output, first_seen_at, last_seen_at, removed_at)
SELECT id, user_id, name, platform,
  CASE audio_output WHEN 'tv_speakers' THEN 'built_in_speakers' ELSE audio_output END,
  first_seen_at, last_seen_at, removed_at
FROM device;
UPDATE sqlite_sequence SET seq = (SELECT seq FROM sqlite_sequence WHERE name = 'device')
WHERE name = 'device_new' AND EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'device');
DROP TABLE device;
ALTER TABLE device_new RENAME TO device;
CREATE INDEX device_owner ON device(user_id) WHERE removed_at IS NULL;
