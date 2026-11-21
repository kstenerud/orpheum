-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Each video's sound measured for what it shares with its season's others (D101, D115;
-- plans/media-segments.md): the fingerprint detector's record that a file's bytes were compared, so
-- each is measured once. It is kept by payload hash, as the loudness is, because what was measured
-- is the bytes, and by the measurement's version, so a better comparison measures every file again.
-- The outcome is what the comparison found: a repeat at the opening or the closing (found), none
-- (none), or FFmpeg's word that the sound could not be fingerprinted (failed), a fact about the
-- bytes, which measuring again would not change. A file the node did not read, changed or gone, has
-- no row, and the next sweep asks again. The segments found are media_segment's, provider
-- 'fingerprint'.
CREATE TABLE repeat_measurement (                          -- class O: what the node measured
  payload_hash BLOB NOT NULL,                                -- the file's payload hash
  version      INTEGER NOT NULL CHECK (version > 0),         -- the core's measurement version
  outcome      TEXT NOT NULL CHECK (outcome IN ('found','none','failed')),
  failure      TEXT,                                         -- FFmpeg's words; set only when it failed
  measured_at  INTEGER NOT NULL,
  CHECK ((outcome = 'failed') = (failure IS NOT NULL)),
  PRIMARY KEY (payload_hash, version)
) WITHOUT ROWID, STRICT;
