-- SPDX-License-Identifier: GPL-3.0-or-later
-- The settings a naming audit renamed (2026-09-26; agent-failures A8) keep the values a person
-- set: each stored row moves to its new key. job_retry_limit counts attempts, not retries;
-- dialogue_boost_default is whether it is on; media_token_idle_h is an expiry after idling, as
-- session_idle_expiry_h is; update_check is whether to check; secret_hint_length counts
-- characters; session_last_seen_resolution_s is the interval between writes; debug_pprof and
-- debug_metrics are whether to serve them.
-- An older binary reads the old keys, finds none, and writes their starting values beside
-- these, which its doctor lists as unused: nothing breaks, so compatible_version stays.

UPDATE setting SET key = 'job_attempt_limit' WHERE key = 'job_retry_limit';
UPDATE setting SET key = 'dialogue_boost_on_by_default' WHERE key = 'dialogue_boost_default';
UPDATE setting SET key = 'media_token_idle_expiry_h' WHERE key = 'media_token_idle_h';
UPDATE setting SET key = 'check_for_updates' WHERE key = 'update_check';
UPDATE setting SET key = 'secret_hint_chars' WHERE key = 'secret_hint_length';
UPDATE setting SET key = 'session_last_seen_write_interval_s' WHERE key = 'session_last_seen_resolution_s';
UPDATE setting SET key = 'serve_debug_pprof' WHERE key = 'debug_pprof';
UPDATE setting SET key = 'serve_debug_metrics' WHERE key = 'debug_metrics';
