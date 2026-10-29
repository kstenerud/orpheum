> **ABOUTME:** Research behind three groups of guessed values from the SWAG audit: how long a slow
> start waits before it counts as a stall and how much a player loads before it plays on (group 1);
> the real chunk lengths in staging's and nas.example's databases, and the values that reason from a
> "ten-second grid" (group 4); the TV remote's timings (group 5); plus "Still watching?" and the 3 s
> wait for a direct path. Per value: current, evidence, recommendation, confidence, a test.
> Proposes, decides nothing. 2026-10-29.

# Starts, chunks and the remote

- **Date:** 2026-10-29, code read at an earlier commit.
- **Asked by** the owner (2026-10-29): *"Go ahead and schedule the research so it can run while we
  test"*, for groups 1, 4 and 5 of [swag-values-audit-2026-10-29.md](swag-values-audit-2026-10-29.md)
  and its two smaller findings (§5 item 19, §7 item 24).
- **Builds on:** [slow-links.md](slow-links.md) §6, [abr-adaptation-2026-10-27.md](abr-adaptation-2026-10-27.md)
  §4, [bandwidth-sampling-2026-10-29.md](bandwidth-sampling-2026-10-29.md) and
  [forward-buffer-2026-10-29.md](forward-buffer-2026-10-29.md). Their sources are not repeated.
- **Data read here, read-only:** staging's database (`.staging/core/persistent/persistent.db`,
  `?mode=ro`) and nas.example's (`/srv/orpheum/core/persistent/persistent.db`, `?mode=ro` from a
  `nice -n 19` Python on nas.example, only ids and numbers sent back); staging's logs from 2026-10-24 to
  today (`.staging/logs`, 133 files); nas.example's core log since its last restart (2026-10-27). No
  video is named; nas.example's library 3 is counted, never listed.
- **Cache:** `/yoloai/cache/research-groups-1-4-5/` (the export and analysis scripts, the
  exported grids as `data/*.jsonl.gz`, `web-notes.md` with every web source, the Kodi, Media3 and
  Android sources fetched).
- **Labels:** **[1P]** read at the source; **[S]** a search engine's or fetch tool's summary only;
  **[M]** measured here, from the databases or logs; **[I]** inference or my arithmetic; **[G]** a
  guess, to be measured.
- **Spent:** 19 web searches.

---

## Summary

| # | Value | Now | Recommended | Confidence |
| --- | --- | --- | --- | --- |
| 1.1 | `slowLoadAfterMs`: a load that cannot play counts as a stall | 15 s, whatever the cause | **8 s**, counted only while the wait is the link's (bytes arriving slowly), not the server's (425, being made); and never while paused | low–medium |
| 1.2 | `ready_ahead_of_next_chunk_percent` (start, seek) | 150% of a lower-quantile load time | **keep 150%** | medium |
| 1.3 | `refillAheadMs` after a stall in Watch Together | 10 s fixed | **the seek's rule, and where the link is slower than the quality, enough for a minute's run**: 60 s × (1 − link ÷ rate) | medium (direction), low (the 60 s) |
| 1.4 | `standsStillAtMostMs` / chunks 2 / 2 / 4 s | 6 s | **keep**; log every give-up with its device | medium |
| 1.5 | `stallAfterMs` | 1 s | keep (Shaka's 1 s) | high |
| 1.6 | `play_ladder_wait_ms` | 2 s | keep; log index-read times | low (no data) |
| 4.1 | `hls_segment_length_seconds` | "2 s" | keep the setting; **say in its text what it yields**: median chunk 3.5 s by count, **6.5–6.8 s by film time**, 44% of film time in chunks ≥ 8 s | high (data) |
| 4.2 | `measuresFromBytesAtLeast` | 128 KiB per sample | **merge back-to-back small chunks into one sample**, or 64 KiB; Shaka's 128 kB is a *total* before trusting, its per-sample floor 16 kB | medium |
| 4.3 | `preloadAheadS`, `keptBehindS`, `loadsAheadBelowTopS`, core `readyAheadMs` | 10, 30, 30, 10 s | **keep**; fix `preloadAheadS`'s "one chunk" (it is one to five) | high |
| 5.1 | Held seek | 10 s a press, 30 s from 2 s, 60 s from 4 s, per key repeat | **move by wall time, not by repeat**: a speed per stage, scaled by the film's length | medium (mechanism), low (numbers) |
| 5.2 | `seekRestMs` | 500 ms | **750 ms** (Kodi's) | low–medium |
| 5.3 | `doublePressWithinMs` | 500 ms, key repeats counted | **ignore key repeats**; keep 500 ms down-to-down until the lab measures | high (repeat), medium (500) |
| 5.4 | `stickRepeatFirstMs`, `stickRepeatEachMs` | 400, 100 ms | keep | medium |
| 5.5 | Long press | none used | if one is added: the platform's own first key repeat is the signal | high |
| 5.6 | `overlayShownForMs` | 5 s | keep (Media3's) | high |
| 5.7 | `upNextLeadMs` | 30 s | Jellyfin's length rule until credits detection | medium |
| S1 | `stillWatchingAfterAutoplays` | 3 | **3 autoplays and 90 min with no press, on a TV**; 3 elsewhere (Netflix, verified) | high |
| S2 | `directPathWaitMs` | 3 s before WebRTC starts | **start WebRTC 250 ms after the race if no name has answered** (Happy Eyeballs), keep the 3 s as each name's limit; log each name's time | medium |

**Three things found on the way, which are not tunings:**

1. **A held OK on a cast's receiver probably leaves controlled mode** (§3.3). A key's repeats
   come as `select` commands, and Android's first repeat arrives 400–500 ms after the press,
   inside the 500 ms window.
2. **A paused player that has loaded nothing counts a stall every 15 s** (§1.1): 83 of staging's
   103 stall reports are this, at position 0, paused, readyState 0. Each one tells the bitrate
   controller of a drop. What these players are is not settled.
3. **Chunks can be shorter than the 2 s setting**, 15–18% of them, down to 33 ms (§2.1):
   `GridOf` counts the next multiple from each chunk's own keyframe, not from a whole step after
   it. Harmless for playback as far as the data shows; it matters to the byte floor (4.2).

**Where the "2 s abandonment" comparison stands.** The audit set the 15 s against Krishnan and
Sitaraman's 2 s. They measure different things. The 2 s is when people start to leave a video
that has not started (in data dominated by short clips, median play 35 s). The 15 s is when a
start that has not played is *treated as a stall*, which lowers the bitrate ceiling. Conviva counts
an exit before start as abandonment only after 8–10 s of waiting, and calls a start under 10 s
good. Our starts on staging take 2.5 s at the median and 7.7 s at the 90th percentile. On nas.example,
0.3 s and 0.8 s. So 15 s is late, and 2 s would fire on most of staging's starts; 8 s sits at
the industry's own line. *[M], [1P]*

---

## 1. Starting and stalling

### What the data says

**Start to playing** (`a playback started playing`, `playingAfterMs`: from when the player asked
how the file plays to playing, so a film page's preload counts time before Play was pressed) and
**seek to ready** (`a player's seek was ready to play`, `readyAfterMs`). *[M]*

| | n | p50 | p75 | p90 | p95 | max | > 2 s | > 5 s | > 10 s | > 15 s |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Staging, starts | 254 | 2.5 s | 4.2 s | 7.7 s | 10.1 s | 55 s | 61% | 17% | 6% | 3% |
| Staging, seeks while playing | 145 | 0.6 s | 2.4 s | 3.5 s | 4.8 s | 10.3 s | 29% | 4% | 1% | 0% |
| nas.example, starts | 12 | 0.3 s | 0.5 s | 0.8 s | 2.4 s | 2.4 s | 8% | 0% | 0% | 0% |
| nas.example, seeks while playing | 11 | 0.02 s | 2.2 s | 2.2 s | 2.9 s | 2.9 s | 27% | 0% | 0% | 0% |

Staging is the owner's test bench: Fire TV over a hairpinned router path, shaped tests, and
debugging, so it is the pessimistic case. The Fire TV (device 33) started at 3.4 s median, 10.2 s
at p90.

**Stalls.** Staging logged 103 stall reports. 83 are a paused player at position 0 with readyState
0 and nothing loaded, repeating every 15 s for minutes (device 60, 2026-10-29 09:12–09:15: twelve).
That is `slowLoads` in `stalls.ts` firing on a player that is not playing: it does not check
`paused`. Of the 20 others, 10 came within 30 s of a start (4% of 254 starts) and 3 within 30 s of a
seek that was playing (2% of 145). One run is the stall loop slow-links §6 predicted for a fixed
10 s refill: device 64 on 2026-10-28 stalled at 2405, 2415, 2425 and 2435 s of the film, each 10 s
of film after the last, every 10–20 s of wall time. *[M]*

### What the players do *[1P]*

| | Start | After a stall | "Cannot get going" |
| --- | --- | --- | --- |
| Media3 | 1 s loaded | 2 s loaded | HTTP connect and read timeouts 8 s each; 3 retries, waits of 1–5 s; a failing track excluded 60 s |
| hls.js 1.7.3 | at the first fragment | the browser's | first byte within 10 s or a retry; start level chosen so that the first fragment loads within `maxLoadingDelay` 4 s; abandons a fragment only after half its duration has loaded, and not before playback begins |
| dash.js | — | stall at 0.3 s | abandons a segment that will take more than 1.8× its duration, judged after 500 ms; no switch-up for 10 s after |
| Shaka | `rebufferingGoal` 0 | the same | stall detection at 1 s, then a 0.1 s nudge |
| Netflix, YouTube | a first quality from the device's history (Sammy, SIGCOMM 2023) | — | not published |

None of them has a "slow start, so lower the quality" timer like ours. They have two
mechanisms instead: a load timeout that errors and retries (8–10 s), and an abandonment rule that
gives up a fragment early (hls.js after half its duration, dash.js at 1.8× its duration). Ours has
the second already: `giveUpFor` (`abr-choice.ts`) gives up a chunk after half its duration, before
playback too, except at the lowest quality. So `slowLoadAfterMs` is left with the cases where giving
up a chunk cannot help: the lowest quality loading slowly, the server still making the chunk (425),
or nothing arriving at all. *[I]*

### What viewers mind *[1P]*

- **Krishnan and Sitaraman, IMC 2012** (23 million Akamai views): abandonment starts after 2 s of
  start delay, then 5.8% more per second; viewers of videos over 30 minutes wait longer than those of
  short ones. Rebuffering for 1% of a video's length cuts viewing by 5%.
- **Hoßfeld et al., QoMEX 2012**, via Seufert et al.'s 2015 survey: "initial delays up to 16 s
  reduce the perceived quality only marginally"; about 90% of people prefer a longer start to a
  stall; "one long stalling event is preferred to frequent short ones".
- **Hoßfeld et al., *Quantification of YouTube QoE via Crowdsourcing***: people tolerate one stall
  per clip under 3 s; two or more leave them highly dissatisfied.
- **Conviva's Streaming Performance Index**: a start under 10 s is "good"; an exit before start is
  counted as abandonment past 8 s of waiting ("best") or 10 s ("good"). Rebuffering time per view
  under 2 s is good, under 1 s best. Conviva's industry average start was about 4.1–4.25 s in
  2019–2022 *[S]*.

The studies agree: a start may take a few seconds, but a stall, and above all a repeated one, is
what drives people away.

### 1.1 `slowLoadAfterMs` = 15 s (`web/packages/player/src/stalls.ts:17`)

- **Evidence.** The industry's line for a failed start is 8–10 s (Conviva), and its load timeouts are
  8–10 s (Media3, hls.js). 15 s is past both. Our staging starts exceed 8 s about 9% of the time,
  15 s 3%. The 2 s figure is not the right comparison: below 8 s, a start is slow but not failed,
  and lowering the quality then costs picture for a wait that may be the server's (an on-demand
  chunk: D285 §3 measured 4.5–5.8 s for a 10 s chunk). The paused-player reports show the timer
  firing where nobody waits.
- **Recommendation.** 8 s, counted only while (a) the player is asked to play (not paused, or
  paused only by the start gate) and (b) the wait is on bytes arriving, not on 425 answers. A
  server-side wait goes to the server's log instead, which is where its cause is.
- **Confidence:** low–medium. The 8 s rests on a dashboard's threshold, not on a study of when to
  lower quality.
- **Test.** Unit: a paused player at readyState 0 for 60 s reports no stall (red today: four).
  Lab: a 1 Mbit/s shaped link and a fresh device at the top quality; time to the first frame at 15 s
  and at 8 s, five starts each.

### 1.2 `ready_ahead_of_next_chunk_percent` = 150 (registry, `web/packages/sdk/src/ready-ahead.ts`)

- **Evidence.** The load time it multiplies is already a lower-quantile bound of the link
  (`link-estimate.ts`, "the next chunk's bound"), plus the first byte's wait. So the 150% is a
  margin on a conservative number. Seeks became ready in 0.6 s at the median and 3.5 s at p90 on
  staging, and 2% of them stalled within 30 s. That is cheap and rarely wrong. No player documents
  its resume number (slow-links §6). Media3's fixed 1 s and 2 s are not link-aware; ours is, which
  slow-links §6 asked for.
- **Recommendation:** keep 150%. The logs mix days before and after 2026-10-28, when it came in,
  so re-read them after a week.
- **Confidence:** medium.
- **Test.** From the logs, the share of seeks and starts followed by a stall within 30 s, by week.
  It should stay at or under the 2–4% seen. In the lab, 100% against 150% on a link at 1.2× the
  quality, ten seeks each.

### 1.3 `refillAheadMs` = 10 s (`web/packages/sdk/src/together-media.ts:18`)

It applies after a stall in Watch Together (`together-refill.ts`), and wherever a player cannot say
how long its next chunk takes.

- **Evidence.** When the link is slower than the quality, a fixed refill gives a loop: play 10 s,
  stall, refill, again. The device 64 run above is that. The QoE studies prefer one long stall to
  several short ones. The refill that buys a run of T seconds at a link of r × the quality's rate
  is T × (1 − r) *[I]*: for r = 0.8, 12 s buys a minute; for r = 0.5, 30 s.
- **Recommendation.** After a stall, the seek's rule (1.2) where the link keeps up. Where it does
  not (next chunk's load longer than its duration) and the player is already at its lowest quality,
  refill 60 s × (1 − r), capped by the forward buffer. Keep 10 s only where nothing is known.
- **Confidence:** medium for the direction (the literature and our own loop), low for 60 s.
- **Test.** Lab: a link at 0.8× the lowest rung, a 10-minute Watch Together run. Count stalls
  and total stalled time at 10 s and at the new rule. Fewer stalls of similar total time is the
  pass.

### 1.4 `standsStillAtMostMs` 6 s, `chunksWhileStill` 2, `chunksNotHeld` 2, `heldAheadAtLeastS` 4 (`abr-choice.ts:339–366`)

- **Evidence.** hls.js's own watchdog, which runs under ours, nudges a playhead stuck with data
  ahead after 2 s (`highBufferWatchdogPeriod`), up to 3 times, then raises a stall error. That is
  about 6–8 s before giving up, the same order as ours. Shaka detects at 1 s and nudges. No player
  gives up a quality for a decoder; they error out. No give-up of this kind appears in staging's
  logs since 2026-10-24. *[1P], [M]*
- **Recommendation:** keep. Add the device and the quality to the give-up's log line, so a false
  one (a device that plays that quality later) can be counted.
- **Confidence:** medium.
- **Test.** A Fire TV seek into a 1080p chunk, ten times; no give-up. The existing D266 test
  stays red without the rule.

### 1.5 The rest of the group

- **`stallAfterMs` 1 s:** Shaka's `stallThreshold` is 1 s. Keep. *[1P]*
- **`standsStillReportAfterMs` 5 s:** only a report; keep.
- **`play_ladder_wait_ms` 2 s:** nothing logs index-read times, and no `noGridYet` start appears on
  staging (224 ladder starts, 0 without a grid): read-throughs reach almost every file first
  (nas.example: 5563 grids of 5719 timelines). Keep, add a log line for the read's time. *[M]*
- **`mediaBack…` waits, `askReadyEveryMs`, `reopenAtMostEveryMs`:** no evidence either way here;
  leave until the next outage's log.

---

## 2. The chunk length

### 2.1 What the grids are *[M]*

Every picture's keyframes (`video_timeline.keyframes_ms_json`) cut by `GridOf`
(`internal/core/chunks/grid.go`) at the setting in force, 2 s on both servers.

| | Staging | nas.example |
| --- | --- | --- |
| Files with a grid | 5141 | 5563 |
| Chunks | 2.58 million | 2.86 million |
| Chunk length, by count: p10 / p50 / p90 / p99 | 1.7 / 3.6 / 10.0 / 10.4 s | 1.6 / 3.5 / 10.0 / 10.4 s |
| **By film time** (the chunk a viewer is in): p25 / p50 / p75 / p90 | 3.6 / **6.5** / 10.0 / 10.4 s | 3.7 / **6.8** / 10.0 / 10.4 s |
| Film time in chunks ≥ 8 s | 42% | 44% |
| Film time in chunks < 4 s | 28% | 28% |
| Chunks under 2 s / under 1 s | 15.3% / 0.3% | 17.7% / 0.5% |
| Files whose median chunk is ≥ 8 s | 15% | 18% |
| Files with a chunk over 20 s | 19 | 69 |

The peaks at 10.0 and 10.4 s are encoders' default keyframe spacing: 240 and 250 frames at
23.976 fps. A film from such a file is cut at its keyframes, whatever the setting says. The longest
chunks, minutes long, are files whose timeline has almost no keyframes; 69 on nas.example, most in
library 3. Library 3's files have a median chunk of 8.3 s; libraries 1 and 2, 4.0 s.

**Chunks under the setting.** `GridOf` starts the next chunk at the first keyframe at or after
the next multiple of 2 s *after the current chunk's keyframe*. A chunk starting at 3.9 s ends at a
keyframe at 4.0 s. So chunks run from 33 ms up. The doc comment on `Grid` ("merged so each chunk is
at least hls_segment_length_seconds long") says otherwise, and the code does not merge. Playback
handles them; each is a request, and they fall under the byte floor (2.2).

### 2.2 The values that reason from a ten-second grid

| Value | Its reasoning | Against the data | Verdict |
| --- | --- | --- | --- |
| `hls_segment_length_seconds` 2 (D217 §3: the dialogue boost switches sooner) | chunks of about 2 s | the viewer is in a chunk of 6.5–6.8 s at the median, ≥ 8 s 44% of the time. The sound's chunks appear to share the picture's grid (`StoreDir(…, sound, film.grid.ID())`), so the boost waits for those *[I]* | keep the setting; say in its help text that a film's keyframes decide, and that 2 s is a floor that is often not reached |
| `measuresFromBytesAtLeast` 128 KiB (`abr-choice.ts:60`): "the lowest quality's 10 s chunk is about 500 KB" | 10 s chunks | at 400 kb/s, **35% of chunks are under 128 KiB** and never measure the link; at 700 kb/s 6–7%; at 1200 kb/s 0.2–0.3%. On a time grid (2 s), every 400 kb/s chunk is under it | see 4.2 below |
| `preloadAheadS` 10 (`watching.ts:84`): "one chunk of the ladder's ten-second grid" | one chunk | one to five chunks; one where chunks are 10.4 s | keep the value, fix the comment, or say "the first chunk or two" in bytes |
| `keptBehindS` 30 (`chunked.ts:79`): "three 10 s steps back … its chunk on the ten-second grid kept with it" | seek steps of 10 s, chunks ≤ 10 s | holds: p99 chunk 10.4 s | keep |
| `loadsAheadBelowTopS` 30 (`forward-buffer.ts:23`): 20 s step-up buffer "and room for one chunk of the ten-second grid" | chunks ≤ 10 s | holds for 99% of chunks; the 19–69 files with minute-long chunks cannot fit it, and do not today either | keep |
| core `readyAheadMs` 10 s (`internal/core/chunks/ready.go:13`) | = `refillAheadMs` | one to five chunks; follows 1.3 if that changes | keep, tied to 1.3 |

**4.2, the byte floor.** Its source, Shaka, has two numbers, and ours took the wrong one for a
per-sample floor: `abr.advanced.minBytes` is **16 kB per sample**, and `minTotalBytes` is **128 kB in
total** before the estimate is used at all (`shaka-player_configuration.js`). *[1P]* Our 128 KiB per
sample came from a 44.8 KB first chunk that measured a LAN at 676 kb/s (2026-10-18), so a floor of
16 kB would let that through again.

- **Recommendation.** Merge chunks of one quality that loaded back to back (the next one asked
  within a few ms of the last one's end) into one sample until it reaches 128 KiB. That keeps
  2026-10-18's protection and lets the lowest rung measure. Failing that, 64 KiB.
- **Confidence:** medium. The lowest rung is where a slow link sits and most needs samples to
  climb, and a third of them are lost now. How much this matters with D352's prior and the given-up
  samples (P3) is not measured.
- **Test.** Unit: four 400 kb/s chunks of 2.2 s loaded back to back give one sample (none today).
  Lab: start at 400 kb/s on a link that carries 2 Mb/s; time to the first step up, before and after.

---

## 3. The TV remote

### What the platforms and players do

| | A press | Held | When the seek happens | Source |
| --- | --- | --- | --- | --- |
| Android TV guidelines | +N s (N unspecified) | "scrubbing" | — | developer.android.com, TV playback controls *[1P]* |
| Leanback (`PlaybackTransportRowPresenter`) | **1% of the length** | the same per key repeat; no acceleration | on OK; Back cancels | AOSP source *[1P]* |
| Media3 `DefaultTimeBar` | **1/20 of the length** | the same per key repeat | **1000 ms after the last key**, or OK | androidx/media source *[1P]* |
| Kodi | press n moves the total to the n-th step: 10, 30, 60, 180, 300, 600 s | each repeat is a press | **750 ms** after the last | `settings.xml`, `SeekHandler.cpp` *[1P]* |
| Apple TV | 10 s, again 10 s | fast-forward; each press cycles 2×, 3×, 4× | Play commits, Back cancels to where it was | Apple Support *[1P]* |
| Roku | left/right step through thumbnails, held scrolls | FF/RW: 3 speeds for long-form, press cycles, hold jumps to the fastest | — | Roku developer docs *[S]* |
| Fire TV | "rewind or skip backwards", "fast-forward" | no guidance; up to the app | — | Amazon docs *[1P]* |
| YouTube, Plex, Jellyfin (Samsung TV) | — | "the seek bar slides very fast and smoothly"; Emby's 10 s steps with 500 ms pauses drew complaints | — | Emby forum *[S]* |
| Jellyfin Android TV | 10 s / 30 s | repeated skips; acceleration and Kodi-like "smart skipping" requested (#146, #5912), not shipped | — | GitHub issues *[S]* |

**Android's own timings** (`ViewConfiguration`, AOSP main) *[1P]*: long press 400 ms (500 ms before
Android 12; Fire OS 7 and 8 are Android 9 and 11); the first key repeat at the long-press time,
then **every 50 ms**; a double tap or multi-press within **300 ms from the first press's release to
the second's press**; double taps under 40 ms ignored.

### 3.1 The held seek (`web/apps/web/src/seek-ahead.ts:29–40`)

- **Now.** Each key-down, repeats included, moves `heldStepMs`: 10 s, 30 s from 2 s of holding,
  60 s from 4 s.
- **The problem: its speed is the device's repeat rate times the step**, and repeat rates differ
  8×. Where a Fire TV's Silk passes Android's 50 ms repeats, a 4 s hold has moved about 25 minutes,
  and each further second 20 minutes. At the 150-a-minute the file's comment reports, 3.5 minutes
  and 2.5 minutes a second. *[I]* No measured repeat rate is in the logs: the button test logged no
  `isRepeat=true` press. Leanback and Media3 scale by the film's length, so a hold crosses a film in
  a fixed time whatever its length; Kodi grows the step per press.
- **Recommendation.** While an arrow is held (from its first repeat, the platform's own long
  press), move the marker by wall time, not by repeats: for example the larger of 20 s a second and
  1% of the length a second for the first 2 s, of 60 s and 3% from 2 s, of 120 s and 8% from 4 s. A
  2-hour film then crosses in about 12 s of holding at the top speed, a 22-minute episode at
  120 s a second. Single presses stay 10 s. *[G]* for the numbers.
- **Confidence:** medium for moving by time (it removes a device-dependent factor of 8), low for the
  numbers.
- **Test.** First, the lab's button test on the Fire TV and the TCL: hold Right for 5 s, three
  times; the gaps between repeats give each device's rate. Then the owner finds a named moment in a
  2-hour film and in a 20-minute episode, five times each, at the current and proposed rules;
  count the overshoots and the seconds taken.

### 3.2 `seekRestMs` = 500 ms

- **Evidence.** Kodi waits 750 ms after the last press, Media3 1000 ms; Leanback and Apple wait for
  OK. Each rest that ends a run early makes a real seek, which loads chunks. People who press
  slowly (older viewers; a remote with lag) may leave more than 500 ms between deliberate
  presses. *[1P], [I]*
- **Recommendation:** 750 ms. The held key's release still seeks after it.
- **Confidence:** low–medium.
- **Test.** The lab: the owner presses Right five times "as you would", ten runs; the gaps' p95
  should sit under the rest. The core's log counts one `kind=seeked` per run.

### 3.3 `doublePressWithinMs` = 500 ms (`web/apps/web/src/cast-receiver.tsx:451`)

- **A held OK probably leaves controlled mode.** `startInput` turns every key-down into a command,
  repeats included, and `useLeavingPresses` counts two `select` commands within 500 ms as a double.
  Android's first repeat comes 400 ms (Android 12+) or 500 ms (Fire OS) after the press, at or
  inside the window (the check is `<=`). *[I]* from the code and `ViewConfiguration`; not seen on a
  device yet.
- **The window itself.** Android's 300 ms runs from the first release to the second press. Measured
  press to press, as ours is, that is about 300 ms plus a press's own length (often around 100 ms),
  so 400 ms or so *[I]*. Windows' 500 ms is the desktop default *[S]*. A remote's own latency may
  stretch the gap. 500 ms press to press is a reasonable start.
- **Recommendation.** Ignore `event.repeat` for the double press (and a pad's held button). Keep
  500 ms press to press until the lab measures deliberate double presses on the Fire TV and the TCL.
- **Confidence:** high for ignoring repeats; medium for 500 ms.
- **Test.** Unit: OK held (a key-down, then repeats every 50 ms from 400 ms) does not leave
  controlled mode; red today if the reasoning holds. Lab: ten deliberate double presses on each TV;
  the slowest should fall inside the window with room.

### 3.4 The rest of the group

- **`stickRepeatFirstMs` 400, `stickRepeatEachMs` 100** (`input-pads.ts:181`). Android: 400 ms
  (500 ms before 12), then 50 ms. Moving focus every 50 ms outruns the eye on a tile row; 100 ms is
  a fair choice. Keep; the button test's H8 can confirm. *Confidence: medium.*
- **Long press.** The app uses none. If one is added, a key's first `repeat` event is the
  platform's own long-press moment, which keeps it consistent with the device. *High.*
- **`overlayShownForMs` 5 s:** Media3's default (the audit's §8). Keep.
- **`upNextLeadMs` 30 s:** jellyfin-web uses 30 s, 35 s from 40 minutes long and 40 s from 50, none
  under 10 minutes (the audit's §5 item 20). Copy that until D339's credits detection. *Medium.*

---

## 4. "Still watching?" (`web/apps/web/src/still-watching.tsx:14`)

- **Now:** after 3 autoplays in a row with no press.
- **Evidence.** Netflix's help page, read today: on TVs and streaming devices, "after 3 episodes and
  90 minutes of uninterrupted watching"; on other devices, "after watching 3 episodes of a TV show
  in a row without using any video player controls". *[1P]* YouTube asks after 30 minutes on mobile,
  60 on the web and 180 on a TV, with autoplay on (TeamYouTube) *[S]*. Both give a TV more time.
  Ours asks after half an hour of 11-minute episodes.
- **Recommendation:** on a TV (the app's TV mode), 3 autoplays **and** 90 minutes since the last
  press; elsewhere 3 autoplays, as now.
- **Confidence:** high. It copies a published rule.
- **Test.** Unit with a fake clock: three 11-minute autoplays ask nothing on a TV; the next autoplay
  after 90 minutes asks; on a phone, the third asks.

---

## 5. The 3 s wait for a direct path (`web/packages/sdk/src/direct-path.ts:14`)

- **What it is.** `reachCore` (`web/apps/web/src/discovery-start.ts`) races the core's published
  names first. WebRTC through the discovery starts only once each name has answered, failed, or
  waited `directPathWaitMs`. A name that fails at once costs nothing. One whose packets vanish
  (a private address from a phone on mobile data, a public IPv6 name behind a firewall that drops
  rather than refuses) costs the full 3 s, then WebRTC's own setup, before the first request.
- **Why it exists.** A direct path is better than WebRTC: at home, the LAN name keeps admin and the
  house's speed (D300 §4). The discovery offers the LAN name only to a device that seems to be at
  home (D290 §2, D356), which already avoids most of the cost away from home.
- **What it costs today** *[M]*. Staging's 208 and nas.example's 4 path choices since 2026-10-24 offered
  no public name. On staging the LAN name was offered to 65 pages, left out of the race for 63
  (permission prompt pending, denied, or a Firefox that fails such requests) and answered for 2; on
  nas.example it was offered to all 4 and left out of each. So the wait is rare today. It
  grows once public names are offered, and whenever the discovery judges "at home" wrongly. The log
  line says what came of each name but not how long each took, so the cost cannot be read.
- **A better rule.** Happy Eyeballs (RFC 8305) solved the same race between two paths: start the
  next attempt 250 ms after the first if it has not answered (at least 100 ms, at most 2 s), and take
  whichever connects first. *[1P]* For us: start WebRTC 250 ms after the race begins if no name has
  answered, keep 3 s as each name's own limit, and keep `betterPathGraceMs`, so a direct name that
  answers after WebRTC still takes over. At home the LAN name answers well under 250 ms, so WebRTC
  never starts there. Away, the first request waits 250 ms rather than 3 s. The cost is a WebRTC
  attempt where a slow direct name would have won, which signals the discovery once more, and is
  dropped when the direct path takes over.
- **Also:** add each name's time to `a page chose its path to the core`.
- **Confidence:** medium.
- **Test.** Unit with a fetch that never answers: WebRTC starts at 250 ms, not 3 s (red today).
  On a phone on mobile data with the LAN permission granted, time to the first page, before and
  after.

---

## 6. What the evidence does not settle

- **The Fire TV's real key repeat in Silk** (3.1, 3.3). The button test can show it in five minutes.
- **Whether the 83 paused "slow load" reports hurt anything.** They tell the bitrate controller of
  a drop each time. What these players are is not settled: a film page's hidden player, a start
  gate holding, or a page left open.
- **When a lower quality helps a slow start.** The data cannot separate link-bound from server-bound
  starts. The proposal in 1.1 makes the log say which, and the next read can set the number.
- **Any study of scrubbing speed.** None was found. The players' own choices (Leanback 1%, Media3
  5% per press, Kodi's steps, Apple's 2–4×) are the evidence.
- **nas.example's logs** cover two days. Its starts and seeks were fast (0.3 s median).

## Sources

All looked up 2026-10-29; notes and copies in `/yoloai/cache/research-groups-1-4-5/`.

- Krishnan, Sitaraman, "Video Stream Quality Impacts Viewer Behavior", ACM IMC 2012 (notes in
  `/yoloai/cache/abr-research/notes-krishnan2012.txt`).
- Hoßfeld et al., "Initial Delay vs. Interruptions: Between the Devil and the Deep Blue Sea", QoMEX
  2012, doi:10.1109/QoMEX.2012.6263849, via Seufert et al., IEEE COMST 17(1), 2015
  (`/yoloai/cache/abr-research/notes-seufert2015.txt`).
- Hoßfeld, Seufert et al.,
  [Quantification of YouTube QoE via Crowdsourcing](https://www.tu-ilmenau.de/fileadmin/Bereiche/EI/mt-nam/publications/Quantification_of_YouTube_QoE_via_Crowdsourcing.pdf).
- Conviva, [SPI thresholds](https://docs.conviva.ai/sei/video/ea-features/spi-intro/) and
  [Exits Before Video Start](https://docs.conviva.ai/sei/video/metric-dictionary/exits-before-video-start/);
  State of Streaming Q4 2019 and Q2 2022 *[S]*.
- Media3: `DefaultLoadControl.java` (`/yoloai/cache/forward-buffer/`), `DefaultHttpDataSource.java`,
  `DefaultLoadErrorHandlingPolicy.java`, `DefaultTimeBar.java` (androidx/media, `release`).
- hls.js 1.7.3 `config.ts` and `abr-controller.ts`; dash.js `Settings.js`; Shaka
  `player_configuration.js` and `externs/shaka/player.js` (cached in `abr-research/` and
  `forward-buffer/`).
- Android: [`ViewConfiguration.java`](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/core/java/android/view/ViewConfiguration.java),
  [Leanback `PlaybackTransportRowPresenter`](https://android.googlesource.com/platform/frameworks/support/+/300475fc42431cefb0d8f1bfc70de1a7a052a501/leanback/src/main/java/android/support/v17/leanback/widget/PlaybackTransportRowPresenter.java),
  [TV playback controls](https://developer.android.com/training/tv/playback/controls),
  [TV app quality](https://developer.android.com/docs/quality-guidelines/tv-app-quality).
- Kodi: [`system/settings/settings.xml`](https://github.com/xbmc/xbmc/blob/master/system/settings/settings.xml),
  [`xbmc/SeekHandler.cpp`](https://github.com/xbmc/xbmc/blob/master/xbmc/SeekHandler.cpp).
- Apple, [Control playback on Apple TV 4K](https://support.apple.com/guide/tv/control-playback-atvb120650e1/tvos).
- Roku, [Remote control buttons](https://developer.roku.com/dev/docs/remote-control-buttons).
- Amazon, [Remote Control Input](https://developer.amazon.com/docs/fire-tv/remote-input.html),
  [Controller Behavior Guidelines](https://developer.amazon.com/docs/fire-tv/controller-behavior-guidelines.html).
- [Emby community, "Fast sliding seek bar"](https://emby.media/community/topic/124408-fast-sliding-seek-bar-to-fast-forward-just-like-youtubeplex-jellyfin/);
  jellyfin-androidtv issues [#146](https://github.com/jellyfin/jellyfin-androidtv/issues/146),
  [#5912](https://github.com/jellyfin/jellyfin-androidtv/issues/5912).
- Netflix, [help article 114059](https://help.netflix.com/en/node/114059);
  [TeamYouTube on X](https://x.com/TeamYouTube/status/2002491176369152416) *[S]*.
- [RFC 8305, Happy Eyeballs v2](https://www.rfc-editor.org/rfc/rfc8305.html).
- Microsoft, [GetDoubleClickTime](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getdoubleclicktime).
