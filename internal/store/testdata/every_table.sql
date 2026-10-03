-- SPDX-License-Identifier: GPL-3.0-or-later
-- One realistic row in every table of the current schema, in reference order, for the backup and restore
-- tests: a copy that reproduces this reproduces every table. Synthetic titles only (D187).

INSERT INTO node (id, name, enrolled_at, last_status_json, last_status_at)
  VALUES (1, 'nas', 1790000000000, '{"sentAt":1790000001000}', 1790000001000);
INSERT INTO library (id, name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at)
  VALUES (1, 'Films', 'movies', 'nfo_then_tmdb', 'interval', 6, 0, 1790000000000);
INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/media/films');
INSERT INTO root_scan (node_id, root_path, completed_at) VALUES (1, '/media/films', 1790000002000);
INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present) VALUES (1, '/media/films', '/media/films/disk2', 2050, 1);
INSERT INTO root_state (node_id, root_path, filesystem_device, is_mount_point, last_scan_outcome, last_scan_at) VALUES (1, '/media/films', 2049, 1, 'complete', 1790000002000);

INSERT INTO user (id, name, role_id, password_hash, is_outsider, created_at)
  VALUES (1, 'owner', 1, CAST('$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA' AS BLOB), 0, 1790000000000),
         (2, 'partner', 2, NULL, 1, 1790000000000);
INSERT INTO device (id, user_id, name, platform, audio_output, added_at, last_signed_in_at)
  VALUES (1, 2, 'Living room TV', 'webos', 'built_in_speakers', 1790000000000, 1790000100000);

INSERT INTO item (id, kind, library_id, added_at) VALUES
  (1, 'title', 1, 1790000000000),
  (2, 'title', 1, 1790000000000),
  (3, 'collection', 1, 1790000000000);
INSERT INTO item_provider_id (item_id, provider, provider_id) VALUES (1, 'tmdb', '10001');
INSERT INTO title (id, kind, media_kind, year, runtime_ms, original_language, followed_source, followed_source_id, identification_state)
  VALUES (1, 'movie', 'video', 2020, 5400000, 'en', 'tmdb', '10001', 'identified'),
         (2, 'extra', 'video', 2020, 120000, 'en', 'none', NULL, 'unidentified');
INSERT INTO edition (id, title_id, created_by, created_at) VALUES (1, 1, 'user', 1790000000000);
INSERT INTO version (id, title_id, edition_id, created_at) VALUES (1, 1, 1, 1790000000000);
INSERT INTO extra_relation (owner_title, owner_edition, extra_title, extra_type, position, asserted_by)
  VALUES (1, 1, 2, 'trailer', 1, 'scanner');

INSERT INTO file (id, library_id, node_id, path, size_bytes, modified_at, payload_hash, payload_bytes, is_identity_confirmed,
                  filing, filing_reason, probe_json, version_id, part_index, first_seen_at, last_seen_at,
                  opensubtitles_hash)
  VALUES (1, 1, 1, '/media/films/Direct Play Film (2020)/Direct Play Film (2020).mp4', 1048576, 1780000000000,
          X'00112233445566778899AABBCCDDEEFF', 1000000, 1, 'listed', 'movie folder',
          '{"format":"mp4"}', 1, 1, 1790000000000, 1790000002000, '8e245d9679d31e12');
INSERT INTO file (id, library_id, node_id, path, size_bytes, modified_at, is_identity_confirmed, filing, filing_reason,
                  sidecar_kind, attached_to_file, first_seen_at, last_seen_at)
  VALUES (2, 1, 1, '/media/films/Direct Play Film (2020)/Direct Play Film (2020).en.srt', 2048, 1780000000000,
          0, 'attached', 'a subtitle beside its video', 'subtitle', 1, 1790000000000, 1790000002000);
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

INSERT INTO item_text (item_id, language, is_original, name, overview, tagline)
  VALUES (1, 'en', 1, 'Direct Play Film', 'A synthetic film.', 'It plays.');
INSERT INTO edition_text (edition_id, language, name) VALUES (1, 'en', 'Director''s Cut');
INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (1, 'en', 'movie', X'4450');
INSERT INTO field_override (item_id, field, language, override_value, origin, set_by_user_id, set_at)
  VALUES (1, 'name', 'en', 'Direct Play Film', 'typed', 1, 1790000004000);
INSERT INTO artwork (id, item_id, kind, language, source, source_ref, source_sha256, is_chosen)
  VALUES (1, 1, 'poster', 'en', 'nfo', 'poster.jpg', X'AABB', 1);
INSERT INTO artwork (id, item_id, kind, source, source_ref, refused_at, refusal, is_missing_at_source)
  VALUES (2, 1, 'backdrop', 'tmdb', '/gone.jpg', 1790000700000, 'no image there', 1);
INSERT INTO tag (id, name) VALUES (1, 'favourite of the house');
INSERT INTO item_tag (item_id, tag_id) VALUES (1, 1);

INSERT INTO collection (id, kind, membership, created_by, followed_source, followed_source_id, identification_state)
  VALUES (3, 'series', 'explicit', 'scanner', 'tmdb', '7615', 'identified');
INSERT INTO collection_member (collection_id, member_title, position) VALUES (3, 1, 1.0);
INSERT INTO ordering (id, collection_id, name, provider) VALUES (1, 3, 'aired', 'tmdb');
INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order) VALUES (1, 1, 1, 1, 1.0);
INSERT INTO group_text (collection_id, group_number, language, name, overview) VALUES (3, 1, 'en', 'Season 1', 'The first.');

INSERT INTO user_deny (user_id, item_id) VALUES (2, 2);
INSERT INTO "grant" (secret_sha256, kind, user_id, scope, issued_at, expires_at, last_used_at, device_name, device_id)
  VALUES (X'01', 'session', 2, 'role', 1790000000000, NULL, 1790000100000, 'phone', 1);
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
                       is_unterminated,
                       is_unattended)
  VALUES (1, 2, 1, 1, 0, 3600000, 1790000100000, 1790000200000, 0, 0);
INSERT INTO favourite (user_id, item_id, added_at) VALUES (2, 1, 1790000000000);
INSERT INTO user_rating (user_id, item_id, rating, rated_at) VALUES (2, 1, 8, 1790000000000);
INSERT INTO watchlist (user_id, item_id, added_at) VALUES (1, 1, 1790000000000);
INSERT INTO progress_hide (user_id, item_id, hidden_at) VALUES (1, 1, 1790000000000);
INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
  VALUES (2, 'file', 1, 'subtitle_offset_ms', '-250', 1790000000000),
         (2, 'work', 3, 'audio_language', '{"language":"en","kind":"plain"}', 1790000000000);
INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
  VALUES (1, 'tmdb', '', 1790000000000, 1790000300000, X'1F8B0800');
INSERT INTO provider_response (item_id, provider, part, fetched_at, last_asked_at, body_gzip)
  VALUES (3, 'tmdb', 'season/2', 1790000000000, 1790000000000, X'1F8B0800');
INSERT INTO fetch_failure (item_id, failure, failed_at) VALUES (2, 'TMDB answered 500 five times', 1790000400000);
INSERT INTO refresh_request (item_id, requested_at, should_fetch_images) VALUES (1, 1790000500000, 1);
INSERT INTO subtitle_text (source_hash, stream_index, ass, vtt, failure, made_at)
  VALUES (X'0102', 3, CAST('[Script Info]' AS BLOB), CAST('WEBVTT' AS BLOB), NULL, 1790000600000),
         (X'0103', 0, NULL, NULL, 'Invalid data found when processing input', 1790000600000);
INSERT INTO device_reader_limit (node_id, device_key, reader_limit, set_at) VALUES (1, 'disk:sdc', 4, 1790000700000);
