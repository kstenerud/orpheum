-- SPDX-License-Identifier: GPL-3.0-or-later
-- Every column a reader had to look up says what it holds (D227; the list, with what each holds,
-- is contract-names-2026-09-29.md). Renames keep their data. Three change more than a name:
--   - file.path_raw goes: since D211 reported_path_bytes (was path_key) always holds the bytes;
--   - file.identity becomes is_identity_confirmed, 0 or 1;
--   - a sidecar's kind moves from filing_reason to its own sidecar_kind, and filing_reason says
--     why it is attached, as every filing but listed does (D163);
--   - loudness.sound's 'source' is 'multichannel', which its CHECK needs a rebuild to say.
-- An older binary names the old columns, so the step raises compatible_version.
-- orpheum:raises-compatible-version

ALTER TABLE "grant" RENAME COLUMN hash TO secret_sha256;
ALTER TABLE "grant" RENAME COLUMN subject TO user_id;
ALTER TABLE "grant" RENAME COLUMN parent TO parent_secret_sha256;
ALTER TABLE "grant" RENAME COLUMN label TO device_name;
DROP INDEX grant_subject;
CREATE INDEX grant_user ON "grant"(user_id) WHERE revoked_at IS NULL;

ALTER TABLE play_position RENAME COLUMN watched_at TO position_read_at;
ALTER TABLE play_position RENAME COLUMN seq TO report_seq;
ALTER TABLE play_event RENAME COLUMN server_at TO happened_at;
ALTER TABLE play_span RENAME COLUMN unterminated TO is_unterminated;
ALTER TABLE play_span RENAME COLUMN unattended TO is_unattended;

ALTER TABLE file RENAME COLUMN path_key TO reported_path_bytes;
ALTER TABLE file RENAME COLUMN sidecar_body TO nfo_body;
ALTER TABLE file RENAME COLUMN identified_path TO identified_from_path;
ALTER TABLE file RENAME COLUMN attached_to TO attached_to_file;
ALTER TABLE file RENAME COLUMN probe TO probe_json;
ALTER TABLE file DROP COLUMN path_raw;
ALTER TABLE file ADD COLUMN is_identity_confirmed INTEGER NOT NULL DEFAULT 1
  CHECK (is_identity_confirmed IN (0, 1));           -- D179: 0 after a rebuild, until re-seen
UPDATE file SET is_identity_confirmed = (identity = 'confirmed');
ALTER TABLE file DROP COLUMN identity;
ALTER TABLE file ADD COLUMN sidecar_kind TEXT CHECK (sidecar_kind IN ('nfo', 'subtitle', 'image'));
UPDATE file SET sidecar_kind = filing_reason,
  filing_reason = CASE filing_reason WHEN 'nfo' THEN 'an NFO beside its video'
                                     WHEN 'subtitle' THEN 'a subtitle beside its video'
                                     ELSE 'an image beside its video' END
WHERE filing = 'attached' AND filing_reason IN ('nfo', 'subtitle', 'image');

ALTER TABLE stream RENAME COLUMN disposition TO disposition_flags;
ALTER TABLE stream RENAME COLUMN channels TO channel_count;

CREATE TABLE loudness_new (                                -- class O: what the node measured
  payload_hash    BLOB NOT NULL,
  stream_index    INTEGER NOT NULL,
  measured_mix    TEXT NOT NULL CHECK (measured_mix IN ('multichannel','stereo')),  -- as it is, or as stereo
  integrated_lufs REAL,                  -- -70 for silence; NULL when it failed
  range_lu        REAL,
  true_peak_dbtp  REAL,                  -- NULL for silence, which has no peak, or a failure
  failure         TEXT,                  -- FFmpeg's words when it could not be measured
  measured_at     INTEGER NOT NULL,
  PRIMARY KEY (payload_hash, stream_index, measured_mix)
) WITHOUT ROWID;
INSERT INTO loudness_new (payload_hash, stream_index, measured_mix, integrated_lufs, range_lu, true_peak_dbtp,
  failure, measured_at)
SELECT payload_hash, stream_index, CASE sound WHEN 'source' THEN 'multichannel' ELSE sound END, integrated_lufs,
  range_lu, true_peak_dbtp, failure, measured_at
FROM loudness;
DROP TABLE loudness;
ALTER TABLE loudness_new RENAME TO loudness;

ALTER TABLE root_mount RENAME COLUMN path TO mount_path;
ALTER TABLE root_state RENAME COLUMN path TO root_path;
ALTER TABLE root_state RENAME COLUMN removals_approved TO are_removals_approved;
ALTER TABLE root_scan RENAME COLUMN path TO root_path;

ALTER TABLE user RENAME COLUMN outsider TO is_outsider;
ALTER TABLE library RENAME COLUMN writable TO is_writable;
ALTER TABLE role RENAME COLUMN builtin TO is_builtin;
ALTER TABLE artwork RENAME COLUMN chosen TO is_chosen;

ALTER TABLE fetched_subtitle RENAME COLUMN body TO body_gzip;
ALTER TABLE fetched_subtitle RENAME COLUMN provider_ref TO provider_file_id;
ALTER TABLE provider_response RENAME COLUMN body TO body_gzip;

ALTER TABLE node RENAME COLUMN report_seq TO last_ingested_report_seq;
ALTER TABLE node RENAME COLUMN status TO last_status_json;
ALTER TABLE node RENAME COLUMN status_at TO last_status_at;

ALTER TABLE setting RENAME COLUMN value TO value_json;
ALTER TABLE setting RENAME COLUMN set_by TO set_by_user_id;
ALTER TABLE secret RENAME COLUMN set_by TO set_by_user_id;
ALTER TABLE field_override RENAME COLUMN value TO override_value;
ALTER TABLE field_override RENAME COLUMN set_by TO set_by_user_id;

ALTER TABLE sort_key RENAME COLUMN key TO collation_key;
ALTER TABLE position RENAME COLUMN number TO number_in_group;
ALTER TABLE position RENAME COLUMN sort TO sort_order;
DROP INDEX position_sort;
CREATE INDEX position_order ON position(ordering_id, sort_order);
ALTER TABLE extra_relation RENAME COLUMN type TO extra_type;
ALTER TABLE title RENAME COLUMN source TO followed_source;
ALTER TABLE title RENAME COLUMN source_id TO followed_source_id;
ALTER TABLE collection RENAME COLUMN source TO followed_source;
ALTER TABLE collection RENAME COLUMN source_id TO followed_source_id;
ALTER TABLE item RENAME COLUMN origin TO origin_server;
