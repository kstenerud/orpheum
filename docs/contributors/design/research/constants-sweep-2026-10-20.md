# Constants sweep, 2026-10-20

The owner decided on 2026-10-20: *"we should be looking at constants in the code and preferring to
have them configurable db values unless there's a solid reason not to."* This moves the three-way
rule of 2026-09-30 (an admin's choice of behaviour is a setting, a fact about the deployment is a
config key, a protocol value or internal safety bound is a named constant) towards settings. A
constant now needs a solid reason to stay: changing it breaks something (a wire protocol both sides
agree on, a format, an ordering another value depends on, a security bound), or it is derived from
another value. "Nobody would want to change it" is not enough on its own. This report lists every
candidate in the non-test Go code under `internal/`, `cmd/` and `discovery/`, notes the web
client's equivalents, and gives each one a verdict.

## How to read it

- **SETTING**: should become a row in `internal/core/settings/registry.go`. Each entry gives a
  proposed key (unit in the name), the starting value (the current one), bounds, and the reader.
  "Reader: yes" means the package's Parts already holds an `Int`/`Duration` settings reader;
  "Reader: new" means one has to be wired in from `cmd/orpheum`, as an earlier commit did for discovery.
- **KEEP**: stays a constant, with the reason. **KEEP-derived** means it is computed from, or must
  stay ordered against, another value. **KEEP-by-decision** means a decision fixed it on purpose.
- **CONFIG→SETTING**: a config-file key that is a behaviour tunable, not a deployment fact.
- **NODE**: on the media node, which has no database (see the next section).

**How decisions were read.** Where a decision only *chose a number* (D250 §1, D309 §8 and others
did this, and those values are settings today), the entry is SETTING, and the decision becomes its
`StartSource`. KEEP-by-decision is used only where the decision or the comment says the value is
*fixed, not a setting*, and gives a reason. Pure check intervals (how often a loop asks whether a
setting's time has come) are KEEP-derived: they bound how late something runs, and the behaviour is
already the setting they serve.

## How the node gets values from the core today

The node has no database and reads only its config file (`cmd/orpheum/config.go`, `[node]`). The
core sends it values three ways:

1. **On each command** (`internal/nodeproto`): `Prepare.SegmentLengthSeconds` from
   `hls_segment_length_seconds`, `MakeChunks.SoundKbps` from `chunk_sound_kbps`, quality rungs from
   `chunk_qualities*`, and an `Examine` batch that `scan_video_extensions`, `scan_subtitle_extensions`
   and `scan_videos_per_examine_batch` shape (`internal/core/library/scans.go:671`).
2. **On every long-poll answer** (`nodeproto.Commands`): `DeviceReaderLimits` from the
   `*_reader_limit` settings and `StoreTiers` from the `chunk_kept_*` settings. A change reaches the
   node with the next answer (D240 §5, D309 §8).
3. **In the hello reply**: `DeviceReaderLimits` again (`nodeproto/hello.go:136`).

So a node-side constant can become a core setting by (a) adding a field to the command that uses
it, for values that belong to one run (a watchdog's windows), or (b) adding a field to `Commands`,
for values the node holds all the time (the watcher's quiet time). Either way the protocol version
goes up (`nodeproto.ProtocolVersion`, 33), and the node still needs a starting value for the time
before it hears from the core. The `nodeproto.StartingSegmentLengthSeconds` pattern solves this: one
constant in nodeproto that both the registry and the node read. Two things complicate it:

- **Values the node needs before its hello cannot come from the core.** The encoder trial runs at
  startup (`trialTimeout`).
- **Several node windows are ordered against core values.** The prepare stall window (3 min) must
  stay under the core's `prepareMaxSilence` (5 min), and every node window must stay under
  `nodes.MovementWindow` (30 min). `cmd/orpheum/config.go:634` checks one of these orderings
  (probe timeout against MovementWindow). If the node's windows become settings, these orderings
  have to move into the registry's validation, or be derived.

## cmd/orpheum

- `metadata.go:312` scheduledScansCheckEvery = 15 min: how often the scan schedule asks whether a root is due. **KEEP-derived**: how late a scan starts, against `check_every_hours`.
- `metadata.go:316` retentionSweepEvery = 24 h: how often `*_kept_days` pruning runs (D304 §6). **SETTING** `retention_sweep_every_hours`, 24, 1–168. Reader: the `lengthOrStarting` helper `artwork.go` already uses.
- `metadata.go:352` invariantsCheckEvery = 24 h: periodic invariant check (D303 M7 chose daily). **SETTING** `invariants_check_every_hours`, 24, 1–720, StartSource D303. Reader: `lengthOrStarting`.
- `metadata.go:380` certificateCheckEvery = 24 h: how often renewal is checked. **KEEP-derived**: how late a renewal starts, against a 30-day margin (`renewBefore`).
- `metadata.go:401` scheduledBackupCheckEvery = 5 min: whether a backup is due. **KEEP-derived**: how late a backup starts, against `backup_every_hours`. Could be computed as a twelfth of that setting.
- `metadata.go:410` sweepCheckEvery = 1 min: the sweeping loop's tick. **KEEP-derived**: the comment says it bounds lateness and is not a choice.
- `metadata.go:532` job runner tick = 1 s: how often a runner looks for due retries. **KEEP-derived**: the queue wakes the runner itself, so the tick only bounds lateness.
- `metadata.go:599` uploadLogEvery = 20 s: how often upload need is logged. **KEEP-derived**: tied to chunks' `speedWindow` (D286 §5). It would follow that window if the window became a setting.
- `metadata.go:604` uploadKeptEvery = 5 s: how often qualities are lowered or raised against the upload. **KEEP-derived**: about one chunk's time (D286 §5).
- `metadata.go:609` silenceCheckEvery = 1 s: how often node silence is checked. **KEEP-derived**: how late a silence is noticed, against `node_silent_after_seconds`.
- `metadata.go:615` demoteUnwantedEvery = 5 s: demotes play work nobody waits for. **KEEP-derived**: the comment ties it to half the minimum of `play_priority_kept_seconds`.
- `metadata.go:647` pruneFinishedEvery = 1 h: forgetting ended jobs. **KEEP-derived**: how late, against `finished_jobs_kept_days`.
- `serve.go:198` prepareMaxSilence = 5 min: how long a prepare or a subtitle conversion may go without a word from its node before it is retried. **KEEP-derived**: it must stay above the node's prepareStallWindow (3 min) and its progress cadence (2 s). It moves only if those do.
- `https.go:27` dnsPropagationWait = 30 s: how long a DNS-01 TXT record gets before the CA looks (D243 §6). **SETTING** `certificate_dns_wait_seconds`, 30, 5–600. Reader: new; certificate Parts takes `PropagationWait` at start, so it could also be read once there.
- `nodes.go:24` commandPollHold = 25 s: how long an idle node long-poll is held. **KEEP**: a transport bound below idle timeouts. A command is sent at once whatever the hold, so nothing anyone sees changes with it.
- `reachability.go:16` reachabilityDialWait = 5 s: a TCP dial timeout for the reachability check. **KEEP**: an IO bound with no behaviour to choose.

### Config-file keys (core)

- `server.read_header_timeout_seconds` 10 and `server.shutdown_grace_seconds` 10. **KEEP-by-decision D201**: needed before the database opens, or by a failed start.
- `database.read_connections_minimum`, `migrate_free_space_db_multiple` and `when_unmigratable`. **KEEP-by-decision D200 §2/D201**: needed before the database opens.
- `discovery.retry_first_milliseconds` 1000 and `discovery.retry_most_milliseconds` 300000: the backoff when a call to the discovery fails or signalling drops. **CONFIG→SETTING** `discovery_retry_first_ms`, 1000, 100–60000; `discovery_retry_most_ms`, 300000, 1000–3600000. Reader: yes, `discovery.Parts.Settings` since an earlier commit. The development stack's "most near a second" (D292 §9) would move into the dev seed (D197).
- `https.*`, `turn.*`, `webrtc.*`, `server.listen`/`host_names`/`home_networks`, `log.*`. **KEEP**: deployment facts (addresses, ports, firewall ranges, the CA).

## internal/core/discovery

- `discovery.go:44` addressesEvery = 1 min: how often the core checks whether its addresses changed. **SETTING** `discovery_address_check_every_seconds`, 60, 10–3600. Reader: yes. It sends only on a change, so it adds no load on the service.
- `discovery.go:48` heartbeatEvery = 5 min: the address heartbeat. **KEEP**: protocol §3.4. The discovery's 30-minute offline rule, `probeNonceLifetime` (10 min) and `turn.foundLifetime` (15 min) all assume it.
- `discovery.go:52` anotherCopyEvery = 3 min: how often a refused duplicate core asks again (D293 §1). **SETTING** `discovery_another_copy_check_every_seconds`, 180, 30–3600. Reader: yes. The minimum protects the paid service's load.
- `discovery.go:55` endedEvery = 1 h: how often a core whose discovery ended asks again. **SETTING** `discovery_ended_check_every_minutes`, 60, 5–1440. Reader: yes. The minimum protects the service.
- `discovery.go:58` sessionMargin = 5 min: renews the session before it ends. **KEEP-derived**: against the discovery-issued 1 h session (`single.sessionLasts`). A margin of the session's length or more breaks renewal.
- `discovery.go:133` retryJitter = 0.1 and `signal.go:32` pingJitter = 0.09: how much waits are spread. **KEEP**: spreading, with no behaviour to choose.
- `signal.go:24` signalCheckEvery = 10 s: whether to hold signalling when not claimed. **KEEP-derived**: a newly claimed core wakes the loop at once, so this is a backstop.
- `signal.go:31` pingEvery = 27.5 s and pingsMissedAt = 2: the signalling keepalive. **KEEP**: protocol §2.5. It stays under Cloudflare's 100 s idle close (C11, D292 §9).

## internal/core/auth

- `password.go:114` hashesAtOnce = 4: how many argon2id hashes run at once. Each uses `argon2_memory_kib`, so this sets memory. **SETTING** `password_hashes_at_once`, 4, 1–64. Reader: yes (policy). Read at start, since it sizes a semaphore.
- `password.go:115` hashWaitMost = 10 s: how long a sign-in queues for a hash slot before "busy". **SETTING** `password_hash_wait_most_seconds`, 10, 1–60. Reader: yes.
- `auth.go:396` networkTimes = 4: one network's failed-sign-in limit as a multiple of `signin_failure_limit` (K2). **SETTING** `signin_failure_limit_network_multiple`, 4, 1–100. Reader: yes.
- `auth.go:397` accountEverywhereTimes = 10: the multiple across all networks for one account. **SETTING** `signin_failure_limit_everywhere_multiple`, 10, 1–1000. Reader: yes.
- `auth.go:403` accountPace = 10 s: the wait once the everywhere limit is passed (A13). **SETTING** `signin_everywhere_pace_seconds`, 10, 1–300. Reader: yes.
- `devicecodes.go:35` revealWait = 15 s: how long code entry waits for the TV to reveal its nonce (D321). **SETTING** `device_code_reveal_wait_seconds`, 15, 3–120. Reader: yes. It must stay above the device's 2 s poll, hence the minimum of 3.
- `devicecodes.go:36` revealEvery = 500 ms: how often the core asks the service while waiting. **KEEP**: a polling granularity that adds load on the service.
- `handoff.go:22` handOffLife = 1 min. **KEEP-by-decision**: the comment says "fixed, not a setting". It is a security bound on a code left in browser history (D308 §4).
- `handoff.go:26` deviceHandOffLife = 3 min: how long a TV's hand-off works (L2). **KEEP**: a security bound of the same kind as handOffLife. The owner may want to revisit it.
- `handoff.go:29` handOffsAtMost = 1000, `pairing.go:31` pairingsAtMost = 1000, `limiter.go:24` maxLimiterKeys = 50 000. **KEEP**: memory bounds against floods. The owner chose 50 000 on 2026-10-20.
- `pairing.go:32` pairingsPerNetworkAtMost = 5 and `pairing.go:35` pairingsPerSiteAtMost = 20: abuse limits on pending TV codes (S13, K5). **SETTING** `pairing_codes_per_network_most`, 5, 1–100, and `pairing_codes_per_site_most`, 20, 1–1000. Reader: yes. They are the same kind of limit as `signin_failure_limit`.
- `pairing.go:26` pairingLife = 10 min. **KEEP-by-decision D218 §3**: the comment says it is fixed, as a bound on how long an on-screen code can be taken.
- `limiter.go:18` sweepEvery = 1 min. **KEEP**: memory housekeeping.
- `locators.go:28` ownLinkLife = 90 days: a person's own link at the discovery. **KEEP**: the protocol's number (marked [I]). It matches D154's 90-day session expiry. It could instead be derived from `session_idle_expiry_hours`; the owner may want that.

## internal/core/jobs and internal/platform/jobqueue

- `jobs/view.go:108` healthWindow = 24 h: how far back the job view counts failures (D233 §8 chose a day). **SETTING** `job_health_window_hours`, 24, 1–720. Reader: yes (jobs queue settings).
- `jobqueue/backoff.go:15` LongestRetryDelay = 24 h: the cap on a failing job's doubling retry. The registry's own text says "up to a day". **SETTING** `job_longest_retry_delay_hours`, 24, 1–168. Reader: jobqueue has none, but its policy interface already carries `FirstRetryDelay(ctx)`, and a `LongestRetryDelay(ctx)` beside it in `jobs/queue.go`'s settingsPolicy is the whole change.
- `jobqueue/backoff.go:19` jitterFraction = 0.1. **KEEP**: spreading (D231 §10).
- `jobqueue/destination.go:68-69` FirstProbeDelay/LongestProbeDelay. **KEEP**: these are the fallbacks for settings that already exist.

## internal/core/playback

- `crowding.go:16` slowUnderTenths = 15: a player is "slow" when its link is under 1.5× its quality. **SETTING** `upload_crowded_slow_under_percent`, 150, 100–500. Reader: yes.
- `crowding.go:19` slowPlayersAtLeast = 2: how many slow players count as a crowded house. **SETTING** `upload_crowded_slow_players_at_least`, 2, 1–20.
- `crowding.go:21` linkReportedWithin = 30 s. **KEEP-derived**: three of the player's 10 s reports.
- `crowding.go:24` crowdedKeepsPercent = 80, `:25` crowdedLowerEvery = 10 s, `:26` crowdedFloorPercent = 10, `:29` uncrowdedFor = 1 min, `:30` crowdedGrowsPercent = 10, `:31` crowdedRaiseEvery = 30 s: the crowding controller's step, rate, floor and recovery (upload-budget.md §4c). **SETTING** each: `upload_crowded_keeps_percent` 80 (10–99), `upload_crowded_lower_every_seconds` 10 (1–300), `upload_crowded_floor_percent` 10 (1–100), `upload_uncrowded_for_seconds` 60 (5–3600), `upload_uncrowded_grows_percent` 10 (1–100), `upload_uncrowded_raise_every_seconds` 30 (1–3600). Reader: yes. The controller is built at start, so reading at each look needs `ctx` passed into `look`.
- `lowering.go:18` raiseUnderPercent = 75 and `:20` roomFor = 30 s: when a lowered playback is raised again (D286 §4). **SETTING** `upload_raise_under_percent` 75 (10–99) and `upload_raise_room_seconds` 30 (1–600). Reader: yes.
- `upload.go:22` filmsShareOfUploadPercent = 85: the share of `server_upload_mbps` films may use. That setting's description quotes the 85%. **SETTING** `upload_films_share_percent`, 85, 10–100. Reader: yes. Update `server_upload_mbps`'s description to name the new key.
- `upload.go:26` askedWithin = 30 s. **KEEP-derived**: a playing player asks every chunk. It matches the chunks' inUseWithin.
- `upload.go:145` overAtLowestFor = 20 s: how long the upload is over budget at the lowest qualities before the newest Watch Together member is sent away. **SETTING** `upload_send_away_after_seconds`, 20, 5–600. Reader: yes. `sentAwaySettles` stays derived from it.
- `interest.go:19` interestLapsesAfter = 2 min: how long a silent player keeps its chunks' encoder (D309 §3). **SETTING** `chunk_interest_lapses_after_seconds`, 120, 30–1800. Reader: yes. The minimum stays above three 10 s reports.
- `playback.go:299` ladderLookEvery = 100 ms. **KEEP-derived**: the granularity of `play_ladder_wait_ms`.
- `report.go:213` nowPlayingHeardWithin = 30 s. **KEEP-derived**: its comment says it is "tied to the player's interval, not an opinion".
- `cpu.go:15` cpuCeilingFor = 2 min: how long a playback moved off a quality its node could not make stays below it (D286 §4). **SETTING** `chunk_cpu_ceiling_kept_seconds`, 120, 10–3600. Reader: yes.
- `speeds.go:14` speedFreshFor = 24 h: how long a measured link speed counts (D257 §1 chose a day). **SETTING** `link_speed_fresh_hours`, 24, 1–720. Reader: yes.

## internal/core/chunks

- `capacity.go:21` speedWindow = 20 s: how far back a run's speed is measured, and the settle time after an eviction (D250 §6). **SETTING** `chunk_run_speed_window_seconds`, 20, 5–300. Reader: yes.
- `capacity.go:23` speedSpanAtLeast = 10 s. **KEEP-derived**: half the window. It must be at most the window.
- `offer.go:75` qualityPeakPerAverage = 1.5. **KEEP**: it must equal the node encoder's `-maxrate kbps*3/2` (`node/prepare/encoder.go:79`) and `chunk_qualities`' description. Core and node must agree, so changing it needs a MakeChunks field.
- `stall.go:15` linkShareForQuality = 0.7: the share of a measured link a quality may take (D247 §6). The same 0.7 is in `web/packages/player/src/abr-choice.ts:25` and `web/apps/web/src/moving-down.ts:18`, and nothing holds the three equal. **SETTING** `link_share_for_quality_percent`, 70, 10–100. Reader: yes. It should be sent to clients through `/client-settings` so one value governs all three. Medium risk.
- `ready.go:13` readyAheadMs = 10 s. **KEEP-derived**: as far as a stalled player loads before it plays again (D247 §5, D253 §1), which is the player's behaviour.
- `job.go:156` seekMovedEarlierMs = 131. **KEEP**: an FFmpeg fact (idiosyncrasies.md).
- `ensure.go:18` lookaheadMs = 60 s: how far ahead of a player chunks are on their way (D247 §10). **SETTING** `chunk_lookahead_seconds`, 60, 10–600. Reader: yes.
- `ensure.go:23` runSpanMs = 300 s: the most film one FFmpeg run makes (D247 §10). **SETTING** `chunk_run_span_seconds`, 300, 30–3600. Reader: yes. It must be at least the lookahead.
- `qualities/qualities.go:25-28` Min/MaxKbps, Min/MaxHeight. **KEEP**: they validate a setting, and are format bounds.

## internal/core/nodes

- `exchange.go:158` MovementWindow = 30 min. **KEEP-derived**: a safety bound above every node window. `config.go:634` checks it.
- `send.go:93` unsentGrace = 1 min and `reconcile.go:88` stoppingGrace = 2 min. **KEEP**: bounds for invariant checks, not behaviour.
- `quickpass.go:32` quickPassRecheckEvery = 30 s. **KEEP-derived**: polling granularity for the quick pass's end.

## internal/core/together

- `group.go:12` startingSyncWait = 20 s. **KEEP**, as a fix: it copies `together_sync_wait_seconds`' starting value. It should use `settings.StartingDuration` (the registry's rule: "the fallback is the registry's and never a copy").
- `group.go:127` snapSeeksBeyond = 30 s: how far a seek must jump before it is snapped to a keyframe (D285 §1). **SETTING** `together_snap_seeks_beyond_seconds`, 30, 5–600. Reader: new. together reads settings only through `Tuning`, which `api` fills at attach, so this needs a server-side field there.
- `discovery.go:22` locateDay = 24 h. **KEEP**: the unit of a link's life at the discovery (protocol, [I]).
- `memberships.go:18` credentialsKeptPerMember = 16 and `messages.go:237` diagnosisTextMost = 300. **KEEP**: memory and message bounds. The second matches the web's `noteTextMost`.

## internal/core/api

- `idempotency.go:24` idempotentAnswerKept = 10 min. **KEEP-derived**: longer than any client retry wait (the client's longest is 30 s).
- `idempotency.go:31-33` idempotentAnswersAtMost 10 000, idempotentAnswersSignedOutAtMost 1 000, idempotentSweepEvery 30 s. **KEEP**: memory bounds (K4).
- `slow_requests.go:16` slowRequestAfter = 1 s: the threshold for logging a slow API request. **SETTING** `slow_request_log_after_ms`, 1000, 50–60000. Reader: yes (dependencies.Settings). Reading it on every request costs a lookup, so it should wait for the in-memory settings cache (queued-db-performance-work), or be read at start.
- `slow_requests.go:22` largeResponseAfter = 1 MiB: the threshold for logging a large response. **SETTING** `large_response_log_after_kib`, 1024, 16–1048576. Same note.
- `together_socket.go:30` helloWait = 10 s. **KEEP**: a handshake bound.
- `together_socket.go:39` clientPingEvery = 15 s. **KEEP**: a protocol value equal to the web's `pingEveryMs`. TestTogether_TheSilenceLimitIsThreeOfTheClientsPings holds them equal. `silenceLimit` (3×) is derived from it.
- `chunks.go:42` chunkAskEvery = 250 ms. **KEEP-derived**: the granularity of `chunk_hold_ms`.
- `pacing.go:27-28` paceLeast/MostKbps. **KEEP**: protocol bounds on a request header.
- `media_cookie.go:31` mediaCookieMaxAgeSeconds = 400 days. **KEEP**: the browsers' cap.
- `together_socket.go:319` clientScriptMost, and the huma `maximum:` tags on `limit` (`browse.go:75`, `libraries.go:176`). **KEEP**: API contract and log bounds.

## internal/core/library, readthrough, identify, artwork

- `library/waiting.go:16` namedRemovalsLimit = 10: how many of a withheld scan's removals are named. **SETTING** `scan_withheld_removals_named`, 10, 1–1000. Reader: yes. It is a display count, as `job_view_max_jobs` is.
- `library/home.go:21` recentPositionsReadPerWork = 10. **KEEP-derived**: a read factor per `continue_watching_max_works`.
- `library/purge.go:32` filesPurgedInOneWrite = 200 and `readthrough/readthrough.go:92` fileIDsPerSweepRead = 1000. **KEEP**: internal batch sizes. Any value gives the same answer (D303 DL1).
- `identify/pass.go:163` passBatchRows = 400 and `:170` passReadsOnAReader = 3. **KEEP**: writer batching (D304 §2) with no visible behaviour.
- `identify/match.go:317` searchFoundTheWorkAtMost = 2: matching heuristic (D307 §1). **SETTING** `match_search_found_work_at_most`, 2, 1–20. Reader: new (identify has a reader only in refresh.go). Low priority.
- `identify/placed_by_name.go:106` nearEnough = 0.85: how similar a file name must be to an episode's. **SETTING** `episode_name_match_percent`, 85, 50–100. Reader: new. Low priority.
- `identify/episodes.go:205` minimumSpecialName = 8: the shortest special's name matched by a name's ending. **SETTING** `special_name_min_chars`, 8, 1–100. Low priority.
- `identify/unplaced.go:26` unplacedAtLeast = 5: unplaced episodes before a series is reported. **SETTING** `unplaced_episodes_reported_at_least`, 5, 1–1000.
- `identify/match.go:342` yearOffTier, `refresh.go:107` rollingPriority, `artwork/serve.go:33` makePriority, and refresh.go's 3/4 spread. **KEEP**: ordinals and spreading.
- `identify/unplaced.go:30` unplacedFilesRead, `placed_by_name.go:18` unplacedPathsRead, `report.go:54` filesReported. **KEEP**: read and size bounds.
- `artwork/ceiling.go:23` lowWaterFraction = 0.9: where an eviction sweep stops (D212 §6). **SETTING** `artwork_cache_sweep_to_percent`, 90, 50–99. Reader: yes.
- `artwork/ceiling.go:25` headroomFactor = 1.15: the recommended ceiling's margin, a display value (D212 §7). **SETTING** `artwork_cache_recommended_headroom_percent`, 115, 100–300. Low priority.

## internal/core: decide, textindex, turn, reachability, rtcapi, portmap, certificate, providers

- `decide/levels.go:16` peakCeilingDBTP = -1.0. **KEEP-by-decision D226 §3**: a sound clips above it (the comment says "fixed, not a setting").
- `decide/encode_rate.go:8` encodeKbpsMost = 8000. **KEEP-by-decision cut-a §6**: the h264 profile's rate. The same value is in `node/prepare/encoder.go:35` as the node's fallback, and nothing holds the two equal.
- `textindex/*` maxMarkRun, MaxNameRunes, MaxProseRunes, minTrigramRunes. **KEEP**: format and safety bounds (D48).
- `turn/credentials.go:23` credentialLifetime = 24 h: voice relay credentials (D243 §8). **SETTING** `voice_relay_credential_hours`, 24, 1–168. Reader: new. Low priority.
- `turn/credentials.go:82` foundLifetime = 15 min. **KEEP-derived**: above the 5-minute heartbeat.
- `reachability.go:32` nonceLifetime and `:37` probeNonceLifetime. **KEEP-derived**: the protocol's §2.6 and the heartbeat. stunWait 3 s and stunTries 3. **KEEP**: IO bounds.
- `rtcapi.go:44` gatherTimeout 5 s, `:50` stunGatherTimeout 2 s, `:62` connectWithin 10 s, `:65` requestDeadline 30 s. **KEEP-derived**: ordered stun < gather < the client's `connectTimeoutMs` 10 s (`web/packages/sdk/src/rtc-transport.ts:21`), which connectWithin matches.
- `rtcapi.go:27` MaxConnections = 64 and `:63-64` perSourceLimit/channelLimit = 16. **KEEP**: DoS bounds. MaxConnections could become `webrtc_connections_most` if a household outgrows it. Low priority.
- `rtcapi.go:32` ClientConnections = 4 and `relayed.go:17` MaxRelayedCandidates = 10. **KEEP**: protocol values the client and discovery share.
- `portmap/portmap.go:30` lifetime = 1 h: the mapping lease asked of the router. **SETTING** `port_mapping_lease_minutes`, 60, 2–1440. Reader: new.
- `portmap/portmap.go:37` retryAfter = 5 min: the wait after a failed mapping. **SETTING** `port_mapping_retry_seconds`, 300, 10–86400. Reader: new.
- `portmap/portmap.go:41` permanentRenewal = 30 min: how often a permanent lease is asked for again. **SETTING** `port_mapping_permanent_renew_minutes`, 30, 1–1440. Reader: new.
- `portmap` checkEvery 1 min, releaseWait 3 s, `upnp.go:28` searchWait/searchMX, `pcp.go:22-23` gatewayTries/firstWait. **KEEP**: granularity, IO bounds, and RFC 6886 §3.1 and UPnP values.
- `certificate/certificate.go:60` renewBefore = 30 days: how early a certificate is renewed (a third of Let's Encrypt's 90 days). **SETTING** `certificate_renew_before_days`, 30, 1–60. Reader: new. Let's Encrypt plans shorter lifetimes, so "a fraction of the lifetime" may be the better rule.
- `providers/service.go:153` quotaUnknownResetWait and `:159` maxPaceWait. **KEEP**: their comments say "fixed, not a setting", with reasons.
- `opensubtitles` linkLife 3 h and tokenLife 23 h, `tmdb/images.go:56` notFoundCachedFor 12 h, `cloudflare` challengeTTLSeconds 60. **KEEP**: provider facts. The opensubtitles link retry of 1 s and 2 s: **KEEP**, internal.

## internal/platform and internal/store

- `outbound/retryafter.go:14` maxRetryAfter = 1 h: the cap on a provider's Retry-After (D213 §5 chose it). **SETTING** `provider_retry_after_most_seconds`, 3600, 60–86400. Reader: new, since platform knows no settings. It would be passed in by the providers service. Low priority.
- `outbound` ipv4Delay 300 ms, dialTimeout/KeepAlive 30 s, unnamedRetryAfter 1 s, maxRedirects 10. **KEEP**: RFC 8305, Go's defaults, fallbacks.
- `store/open.go` busyTimeoutMs 15000, cacheSizeKiB 16384, journalSizeLimitBytes 64 MiB. **KEEP-by-decision D200 §1**: pragmas "stay constants because they are not opinions" (DL16 measured them).
- `store/writer_gate.go:16` backgroundPassedOverAtMost = 4. **KEEP**: its comment calls it "a safety bound, not a tuning" (DL11).
- `store/open.go:100` SlowWriteAfter = 1 s: the threshold for logging a slow write. **SETTING** `slow_write_log_after_ms`, 1000, 10–60000. Reader: new. The store opens before settings can be read, so it would need a setter called after open.
- `store/invariants.go:22` violationRowsShown = 10. **KEEP**: a log bound.

## discovery/ (the module)

`discovery/single` is the one-core discovery that `orpheum discovery` serves on a VPS. It has no
settings registry and no database, and `discoveryproto` and `proof` are shared with the closed
service. There is nowhere to put a DB setting, so:

- `single.go:30-32` nonceLasts 5 min, sessionLasts 1 h, claimLasts 30 min; `client.go:21` bootstrapLasts 5 min. **KEEP**: protocol lifetimes (spec §3.1–3.3).
- `realm.go:16` realmAnswerWait 5 s. **KEEP-by-decision D297 §2, D312 §17**: equal timing for every failed sign-in.
- `session.go:17` instancePingWait 10 s (D293 §1) and `client.go:268` offerWait 15 s. **KEEP**, or config keys of `orpheum discovery` if they ever need tuning.
- `client.go:221-222` deadEndWindow 7 days, deadEndsKept 10 000; `client.go:265` openOffersMost 8. **KEEP**: protocol and memory bounds (S5).
- `discoveryproto` MaxOffersAtOnce 4 and realm name lengths; `proof/realm.go:17` RealmRounds 600 000. **KEEP**: protocol. The web's `realmRounds` must match.

## NODE (internal/node and the [node] config keys)

- `prepare/prepare.go:29` prepareStartGrace 2 min and `:36` prepareStallWindow 3 min; `chunks/maker.go:40-41` runStartGrace 1 min and runStallWindow 2 min; `readthrough/read.go:23,28` readStartGrace 2 min and readStallWindow 3 min; `subtitle/extract.go:30-34` track and convert windows (10 min, 10 min, 1 min, 1 min, 5 min). **NODE**: these are the watchdog windows. Each could ride on its command (`Prepare`, `MakeChunks`, `ReadThrough`, `ExtractSubtitle`) as a field filled from a core setting. They are ordered under `prepareMaxSilence` (5 min) and `MovementWindow` (30 min), and the NFS-retry reasoning in their comments describes the deployment. Recommendation: leave them until a stall misfires, then convert them as one group with the orderings checked in the registry.
- `execx/watch.go:45` deadlineFactor 3 and `:49` shortestDeadline 10 min. **NODE**: these could ride on each `WatchedRun`'s command in the same way. `execx` watchCheckEvery, quit/term graces, waitDelay, defaultKillGrace, spinningShare, clockTicksPerSecond and unknownLengthDeadline: **KEEP**, process-supervision bounds ordered within waitDelay, and kernel facts.
- `prepare/encoder.go:83` trialTimeout 30 s. **KEEP**: it runs at startup, before the hello, so the core cannot supply it.
- `prepare/prepare.go:40` and `chunks/maker.go:44` unmeasuredRealtimeFactor 0.5. **KEEP**: a first guess that measurement replaces.
- `chunks` cutEarlyMs, soundSeekEarlyMs, spanToleranceMs, TimelineOffsetMs and aacPriming; `readthrough/timeline.go:35` longestTimeline. **KEEP**: format, FFmpeg facts, and sanity bounds.
- `agent/answer.go:29` result retry 1 s→30 s and `agent/agent.go:97` defaultReadStallWindow. **KEEP-derived**: under `node_silent_after_seconds`, and derived from the probe timeout.
- `watch/watcher.go:25` maxHoldQuietSpells 20: how long a never-pausing copy is held. **NODE**: it goes with `watch_quiet_seconds` (below).
- `nodeproto/prepare.go:24` PreparedSoundKbps 192: a prepared copy's AAC rate. **NODE / SETTING** `prepared_sound_kbps`, 192, 32–320, carried on `Prepare` as `chunk_sound_kbps` is on `MakeChunks`. A change has to remake copies, as a segment-length change does (D217 §3). Medium risk.
- namedfile, hardware, probe, container and hash bounds. **KEEP**: memory and format bounds.

Config keys:

- `name`, `core_socket`, `root_paths`, `ffmpeg_dir`, `confinement_policy`, `read_header_timeout_seconds`. **KEEP**: deployment and security (D107, D200 §2).
- `encode_slot_count`, `remux_slot_count`, `slowest_read_mib_per_second`. **KEEP-by-decision**: P36 and cut-a §5.3 treat them as facts about the machine.
- `prepared_budget_gib`, `prepared_disk_used_limit_percent`, `prepared_disk_min_free_mib`. **KEEP-by-decision D217 §5**: facts about the node's own disk.
- `prepared_kept_after_play_hours` 72 and `prepared_watched_minutes` 10. **NODE**: behaviour opinions, not disk facts. They could be sent on `Commands` as `StoreTiers` is. D217 §5 put them in the node's config, so the owner may want to revisit that.
- `status_interval_seconds` 10. **NODE**: it must stay "well under" the *core setting* `node_silent_after_seconds`, and nothing checks this across the two processes. The core could send it in the hello reply, derived as a third of `node_silent_after_seconds`.
- `progress_every_seconds` 2 (D234 §5). **NODE**: the core's prepareMaxSilence relies on it. It could ride on `Prepare`. Low value.
- `watch_quiet_seconds` 30 (D272 §9). **NODE**: a behaviour opinion (how soon a new film appears). It could be sent on `Commands`.
- `max_errors_per_root` 1000 (D40). **NODE**: a behaviour opinion. It could ride on `List`.
- `probe_timeout_seconds` 60 and `ended_work_kept_hours` 168. **KEEP** as config: the first is ordered against MovementWindow by config.go's check, and the second is a dedupe safety bound (D238 §2).

## Web client (brief)

The client gets server settings two ways: `GET /client-settings` (`internal/core/api/client_settings.go`;
the three `screen_refresh_*` and `should_show_developer_pages`), and the Watch Together welcome's
`Tuning` (`internal/core/together/messages.go:486`; the `together_*` settings). A client constant
needs a starting value before it reads either. The pattern exists: `polling.ts`'s
`backgroundChangeWaits`, held equal to the registry by
TestRegistry_TheWebClientsWaitsBeforeItReadsThemAreTheScreenRefreshSettingsStarts.

- **Candidates for /client-settings:** the player's ABR tunables (`player/src/abr-choice.ts`
  linkShareForQuality 0.7, linkShareWhenAhead 0.95, voiceLinkShare 0.5, commonStartHeight 720;
  `step-up.ts` holdBeforeStepUpMs 120 s, stepUpHeadroom 1.5, stepUpBufferMs 20 s; `stalls.ts`
  stallAfterMs 1 s, slowLoadAfterMs 15 s; `chunked.ts` chunkFirstByteWaitMs 30 s,
  chunkAskAgainCount 90); `apps/web/src/moving-down.ts` linkShareForQuality 0.7 and askReadyEveryMs
  3 s; `kept-path.ts` betterPathFirstMs 30 s → betterPathMostMs 10 min; `signed-out-watch.ts`
  sessionCheckEveryMs 60 s; `use-request.ts` slowAfterMs 2 s; `watching.ts` warmUpKeptMs 30 min.
  The three copies of 0.7 (two client files and the core's `chunks/stall.go`) argue most strongly
  for one setting.
- **Candidates for Tuning:** `together-load-speed.ts` (aimAheadAtMostMs, playerLoadsAheadMs,
  loadWindowMs), `together-follow.ts` checkDriftEveryMs 1 s, `together-drift.ts` nudgeForMs 1 s,
  `together-lead.ts` pause windows, `together-refill.ts` seekReadyAheadMs, and the voice levelling
  constants (`together-voice-level.ts`).
- **KEEP (protocol pairs):** `together-connection.ts` pingEveryMs 15 s, which equals the core's
  clientPingEvery and a test holds them equal; `rtc-transport.ts` clientConnections 4,
  maxMessageBytes 64 KiB and connectTimeoutMs 10 s, which equal rtcapi's values; `relayed-offer.ts`
  relayedCandidatesMost 10; `realm.ts` realmRounds; `player/src/report.ts` reportEveryMs 10 s,
  which the core's 30 s windows are derived from. If reportEveryMs becomes a setting, those windows
  must be derived from it.
- The reconnect backoffs (`together-connection.ts` 500 ms → 30 s; `rtc-transport.ts` replace
  1 s → 30 s) are internal backoff, like the Go retry jitters. **KEEP**.

## Summary

| Verdict | Count |
| --- | --- |
| SETTING (Go, core; proposed keys) | 52 |
| CONFIG→SETTING | 2 |
| NODE (could come from the core; entries) | 10 |
| KEEP-by-decision (explicit) | 13 |
| KEEP / KEEP-derived | about 110 (named constants and groups listed above) |
| Web: candidate client settings | about 25, in two groups |

The counts treat a named group, such as the six crowding constants or the subtitle windows, as its
members.

## Proposed batching, smallest risk first

1. **Already-read packages, plain durations and counts** (reader exists; each is a registry row
   plus a read at the use site, as an earlier commit): discovery's `addressesEvery`, `anotherCopyEvery` and
   `endedEvery`, plus the two `discovery.retry_*` config keys (CONFIG→SETTING; dev seed takes over
   D292 §9's near-a-second); `jobs` healthWindow; `library` namedRemovalsLimit; cmd/orpheum's
   `retentionSweepEvery`, `invariantsCheckEvery` and `dnsPropagationWait` (the last through
   lengthOrStarting at start).
2. **auth limits**: networkTimes, accountEverywhereTimes, accountPace, pairing per-network and
   per-site, revealWait, hashWaitMost, hashesAtOnce (read at start). The reader exists in auth's
   policy. They are security-relevant, so the bounds need care.
3. **jobqueue's longest retry**: add `LongestRetryDelay(ctx)` to the policy interface beside
   `FirstRetryDelay`.
4. **playback upload and crowding controller** (11 settings, reader exists). They form one
   controller and should be tuned together. Each needs `ctx` plumbed into `look` and the lowering
   functions. Includes `filmsShareOfUploadPercent`, whose number `server_upload_mbps`' description
   quotes.
5. **chunks and playback timing**: speedWindow, lookaheadMs, runSpanMs, interestLapsesAfter,
   cpuCeilingFor, speedFreshFor. These have ordering bounds: run span at least lookahead, lapse
   above three reports.
6. **New readers wired into Parts**: portmap (three settings), certificate (renewBefore, plus the
   DNS wait if it is not read in cmd), turn (credentialLifetime), identify's matching heuristics
   (four settings, low priority), together's snapSeeksBeyond (through `Tuning`), and outbound's
   maxRetryAfter (passed in by providers). The store's SlowWriteAfter needs a setter after open.
   The api's slow and large request thresholds should wait for the in-memory settings cache.
7. **Shared with the client**: linkShareForQuality as one setting in `/client-settings`, then the
   player's ABR tunables, each with a registry-equality test as `polling.ts` has.
8. **Node, last, as one protocol bump**: the watchdog windows on their commands; `status_interval`
   derived in the hello reply; `watch_quiet_seconds`, `prepared_kept_after_play_hours`,
   `prepared_watched_minutes` and `max_errors_per_root` on `Commands` or their command;
   `PreparedSoundKbps` on `Prepare`. The cross-process orderings (MovementWindow,
   prepareMaxSilence) move into the registry's checks.

## Surprises and owner questions

- **Copies the registry's rule forbids:** `together/group.go:12` startingSyncWait copies
  `together_sync_wait_seconds`' starting value instead of using `settings.StartingDuration`.
- **Unenforced cross-file agreements:** linkShareForQuality 0.7 (core, player, web app) and
  encodeKbpsMost 8000 (core decide, node prepare) are each defined twice or three times with nothing
  holding them equal. The ping pair has a test; these do not.
- **An ordering across processes nothing checks:** the node's `status_interval_seconds` (config)
  must stay well under the core's `node_silent_after_seconds` (setting). An admin can set the
  setting to 5 and every node goes silent.
- **Explicitly fixed by comment or decision:** handOffLife, pairingLife (D218 §3),
  peakCeilingDBTP (D226 §3), quotaUnknownResetWait, maxPaceWait, the store pragmas (D200 §1),
  backgroundPassedOverAtMost, realmAnswerWait (D297/D312 §17), and the D217 §5 node disk budgets.
- **Revisit candidates:** D217 §5 put `prepared_kept_after_play_hours` and
  `prepared_watched_minutes` in the node's config, but they are behaviour, not disk facts.
  `ownLinkLife` (90 days, [I]) could be derived from `session_idle_expiry_hours`.
- **The discovery's own load:** `endedEvery`, `anotherCopyEvery` and `revealEvery` are polls of the
  owner's paid service. As settings, their minimums are what protects the service. Connect limits
  enrolments, not status calls.
