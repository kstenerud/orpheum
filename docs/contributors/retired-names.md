> **ABOUTME:** Names a decision retired, which no tracked file outside history may use again.
> `make check-claims` searches the tree for each; `make check-test-weakening` asks a trailer of
> a commit that takes one off.

# Retired names

When a decision renames something, the old name was found left live three times after D227's
renames: in error text, fixtures, log keys, doc comments and the byte socket's field, each time
by an audit and never by a gate (agent-failures A12). This list is that gate's input.

**The rule.** A decision that renames a thing adds a row for each old name, in the commit that
does the rename. `scripts/check_claims.py` then fails on any tracked file that uses the old name
as a whole word (a letter, digit or `_` on either side makes it part of another word:
`serve_debug_pprof` is not found in `should_serve_debug_pprof`). History may use it: the
decision logs, `design/research/`, and [agent-failures.md](agent-failures.md), which quotes
what went wrong. So may this file.

**What goes in.** Only a name that nothing else in the tree means. A contract name as common as
`source`, `hash` or `job` is left out, because a Go variable or a sentence uses the same word
for something else, and a gate that fails on those is one people learn to argue with. The rows
below are the names of [contract-names-2026-09-29.md](design/research/contract-names-2026-09-29.md)
that no file used when the list began (2026-09-30).

**The form.** One row per old name: the name in a code span in the first cell, what replaced it,
and the decision. Removing a row stops the search for that name, so
`scripts/check_test_weakening.py` reports it as a weakening that needs a `Test-Weakened:`
trailer on the commit that removes it.

| Retired | Now | By |
| --- | --- | --- |
| `sourceTrack` | `audioTrackIndex` | D227, D227 §6 |
| `sourceAudioIndex` | `audioTrackIndex` | D227 §6 |
| `checkEveryH` | `checkEveryHours` | D227 |
| `setBy` | `setByUserId` | D227 |
| `rootsAsked` | `askedRootCount` | D227 |
| `outcomeAt` | `lastScanAt` | D227 |
| `diskLimitPercent` | `diskUsedLimitPercent` | D227 |
| `titlesWatched` | `titlesWatchedCount` | D227 |
| `MenuBody` | `PlayableFormatsBody` | D227 §5 |
| `inForce` | `isInForce` | D227 |
| `clockMs` | `sentAt` | D227 |
| `payloadLength` | `payloadBytes` | D227 |
| `repeatCount` | `occurrenceCount` | D227 |
| `maxPixels` | `maxSourcePixels` | D227 |
| `noAccess` | `noAccessReason` | D227 |
| `afterSeq` | `afterCommandSeq` | D227, D227 §5 |
| `positionMonoMs` | `positionReadMonoMs` | D227 |
| `atMonoMs` | `happenedMonoMs` | D227 §6 |
| `preparedMs` | `preparedThroughMs` | D227 §5 |
| `fetchedId` | `fetchedSubtitleId` | D227 §5 |
| `didAskByFileHash` | `didAskByOpensubtitlesHash` | D227 §6 |
| `isHashMatch` | `isOpensubtitlesHashMatch` | D227 §6 |
| `retiredWhy` | `retiredReason` | D227 §6 |
| `posterHash` | `posterSha256` | D227 §6 |
| `path_key` | `reported_path_bytes` | D227 |
| `path_raw` | none: `reported_path_bytes` always holds the bytes | D227 |
| `sidecar_body` | `nfo_body` | D227 |
| `identified_path` | `identified_from_path` | D227 |
| `attached_to` | `attached_to_file` | D227 |
| `server_at` | `happened_at` | D227 |
| `removals_approved` | `are_removals_approved` | D227 |
| `status_at` | `last_status_at` | D227 |
| `provider_ref` | `provider_file_id` | D227 |
| `outcome_at` | `last_scan_at` | D227 §5 |
| `check_every_h` | `check_every_hours` | D227 §5 |
| `error_detail` | `error_detail_json` | D227 §5 |
| `original_lang` | `original_language` | D227 §6 |
| `lang_raw` | `container_language` | D227 §6 |
| `title_raw` | `container_title` | D227 §6 |
| `client_mono_ms` | `happened_mono_ms` | D227 §6 |
| `content_hash_hex` | `source_sha256_hex` | D227 §6 |
| `mtime_ms` | `modified_at` | D227 §6 |
| `fetched_by` | `fetched_by_user_id` | D227 §6 |
| `titles_watched` | `titles_watched_count` | D227 §5 |
| `prepared_ms` | `prepared_through_ms` | D227 |
| `poster_ref` | `provider_poster_path` | D227, D227 §6 |
| `audio_gains_db` | `carried_gains_db_json` | D227 §5, D227 §6 |
| `segment_length_s` | `segment_length_seconds` | D227 §6 |
| `removal_withhold_percent` | `scan_withhold_removals_over_percent` | D227 |
| `removal_withhold_min_files` | `scan_withhold_removals_over_files` | D227 |
| `node_restart_after_s` | `node_restart_after_silent_seconds` | D227 |
| `node_restart_delay_s` | `node_restart_after_exit_seconds` | D227 |
| `refresh_check_every_h` | `metadata_refresh_check_every_hours` | D227 |
| `artwork_removed_grace_d` | `removed_item_artwork_kept_days` | D227 |
| `job_retry_backoff_s` | `job_first_retry_delay_seconds` | D227 |
| `subtitle_track` | `subtitle_used` (a file's) and `subtitle_language` (a work's) | D227, D230 |
| `migrate_space_factor` | `migrate_free_space_db_multiple` | D227 |
| `readers_minimum` | `read_connections_minimum` | D227 |
| `prepared_disk_limit_percent` | `prepared_disk_used_limit_percent` | D227 §5 |
| `loudness_levelling` | `levelling` | D227 §6 |
| `check_for_updates` | `should_check_for_updates` | D227 §6 |
| `serve_debug_pprof` | `should_serve_debug_pprof` | D227 §6 |
| `serve_debug_metrics` | `should_serve_debug_metrics` | D227 §6 |
| `scan_on_node_hello` | `should_scan_on_node_hello` | D227 §6 |
| `secret_hint_chars` | `secret_last_chars_shown` | D227 §6 |
| `prepared_grace_h` | `prepared_kept_after_play_hours` | D227 §6 |
| `prepared_kept_after_play_h` | `prepared_kept_after_play_hours` | D227 §6; owner 2026-09-30 |
| `read_header_timeout_s` | `read_header_timeout_seconds` | D227 §6; owner 2026-09-30 |
| `shutdown_grace_s` | `shutdown_grace_seconds` | D227 §6; owner 2026-09-30 |
| `probe_timeout_s` | `probe_timeout_seconds` | D227 §6; owner 2026-09-30 |
| `status_interval_s` | `status_interval_seconds` | D227 §6; owner 2026-09-30 |
