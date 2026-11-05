-- SPDX-License-Identifier: AGPL-3.0-or-later
-- One realistic row in every table of the current schema, in reference order, for the backup and restore
-- tests: a copy that reproduces this reproduces every table. Synthetic titles only (D187).

INSERT INTO node (id, name, enrolled_at) VALUES (1, 'nas', 1790000000000);
INSERT INTO library (id, name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at)
  VALUES (1, 'Films', 'movie', 'nfo_then_tmdb', 'interval', 6, 0, 1790000000000);
INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/media/films');
INSERT INTO root_scan (node_id, root_path, completed_at) VALUES (1, '/media/films', 1790000002000);
INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present) VALUES (1, '/media/films', '/media/films/disk2', 2050, 1);
INSERT INTO root_state (node_id, root_path, filesystem_device, is_mount_point, last_scan_outcome, last_scan_at) VALUES (1, '/media/films', 2049, 1, 'complete', 1790000002000);
INSERT INTO unavailable_place (node_id, path, root_path, since, cause) VALUES (1, '/media/films/disk2', '/media/films', 1790000003000, 'not_answering');

INSERT INTO user (id, name, role_id, password_hash, admin_note, created_at)
  VALUES (1, 'owner', 1, CAST('$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA' AS BLOB), NULL, 1790000000000),
         (2, 'partner', 2, NULL, 'Anna, from the bus station', 1790000000000);
INSERT INTO role_library (role_id, library_id, may_rebroadcast) VALUES (2, 1, 0);
INSERT INTO device (id, user_id, name, platform, added_at, last_signed_in_at)
  VALUES (1, 2, 'Living room TV', 'webos', 1790000000000, 1790000100000);

INSERT INTO item (id, kind, filing_library_id, added_at, year, released_on, followed_source, followed_source_id,
    identification_state) VALUES
  (1, 'movie', 1, 1790000000000, 2020, 20200314, 'tmdb', '10001', 'identified'),
  (2, 'extra', 1, 1790000000000, 2020, NULL, 'none', NULL, 'unidentified'),
  (3, 'series', 1, 1790000000000, 2019, NULL, 'tmdb', '615', 'identified'),
  (4, 'person', NULL, 1790000000000, NULL, NULL, NULL, NULL, NULL);
INSERT INTO item_provider_id (item_id, provider, provider_id) VALUES (1, 'tmdb', '10001');
INSERT INTO work (id, kind, runtime_ms) VALUES (1, 'movie', 5400000), (2, 'extra', 120000);
INSERT INTO version (id, work_id, created_at) VALUES (1, 1, 1790000000000);
INSERT INTO extra_relation (owner_work, extra_work, extra_type, asserted_by)
  VALUES (1, 2, 'trailer', 'scanner');

INSERT INTO file (id, size_bytes, payload_hash, payload_bytes, opensubtitles_hash, sidecar_kind)
  VALUES (1, 1048576, X'00112233445566778899AABBCCDDEEFF', 1000000, '8e245d9679d31e12', NULL),
         (2, 2048, NULL, NULL, NULL, 'subtitle'),
         (3, 512, NULL, NULL, NULL, 'nfo');
INSERT INTO place (id, file_id, node_id, path, reported_path_raw, modified_at, filing, filing_reason,
                   first_seen_at, last_seen_at)
  VALUES (1, 1, 1, '/media/films/Direct Play Film (2020)/Direct Play Film (2020).mp4',
          CAST('/media/films/Direct Play Film (2020)/Direct Play Film (2020).mp4' AS BLOB), 1780000000000,
          'read', NULL, 1790000000000, 1790000002000);
INSERT INTO file_filing (file_id, library_kind, filing, filing_reason, version_id, part_index, identified_from_path)
  VALUES (1, 'movie', 'listed', 'movie folder', 1, 1,
          '/media/films/Direct Play Film (2020)/Direct Play Film (2020).mp4');
INSERT INTO place (id, file_id, node_id, path, reported_path_raw, modified_at, filing, filing_reason,
                   first_seen_at, last_seen_at)
  VALUES (4, 1, 1, '/media/films/Copies/Direct Play Film (2020).mp4',
          CAST('/media/films/Copies/Direct Play Film (2020).mp4' AS BLOB), 1780000000000, 'read',
          'byte for byte the file found before it, at another place', 1790000001000, 1790000002000);
INSERT INTO whole_file_hash (file_id, sha256, size_bytes, modified_at, failure, hashed_at)
  VALUES (1, X'0F0F', 1048576, 1780000000000, NULL, 1790000001500);
INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (1, '{"format":{"durationMs":5400000}}', 5400000);
INSERT INTO place (id, file_id, node_id, path, reported_path_raw, modified_at, filing, filing_reason,
                   attached_to_place, first_seen_at, last_seen_at)
  VALUES (2, 2, 1, '/media/films/Direct Play Film (2020)/Direct Play Film (2020).en.srt',
          CAST('/media/films/Direct Play Film (2020)/Direct Play Film (2020).en.srt' AS BLOB), 1780000000000,
          'read', 'a subtitle beside its video', 1, 1790000000000, 1790000002000),
         (3, 3, 1, '/media/films/Direct Play Film (2020)/Direct Play Film (2020).nfo',
          CAST('/media/films/Direct Play Film (2020)/Direct Play Film (2020).nfo' AS BLOB), 1780000000000,
          'read', 'an NFO beside its video', 1, 1790000000000, 1790000002000);
INSERT INTO file_nfo (file_id, body) VALUES (3, CAST('<movie><title>Direct Play Film</title></movie>' AS BLOB));
INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, container_language, language, container_title, disposition_flags,
                    channel_count)
  VALUES (1, 1, 1, 1, 'audio', 'aac', 'eng', 'en', 'Stereo', 1, 2),
         (2, 2, 1, NULL, 'subtitle', 'subrip', 'en', 'en', NULL, 0, NULL);
INSERT INTO fetched_subtitle (id, video_file, provider, provider_file_id, language, format, body_gzip, fetched_by_user_id, fetched_at,
                              release_name)
  VALUES (1, 1, 'opensubtitles', 'os-1', 'de', 'srt', CAST('1\n00:00:01,000 --> 00:00:02,000\nHallo\n' AS BLOB),
          2, 1790000003000, 'Direct.Play.Film.2020.German.srt');
INSERT INTO loudness (payload_hash, stream_index, measured_mix, integrated_lufs, range_lu, true_peak_dbtp, failure,
                      measured_at)
  VALUES (X'0101', 1, 'stereo', -35.7, 22.8, -16.2, NULL, 1790000004000),
         (X'0101', 2, 'stereo', NULL, NULL, NULL, 'FFmpeg could not decode it', 1790000004000);
INSERT INTO video_timeline (payload_hash, video_stream_index, keyframes_ms_json, bytes_in_each_second_json, failure,
                            made_at)
  VALUES (X'0101', 0, '[0,10010]', '[90000,4000]', NULL, 1790000004000),
         (X'0202', 0, '[]', '[]', 'FFmpeg could not read it through', 1790000004000);

INSERT INTO item_text (item_id, language, is_original, name, overview, tagline)
  VALUES (1, 'en', 1, 'Direct Play Film', 'A synthetic film.', 'It plays.');
INSERT INTO item_text (item_id, language, is_original, name, overview, tagline)
  VALUES (4, 'en', 1, 'Ada Player', NULL, NULL);
INSERT INTO person (id, kind, tmdb_id, name_key, picture_ref) VALUES (4, 'person', '501', 'ada player', '/ada.jpg');
INSERT INTO genre (id, name, name_key) VALUES (1, 'Drama', 'drama');
INSERT INTO item_genre (item_id, genre_id, ordinal) VALUES (1, 1, 0);
INSERT INTO credit (item_id, job, ordinal, person_id, character) VALUES (1, 'cast', 0, 4, 'The Lead'),
  (1, 'director', 0, 4, NULL);
INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (1, 'en', 'movie', X'4450');
INSERT INTO other_video (work_id, recorded_at, recorded_from, file_name, file_name_key)
  VALUES (1, 1563100262000, 'tag', 'direct-play.mkv', X'4450');
INSERT INTO field_override (item_id, field, language, override_value, origin, set_by_user_id, set_at)
  VALUES (1, 'name', 'en', 'Direct Play Film', 'typed', 1, 1790000004000);
INSERT INTO artwork (id, item_id, kind, language, source, source_ref, source_sha256, is_chosen)
  VALUES (1, 1, 'poster', 'en', 'nfo', 'poster.jpg', X'AABB', 1);
INSERT INTO artwork (id, item_id, kind, source, source_ref, refused_at, refusal, is_missing_at_source)
  VALUES (2, 1, 'backdrop', 'tmdb', '/gone.jpg', 1790000700000, 'no image there', 1);

INSERT INTO sequence (id, kind, created_by) VALUES (3, 'series', 'scanner');
INSERT INTO sequence_member (sequence_id, member_work, position) VALUES (3, 1, 1.0);
INSERT INTO ordering (id, sequence_id, name, provider, kind) VALUES (1, 3, 'aired', 'tmdb', 'aired');
INSERT INTO ordering (id, sequence_id, name, provider, kind, source_id, label, listed_count)
  VALUES (2, 3, 'tmdb:5e9077d2e640d600151f32bd', 'tmdb', 'dvd', '5e9077d2e640d600151f32bd', 'DVD Order', 23);
INSERT INTO grp (id, ordering_id, number) VALUES (1, 1, 1);
INSERT INTO position (ordering_id, work_id, grp_id, number_in_group, sort_order) VALUES (1, 1, 1, 1, 1.0);
INSERT INTO group_text (grp_id, language, name) VALUES (1, 'en', 'Season 1');

INSERT INTO "grant" (secret_sha256, kind, user_id, scope, issued_at, expires_at, last_used_at, device_name, device_id,
                     locate_hash, offer_key)
  VALUES (X'01', 'session', 2, 'role', 1790000000000, NULL, 1790000100000, 'phone', 1, NULL, NULL),
         (X'02', 'issued_secret', NULL, 'invitation', 1790000000000, 1790600000000, NULL, NULL, NULL, X'0A0B', X'0C0D');
INSERT INTO link_memory (player, network, speed_kbps, speed_log_spread, speed_at, steadiness, steadiness_at, said_at)
  VALUES ('device 1', '2001:db8:1:2::/64', 17000, 0.31, 1790000000000, 1, 1790000000000, 1790000000000);
INSERT INTO locator (session_secret_sha256, hash, issued_at) VALUES (X'01', X'0E0F', 1790000000000);
INSERT INTO realm_key (user_id, master_realm, key, made_at) VALUES (1, 'karlnas', X'0A0B', 1790000000000);
INSERT INTO guessed_match (item_id, guessed_at) VALUES (1, 1790000000000);
INSERT INTO invitation (id, role_id, admin_note, issued_at, made_user_id, made_at)
  VALUES (1, 2, 'Grandma', 1790000000000, NULL, NULL);
INSERT INTO invitation_link (secret_sha256, invitation_id) VALUES (X'02', 1);
INSERT INTO setting (key, value_json, set_by_user_id, set_at)
  VALUES ('should_check_for_updates', 'false', 1, 1790000000000);
INSERT INTO secret (scope, user_id, name, key_id, nonce, ciphertext, last_chars, set_by_user_id, set_at)
  VALUES ('server', NULL, 'tmdb_api_key', 7, X'000102030405060708090A0B0C0D0E0F1011121314151617', X'DEADBEEF',
          'WXYZ', 1, 1790000000000);

INSERT INTO play_position (user_id, file_id, position_ms, report_seq, device_id, updated_at)
  VALUES (2, 1, 3600000, 17, 1, 1790000200000);
INSERT INTO play_event (id, user_id, file_id, device_id, kind, position_ms, happened_at, happened_mono_ms)
  VALUES (1, 2, 1, 1, 'paused', 3600000, 1790000200000, 987654);
INSERT INTO play_span (id, user_id, file_id, device_id, from_position_ms, to_position_ms, started_at, ended_at,
                       is_unattended)
  VALUES (1, 2, 1, 1, 0, 3600000, 1790000100000, 1790000200000, 0);
INSERT INTO purged_watching (user_id, version_id, completed_count, watched_ms, last_watched_at)
  VALUES (2, 1, 2, 7200000, 1790000000000);
INSERT INTO item_hidden (user_id, item_id, hidden_at) VALUES (1, 1, 1790000000000);
INSERT INTO work_watched (user_id, work_id, watched_at, marked_at) VALUES (1, 1, 1790000000000, 1790000000000);
INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
  VALUES (2, 'file', 1, 'subtitle_offset_ms', '-250', 1790000000000),
         (2, 'item', 3, 'audio_language', '{"language":"en","kind":"plain"}', 1790000000000);
INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
  VALUES (1, 'tmdb', '', 1790000000000, 1790000300000, X'1F8B0800');
INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
  VALUES (3, 'tmdb', 'group/2', 1790000000000, 1790000000000, X'1F8B0800');
INSERT INTO fetch_failure (item_id, failure, failed_at) VALUES (2, 'TMDB answered 500 five times', 1790000400000);
INSERT INTO refresh_request (item_id, requested_at, should_fetch_images) VALUES (1, 1790000500000, 1);
INSERT INTO subtitle_text (source_hash, stream_index, ass, vtt, failure, made_at)
  VALUES (X'0102', 3, CAST('[Script Info]' AS BLOB), CAST('WEBVTT' AS BLOB), NULL, 1790000600000),
         (X'0103', 0, NULL, NULL, 'Invalid data found when processing input', 1790000600000);
INSERT INTO device_reader_limit (node_id, device_key, reader_limit, set_at) VALUES (1, 'disk:sdc', 4, 1790000700000);
