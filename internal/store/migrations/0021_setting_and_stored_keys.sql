-- SPDX-License-Identifier: GPL-3.0-or-later
-- Setting keys, a preference axis and an override field say what they hold (D227): the keys a
-- person set keep their values under the new names.

UPDATE setting SET key = 'scan_withhold_removals_over_percent' WHERE key = 'removal_withhold_percent';
UPDATE setting SET key = 'scan_withhold_removals_over_files' WHERE key = 'removal_withhold_min_files';
UPDATE setting SET key = 'node_restart_after_silent_s' WHERE key = 'node_restart_after_s';
UPDATE setting SET key = 'node_restart_after_exit_s' WHERE key = 'node_restart_delay_s';
UPDATE setting SET key = 'metadata_refresh_check_every_h' WHERE key = 'refresh_check_every_h';
UPDATE setting SET key = 'removed_item_artwork_kept_d' WHERE key = 'artwork_removed_grace_d';
UPDATE setting SET key = 'job_first_retry_delay_s' WHERE key = 'job_retry_backoff_s';
UPDATE preference SET axis = 'subtitle_choice' WHERE axis = 'subtitle_track';
UPDATE field_override SET field = 'runtime_ms' WHERE field = 'runtime';
