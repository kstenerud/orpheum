> **ABOUTME:** An audit of the tunable values a person meets (waits, thresholds, margins, steps,
> quality rungs, UI timings) for what backs each one: research, one measurement, reasoning, or a
> guess. Ranks the guesses by how much a wrong value would hurt, groups them, and says what
> research or measurement would settle each group. Proposes, decides nothing. 2026-10-29.

# SWAG values audit

- **Date:** 2026-10-29, read from the code at an earlier commit.
- **Asked by** the owner: *"Run an audit … that checks the codebase for SWAG (some wild-ass guess)
  values that have no research to back them. Likely not every value needs scrutiny, but there are
  probably values we've chosen that affect the usability of the app, without doing any research
  about it."*
- **Scope:** the player (`web/packages/player/src`), the SDK (`web/packages/sdk/src`), the web
  app and cast receiver (`web/apps/*/src`), the settings registry (`internal/core/settings`), and
  the core and node packages that shape playback (`internal/core/{chunks,playback,together,decide}`,
  `internal/node/{encode,chunks}`). Lab pages, tests, protocol pairs, memory and log bounds, and
  security limits are left out: they do not shape what a person experiences, or a spec fixes them.
- **Relation to earlier work:** [constants-sweep-2026-10-20.md](constants-sweep-2026-10-20.md)
  asked *where* each value should live (setting, config key, constant). This audit asks *why it
  has its number*. The sweep turned many constants into settings; a setting's `StartSource` now
  says where its start came from, which made the registry the easiest part to audit.

## Summary

**160 values (or tight groups of values) shape what a person experiences.** Counted by
their backing, as the code and the documents it cites show it:

| Class | What it means | Count |
| --- | --- | --- |
| RESEARCHED | cites research, a spec, or other players' code, and the source says it | 31 |
| MEASURED-ONCE | one observation on one device or one film | 25 |
| REASONED | derived from other values or first principles; the base may itself be a guess | 28 |
| SWAG | a round number, "best guess", the owner's pick with no evidence, or a comment that restates the value | 76 |

The counts are from the inventory at the end; a group of values that moves together, such as the
four chunk keep times, counts once. Values that are *policy* (how long an invitation lasts, when an
idle session ends) are left out: no research settles them.

**The guesses are not where the engineering effort went.** The bitrate controller and Watch
Together's drift rules are the best-researched parts of the code, with other players' source read
and papers cited. The guesses cluster in four other places: what happens around a stall or a slow
start, how the server shares a home's upload, the quality ladder's real picture quality, and the
TV remote's timings.

**The top five groups to research first**, with an effort estimate each:

| # | Group | Why first | Effort |
| --- | --- | --- | --- |
| 1 | **Starting and stalling** (§1): 15 s before a slow start is offered a lower quality, 10 s / 150% before resuming, 6 s before a quality is given up as unplayable | Every playback meets it; a wrong value is a stall or a long black screen. The research we have says abandonment starts after about 2 s of start delay | 1 day: read the start and stall reports already in staging's and nas.example's logs, then 2–3 shaped-link runs on the Fire TV |
| 2 | **Sharing the upload** (§2): eleven controller values, six of them marked "best guess" in their own plan | Every viewer away from home on a household with a thin upload (the owner's own DS-Lite line). A wrong value lowers everyone's picture, or sends a Watch Together member away after 20 s | 2–3 days: a shaped-upload simulation with three players, plus half a day reading how Plex and Jellyfin share a remote-streaming cap |
| 3 | **The quality ladder and the encoder** (§3): rungs from Apple's spec "to be tuned on the owner's films", never tuned; x264 `veryfast` measured for speed only | Every encoded playback. Apple's rates assume a good encoder; at `veryfast` the same rate may look worse | 1–2 days: VMAF of each rung at two presets on five of the owner's films, on gpuhost |
| 4 | **The chunk length behind other values** (§4): the setting starts at 2 s, real chunks are 4–10 s, and several comments reason from "the ten-second grid" | Cheap to check, and five values are derived from it | 2 hours: read the grids in the database and list the chunk lengths |
| 5 | **TV remote timings** (§5): the held-seek acceleration, a gamepad's repeat, the receiver's double press, "Still watching?" | Every TV user, every evening. A wrong value is an overshoot, a missed or an accidental exit | 1 day: the lab's input page on the Fire TV and the TCL for repeat cadence and double-press gaps (15 minutes of the owner at the TV), plus the platform guidelines already looked up here |

Some values the comments called "usual" without a source were checked for this audit and match
the field: the controls' 5 s auto-hide (Media3's default), the 10 s seek step, the 10 s autoplay
countdown, the subtitle and sound offset steps, and the 2 s "still waiting" text. They need no
more work (§8).

## How the values were classified

- **RESEARCHED** needs a source that says the number, opened and checked. The cited research was
  opened for the values in the ranked list. Two kinds are marked: *one source* (copied from one
  other project, such as Kyoo's 60 s look-ahead) and *textbook* (a statistics default the comment
  itself says is "not measured on our links").
- **MEASURED-ONCE** is a real observation, but on one device, one film or one evening.
- **REASONED** is derived ("three of the player's 10 s reports"). Where the base is a guess, the
  derived value inherits it, and the entry says so.
- **SWAG** includes the owner's own picks given with no evidence ("10% also sounds fine",
  "5 minutes"). These are valid design choices, but they are guesses about what people will
  experience, which is what the owner asked to find. A comment that only calls a value
  "a justified constant" or "the usual" without a source is also SWAG.

## The ranked findings

Ranked by how often a person meets the value and how bad a wrong one is. Each says where it is,
its value, what it governs, what is wrong with its backing, and what would settle it.

### 1. Starting and stalling

These decide how long a person looks at a spinner, and whether a stall repeats.

1. **`slowLoadAfterMs` = 15 s** (`web/packages/player/src/stalls.ts:17`). How long a first load or
   a seek's load may leave the player unable to play before it counts as a stall, so a lower
   quality is offered. *Backing:* SWAG, the owner's choice (2026-10-05, D247 §2). *Problem:* our own
   research cites abandonment from about 2 s of start delay (slow-links.md §6, Krishnan and
   Sitaraman IMC'12), and Media3 starts after 1 s of buffer. A person on a slow link waits 15 s
   of black before anything changes, and again after each seek. *Settle:* read start-to-play times
   from the player reports in staging's and nas.example's logs; the share of starts over 5, 10 and
   15 s says whether a shorter wait would have helped, and how often it would offer a lower
   quality that was not needed.
2. **`refillAheadMs` = 10 s** (`web/packages/sdk/src/together-media.ts:18`) and
   **`ready_ahead_of_next_chunk_percent` = 150** (registry, `web/packages/sdk/src/ready-ahead.ts:37`).
   How much a player must have loaded before it plays again after a stall or a seek. *Backing:*
   the 10 s is MEASURED-ONCE (the owner's first Watch Together test, 2026-10-05); the 150% is
   SWAG, the owner's choice (2026-10-28, "a margin of 1.5"). *Problem:* slow-links.md §6 found
   that no player documents why it chose its numbers, that Media3 resumes after 2 s, and that a
   fixed 10 s turns a stall loop into "longer runs with pauses, not smooth playback". *Settle:*
   count, from the logs, how often a resume is followed by another stall within 30 s, at each
   margin; try 100%, 150% and 200% on a shaped link in the lab.
3. **`standsStillAtMostMs` = 6 s, `chunksWhileStill` = 2, `chunksNotHeld` = 2,
   `heldAheadAtLeastS` = 4** (`web/packages/player/src/abr-choice.ts:339-366`). When a quality is
   given up as one the device cannot decode. *Backing:* MEASURED-ONCE: one film, one browser
   (Firefox on macOS, D266). *Problem:* too short gives up a good quality on a slow decoder start
   (a Fire TV after a seek); too long freezes the picture. The numbers were set to catch one bug.
   *Settle:* log every give-up with its device; any give-up on a device that later played that
   quality is a false one.
4. **`play_ladder_wait_ms` = 2000** (registry). How long a film not cut yet waits for its
   keyframes before it is cut on time. *Backing:* MEASURED-ONCE, "four times the slowest index read
   measured, an MP4's on the owner's NAS". *Settle:* log index-read times across the library on
   nas.example; set it from the 95th percentile, not one file.
5. **`preloadAheadS` = 10** (`web/apps/web/src/watching.ts:84`). How far a film page's hidden
   player loads. *Backing:* MEASURED-ONCE (one phone, 2026-10-07). Low risk.
6. **`mediaBackWaitSeconds` 20, `mediaBackAskAgainMs` 2 s, `mediaBackAfterFailureMs` 10 s**
   (`web/apps/web/src/playback-failure.ts:64-70`). How a player waits for a share that went away.
   *Backing:* SWAG. *Settle:* low priority; the next NFS outage's log.

### 2. Sharing the upload

All in the settings registry, read by `internal/core/playback/`. They govern every viewer away
from home whenever the house's upload is the limit: which quality each one gets, how fast it
recovers, and when a Watch Together member is sent away.

7. **The crowding controller**: `upload_crowded_keeps_percent` 80, `_lower_every_seconds` 10,
   `_floor_percent` 10, `upload_uncrowded_for_seconds` 60, `_grows_percent` 10,
   `_raise_every_seconds` 30. *Backing:* SWAG; their own source is "upload-budget.md §4c: best
   guess". *Problem:* a controller's step, rate and recovery decide whether it oscillates or
   settles. None was simulated.
8. **`upload_crowded_slow_under_percent` 150 and `_slow_players_at_least` 2**: when the house
   counts as crowded. SWAG ("one and a half times", "two").
9. **`upload_films_share_percent` 85, `upload_raise_under_percent` 75,
   `upload_raise_room_seconds` 30**: D286 §4–§5 chose them. SWAG: no measurement of what else a
   household's upload carries.
10. **`upload_send_away_after_seconds` 20** (D286 §5): how long the upload is over budget at the
    lowest qualities before the newest Watch Together member is sent away. SWAG, and the most
    severe outcome in the group.

*Settle the group together:* a simulation of the controller with recorded link traces (the
player's link reports are logged), checked for oscillation and time to settle; then a shaped
upload (tc on the sandbox, 5 Mbit/s) with three players, one of them in Watch Together. Desk
research first, half a day: how Plex's "remote streaming bandwidth" limit and Jellyfin's remote
bitrate limit share a cap between streams.

### 3. The quality ladder and the encoder

11. **`chunk_qualities` = `3000k@720p,2000k@540p,1200k@480p,700k@432p,400k@360p`** (registry).
    *Backing:* RESEARCHED, but its own source says "Apple's HLS authoring spec's low rungs, to be
    tuned on the owner's films", and they never were. Apple's rungs are 730 and 1100 kbit/s at
    432p, 2000 at 540p, 3000 and 4500 at 720p (slow-links.md §4); 1200k@480p is not one of them,
    and 400k@360p was added for one airplane link (MEASURED-ONCE).
12. **x264 `-preset veryfast`, peaks `-maxrate` 1.5× and `-bufsize` 2×**
    (`internal/node/encode/encoder.go:54,61`); NVENC `p4` (`:68`). *Backing:* the preset is
    MEASURED-ONCE, for *speed* only (D285 §3: a 10 s chunk took 4.5–5.8 s "whatever the preset");
    its picture quality at each rate was never measured. The peak ratios are SWAG. *Problem:*
    Apple's rates assume a slower encoder than `veryfast`; at the same rate the picture may be
    visibly worse, and Quick Sync needs 10–25% more rate than x264 for the same quality
    (slow-links.md §4). *Settle:* VMAF of each rung at `veryfast` and `medium`, x264 and Quick
    Sync, on five of the owner's films of different kinds (dark, grain, animation, sport, talk), on
    gpuhost. A rung under VMAF 80 at its height is too low a rate for that encoder.
13. **`chunk_sound_kbps` = 96** (registry). MEASURED-ONCE (the airplane link) and inside Apple's
    range for stereo AAC. Low risk; a listening check of one dialogue-heavy film would close it.
14. **`loudness_max_raise_db` = 20** (registry). How far a quiet film's sound is raised. SWAG:
    its own source says "best guess, not measured". The target, -23 LUFS, is EBU R128 and
    RESEARCHED. *Settle:* raise the quietest of the owner's films (-30 LUFS measured) by 7 dB and
    the quietest track found by 20, and listen for hiss and pumping.

### 4. The chunk length behind other values

15. **`hls_segment_length_seconds` = 2** (registry; `internal/nodeproto/sound.go:12`). The owner's
    two seconds over six (D217 §3), so the dialogue boost switches sooner. Apple's spec says 6 s.
    A chunk starts at the first keyframe at or after each multiple, so real chunks follow the
    film's keyframes: D352 says "our chunks are 4–10 s".
    *Finding:* several values reason from "the ladder's ten-second grid":
    `preloadAheadS` 10 ("one chunk"), `keptBehindS` 30 ("three of the player's 10 s steps"),
    `loadsAheadBelowTopS` 30 ("room for one chunk of the ten-second grid"), the core's
    `readyAheadMs` 10 s, and `measuresFromBytesAtLeast` ("the lowest quality's 10 s chunk is about
    500 KB"). If a film's chunks are 4 s, a 10 s preload is two or three chunks, and the 128 KiB
    floor drops most low-quality samples. *Settle:* list the chunk lengths of every grid in
    nas.example's database (an SQL read, two hours), and check each derived value against the spread.

### 5. TV remote and on-screen timing

16. **The held seek** (`web/apps/web/src/seek-ahead.ts:29-40`): 10 s a press, 30 s from 2 s of
    holding, 60 s from 4 s; the seek is made 500 ms after the last press. *Backing:* SWAG, the
    owner's picks of today. *Problem:* at the Fire TV's repeat (about 150 a minute, the file's own
    comment) a 4 s hold moves 2.5 minutes of film a second; an overshoot is likely on a short
    episode. *Settle:* on the lab, have the owner find a given moment in a film and a 20-minute
    episode, and count overshoots; compare with how Android TV's own player and Apple TV scrub.
17. **`stickRepeatFirstMs` 400, `stickRepeatEachMs` 100** (`web/apps/web/src/input-pads.ts:181`).
    A gamepad's held direction. SWAG ("close to what a keyboard does"). Android's own defaults are
    500 ms before the first repeat and 50 ms between repeats (`ViewConfiguration`). Low risk; the
    button test (tv-device-tests.md H8) can settle it.
18. **`doublePressWithinMs` = 500** (`web/apps/web/src/cast-receiver.tsx:451`). Two presses of OK
    within it leave controlled mode on a receiver. *Backing:* REASONED from the wrong device:
    Windows' desktop double-click. Android's double-tap window is 300 ms. A remote adds its own
    latency and jitter, so two deliberate presses may arrive further apart, and two quick presses
    meant for something else may exit. *Settle:* the lab's input page records the gap between two
    deliberate OK presses on the Fire TV and the TCL, ten tries each.
19. **`stillWatchingAfterAutoplays` = 3** (`web/apps/web/src/still-watching.tsx:14`). *Backing:*
    RESEARCHED in part. Netflix asks after 3 episodes **and** 90 minutes without a press,
    whichever is longer, on TVs. We have no time floor: three 11-minute episodes ask after half an
    hour. *Settle:* nothing to measure; add the 90-minute floor if the owner agrees.
20. **`upNextLeadMs` = 30 s** (`web/apps/web/src/up-next.tsx:42`). Where the offer shows before
    the end, until credits are detected (D339). SWAG ("a typical episode's credits"). jellyfin-web
    shows its offer 30 s before the end, 35 s from 40 minutes long and 40 s from 50 minutes, and
    not at all under 10 minutes. Close to ours; a length rule as Jellyfin's is the cheap step
    before credits detection.

### 6. Watch Together catching up

The drift rules are RESEARCHED (watch-together.md, sync-review-2026-10-08.md: Jellyfin, Syncplay,
SyncLounge, watchparty). The values around them are not:

21. **`together_catch_up_buffer_percent` 80** (registry): SWAG, the owner's choice.
    **`waitAheadAtMostMs` 60 s** (`web/packages/sdk/src/together-load-speed.ts:62`): SWAG.
    **`aimAheadAtMostMs` 10 s** (`:22`): one source, Silo-Server's number.
    **`cooperativeRunwayS` 10** (`together-loading.ts:23`) and
    **`together_snap_seeks_beyond_seconds` 30** (D285): SWAG.
    *Settle:* the together diagnostics already log each catch-up; count how many land and play on
    without a second stall, per device.
22. **`scrubPauseWindowMs` 300 and `browserPauseWindowMs` 500** (`together-lead.ts:20,28`): when a
    pause is a person's and when it is a drag or a locked screen. MEASURED-ONCE (the owner's tests,
    one iPhone). Too short pauses the group for a lock; too long delays every deliberate pause.
    *Settle:* time a lock's pause on two more phones (an Android, an older iPhone).
23. **`deadAfterUnansweredPings` = 2** at a 15 s ping (`together-connection.ts:59`): up to 30 s of
    missed pauses and seeks before a dropped phone reconnects. MEASURED-ONCE (staging,
    2026-10-06).

### 7. The rest worth a look

24. **`directPathWaitMs` = 3 s** (`web/packages/sdk/src/direct-path.ts:14`). How long a page
    reached by a published name waits for a direct path before going on without it. SWAG, no
    reason given. Where no direct path answers (a firewalled home, a phone away), it is 3 s added
    to the first load. *Settle:* the path race's tell is logged; read how long the winning path
    took, at home and away.
25. **Bitrate controller corners** (`web/packages/player/src/`): the share's rise from 0.7 at 10 s
    to 0.95 at 25 s loaded (`abr-choice.ts:34`), `voiceLinkShare` 0.5 (`:46`, D262), a group's
    `commonStartHeight` 720 for 30 s (`:444`), the steadiness relax of 10 minutes
    (`steadiness.ts:28`) and `link_steadiness_relaxes_seconds` 3600 derived from it,
    `arrivingWithinMs` 1 s (`abr-choice.ts:43`, "to be measured"), `link_speed_fresh_hours` 24.
    SWAG inside an otherwise researched controller. The calibration log that
    bandwidth-sampling-2026-10-29.md proposes is the instrument for all of them.
26. **`chunk_run_slowest_speed_percent` 110 and `chunk_run_speed_window_seconds` 20** (registry,
    D250 §6): when a node making two runs evicts one, which stops someone's playback. SWAG, the
    owner's "fall below 1.1x". *Settle:* log each eviction with both runs' speeds; a run evicted at
    1.05× that would have kept up is a wrong eviction.
27. **Keep times that decide whether a resume plays at once**: `chunk_kept_ahead_seconds` 30,
    `_likely_` 300, `_left_` 600, `_maybe_` 1200 (D309, the owner's numbers),
    `chunk_interest_lapses_after_seconds` 120, `play_priority_kept_seconds` 600,
    `media_token_idle_expiry_hours` 4. SWAG. *Settle:* from the logs, how long people pause and
    leave before they come back; the keep times should cover most of that spread.
28. **Voice levels** (`web/packages/sdk/src/together-voice-level.ts`, `together-voice.ts:236`):
    `voiceTargetLevel` 0.1, `voiceSpeechFloor` 0.002, `partnerGainMost` 4. MEASURED-ONCE (the
    owner's voice, two phones). *Settle:* record the arriving level from four more phones and a
    laptop.

### 8. Checked now, and they hold

These carried no source in the code, so they were looked up for this audit. They match the field,
and need nothing more than the source added to their comment.

| Value | Where | Ours | The field |
| --- | --- | --- | --- |
| Controls auto-hide | `tv-controls.tsx:21` `overlayShownForMs` | 5 s | Media3 `PlayerControlView` 5 s; jellyfin-web 3 s |
| Seek step | `seek-ahead.ts:21` `seekStepMs` | 10 s | jellyfin-web 10 s back, 30 s on; Media3 5 s back, 15 s on |
| Autoplay countdown | `up-next.tsx:49` `upNextCountdownMs` | 10 s | Netflix 5–15 s over the years (10 s chosen by its A/B test), YouTube about 8–10 s |
| Subtitle offset step | `subtitles.tsx:19` `offsetStepMs` | 100 ms | mpv 100 ms, VLC 50 ms |
| Sound offset step | `sound-delay.tsx:21` `soundOffsetStepMs` | 50 ms | VLC 50 ms, mpv 100 ms, Kodi 25 ms |
| "Still waiting" text | `use-request.ts:13` `slowAfterMs` | 2 s | NN/g: a busy indicator for waits of 2–10 s |
| Resume and finished marks | registry `in_progress_after_percent`, `finished_after_percent` | 5%, 90% | Jellyfin 5% and 90% (play-state.md); Plex 90% (watched-marks-2026-10-27.md) |

Sources, looked up 2026-10-29 (notes in the cache at `swag-audit/web-notes-2026-10-29.md`):
[Media3 PlayerControlView](https://developer.android.com/reference/androidx/media3/ui/PlayerControlView),
[Media3 C](https://developer.android.com/reference/androidx/media3/common/C),
[Android ViewConfiguration](https://developer.android.com/reference/android/view/ViewConfiguration),
[Netflix: why it asks "Are you still watching?"](https://help.netflix.com/en/node/114059),
[Netflix autoplay's developer on HN](https://news.ycombinator.com/item?id=20566514),
[VLC subtitle delay](https://wiki.videolan.org/VLC_HowTo/Adjust_subtitle_delay/),
[mpv manual](https://mpv.io/manual/master/),
[Kodi audio offset step](https://forum.kodi.tv/showthread.php?tid=164930); jellyfin-web read
locally (`apps/legacy/controllers/playback/video/index.js`, `scripts/settings/userSettings.js`).

## Inventory

Every value counted in the summary. R researched, M measured once, D reasoned, S SWAG. Where a
reasoned value rests on a guess, the note says so. Settings are named by key; constants by file
and name.

### Starting, stalls and recovery

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| slowLoadAfterMs | player stalls.ts:17 | 15 s | S | owner, D247 §2 |
| stallAfterMs | player stalls.ts:9 | 1 s | D | "the browser catching its breath" |
| standsStillReportAfterMs | player standstill.ts:11 | 5 s | M | one phone's Brave |
| standsStillAtMostMs | player abr-choice.ts:339 | 6 s | M | D266, one film, one browser |
| chunksWhileStill, chunksNotHeld | player abr-choice.ts:342,348 | 2, 2 | S | |
| heldAheadAtLeastS | player abr-choice.ts:366 | 4 s | S | |
| refillAheadMs | sdk together-media.ts:18 | 10 s | M | owner's first test |
| ready_ahead_of_next_chunk_percent | registry | 150 | S | owner, 2026-10-28 |
| play_ladder_wait_ms | registry | 2000 | M | one index read, ×4 |
| startsBeforeFirstLoadedS | sdk start-gate.ts:75 | 0.5 s | M | 0.08 s seen on staging |
| startCheckEveryMs | sdk start-gate.ts:30 | 250 ms | D | |
| chunkFirstByteWaitMs | player chunked.ts:57 | 30 s | D | above chunk_hold_ms's maximum |
| chunkAskAgainCount | player chunked.ts:64 | 90 | D | rests on waitAheadAtMostMs (S) |
| chunk_hold_ms | registry | 0 | R | playback-in-practice.md §3 |
| mediaBack waits | web playback-failure.ts:64-70 | 20 s, 2 s, 10 s | S | |
| reopenAtMostEveryMs | web reopen.ts:55 | 60 s | S | |
| preloadAheadS | web watching.ts:84 | 10 s | M | one phone; assumes 10 s chunks |
| warmUpKeptMs | web watching.ts:68 | 30 min | D | half the token's least idle life |
| unavailable_root_rescan_seconds | registry | 10 | M | staging, 2026-10-21 |
| readyAheadMs | core chunks/ready.go:13 | 10 s | D | from refillAheadMs (M) |
| askReadyEveryMs | web moving-down.ts:49 | 3 s | S | |

### Quality ladder, encoding and sound

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| chunk_qualities | registry | five rungs, 3000k@720p to 400k@360p | R | Apple's rungs, "to be tuned", never tuned |
| chunk_qualities_hardware | registry | adds 6000k@1080p | R | Apple's 1080p rung |
| the 400k@360p rung | registry | 400k | M | one airplane link |
| x264 preset | node encode/encoder.go:61 | veryfast | M | speed measured, quality not |
| NVENC preset | node encode/encoder.go:68 | p4 | S | |
| peak and buffer | node encode/encoder.go:54 | 1.5×, 2× | S | |
| hls_segment_length_seconds | registry | 2 | S | owner, over Apple's 6 |
| chunk_sound_kbps | registry | 96 | M | one link, within Apple's range |
| encodeKbpsMost | core decide/encode_rate.go:8 | 8000 | D | cut-a §6 |
| unknownOwnSoundKbpsPerChannel | core chunks/ownsound.go:20 | 128 | D | |
| loudness_target_lufs | registry | -23 | R | EBU R128, checked against the owner's films |
| loudness_max_raise_db | registry | 20 | S | "best guess, not measured" |
| soundLatencyChangeIgnoredMs | player gain.ts:75 | 40 ms | R | EBU R37 |
| artwork_webp_quality, artwork_jpeg_quality | registry | 82, 85 | S | "not measured" |

### Choosing a quality

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| link_share_for_quality_percent | registry, player abr-choice.ts:28 | 70 | R | hls.js, Media3 (slow-links.md §2) |
| linkShareWhenAhead | player abr-choice.ts:31 | 0.95 | R | hls.js's |
| shareRisesFromS, shareRisesToS | player abr-choice.ts:34 | 10 s, 25 s | S | the ramp's shape |
| voiceLinkShare | player abr-choice.ts:46 | 0.5 | S | D262 |
| paceAboveQuality | player abr.ts:82 | 1.25 | S | |
| fastHalfLifeS, slowHalfLifeS | player link.ts:11 | 3 s, 9 s | R | hls.js's |
| trustAfterLoadingS | player link.ts:15 | 2 s | R | Media3's |
| measuresFromBytesAtLeast | player abr-choice.ts:60 | 128 KiB | R | Shaka's 128 kB; assumes 10 s chunks |
| cacheHitUnderMs | player abr-choice.ts:52 | 10 ms | R | Shaka, dash.js |
| arrivingWithinMs | player abr-choice.ts:43 | 1 s | S | "to be measured" |
| startingSamples, startingPlayedMs | player abr.ts:57 | 4, 20 s | D | D352 §1, with why |
| samplesAgreeWithinError | player link-estimate.ts:20 | 0.2 | D | one rung's gap |
| clipAtSpreads, logSpreadAtLeast, dropsPrior… | player link-estimate.ts:34-71 | 2, 0.3, 2/4 | R | textbook, untuned |
| assumedLogSpread | player link-estimate.ts:48 | 0.38 | M | one Fire TV, and CS2P |
| stepUpHeadroom (1.5, 1.75, 2) | player step-up.ts:8 | 1.5× | D | about 1/0.7 |
| step-up holds 30/60/120/240 s | D352 §3 | | D | Shaka's 8 s, Media3's 10 s |
| stepUpBufferMs | player step-up.ts:11 | 20 s | S | D247 §9 |
| relaxEveryMs, dropsCountWithinMs | player steadiness.ts:28 | 10 min | S | |
| link_steadiness_relaxes_seconds | registry | 3600 | D | six of the 10 min (S) |
| commonStartHeight, commonStartForMs | player abr-choice.ts:444 | 720p, 30 s | S | |
| link_speed_fresh_hours | registry | 24 | S | owner, D257 |
| loadsAheadBelowTopS, loadsAheadAtTopS | player forward-buffer.ts:23,31 | 30 s, 60 s | R | forward-buffer-2026-10-29.md |
| lowMemoryAtMostGb | player forward-buffer.ts:72 | 1 GB | R | Chromium's threshold |
| keptBehindS | player chunked.ts:79 | 30 s | D | three 10 s steps |

### Making chunks on the server

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| chunk_lookahead_seconds | registry | 60 | R | one source, Kyoo |
| chunk_run_span_seconds | registry | 300 | R | one source, Kyoo |
| unmeasuredJumpGapMs | core chunks/jump.go:19 | 24 s | R | Jellyfin's rule |
| chunk_run_slowest_speed_percent | registry | 110 | S | owner |
| chunk_run_speed_window_seconds | registry | 20 | S | D250 §6 |
| chunk_kept_ahead/likely/left/maybe | registry | 30, 300, 600, 1200 s | S | owner, D309 |
| chunk_interest_lapses_after_seconds | registry | 120 | S | D309 §3 |
| chunk_cpu_ceiling_kept_seconds | registry | 120 | S | D286 §4 |
| play_priority_kept_seconds | registry | 600 | S | owner |
| media_token_idle_expiry_hours | registry | 4 | S | D217 §2 |
| interest_said_every_seconds, interest_lapses_after_seconds | registry | 15, 45 | D | three missed words |

### Sharing the upload

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| upload_films_share_percent | registry | 85 | S | D286 §5 |
| upload_crowded_slow_under_percent | registry | 150 | S | |
| upload_crowded_slow_players_at_least | registry | 2 | S | |
| upload_crowded_keeps_percent | registry | 80 | S | "best guess" |
| upload_crowded_lower_every_seconds | registry | 10 | S | "best guess" |
| upload_crowded_floor_percent | registry | 10 | S | "best guess" |
| upload_uncrowded_for_seconds | registry | 60 | S | "best guess" |
| upload_uncrowded_grows_percent | registry | 10 | S | "best guess" |
| upload_uncrowded_raise_every_seconds | registry | 30 | S | "best guess" |
| upload_raise_under_percent, upload_raise_room_seconds | registry | 75, 30 | S | D286 §4 |
| upload_send_away_after_seconds | registry | 20 | S | D286 §5 |

### Watch Together

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| together_drift_left_alone_ms | registry | 150 | R | compared with five others; one guest measured |
| together_nudge_most_percent | registry | 10 | R | watchparty 1.1×; the owner's "sounds fine" |
| together_seek_drift_beyond_ms | registry | 3000 | R | Jellyfin's 3000 |
| together_seek_only_drift_left_alone_ms | registry | 400 | R | sync review, hls.js#7616 |
| together_settle_after_change_ms | registry | 300 | R | safari-versions.md §4 |
| nudgeForMs | sdk together-drift.ts:11 | 1 s | R | Jellyfin |
| pingEveryMs, youngPings | sdk together-connection.ts:37,43 | 15 s, 3 | R | Jellyfin's pattern |
| assumedClockSkewPpm | sdk together-connection.ts:51 | 50 | R | DVB-CSS |
| stateEveryMs | sdk together-connection.ts:31 | 60 s | R | cut-a §5.1, drift research |
| together_catch_up_buffer_percent | registry | 80 | S | owner |
| aimAheadAtMostMs | sdk together-load-speed.ts:22 | 10 s | R | one source, Silo-Server |
| loadedCatchUpLeadMs | sdk together-load-speed.ts:31 | 1 s | M | |
| waitAheadAtMostMs | sdk together-load-speed.ts:62 | 60 s | S | |
| loadWindowMs, loadSpanAtLeastMs, loadSampleMarginMs | sdk together-load-speed.ts:70 | 10, 3, 5 s | S | |
| cooperativeRunwayS | sdk together-loading.ts:23 | 10 s | S | D285 §2 |
| together_snap_seeks_beyond_seconds | registry | 30 | S | D285 §1 |
| checkDriftEveryMs | sdk together-follow-options.ts:15 | 1 s | S | |
| scrubPauseWindowMs | sdk together-lead.ts:20 | 300 ms | M | owner's tests |
| browserPauseWindowMs | sdk together-lead.ts:28 | 500 ms | M | one iPhone |
| ownMarkLastsMs | sdk together-own-moves.ts:13 | 2 s | D | |
| deadAfterUnansweredPings | sdk together-connection.ts:59 | 2 | M | staging, 2026-10-06 |
| reconnect 500 ms → 30 s | sdk together-connection.ts:62 | | D | a doubling backoff |
| soundToldAgainAfterMs | sdk together-sound.ts:17 | 2 s | M | VQ30 |
| lossAtMostMs | sdk together-drift.ts:92 | 500 ms | S | |
| diagnoseAtMostEveryMs | web together-diagnosis.ts:41 | 5 s | S | |

### Voice

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| voiceTargetLevel | sdk together-voice-level.ts:9 | 0.1 | M | the owner's iPhone |
| voiceSpeechFloor | sdk together-voice-level.ts:15 | 0.002 | M | |
| voiceGainMost | sdk together-voice-level.ts:18 | 30 | D | from 0.003 to the target |
| voiceGainRise | sdk together-voice-level.ts:25 | 1.15 | D | |
| partnerGainMost | sdk together-voice.ts:236 | 4 | M | |
| should_together_voice_cancel_echo, _browser_level_microphone | registry | off | M | two Android tests |
| callConnectsWithinMs | sdk together-voice.ts:264 | 15 s | S | D281 §6 |

### TV remote and on-screen timing

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| overlayShownForMs | web tv-controls.tsx:21 | 5 s | S | matches Media3 (§8) |
| seekStepMs | web seek-ahead.ts:21 | 10 s | S | matches the field (§8) |
| heldSeekSteps | web seek-ahead.ts:29 | 30 s at 2 s, 60 s at 4 s | S | owner, today |
| seekRestMs | web seek-ahead.ts:40 | 500 ms | S | owner |
| heldKeyRestMs, landedWithinMs | web seek-ahead.ts:47,54 | 1.5 s, 1.5 s | D | |
| landingShownForMs | web seek-ahead.ts:60 | 10 s | S | |
| stickRepeatFirstMs, stickRepeatEachMs | web input-pads.ts:181 | 400, 100 ms | S | Android: 500, 50 |
| axisLeansPast | web input-pads.ts:146 | 0.5 | S | "the usual dead zone" |
| doublePressWithinMs | web cast-receiver.tsx:451 | 500 ms | D | from Windows' desktop |
| samePressWithinMs | web cast-receiver.tsx:458 | 100 ms | S | |
| castNoteShownMs | web cast-receiver.tsx:40 | 5 s | S | |
| castAnswerWithinMs | web cast.ts:28 | 60 s | S | |
| sendingKnownWithinMs | web cast-to.tsx:233 | 3 s | S | |
| fullscreenAnswerWithinMs | web film-fullscreen.ts:101 | 1 s | S | |
| dragStepMs | web timeline.tsx:10 | 1 s | S | |
| soundOffsetStepMs | web sound-delay.tsx:21 | 50 ms | S | matches the field (§8) |
| offsetStepMs (subtitles) | web subtitles.tsx:19 | 100 ms | S | matches the field (§8) |
| soundOffsetKeptAfterMs | web sound-delay.tsx:28 | 500 ms | D | |
| upNextLeadMs | web up-next.tsx:42 | 30 s | S | Jellyfin 30–40 s by length |
| upNextCountdownMs | web up-next.tsx:49 | 10 s | S | matches the field (§8) |
| stillWatchingAfterAutoplays | web still-watching.tsx:14 | 3 | R | Netflix's 3, without its 90 minutes |
| readingGapScreens, readingStepScreens | web focus-navigation.ts:159 | 0.5, 0.8 | S | |
| acrossWeight | web focus-engine.ts:34 | 2 | R | Jellyfin, the W3C draft |
| restoreWaitsMs | web focus-memory.ts:153 | 10 s | S | |
| tilesSettleMs | web tile-interest.ts:13 | 500 ms | S | |
| slowAfterMs | web use-request.ts:13 | 2 s | D | matches NN/g (§8) |

### Lists and play state

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| in_progress_after_percent | registry | 5 | R | Jellyfin |
| finished_after_percent | registry | 90 | R | Jellyfin, Plex |
| continue_watching_max_works | registry | 20 | S | "a row or two" |
| newestShown | web home.tsx:19 | 18 | S | "three screens of a Fire TV's row" |
| entriesPerPage | web browse.tsx:52 | 60 | S | "a best guess" |

### Connections and refreshing

| Value | Where | Start | Class | Note |
| --- | --- | --- | --- | --- |
| directPathWaitMs | sdk direct-path.ts:14 | 3 s | S | no reason given |
| betterPathGraceMs | sdk direct-path.ts:22 | 1 s | M | owner, 2026-10-15 |
| connectTimeoutMs | sdk rtc-connect.ts:14 | 10 s | D | ordered with the core's |
| gatherAfterReflexiveMs | sdk rtc-connect.ts:24 | 300 ms | M | staging, 2026-10-23 |
| gatherForRelayMs | sdk rtc-connect.ts:31 | 1 s | D | a round trip or two |
| disconnectedGraceMs | sdk rtc-transport.ts:40 | 5 s | S | |
| replace backoff 1 s → 30 s | sdk rtc-transport.ts:43 | | D | |
| idleClosesAfterMs, growWhenEachCarries | sdk rtc-transport.ts:197,204 | 30 s, 2 | M | staging, 2026-10-25 |
| betterPath 30 s → 10 min, failuresBeforeGivingUp 2 | web kept-path.ts:85-95 | | M | Firefox on nas.example |
| screen_refresh_first/longest/failed_wait_seconds | registry | 2, 30, 5 | S | owner, D237 |
| askAnswersWithinMs | web polling.ts:75 | 60 s | D | twice D237's longest |
| discovery_pending_check_every_seconds | registry | 2 | M | owner, 2026-10-20 |
| offlineRetryAfterMs | web discovery-gate.tsx:657 | 5 s | D | "a restart takes some ten seconds" |
| sessionCheckEveryMs | web signed-out-watch.ts:33 | 60 s | S | |
| pairsSettleMs | web path-choice.ts:110 | 5 s | S | |
| webrtc_connections_per_network | registry | 64 | S | owner |

## What this audit did not do

- It did not judge whether each guess is wrong. Many may be right; the point is that nothing
  shows it.
- It did not audit the node's watchdog windows, retry counts for providers, backups, sign-in
  limits or retention: they shape no one's evening, or a spec or a security reason sets them
  (constants-sweep-2026-10-20.md covers them).
- Eight web searches were used, only for §8 and the platform defaults in §5.
