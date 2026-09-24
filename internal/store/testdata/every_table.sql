-- SPDX-License-Identifier: GPL-3.0-or-later
-- One realistic row in every table of schema v1, in reference order, for the backup and restore
-- tests: a copy that reproduces this reproduces every table. Synthetic titles only (D187).

INSERT INTO node (id, name, enrolled_at, report_seq, status, status_at)
  VALUES (1, 'nas', 1790000000000, 42, '{"doing":"idle"}', 1790000001000);
INSERT INTO library (id, name, kind, metadata_source, change_detection, check_every_h, writable, created_at)
  VALUES (1, 'Films', 'movies', 'nfo_then_tmdb', 'interval', 6, 0, 1790000000000);
INSERT INTO library_root (library_id, node_id, path) VALUES (1, 1, '/media/films');
INSERT INTO root_scan (node_id, path, completed_at) VALUES (1, '/media/films', 1790000002000);
INSERT INTO root_mount (node_id, root, path, device, present) VALUES (1, '/media/films', '/media/films/disk2', 2050, 1);
INSERT INTO root_state (node_id, path, device, mount_point, outcome, outcome_at) VALUES (1, '/media/films', 2049, 1, 'complete', 1790000002000);

INSERT INTO user (id, name, role_id, password_hash, outsider, created_at)
  VALUES (1, 'owner', 1, CAST('$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA' AS BLOB), 0, 1790000000000),
         (2, 'partner', 2, NULL, 1, 1790000000000);
INSERT INTO device (id, user_id, name, platform, audio_output, first_seen, last_seen)
  VALUES (1, 2, 'Living room TV', 'webos', 'tv_speakers', 1790000000000, 1790000100000);

INSERT INTO item (id, kind, library_id, created_at) VALUES
  (1, 'title', 1, 1790000000000),
  (2, 'title', 1, 1790000000000),
  (3, 'collection', 1, 1790000000000);
INSERT INTO item_provider_id (item_id, provider, provider_id) VALUES (1, 'tmdb', '10001');
INSERT INTO title (id, kind, media_kind, year, runtime_ms, original_lang, source, source_id, ident)
  VALUES (1, 'movie', 'video', 2020, 5400000, 'en', 'tmdb', '10001', 'identified'),
         (2, 'extra', 'video', 2020, 120000, 'en', 'none', NULL, 'unidentified');
INSERT INTO edition (id, title_id, created_by, created_at) VALUES (1, 1, 'user', 1790000000000);
INSERT INTO version (id, title_id, edition_id, created_at) VALUES (1, 1, 1, 1790000000000);
INSERT INTO extra_relation (owner_title, owner_edition, extra_title, type, position, asserted_by)
  VALUES (1, 1, 2, 'trailer', 1, 'scanner');

INSERT INTO file (id, library_id, node_id, path, size, mtime, payload_hash, payload_len, identity, filing,
                  filing_reason, probe, version_id, part_index, first_seen, last_seen)
  VALUES (1, 1, 1, '/media/films/Direct Play Film (2020)/Direct Play Film (2020).mp4', 1048576, 1780000000000,
          X'00112233445566778899AABBCCDDEEFF', 1000000, 'confirmed', 'listed', 'movie folder',
          '{"format":"mp4"}', 1, 1, 1790000000000, 1790000002000);
INSERT INTO file (id, library_id, node_id, path, size, mtime, identity, filing, filing_reason, attached_to,
                  first_seen, last_seen)
  VALUES (2, 1, 1, '/media/films/Direct Play Film (2020)/Direct Play Film (2020).en.srt', 2048, 1780000000000,
          'confirmed', 'attached', 'sidecar subtitle', 1, 1790000000000, 1790000002000);
INSERT INTO stream (id, file_id, media_file, stream_index, kind, codec, lang_raw, lang, title_raw, disposition,
                    channels)
  VALUES (1, 1, 1, 1, 'audio', 'aac', 'eng', 'en', 'Stereo', 1, 2),
         (2, 2, 1, NULL, 'subtitle', 'subrip', 'en', 'en', NULL, 0, NULL);
INSERT INTO fetched_subtitle (id, media_file, provider, provider_ref, lang, format, body, fetched_by, fetched_at)
  VALUES (1, 1, 'opensubtitles', 'os-1', 'de', 'srt', CAST('1\n00:00:01,000 --> 00:00:02,000\nHallo\n' AS BLOB),
          2, 1790000003000);

INSERT INTO item_text (item_id, lang, is_original, name, overview, tagline)
  VALUES (1, 'en', 1, 'Direct Play Film', 'A synthetic film.', 'It plays.');
INSERT INTO edition_text (edition_id, lang, name) VALUES (1, 'en', 'Director''s Cut');
INSERT INTO sort_key (item_id, locale, listing, key) VALUES (1, 'en', 'movie', X'4450');
INSERT INTO field_override (item_id, field, lang, value, origin, set_by, set_at)
  VALUES (1, 'name', 'en', 'Direct Play Film', 'typed', 1, 1790000004000);
INSERT INTO artwork (id, item_id, kind, lang, source, source_ref, content_hash, chosen)
  VALUES (1, 1, 'poster', 'en', 'nfo', 'poster.jpg', X'AABB', 1);
INSERT INTO tag (id, name) VALUES (1, 'favourite of the house');
INSERT INTO item_tag (item_id, tag_id) VALUES (1, 1);

INSERT INTO collection (id, kind, membership, created_by) VALUES (3, 'series', 'explicit', 'scanner');
INSERT INTO collection_member (collection_id, member_title, position) VALUES (3, 1, 1.0);
INSERT INTO ordering (id, collection_id, name, provider) VALUES (1, 3, 'aired', 'tmdb');
INSERT INTO position (ordering_id, title_id, grp, number, sort) VALUES (1, 1, 1, 1, 1.0);
INSERT INTO group_text (collection_id, grp, lang, name, overview) VALUES (3, 1, 'en', 'Season 1', 'The first.');

INSERT INTO user_deny (user_id, item_id) VALUES (2, 2);
INSERT INTO "grant" (hash, kind, subject, scope, issued_at, expires_at, last_seen, label, device_id)
  VALUES (X'01', 'session', 2, 'role', 1790000000000, NULL, 1790000100000, 'phone', 1);
INSERT INTO setting (scope, user_id, key, value, set_by, set_at)
  VALUES ('server', NULL, 'update_check', 'false', 1, 1790000000000),
         ('user', 2, 'caption_size', '"large"', 2, 1790000000000);
INSERT INTO secret (scope, user_id, name, key_id, nonce, sealed, hint, set_by, set_at)
  VALUES ('server', NULL, 'tmdb_api_key', 7, X'000102030405060708090A0B0C0D0E0F1011121314151617', X'DEADBEEF',
          'WXYZ', 1, 1790000000000);

INSERT INTO play_position (user_id, file_id, position_ms, seq, device_id, updated_at)
  VALUES (2, 1, 3600000, 17, 1, 1790000200000);
INSERT INTO play_event (id, user_id, file_id, device_id, kind, position_ms, server_at, client_mono_ms)
  VALUES (1, 2, 1, 1, 'paused', 3600000, 1790000200000, 987654);
INSERT INTO play_span (id, user_id, file_id, device_id, pos_from_ms, pos_to_ms, t_from, t_to, unterminated,
                       unattended)
  VALUES (1, 2, 1, 1, 0, 3600000, 1790000100000, 1790000200000, 0, 0);
INSERT INTO favourite (user_id, item_id, at) VALUES (2, 1, 1790000000000);
INSERT INTO user_rating (user_id, item_id, rating, at) VALUES (2, 1, 8, 1790000000000);
INSERT INTO watchlist (user_id, item_id, at) VALUES (1, 1, 1790000000000);
INSERT INTO progress_hide (user_id, item_id, at) VALUES (1, 1, 1790000000000);
INSERT INTO preference (user_id, scope, scope_id, axis, value, set_at)
  VALUES (2, 'file', 1, 'subtitle_offset_ms', '-250', 1790000000000);
INSERT INTO subtitle_quota (user_id, provider, day, used) VALUES (2, 'opensubtitles', '2026-09-24', 3);
INSERT INTO provider_response (title_id, provider, fetched_at, body)
  VALUES (1, 'tmdb', 1790000000000, X'1F8B0800');
