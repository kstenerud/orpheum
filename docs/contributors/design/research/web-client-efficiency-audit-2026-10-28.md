> **ABOUTME:** An audit of what the web client's JavaScript costs the page's main thread, its
> workers and its memory while a film plays and as it starts, ranked by the expected gain on a Fire
> TV over the cost of the fix. Read before working on the Fire TV's dropped frames, its start, or
> the player's timers, overlays and hooks. It proposes; it decides and changes nothing.

# Web client efficiency audit

- **Date:** 2026-10-28, on an earlier commit. **Requested by** the owner: the client runs on weak devices,
  a Fire TV Stick (Silk 138) above all, where a start dropped 17 of 183 frames in its first 8 s and
  blips black, and chunks on the LAN (WebRTC) path top out at 15–30 Mb/s on a 100 Mb/s Wi-Fi.
- **Scope:** `web/packages/sdk`, `web/packages/player`, `web/apps/web`, `web/apps/cast-receiver`.
  **Not here:** the WebRTC path's own throughput (pion and SCTP settings, the relay's window, the
  core's credit), which another agent measures; the start's black blip at a quality switch, which
  [navigation-and-widgets-answers-2026-10-27.md](../plans/navigation-and-widgets-answers-2026-10-27.md)
  already plans.
- **Labels:** **[M]** measured here (a build, a benchmark, or a count in the code); **[S]**
  suspected, from reading the code and how Chromium works, not measured on a device. No device was
  used. Estimates for the Fire TV take its JavaScript as about **10× slower** than this sandbox's
  desktop core (a Cortex-A53 at 1.7 GHz against a modern x86 core): an assumption, not a measurement.
- **Method:** read the four packages' hot paths; built the client (`vite build`) and walked its
  chunk graph; benchmarked in a scratch copy, never committed: a TV overlay's render per
  `timeupdate` (vitest, happy-dom, React's development build), the WebRTC transport and the page's
  relay to its service worker per 64 KiB message (vitest, node, the real `rtc-request.ts` and
  `rtc-media.ts` over a real `MessageChannel`), and `toLocaleTimeString` (node).

## Findings, ranked by gain on the Fire TV over cost

| # | Finding | Label | Fire TV effect | Gain | Fix cost |
| --- | --- | --- | --- | --- | --- |
| 1 | A film page's preload is downloaded again by the Play page on WebRTC | [M] code | start time, first chunks' throughput | high (TV starts from a film page) | tiny (skip it on WebRTC) to medium (keep it) |
| 2 | A frame-timing callback runs on every video frame of every playback | [M] code, [S] effect | dropped frames | medium–high, unmeasured | tiny |
| 3 | Under a stage that fills the window, the page keeps drawing: an endless pulse, notes changing | [S] | dropped frames on a cast (no full screen) | medium, unmeasured | small (CSS) |
| 4 | Every state change sends two requests, and the start sends more that can wait | [M] code | start, chunk contention | low–medium | small (client) to medium (API) |
| 5 | hls.js keeps every played second in the browser's buffer (`backBufferLength` Infinity) | [M] config, [S] effect | memory on a 1 GB device over a long film | low–medium, unmeasured | tiny |
| 6 | hls.js runs its demuxing on the main thread: no worker in its ESM build | [M] build | main-thread long tasks per chunk, relay latency | low–medium | small |
| 7 | The player's controls render again at every `timeupdate`, hidden or not | [M] | main thread, ~0.4 % | low | tiny |
| 8 | Subtitles (JASSUB) add a worker, a canvas over the film and a per-frame callback | [S] | dropped frames while subtitles show | unknown, possibly high | measure first |
| 9 | Posters and other immutable bytes are never cached on WebRTC | [M] code | the TV's first page and every return to it | medium for browsing, none for playback | small |
| 10 | The first page loads 691 KiB of script; a film page 1.3 MiB | [M] build | app start, film page open | low | medium |

Duplicated work (rule 11) is listed apart, after the findings; what was checked and found cheap
is at the end, so it is not looked at again.

### 1. A film page's preload is downloaded again on WebRTC [M]

- **Where:** `apps/web/src/watching.ts:114` (`preloadInHiddenVideo`), started at `:174`, stopped at
  Play (`:217`); D260 §3: "the chunks it loads are kept by the browser for the Play page's player".
- **What:** the preload fills `preloadAheadS` (10 s, `watching.ts:84`) of the offered quality in a
  hidden video, then is destroyed at Play, and the Play page's player loads from the quality it
  settled on, counting on the browser's HTTP cache (chunks are `private, max-age=86400`,
  `internal/core/api/media_cookie.go:217`). On the WebRTC path every chunk is a response the service
  worker builds from the page's transport (`apps/web/src/sw.ts`, `packages/sdk/src/rtc-media.ts`):
  the HTTP cache never sees it, and nothing else keeps it, so the Play page asks for every chunk the
  preload loaded again, over the same link (the idiosyncrasy "Chromium times a service worker's own
  response as a cache hit" is the same fact seen from Resource Timing).
- **Cost:** 10 s of film at a 12 Mb/s rung is about 15 MB; at the 15–30 Mb/s the Fire TV gets, 4–8 s
  of link spent twice, the second time while the person waits. A preload still loading when Play is
  pressed is cancelled, and what it had loaded is lost too. It also builds a second hls.js, a second
  MSE source and, on Android, likely a second hardware decoder for its first frame, destroyed at Play
  [S].
- **Sure:** high that the bytes are fetched twice (code). The effect on start time is the link's
  time for those bytes, which the play reports' `preloadSteps` and startup times can confirm.
- **Fix:** cheapest, skip the preload where the page's media go over WebRTC (keep the warm-up's
  decision, which starts the server's chunks); or keep what it loaded: hand the preload's video and
  hls.js to the Play page, or keep chunk bodies the preload fetched in the service worker's Cache
  Storage, keyed by URL, for the minutes D258's warm-up lives. The first is a condition in
  `warmUp`; the others are design, the owner's (D260).
- **Affects:** start time and the first chunks' throughput on the LAN path; not dropped frames.
  Casts do not preload, so a cast's start is not affected.

### 2. A frame-timing callback on every video frame [M] code, [S] effect

- **Where:** `apps/web/src/play.tsx:180` renders `FrameTimingNote` under every film;
  `apps/web/src/frame-timing.ts:66` asks `requestVideoFrameCallback` again from each frame's
  callback, for as long as the film plays, for a line of text that measured the TCL TV's A/V lag.
- **Cost:** the callback itself is small: a read of `currentTime`, two pushes and two splices, a sort
  of 48 numbers and a React render every 48 frames. The cost is what the callback asks of Chromium:
  a video frame callback is run in the page's rendering steps, as `requestAnimationFrame` is, so a
  page with one pending runs a main-thread frame (style, layout and paint checks, and a commit to the
  compositor) for each video frame, 24–60 a second, where a film with nothing changing on its page
  needs none. A perpetual `requestAnimationFrame` loop is the known equivalent. On a quad A53 that
  also decodes, composites and relays the film, that is work beside every frame [S].
- **Sure:** that it runs on every frame of every playback, high; how many of Silk's dropped frames
  it explains, unknown.
- **Fix:** time 48 frames once a minute, or only where diagnostics ask (the lab, a developer page,
  or a browser key), rather than on every frame for the film's length. A few lines.
- **Measure:** the play report already sends frames shown and dropped per report
  (`packages/player/src/report.ts`, `framesSinceSent`): the same film started on the Fire TV with the
  note and without it, compared over the first reports.

### 3. Under a stage that fills the window, the page keeps drawing [S]

- **Where:** a cast's receiver gets no full screen (idiosyncrasies.md, "A cast receiver is started
  by a message…"), so its stage fills the window instead: `position: fixed` over the page
  (`apps/web/src/tv-controls.css:41`, `apps/web/src/film-fullscreen.ts:14`). The page stays rendered
  beneath it: the app shell's banner with the status tray, whose sign pulses forever while Watch
  Together or Cast is on (`apps/web/src/status-tray.css:45`, an opacity animation), and the notes
  under the stage, the frame-timing line changing every 48 frames (finding 2) among them.
- **Cost:** an endless animation keeps the compositor producing a frame at the display's rate,
  60 a second, where a 24 fps film alone needs 24; each is a composite of a full-screen video quad on
  the stick's GPU. A text change under the film repaints a layer under it. Chromium culls occluded
  quads when it draws, but still ticks the animation and makes the frames [S].
- **Sure:** that the page is still there and the pulse runs while a sign is on, high; that it costs
  Silk frames, unmeasured.
- **Fix:** while a stage fills the window, take the rest of the page out of rendering
  (`visibility: hidden` or `content-visibility: hidden` on the stage's siblings, or the shell's), and
  stop the pulse while a film plays. Both small; the CSS is the web client's own.
- **Measure:** as finding 2, a cast with and without, compared by `droppedFrames`.

### 4. Two requests for each state change, and more at the start [M] code

- **Where:** at each `playing`, `pause` and `seeking`, `sayInterestOf` sends `PUT
  /playback/{token}/interest` (`apps/web/src/interest.ts:20`, wired at `play-state.ts:563`) and the
  reporter sends `POST /playback/{token}/report` at once (`report.ts:124`–`147`, each
  `recordEvent` flushes). The Play page also asks `GET /files/{id}/up-next` as it mounts
  (`up-next.tsx:85`), wanted only near the film's end; and every visible page asks `GET /me` every
  60 s (`signed-out-watch.ts:63`), playing or not.
- **Cost:** on WebRTC each request is a data channel of its own (`rtc-transport.ts:300`), opened and
  closed, with its messages crossing the page's main thread. At a start that is two to four requests
  beside the first chunks, the playlists and the inits, on a link the first chunks need whole. The
  bytes are small; the cost is the channels and the contention, at the worst moment.
- **Fix:** client side, load Up next once the film has played a while or is within a few minutes of
  its end; API side (the owner's), let the report carry the interest it already implies, so one
  request says both. The `/me` poll could rest while a playback's reports are being answered, a 401
  on them saying the same.
- **Affects:** start time and the first chunks' share of the link; not frames.

### 5. hls.js keeps every played second buffered [M] config, [S] effect

- **Where:** `chunkedHlsConfig` (`packages/player/src/chunked.ts:242`–`286`) sets no
  `backBufferLength`; hls.js 1.7.3's default is `Infinity`, so it never removes what was played, and
  the video's source buffers grow until the browser's own limit (Chromium's is on the order of
  150 MB of video), past which Chromium evicts on each append.
- **Cost:** on a 1 GB Fire TV Stick, up to that limit of video held in the media process for the
  rest of the film, and eviction work at each append once there. At 8 Mb/s it is reached about two and
  a half minutes in. Memory pressure on Android brings garbage collection and, at worst, a renderer
  killed in the background.
- **Fix:** `backBufferLength` of a minute or two (enough for a step back, `seekStepMs`, and a
  follower's hold), one line, the owner choosing the value as a setting or a justified constant.
- **Sure:** the setting, high; the effect on Silk, unmeasured (the lab's memory sampling can show it).

### 6. hls.js demuxes on the main thread [M] build

- **Where:** Vite takes hls.js's ESM build (`hls.mjs`), which has no worker bundle; hls.js then uses
  a worker only when `workerPath` is set, and `chunkedHlsConfig` sets none, so its transmuxer runs
  inline on the page's main thread. Verified in the built chunk: the worker branch tests
  `typeof __HLS_WORKER_BUNDLE__`, which is undefined there.
- **Cost:** for our fMP4 chunks the work is a walk of each chunk's boxes and samples (looking for
  caption SEI in every video sample's NAL units, and reading timing), not a copy of its bytes:
  proportional to the samples, some 240–600 per chunk, so short, but a task on the main thread at
  each chunk of picture and of sound, beside the relay of the next chunk's bytes, which also crosses
  the main thread [S].
- **Fix:** ship `hls.js/dist/hls.worker.js` as an asset and set `workerPath` to it; the policy must
  allow the worker's origin. Small, with a test that a playback's transmuxer is a worker. Worth doing
  with or after finding 2, measured the same way; on its own the gain is likely small.

### 7. The controls render at every `timeupdate`, hidden or not [M]

- **Where:** `useStanding` (`apps/web/src/player-controls.tsx:230`–`254`) sets a new object at each
  `timeupdate` (every 250 ms in Chromium), so `TvControls` (`tv-controls.tsx:101`) or `PageControls`
  render four times a second for the film's length, including while the overlay is hidden
  (`display: none`), and `EndsAt` formats a clock time with `toLocaleTimeString` at each
  (`tv-controls.tsx:180`).
- **Cost, measured:** 320 µs per `timeupdate` for `TvControls` and 195 µs for `PageControls` in
  happy-dom with React's development build (an empty update, 1–3 µs); `toLocaleTimeString` with
  options 38 µs a call, against 1.8 µs for a kept `Intl.DateTimeFormat`. Taking production React as
  about 3× faster and the stick as 10× slower: about 1 ms per update, 4 ms a second, some 0.4 % of
  the main thread, plus the browser's own style and layout when the overlay shows.
- **Fix:** keep the standing to whole seconds (as `useEnding` already does, `up-next.tsx:453`), skip
  it while the overlay is hidden, and keep one `Intl.DateTimeFormat`. A few lines. Low gain; worth it
  only beside the others, as it touches the same files.

### 8. Subtitles: a worker, a canvas over the film, a callback per frame [S]

- **Where:** `drawSubtitles` (`packages/player/src/subtitles.ts:35`) runs JASSUB 2.5 with its
  defaults: libass in WebAssembly in a worker (2.1 MB of wasm), a canvas laid over the video, and a
  render on each video frame through `requestVideoFrameCallback` (its on-demand mode).
- **Cost:** while subtitles show, the page carries finding 2's per-frame main-thread work again, a
  canvas layer over the film that the compositor blends every frame, and the worker's CPU. On a
  stick this is plausibly the largest cost of all, but only while subtitles are on [S].
- **Fix:** measure first (droppedFrames with and without a subtitle). Then, for a plain track with
  no styling, the `<track>` the page already builds for full screen (`fullscreen-captions.ts`) needs
  no canvas and no wasm; JASSUB's own options (`prescaleFactor`, a frame cap) for styled ones. The
  owner's choice.

### 9. Immutable bytes are never cached on WebRTC [M] code

- **Where:** images are `private, max-age=31536000, immutable`, their URL naming their content
  (`media_cookie.go:217`), and subtitle tracks a day; on the WebRTC path the service worker answers
  each from the page's transport and keeps none (`sw.ts`, the fetch handler), so a TV's first page
  fetches every poster again at each launch, each a data channel, where HTTP would have kept them a
  year.
- **Fix:** keep `/bytes/art/` answers in the service worker's Cache Storage by URL (they are
  content-addressed, so never stale), with a size cap. Small. Gains the first page and every return
  to a list, not playback.

### 10. Script loaded at start [M]

- **Measured** (`vite build`, raw / gzip): the first page's static graph is 691 KiB / about 200 KiB:
  `transport-choice` 235/73 (React DOM among it), `client-main` 169/46 (every admin and system
  screen, library management, the folder picker, people, set-up, voice), `router` 159/52 (the
  router, i18next and every locale file, 130 KB of source, the lab's and the gallery's among them).
  A film page and the Play page add `report`, which is hls.js, 577/176, and Play's own 0.3 MiB.
- **Fix:** load the admin and system screens and the lab's and gallery's words lazily, as the later
  screens are (`later-screens.tsx`). hls.js has no smaller build that keeps alternate audio (its
  `light` build drops it). Gain: parse and compile once per load, an estimated 100–300 ms of the
  stick's first-page time; none during playback.

## Duplicated work (rule 11)

Each is cheap on its own; listed so the next change folds them rather than adds a fourth.

- **Three stall watchers on one video:** `chunked.ts:317` (the bitrate controller), `film-player.tsx:605`
  (cannot keep up) and `play-state.ts:507` (the report), each with its own listeners and timers.
  One watcher with three listeners would do.
- **Two standing-still detectors:** `standstill.ts:106` (every second, reports) and the controller's
  `#lookAtPlaying` (`abr.ts:155`, every 500 ms, acts), each tracking position and time.
- **The quality playing, polled and told:** `film-player.tsx:602` reads `playback.playing()` every
  second, while hls.js's `LEVEL_SWITCHED` is already sent to the video as `playerSwitchedQualityEvent`
  (`hls.ts:421`).
- **Where the video stands, read twice per `timeupdate`:** `useStanding` (`player-controls.tsx:230`)
  and `useEnding` (`up-next.tsx:453`), one throttled to the second and one not (finding 7).
- **The session's check:** `GET /me` every 60 s (`signed-out-watch.ts:63`) beside the live
  connection, whose refusal already asks it (`live.tsx`, `useChecksTheSessionWhenRefused`).

## Timers while one film plays on a television

What runs for a chunked film alone on the TV (no Watch Together, no voice), from the code:

| What | Every | Where | Work |
| --- | --- | --- | --- |
| frame timing | each video frame | `frame-timing.ts:66` | finding 2 |
| controls' standing | `timeupdate`, 250 ms | `player-controls.tsx:245` | a React render, finding 7 |
| Up next's ending | `timeupdate`, render each second | `up-next.tsx:453` | episodes only |
| bitrate controller | 500 ms and 10 s | `abr.ts:155`, `:161` | reads buffer and position; cheap |
| overlay's hiding | 500 ms, only while shown | `tv-controls.tsx:205` | cheap |
| standing still | 1 s | `standstill.ts:106` | cheap |
| quality polled | 1 s | `film-player.tsx:602` | a `setState` that bails; cheap |
| play report | 10 s | `play-state.ts:480` | one POST of a few hundred bytes |
| session check | 60 s | `signed-out-watch.ts:63` | one GET |
| start gate | 250 ms, only while waiting to play | `start-gate.ts:185` | a scan of the playlist; cheap |

A cast's receiver adds its session's connection (a ping each 15 s, a young-timeline ping each second
while one is young, state each 60 s, `together-connection.ts:678`), the leader's tick each second
(`together-follow.ts:776`), and a note to the core for each chunk loaded (`together-follow.ts:755`):
all small.

## Checked and found cheap

- **The relay of a chunk's bytes copies nothing** [M]: a data channel's `ArrayBuffer` is wrapped,
  not copied (`rtc-request.ts:155`, `BodyQueue`), moved to the service worker by transfer
  (`rtc-media.ts:354`, `movableBufferOf`, a copy only for a piece that is part of a larger buffer,
  which the transport never hands it), and wrapped again there. Measured per 64 KiB message on this
  sandbox, beyond the fake core's own cost: about 16 µs of CPU for the transport and 7 µs more for the
  relay, both sides in one thread. At 30 Mb/s (57 messages a second) and 10× slower, some 1–2 % of the
  stick's main thread. What limits the relay is not this cost but its latency when the main thread
  is busy: every byte waits for the main thread twice (the channel's message event, the post to the
  worker), so a long task anywhere on the page holds the chunk that is loading. Findings 2, 6 and 7
  are the main thread's share of that; the window is the other agent's.
- **Play reports and notes are small** [M]: a report every 10 s, a few hundred bytes, its arrays
  emptied at each send; a Watch Together note a JSON line of under 1 µs to build.
- **What is kept stays bounded** [M]: the diagnosis keeps 12 steps, 3 choices and one mapping per
  track (`diagnosis.ts`); the link estimate its last samples; frame timing 48 frames; the load speed a
  10 s window; Resource Timing is cleared when full (`cache-hit.ts`). No list grows with a film's
  length; the one exception is finding 5's media buffer.
- **The start gate's 250 ms look** [M]: only while a play waits; reads the buffered ranges and finds
  the next chunk in the playlist.
- **Contexts at the Play page** [M]: `PlayContext`'s value and Up next's are memoised; the page does
  not render again on a tick; only the controls (finding 7) and the frame-timing line do.

## Not covered, worth a look

- **Phones decode above their screen** [S]: `capLevelToPlayerSize: false` (`chunked.ts:276`), so a
  phone may load and decode the top rung for a 400-pixel-wide player. A quality choice, the owner's.
- **Web Audio routing** [S]: where a film is levelled or its sound moved, its sound goes through Web
  Audio for the rest of the film (`packages/player/src/gain.ts`), an audio-thread cost on the stick,
  and moving it at a start throws its loaded sound away (idiosyncrasies.md, the Fire TV entry on the
  sound's move). Known; listed so a measurement of dropped frames records whether it was on.
- **A game controller connected** polls the Gamepad API each animation frame for as long as it is
  connected (`input-pad-poll.ts:261`), a main-thread frame each vsync like finding 2; rare on a
  Fire TV (idiosyncrasies.md).

## Suggested order

1. Finding 1's condition (skip the WebRTC preload) and finding 2's change (time frames on demand):
   both a few lines, each A/B-able on the stick from the play reports' `droppedFrames`, startup
   times and `preloadSteps`.
2. Finding 3's CSS, measured on a cast the same way.
3. Findings 5 and 7, one line and a few, with the next player change.
4. Finding 4's client part, then the API question to the owner; finding 9; then 6 and 10 if the
   measurements still show the main thread busy at a start.
