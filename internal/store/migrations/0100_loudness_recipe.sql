-- SPDX-License-Identifier: AGPL-3.0-or-later
-- orpheum:raises-compatible-version
-- A loudness measurement names the recipe it was made with, in its key (D536 §3): the version of
-- how the node measures, nodeproto.LoudnessRecipe. A measurement of another recipe is none of this
-- one's, so a raised recipe has the read-through sweep measure every file's sound again, once. Every
-- row measured before is of recipe 1, the first, so the deploy that brings this step measures
-- nothing again. A rebuild, since a key cannot be changed in place (standards/sql.md §5); it raises
-- the compatible version, since an older binary's write of a measurement names the key it had, which
-- no longer is one, and lacks the recipe (migrations.md, Expand then contract).
CREATE TABLE loudness_new (                                -- class O: what the node measured
  payload_hash    BLOB NOT NULL,
  stream_index    INTEGER NOT NULL,
  measured_mix    TEXT NOT NULL CHECK (measured_mix IN ('multichannel','stereo')),  -- as it is, or as stereo
  recipe          INTEGER NOT NULL CHECK (recipe >= 1),  -- how it was measured (D536 §3)
  integrated_lufs REAL,                  -- -70 for silence; NULL when it failed
  range_lu        REAL,
  true_peak_dbtp  REAL,                  -- NULL for silence, which has no peak, or a failure
  failure         TEXT,                  -- FFmpeg's words when it could not be measured
  measured_at     INTEGER NOT NULL,
  PRIMARY KEY (payload_hash, stream_index, measured_mix, recipe)
) WITHOUT ROWID, STRICT;
INSERT INTO loudness_new (payload_hash, stream_index, measured_mix, recipe, integrated_lufs, range_lu,
  true_peak_dbtp, failure, measured_at)
SELECT payload_hash, stream_index, measured_mix, 1, integrated_lufs, range_lu, true_peak_dbtp, failure,
  measured_at
FROM loudness;
DROP TABLE loudness;
ALTER TABLE loudness_new RENAME TO loudness;
