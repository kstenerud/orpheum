> **ABOUTME:** Every name in a contract renamed under D227 — the API's JSON, the node protocol,
> library.db's and cache.db's columns, setting and configuration keys — with what each holds.
> Four read-only reviews (2026-09-29) found them; the owner had every one renamed, minor ones too.

# Contract names, 2026-09-29

The M3 audit's lists of names were lost (agent-failures A8's follow-up), so the naming pass over
the four contracts was run again, one review each. Each row is a name a reader had to look up:
what it holds, and what it is called now. [D227](../../decisions/working-notes.md) is the
decision; the rule is standards/go.md §3.

## The API (JSON fields, path and query parameters)

| Was | Holds | Now |
| --- | --- | --- |
| `sourceTrack` | the audio track's place among the file's audio tracks | `sourceAudioIndex` |
| `isDialogueBoost` (an audio rendition) | whether this mix is the boosted one | `isDialogueBoosted` |
| `resumeId` | the device the client was signed in as before (D221) | `previousDeviceId` |
| `?library=`, `?user=` | a library's id, a user's id | `?libraryId=`, `?userId=` |
| `file` (config) | the configuration file's keys | `configFileKeys` |
| `?modified=` (config) | only settings a person changed | `?onlyModified=` |
| override field `runtime` | the runtime, in milliseconds | `runtime_ms`, as it is stored: the API's enum values are snake_case |
| `setBy` (settings, secrets) | the user who set it | `setByUserId` |
| `subject` (a job) | an id whose table depends on the kind | `itemId`, `renditionId`, `streamId`, `fetchedSubtitleId`, `fileId`: one per kind |
| `progress` (a job) | a fraction, 0 to 1 | `progressFraction` |
| `menu` | what the client plays, per axis, in its order | `playableFormats` |
| `video`, `audio`, `subtitle` (in it) | codec and format names | `videoCodecs`, `audioCodecs`, `subtitleFormats` |
| `link` | whether the client is on the local network or remote | `network` |
| `prefer` | the person's override: direct or prepare | `preferredDelivery` |
| `kind` (a playback) | how it plays: direct or hls | `delivery` |
| `reasons` (a playback) | why it is not played directly | `notDirectReasons` |
| `isDialogueBoost`, `isLevelling` (a person's choices) | whether the boost, levelling, is on | `isDialogueBoostOn`, `isLevellingOn` |
| `isCarried` (a subtitle choice) | the choice was carried from another file of the work | `isCarriedFromWork` |
| `allowed`, `remaining` (downloads) | counts of downloads | `allowedDownloads`, `remainingDownloads` |
| `resetAt` (a fetch) | when the download count starts again | `downloadsResetAt` |
| `downloads`, `release` (a candidate) | its download count, the uploader's name for it | `downloadCount`, `releaseName` |
| `?lang=` | a BCP 47 language | `?language=` |
| `bytes`, `diskPercent`, `diskLimitPercent` (prepared) | bytes used, how full its disk is, how full it may get | `usedBytes`, `diskUsedPercent`, `diskUsedLimitPercent` |
| `freeBytes` (a disk) | bytes a non-root user can still write | `availableBytes` |
| `outcome`, `outcomeAt` (a root) | the last scan's result and when it ended | `lastScanOutcome`, `lastScanAt` |
| `checkEveryH` | hours between checks | `checkEveryHours` |
| `node` (a root, a new library, a root action) | the node's configured name | `nodeName` |
| `roots` (a new library, a node) | root paths | `rootPaths` |
| `filings` (a library) | live files counted by filing | `fileCountsByFiling` |
| `rootsAsked` (a scan) | how many roots' nodes were asked | `askedRootCount` |
| `identification`, `?identification=` | the identification state | `identificationState` |
| `source`, `sourceId` (an item) | the source its values follow, and its id there | `followedSource`, `followedSourceId` |
| `posterRef` (a candidate) | the provider's poster path | `providerPosterPath` |
| `season`, `number` (an episode) | its season's number, its number | `seasonNumber`, `episodeNumber` |
| `seq`, `positionMonoMs` (a report); `monoMs` (an event) | the report's number; when the position was read, on the device's monotonic clock; when the event happened, on it | `reportSeq`, `positionReadMonoMs`, `atMonoMs` |
| `speed`, `error` (a finished prepare) | seconds of source per second of running; FFmpeg's failure | `realtimeFactor`, `failure` |
| `encoder` (hardware) | the encoder the node chose | `chosenEncoder` |
| `device`, `noAccess` (a GPU) | its device path; why the node may not open it | `devicePath`, `noAccessReason` |
| `ffmpegHash` | the SHA-256 of FFmpeg's binary | `ffmpegSha256` |
| `watched`, `titlesWatched` | watch statistics; a count of titles | `watchStats`, `titlesWatchedCount` |
| `registration` | the invitation: its secret and expiry | `registrationInvitation` |
| `{file}` (an artwork's bytes) | a variant: size and format | `{variant}` |

## The node protocol

| Was | Holds | Now |
| --- | --- | --- |
| `job` (every command and answer) | the core's job id | `jobId` |
| `mountPoint`, `readOnly` (scan begin) | yes/no | `isMountPoint`, `isReadOnly` |
| `device` (scan begin, mount) | the filesystem's device number | `filesystemDevice` |
| `inForce`, `cause` (confinement) | yes/no; a code from a closed set | `isInForce`, `causeCode` |
| `clockMs`, `doing`, `prepared` (status) | when the node sent it; free text of what it does; its prepared directory's use | `sentAt`, `activity`, `preparedUsage` |
| `bytes`, `diskPercent`, `diskLimitPercent` (prepared usage) | as the API's | `usedBytes`, `diskUsedPercent`, `diskUsedLimitPercent` |
| `freeBytes` (a disk) | bytes a non-root user can still write | `availableBytes` |
| `size` (observed, sidecar) | bytes | `sizeBytes` |
| `payloadLength` | bytes | `payloadBytes` |
| `repeatCount`, `raw`, `mode` (an error) | occurrences from 1; the platform's words; the mode as text | `occurrenceCount`, `platform`, `modeText` |
| `observed`, `sidecars`, `skipped`, `errors`, `stopped` (scan complete) | counts; stopped at its error limit | `observedCount`, `sidecarCount`, `skippedCount`, `errorCount`, `isStoppedByErrors` |
| `reportSeq` (hello reply) | the last report number the core took in | `lastReportSeq` |
| `encoder`; `device`, `noAccess` (hardware, a GPU) | as the API's | `chosenEncoder`; `devicePath`, `noAccessReason` |
| `language`, `channels` (prepare audio) | the BCP 47 tag; the channel count | `languageTag`, `channelCount` |
| `error`, `speed` (a prepare report) | FFmpeg's failure; seconds of source per second | `failure`, `realtimeFactor` |
| `hash`, `?hash=` (a file named on the byte socket) | its payload hash | `payloadHash`, `?payloadHash=` |
| `refused` (subtitle, loudness, artwork answers) | why the node did not try | `refusal` |
| `failed` (loudness) | FFmpeg's words | `failure` |
| `fetched` (extract subtitle) | a fetched subtitle's SubRip text | `fetchedSubRip` |
| `audio` (measure loudness) | the tracks to measure | `tracks` |
| `source` (a track's loudness) | the multichannel sound as it is | `multichannel` |
| `channels` (a loudness track, a probe's audio) | the channel count | `channelCount` |
| `source`, `sizes`, `quality`, `maxPixels` (artwork) | the image's bytes; size names; libwebp quality; the most source pixels | `sourceImage`, `sizeNames`, `webpQuality`, `maxSourcePixels` |
| `width`, `height` (artwork result) | the source's size | `sourceWidth`, `sourceHeight` |
| `size`, `bytes` (an artwork image) | a size name; the WebP bytes | `sizeName`, `webp` |
| `index`, `language`, `profile` (a probe's stream) | the stream's index; the container's language tag; the codec's profile | `streamIndex`, `containerLanguage`, `codecProfile` |
| command `artwork` | encode an image | `encode_artwork` |
| `?after=` (commands) | the last command sequence number taken | `?afterSeq=` |

## library.db

| Was | Holds | Now |
| --- | --- | --- |
| `grant.hash`, `grant.subject`, `grant.parent`, `grant.label` | the secret's SHA-256; the user; the parent grant's; the name of the device a code makes | `secret_sha256`, `user_id`, `parent_secret_sha256`, `device_name` |
| `play_position.watched_at`, `play_position.seq` | when the device read the position; the report's number | `position_read_at`, `report_seq` |
| `play_event.server_at` | when the change happened, on the server's clock | `happened_at` |
| `play_span.unterminated`, `play_span.unattended` | yes/no | `is_unterminated`, `is_unattended` |
| `file.path_key` | the path's bytes as the node reported them | `reported_path_bytes` |
| `file.path_raw` | the same bytes, only when they differ from the text | dropped: `reported_path_bytes` always holds them (D211) |
| `file.sidecar_body` | an NFO's bytes | `nfo_body` |
| `file.filing_reason` (for a sidecar) | its kind: nfo, subtitle, image | a new `sidecar_kind`; the reason column keeps reasons |
| `file.identity` | whether the payload identity is re-confirmed since a rebuild | `is_identity_confirmed` (0 or 1) |
| `file.identified_path`, `file.attached_to`, `file.probe` | the path identification read; the video a sidecar belongs to; FFmpeg's probe, JSON | `identified_from_path`, `attached_to_file`, `probe_json` |
| `stream.disposition`, `stream.channels` | FFmpeg's flags; the channel count | `disposition_flags`, `channel_count` |
| `loudness.sound` | which mix was measured: `source` or `stereo` | `measured_mix`: `multichannel` or `stereo` |
| `root_mount.path`, `root_state.path`, `root_scan.path` | the mount's path; the root's path | `mount_path`, `root_path` |
| `user.outsider`, `library.writable`, `role.builtin`, `artwork.chosen`, `root_state.removals_approved` | yes/no | `is_outsider`, `is_writable`, `is_builtin`, `is_chosen`, `are_removals_approved` |
| `fetched_subtitle.body`, `provider_response.body` | gzipped | `body_gzip` |
| `fetched_subtitle.provider_ref` | OpenSubtitles' file id | `provider_file_id` |
| `node.report_seq`, `node.status`, `node.status_at` | the last report number ingested; the last status, JSON; when it came | `last_ingested_report_seq`, `last_status_json`, `last_status_at` |
| `setting.value`, `field_override.value` | JSON; the frozen value | `value_json`, `override_value` |
| `sort_key.key` | CLDR collation bytes | `collation_key` |
| `position.number`, `position.sort` | the number within its group; the total order | `number_in_group`, `sort_order` |
| `extra_relation.type` | the extra's kind | `extra_type` |
| `title.source`, `.source_id`; the same on `collection` | the source a work follows, and its id there | `followed_source`, `followed_source_id` |
| `item.origin` | the server that gave it; NULL for this one | `origin_server` |

`user_rating.rating` stays: its scale was never decided, and a name would decide it.

## cache.db

| Was | Holds | Now |
| --- | --- | --- |
| `job.subject` | an id whose table depends on the kind | `item_id`, `rendition_id`, `stream_id`, `fetched_subtitle_id`, `file_id`: one per kind |
| `job.args`, `job.progress` | JSON; a fraction | `args_json`, `progress_fraction` |
| `subtitle_text.payload_hash`, `.error` | the hash of the file the track's bytes are in; FFmpeg's failure | `bytes_payload_hash`, `failure` |
| `art_served.hash` | the image's content hash, hex | `content_hash_hex` |
| `prepared_rendition.payload_hash`, `.prepared_ms`, `.error`, `.speed` | the source's hash; how far it is prepared; FFmpeg's failure; seconds of source per second | `source_payload_hash`, `prepared_through_ms`, `failure`, `realtime_factor` |
| `match_candidate.poster_ref` | the provider's poster path | `poster_path` |

## Settings, configuration and stored keys

| Was | Holds | Now |
| --- | --- | --- |
| `removal_withhold_percent` | the share of a root's files a scan would remove, above which its removals wait for an admin | `scan_withhold_removals_over_percent` |
| `removal_withhold_min_files` | the file count it must also be above | `scan_withhold_removals_over_files` |
| `node_restart_after_s`, `node_restart_delay_s` | restart after the node is silent; after it exits | `node_restart_after_silent_s`, `node_restart_after_exit_s` |
| `refresh_check_every_h` | hours between looks for metadata due a refresh | `metadata_refresh_check_every_h` |
| `artwork_removed_grace_d` | days a removed item's artwork is kept | `removed_item_artwork_kept_d` |
| `job_retry_backoff_s` | the first retry's delay; each later one doubles | `job_first_retry_delay_s` |
| override field `runtime` | milliseconds | `runtime_ms` |
| preference axis `subtitle_track` | a subtitle choice: a track, or off | `subtitle_choice` |
| config `migrate_space_factor` | the free space a migration needs, as a multiple of the database | `migrate_free_space_db_multiple` |
| config `readers_minimum` | the fewest read connections | `read_connections_minimum` |
| config `confinement` | a policy: require or report | `confinement_policy` |

## Addendum: one thing, one name, in every layer

A second audit found values the tables above left under two names in two layers, and unclear
names no table listed. The owner: *"yes, align them all"* (D227 §5).

| Was | Where | Holds | Now |
| --- | --- | --- | --- |
| `roots` | node hello | the node's configured root paths, as the API's nodes already say | `rootPaths` |
| `ffmpegHash` | node hello | the FFmpeg binary's SHA-256, as the API says | `ffmpegSha256` |
| `seq` | node report record | the record's place in the node's report stream | `reportSeq` |
| `seq`, `?afterSeq=` | node command, commands long-poll | a command's sequence number | `commandSeq`, `?afterCommandSeq=` |
| `preparedMs` | prepare progress | how far the rendition is prepared, as the cache column says | `preparedThroughMs` |
| `body` | sidecar record | an NFO's bytes, as the column says | `nfoBody` |
| `raw` | a reported path | the path's exact bytes, when they are not UTF-8 | `bytes` |
| `sourceHash` | artwork result | the source image's SHA-256, hex | `sourceSha256` |
| `cause` | API, a node's confinement | why a mechanism is not in force, as the protocol says | `causeCode` |
| `prepared` | API, a node | its prepared copies against their budget, as the protocol says | `preparedUsage` |
| `channels` | API, a stream | the channel count, as the column says | `channelCount` |
| `fetchedId` | API subtitle choice, track, fetch answer; stored choice | the fetched subtitle's id, as the job target says | `fetchedSubtitleId` |
| `isCarried` | stored subtitle choice | carried from another file of the work, as the API says | `isCarriedFromWork` |
| `resetAt` | API, the quota error's detail | when downloads are allowed again, as the success body says | `downloadsResetAt` |
| `MenuBody` | API schema | the formats a device plays, as its field says | `PlayableFormatsBody` |
| `root_state.outcome`, `.outcome_at` | library.db | the last scan's outcome and when, as the API says | `last_scan_outcome`, `last_scan_at` |
| `library.check_every_h` | library.db | hours between scans, as the API says | `check_every_hours` |
| `file.error_detail`, `preference.value` | library.db | JSON | `error_detail_json`, `value_json` |
| `prepared_rendition.audio_gains_db` | cache.db | a JSON array of gains | `audio_gains_db_json` |
| `titles_watched` | a query's column | a count | `titles_watched_count` |
| `source`, `hash`, `sizes` | artwork job's arguments | the source the item follows; the stored image's hash; size names | `followedSource`, `imageHash`, `sizeNames` |
| `refresh` | fetch job's arguments | whether it refreshes rather than matches | `isRefresh` |
| config `prepared_disk_limit_percent` | node config | the disk's used share past which prepares stop, as the protocol says | `prepared_disk_used_limit_percent` |
| seed `outsider`, `writable` | dev seed | booleans, as the columns say | `is_outsider`, `is_writable` |
