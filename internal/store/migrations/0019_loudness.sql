-- SPDX-License-Identifier: GPL-3.0-or-later
-- Loudness is levelled (D226): the node measures each audio track of a video once, as it is when
-- it has more than two channels and always as a stereo device hears it, and the core sets a gain
-- from the measurement. Kept by the file's payload hash and the stream's index, not by the file,
-- so a rescan, a move or a second copy of the same bytes keeps a measurement that cost a read of
-- the whole file; a row outlives its file, at a few dozen bytes, and serves the bytes if they come
-- back.

CREATE TABLE loudness (                                    -- class O: what the node measured
  payload_hash    BLOB NOT NULL,
  stream_index    INTEGER NOT NULL,
  sound           TEXT NOT NULL CHECK (sound IN ('source','stereo')),  -- as it is, or as stereo
  integrated_lufs REAL,                  -- -70 for silence; NULL when it failed
  range_lu        REAL,
  true_peak_dbtp  REAL,                  -- NULL for silence, which has no peak, or a failure
  failure         TEXT,                  -- FFmpeg's words when it could not be measured
  measured_at     INTEGER NOT NULL,
  PRIMARY KEY (payload_hash, stream_index, sound)
) WITHOUT ROWID;
