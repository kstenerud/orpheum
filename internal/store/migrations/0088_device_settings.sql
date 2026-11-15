-- SPDX-License-Identifier: AGPL-3.0-or-later
-- A device's settings are kept by the core, its browser keeping a copy (D493).
--
-- Its data caps (D488, D489, D491): the most a film may use on each kind of network its browser
-- says it is on, a picture's average with its sound's, kbit/s; NULL is No limit, as every device
-- has here. The core applies the cap of the kind a start says: it chooses the film's ladder and the
-- rung it starts on under it.
ALTER TABLE device ADD COLUMN data_cap_wifi_kbps INTEGER CHECK (data_cap_wifi_kbps > 0);
ALTER TABLE device ADD COLUMN data_cap_mobile_kbps INTEGER CHECK (data_cap_mobile_kbps > 0);
ALTER TABLE device ADD COLUMN data_cap_unknown_kbps INTEGER CHECK (data_cap_unknown_kbps > 0);

-- The films whose own picture a device's player gave up as one it cannot play (D266), so that its
-- later playbacks of the film start at the twin (D323 section 2): kept until the device or the file
-- goes, across restarts of the core, where they were kept in its memory before.
CREATE TABLE own_picture_given_up (                         -- class A (D493)
  device_id   INTEGER NOT NULL REFERENCES device(id) ON DELETE CASCADE,
  file_id     INTEGER NOT NULL REFERENCES file(id) ON DELETE CASCADE,
  given_up_at INTEGER NOT NULL,
  PRIMARY KEY (device_id, file_id)
) WITHOUT ROWID, STRICT;
CREATE INDEX own_picture_given_up_file ON own_picture_given_up(file_id);
