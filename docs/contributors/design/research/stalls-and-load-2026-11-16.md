> **ABOUTME:** Evidence for designing how the core and the player handle a core that cannot keep
> up, apart from a slow link: how other servers detect and cope with transcoder overload, how
> players' bitrate controllers treat server-side delay, what CMSD, CMCD and Server-Timing can carry
> between the two, how a transcoder's speed is measured, and who shares one encode among viewers.

# Stalls and load: a slow core is not a slow link

- **Date:** 2026-11-16 (searched, fetched and read the same day; notes, fetched pages and source
  clones in `/yoloai/cache/research/stalls-2026-11-16/` and `/yoloai/cache/src/`).
- **Asked by** the lead, for the owner's direction of 2026-11-16: a core having trouble keeping up
  eases the processor, not the network; a slow core may drop quality, but not for the same reason or
  in the same way as a network stall; each side tells the other what helps it decide. Builds on
  [morning-questions-2026-11-16.md](morning-questions-2026-11-16.md) §1, which covers the players'
  stall detectors and the QoE rebuffer; that is not repeated here.
- **Sources:** source code read locally: Jellyfin (main as of 2026-11-14), dash.js 5.3.1, Shaka
  Player (main as of 2026-11-16), hls.js 1.7.3 (`web/node_modules`), the SVTA Common Media Library
  (the CMSD and CMCD key definitions, quoting CTA-5006 and CTA-5004-B), FFmpeg's
  `fftools/ffmpeg.c` (master); vendor pages from Plex, Emby, NVIDIA, MDN and kernel.org; vendor
  forums, which are anecdotes, dated. A claim not checked against its vendor's own page or source is
  **UNVERIFIED**.
- **The design is the owner's** (AGENTS.md rule 1). Every recommendation below is the research
  author's, marked as one, for the owner to accept, change or drop.

## What Orpheum already does

Worth stating first, because the core already has more of a load policy than any self-hosted server
surveyed:

- **Speed is measured per run, over a window.** The run tracker samples the end of each chunk made
  and computes film made per wall second over `chunkRunSpeedWindowSeconds`, with the first chunk's
  time kept apart as the store's startup (`internal/core/chunks/capacity.go`).
- **Every run stays faster than playback** (D250 §6): below `chunkRunSlowestSpeedPercent` (110 %),
  with two or more runs, one is cancelled (the oldest no one plays), else the newest's players are
  moved down a quality already made (D309 §6), else it is evicted and its players refused
  (`ErrNoRoomToMake`, 429). Before any eviction a played software encode that is behind holds the
  node back (D324 §5).
- **A machine too slow at its start offers no encode** (D457): a ten-second trial at the lowest
  rung, FFmpeg's `speed=`, refused as `tooSlowToConvert` and said on the System page.
- **A chunk not made yet is 425 at once** (`chunkHoldMs` 0, D250 §5), because hls.js counts a held
  request as download time.
- **The player tells the core** its picture, sound, position and pause (`PUT /playback/{token}/wants`)
  and its stalls; **it does not tell** its buffer or its measured throughput. **The core tells the
  player** nothing per chunk beyond the 425 and its `Retry-After: 1`.

What is missing against the owner's direction: an easier encoder or preset as a response (today the
response is fewer runs or a lower rung, which is a network-shaped answer); a cap the admin sets; a
word in the log when the machine is loaded by something else; and the two-way information.

## Summary

| Question | Headline finding | What most changes Orpheum's design |
| --- | --- | --- |
| 1. Overload in Plex, Jellyfin, Emby | None detects that its transcoder is falling behind or responds to it. Plex and Emby fall back from hardware to software on a failure; Plex caps simultaneous transcodes (the refused viewer sees "Not Enough CPU Conversion"); Jellyfin caps sessions per user only; all three run one transcode per viewer | Orpheum's per-run speed watch is already beyond them; the response (an easier preset, a cap) is new ground, not an industry pattern to copy |
| 2. ABR and server delay | No player models server-side delay as its own thing. dash.js drops it from throughput (dead time); hls.js keeps a separate time-to-first-byte estimate and adds it to every rung's fetch time; Shaka counts it as slow download | The player cannot tell a slow core from a slow link from timing alone, so the core must say it |
| 3. CMSD, CMCD, Server-Timing | CMSD has `rd` (request to first body byte), `mb` (max suggested bitrate), `du` (duress), `etp`; Shaka applies `mb` and `etp` by default, dash.js behind a setting; nobody reads `rd` or `du`. CMCD's `bl`, `mtp`, `dl`, `su`, `bs` are what the core would want. Nothing standard carries encode time, encoder speed or queue wait | `mb` is the standard way for a server to lower quality for its own reasons; encode time and speed need Orpheum's own (prefixed) keys |
| 4. Measuring speed | FFmpeg's `speed=` and `fps=` are averages since the run began, so they hide a slowdown and include the start-up | Keep measuring from chunks made over a window, as now; never judge a fresh run, a seek's first chunk or a spun-down disk by its first seconds |
| 5. Sharing an encode | Plex, Emby and Jellyfin give each viewer its own transcode (Jellyfin's output file is keyed by the play session) | Orpheum's stores are per film and quality, already shared: two viewers on one rung cost one encode, which every load figure should count |

## 1. Transcoder overload in Plex, Jellyfin and Emby

**Jellyfin** (source read, main as of 2026-11-14):

- **No detection of a transcoder behind.** FFmpeg's stderr is parsed for `fps=` and `time=`
  (`JobLogger.cs`) into `TranscodingInfo.Framerate` and `CompletionPercentage`, shown on the
  dashboard; nothing decides on them.
- **Its throttler is for a transcoder ahead**, not behind (`TranscodingThrottler.cs`): every 5 s, when
  the transcode is more than `max(ThrottleDelaySeconds, 60)` s past what the client downloaded, it
  writes `p` to FFmpeg's stdin to pause it, `u` to resume. A 2025 migration turned it off everywhere:
  *"Disable transcode throttling for all installations since it is currently broken for certain video
  formats"* (`20250420050000_DisableTranscodingThrottling.cs`).
- **A segment request waits without limit**: polls every 100 ms until the segment and the next exist
  or FFmpeg exits (`DynamicHlsController.cs`); a request more than `24 / SegmentLength` segments past
  the encoder restarts it there (the "Jellyfin's rule" Orpheum already cites, D353).
- **No easier preset under load:** `EncoderPreset.auto` is `veryfast` for x264/x265, fixed.
- **No fallback on failure:** a non-zero exit is logged ("FFmpeg exited with code {0}") and thrown.
- **Caps:** `UserPolicy.MaxActiveSessions` (sessions per user, refused at sign-in) and
  `RemoteClientBitrateLimit`; no server-wide transcode cap. Every `TranscodeReason` is a client
  capability; none is load.

**Plex** (support pages, fetched 2026-11-16):

- **"Maximum simultaneous video transcode":** *"Set the maximum number of transcoding sessions that
  you will allow simultaneously … even if your computer is capable of more"*
  ([Transcoder settings](https://support.plex.tv/articles/200250347-transcoder/)). One forum user
  reports what the refused viewer sees: "There Was a Problem Playing This Item" (LG TV) or "Playback
  Was Not Possible" (Android), then "Not Enough CPU Conversion For This Item"
  ([forum, 2017-07-15](https://forums.plex.tv/t/remote-user-experience-when-shared-server-limits-are-reached/197443);
  **UNVERIFIED** against Plex's own page, and old).
- **"Transcoder quality"** (Automatic, Prefer higher speed, Prefer higher quality, Make my CPU hurt)
  passes different x264 options; *"Automatic – the Server will select from the other three options
  automatically"*. What Automatic selects on is not documented (**UNVERIFIED** whether it reacts to
  load).
- **Throttling** is the same idea as Jellyfin's: *"a 'throttled' transcode … means … your Plex Media
  Server is able to perform the transcode faster than is necessary"*
  ([throttled](https://support.plex.tv/articles/203064726-if-a-transcode-is-throttled-is-that-bad/));
  "Transcoder default throttle buffer" is the seconds ahead before it throttles.
- **Fallback:** *"Plex supports automatic fallback from hardware to software. If the hardware video
  engine fails to decode or encode a video, Plex will seamlessly switch to using software without
  causing an error"*; Intel encoding has *"no artificial limit to the number of simultaneous
  videos"*; GeForce cards have the NVIDIA driver's limit
  ([hardware-accelerated streaming](https://support.plex.tv/articles/115002178853-using-hardware-accelerated-streaming/)).
- **"This server is not powerful enough to convert video"** is a capability or policy refusal, not a
  measured one: an incompatible file on a server that cannot transcode, a player quality setting that
  asks for a transcode, or transcoding disabled
  ([article](https://support.plex.tv/articles/205002628-why-do-i-get-the-this-server-is-not-powerful-enough-to-convert-video-message/)).
  Users report it on servers that are able, which the client cannot tell apart.

**Emby** (documentation thin; mostly forum):

- Its [Transcoding](https://emby.media/support/articles/Transcoding.html) page names a per-job
  thread count, left at Auto *"as the system will adapt based on current conditions"*, and the app's
  "Max streaming bitrate"; nothing on caps or overload.
- Hardware failure falls back to software: a log line reported as *"Previous transcoding attempt
  failed. Falling back to software transcoding"*, and a 2020 thread where the fallback *"causes a big
  slowdown on my server"*
  ([forum, 2020-01-15](https://emby.media/community/topic/81529-hardware-transcode-failed-fell-back-to-software-and-cause-massive-slowdown/);
  **UNVERIFIED** against Emby's documentation).

**Hardware session caps.** NVIDIA's
[support matrix](https://developer.nvidia.com/video-encode-and-decode-gpu-support-matrix-new) lists
"Max # of concurrent sessions" as **12** for GeForce cards, "Unrestricted" for most professional
and data-centre cards (8 for some entry ones like the T400); no limit on decode sessions. The figure
has been raised over the years (3, then 5, then 8; **UNVERIFIED**, not on the page). Intel Quick
Sync and VAAPI have no session cap; their limit is throughput.

**What the admin sees** in all three: a dashboard list of sessions with the transcoder's frame rate
or speed (Plex shows "Transcode (Speed: N)" and "(throttled)"), and FFmpeg's log. None writes "this
machine is too busy" to its log.

**Pitfalls.**

- A fallback to software is an *escalation* of load hidden as a recovery: Emby's thread shows it
  slowing the whole server. An "easier codec" must mean easier for the processor that is short,
  which after a GPU failure is the CPU.
- A cap counted in transcodes treats a 1080p HEVC encode and a 360p H.264 one as equal; the
  hardware matrix counts sessions, the CPU counts work.
- A refusal without a reason ("not powerful enough") is reported as a lie by users whose machine is
  fine; Plex's own article lists three causes behind one message.

**What this means for Orpheum (options, not decisions).**

1. **Ease the processor by the recipe, not the rung.** On a run behind, try in order: a faster
   preset of the same encoder (x264 `veryfast` → `superfast` → `ultrafast`), then fewer
   pictures' worth of work (a lower rung, which is where today's response starts), then refusal.
   On a run that *failed* (FFmpeg's exit, not slowness), the D332 encoder trials' next choice, with
   the cost of software over hardware counted against the node's speed. Cost: a recipe that varies
   within a store, which the grid's chunks must still splice across (same rung, same timing).
2. **A cap the admin sets.** "At most N runs of encoding at once on this node" (a setting beside
   `chunkRunSlowestSpeedPercent`), refusing the next with its own code and message, as Plex does;
   optionally weighted by rung (a 1080p run counts as more than a 360p one), and, on NVENC, never
   above the card's session limit. A copied picture (no encode) never counts.
3. **A loaded machine says so in the log.** When runs fall behind while the node's own runs do not
   explain it (see §4: the node's CPU pressure, or the trial speed against the window speed), a
   warning naming what was seen, once per episode, on the System page as well.
4. **Say which refusal it is**: too slow at all (D457's `tooSlowToConvert`), too busy now (the cap),
   behind and moved down (D250 §6). Three reasons, three messages; Plex's single one is the
   anti-pattern.

**Recommendation (the author's):** 1, 2 and 4 together; 3 once §4's measure exists. Option 1 is the
owner's "easier codec/preset" made concrete and is the one response that keeps the quality the
viewer's link can carry.

## 2. How players' bitrate controllers treat a slow server

The morning doc covered what each player calls a stall. Here: what each one's throughput estimate
does with the time a server takes before the first byte, which is where a core making a chunk shows.

- **dash.js 5.3.1** (source): by default `useDeadTimeLatency: true`, so throughput is bytes over the
  transfer alone, *excluding* the time to first byte (`ThroughputModel.js`); the latency is kept as
  its own average and reported as a reason, but the throughput rule chooses on throughput only. The
  DYNAMIC strategy uses the throughput rule until the buffer reaches `hybridSwitchBufferTime` (12 s),
  then BOLA (buffer-based) until it falls below half that. The abandonment rule, after 500 ms of
  transfer and six throughput samples, abandons a segment whose remaining download would take more
  than 1.8 times its duration, unless the buffer is above the stable 18 s. **So a slow server is
  invisible to dash.js's throughput, and seen only as a buffer that drains (BOLA, the
  InsufficientBufferRule).**
- **hls.js 1.7.3** (source): keeps a separate EWMA of time to first byte, weighting long samples
  less (`sqrt(2)·exp(-s²/2)`), subtracts `min(this TTFB, the estimate)` from the load time before
  sampling bandwidth (`abr-controller.ts`), and adds the TTFB estimate to every rung's predicted
  fetch time. **So a TTFB far above the estimate is counted mostly as slow transfer** (the reason
  for D250 §5's zero hold), and a high TTFB estimate makes every rung look slower equally; nothing
  models a TTFB that grows with the rung, which an encode's does. Its abandonment check (every
  100 ms) compares the remaining load time with `bufferStarvationDelay`.
- **Shaka Player** (source): its fetch plugin starts its progress clock before `fetch()` resolves,
  so the first progress interval includes the time to first byte: **server delay is counted as low
  bandwidth**. It reads CMSD by default (§3).
- **Abandonment in all three** is judged against the buffer, not the server; none retries a
  slow-to-start segment at a lower rung because the *server* is slow.

**The pattern.** No player has a model of "the server is slow"; the nearest are dash.js's
separation of latency from throughput and hls.js's TTFB estimate. Server slowness reaches the
choice indirectly, through a draining buffer or a polluted throughput sample. The industry's answer
is the server's own signal (§3), which exists precisely because timing cannot separate the two.

**Pitfalls.**

- A 425 retried a second later is, to hls.js, an error and a retry, not a sample; the chunk's
  eventual 200 is fast. The making time never enters the estimate, which is right for the link and
  leaves the player blind to the core.
- A player that moves down for a slow core asks for a rung that may not be made either: the cost of
  a new encode at a seek point (§4) can make the lower rung *later* than the one waited for, unless
  it is already made (which D250 §6 already prefers).

**What this means for Orpheum (options, not decisions).**

1. **Keep the link's estimate clean** (as now: 425 at once, no hold) and add a separate input for the
   core's state, which the bitrate controller reads as a ceiling, not as throughput.
2. **The core decides the move down for its own reasons and says so** (CMSD `mb`, or the move
   D250 §6 already makes, told to the player as such), instead of the player inferring it from
   waits. The player then holds the link's quality as its own, and climbs back when the ceiling
   lifts, not when the link proves itself again.
3. **A slow core is never a stall for D247's move down** (the morning doc's typed events): the wait
   kind "core making the chunk" goes to the report and to the core, not to the link's controller.

**Recommendation (the author's):** 1 with 2: two separate inputs, the link's (throughput, buffer)
and the core's (a ceiling with its reason), as the owner's "not for the same reason or in the same
way".

## 3. CMSD, CMCD and Server-Timing

**CMSD (CTA-5006)**, server to client, in two response headers (definitions as the SVTA library
quotes the specification, `libs/cmsd/src`):

- `CMSD-Dynamic` (per hop): `du` duress, *"included without a value if the server is under duress,
  due to cpu, memory, disk IO, network IO or other reasons … The intent is that the client will use
  this signal to move away to an alternate server"*; `etp` estimated throughput (kb/s, the server's
  view of the transport); `mb` max suggested bitrate, *"the maximum bitrate value that the player
  SHOULD play … If the player is playing a bitrate higher than this value, it SHOULD immediately
  switch to a bitrate lower than or equal to this value"*; `rd` response delay, *"the time elapsed
  between the receipt of the request and when the first byte of the body becomes available to send"*;
  `rtt`; `n`, the intermediary.
- `CMSD-Static` (from the origin): `at` availability time (epoch ms), `br` encoded bitrate, `d`
  object duration, `ht` held time (*"held back by an origin before returning"*, for LL-HLS blocking),
  `nor`/`nrr` next object, `ot`, `st`, `sf`, `su` startup, `v`.
- **Custom keys** *"MUST carry a hyphenated prefix … Clients SHOULD use a reverse-DNS syntax"*.
- **Who reads it:** Shaka, by default (`cmsd.enabled: true`, `applyMaximumSuggestedBitrate: true`,
  `estimatedThroughputWeightRatio: 0.5`: the bandwidth estimate is half its own, half `etp`), and
  filters variants above `mb`. dash.js has the same two uses behind `streaming.cmsd.enabled` and
  `abr.applyMb`/`etpWeightRatio`, off by default. Both parse `rd` and `rtt` and **neither uses
  them**; neither acts on `du`. hls.js 1.7.3 has no CMSD.

**CMCD (CTA-5004, v2 CTA-5004-B)**, client to server, as query or headers (and in v2 as event and
response reports to a collector): `bl` buffer length (per object type, to 100 ms), `mtp` measured
throughput, `dl` deadline (*"from the request time until the first sample of this … needs to be
available … may be equivalent to the player's remaining buffer length"*), `rtp` requested maximum
throughput, `su` startup (*"needed urgently due to startup, seeking or recovery after a buffer-empty
event"*), `bs` buffer starvation and `bsd` its durations, `sta` player state, `pt` playhead, `br`,
`tb`, `d`, `ot`. v2's response mode adds `ttfb`, `ttfbb`, `ttlb`, `rc` and a copy of the CMSD
headers received. **Who sends it:** hls.js (`cmcd` config, v1 by default, v2 selectable), Shaka
(off by default, `version: 1`), dash.js (off by default, query mode).

**Server-Timing** (W3C, [MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Server-Timing)):
`Server-Timing: encode;dur=840, queue;dur=120, disk;dur=35;desc="spun up"`, read in the page as
`PerformanceResourceTiming.serverTiming`; same origin only unless `Timing-Allow-Origin` allows it;
secure contexts only in some browsers; a trailer is seen only by DevTools, not by `fetch`.

**Mapping the owner's wishes onto them.**

| Wish | Standard field | Fit |
| --- | --- | --- |
| Encode took N ms | none (`rd` is request to first byte; Orpheum answers 425 at once, so the 200's `rd` is ~0) | own key |
| Encoder runs at X× real time | none | own key |
| Waited in queue M ms | none (`ht` is a held response, not a queued job) | own key |
| Disk read K ms | none | own key or Server-Timing |
| Core under load | `du` (boolean, meant as "go elsewhere") | partial: Orpheum has no elsewhere |
| Core lowers quality for its own reason | `mb` | exact |
| Player's buffer | CMCD `bl`, `dl` | exact |
| Player's throughput | CMCD `mtp` | exact |
| Seek or start urgency | CMCD `su` | exact |
| Player starved | CMCD `bs`, `bsd` | exact |

**Pitfalls.**

- **Orpheum's WebRTC path.** hls.js's loads cross a service worker and the page's data channel
  (`web/packages/sdk/src/rtc-media.ts`); the response headers are relayed as a list, so a header
  survives, but Resource Timing's `serverTiming` for a response the service worker makes is
  **UNVERIFIED** and should be assumed absent. A response header read by the loader, or the
  existing playback channel, works on both paths; Server-Timing alone does not.
- `du` means "move to another server" in the standard; sending it with no other server tells a
  standards-reading player (Shaka) nothing useful.
- Shaka blends `etp` half-and-half by default: a core that sends `etp` from its own socket view on a
  path through a relay would mislead it. Send none.
- CMCD as query parameters changes each chunk's URL, which defeats any cache keyed by URL; headers
  need CORS preflight on a cross-origin request.

**What this means for Orpheum (options, not decisions).**

1. **CMSD as the envelope, Orpheum's keys inside it:** `CMSD-Dynamic: mb=1200,dev.orpheum-mk=840,
   dev.orpheum-rt=1.8,dev.orpheum-q=120` (made in ms, run speed ×, queued ms), on each chunk's 200 and
   each 425. Standards-reading players get `mb` for free; Orpheum's player reads the rest. Cost: a
   header parser on the player (the SVTA library, a new dependency for the owner, rule 3, or a few
   lines of its own).
2. **Server-Timing**: simplest to emit, readable in DevTools, but not reliable on the WebRTC path.
3. **Orpheum's own JSON on the playback channel** (an answer to `PUT …/wants` or the stall report
   carrying the store's speed and queue), with nothing per chunk. Cheapest; coarser in time.
4. **Client to core:** add CMCD's `bl`, `mtp`, `dl`, `su`, `bs` to the wants the player already
   sends, as named fields, rather than CMCD on every request; or CMCD headers, which hls.js can send
   by configuration.

**Recommendation (the author's):** 1 for core to player (per chunk, the standard's `mb` with
prefixed keys for what no standard has), 4's first form for player to core (the wants already
carry position and pause; buffer and throughput belong beside them, sent when they change, not per
request).

## 4. Measuring a transcoder's speed

- **FFmpeg's numbers are cumulative.** `speed` is the output timestamp over the wall time since the
  run began (`(double)pts / AV_TIME_BASE / t`, with `t = (cur_time - timer_start)`), and `fps` is
  frames over the same time, 0 for the first second (`fftools/ffmpeg.c`, `print_report`). A run that
  slows after five minutes shows it only slowly; a run's start-up (opening, seeking, the first
  keyframe, encoder warm-up) is in every figure. With `-copyts` the first timestamp is subtracted.
- **"Can keep up"** in the servers surveyed is a dashboard figure (Plex's speed, Jellyfin's frame
  rate), never a decision. Orpheum already decides on a windowed speed from chunks made, with the
  first chunk's time kept apart (`capacity.go`), which avoids both faults.
- **Pitfalls specific to on-demand chunks:**
  - **The first chunk after a seek** pays for opening the file, seeking to a keyframe, decoding up to
    the cut and the encoder's look-ahead; it says little about the steady speed (D353 already
    judges a jump by the store's last run's first chunk).
  - **A disk spinning up** adds seconds once; a window that includes it judges the encoder for the
    disk. The node's disk schedule (`internal/node/diskschedule`) knows when a read waited.
  - **Hardware versus software** differ in kind: NVENC or Quick Sync speed is flat until a session or
    engine limit, then falls off; software speed falls with every run sharing the cores. D457's trial
    measures the node alone, at its start, on a test pattern (its own caveat).
  - **Load from elsewhere**: the runs' own speeds cannot say whether the node is busy with something
    else. Linux's pressure stall information can: `/proc/pressure/cpu` and `io` give `some avg10 avg60
    avg300` (the share of time some task waited for the resource); `full` for CPU is *"undefined at
    the system level"*; cgroup v2 has the same per group (`cpu.pressure`), so a node in a container
    can compare its own group's pressure with the machine's; triggers can be polled
    ([kernel docs](https://docs.kernel.org/accounting/psi.html)).

**What this means for Orpheum (options, not decisions).**

1. **Expected against measured.** D457's trial gives the node's speed alone; the window gives it now.
   A run far below what the trial and the count of runs predict, while the node's own cgroup is not
   the one under pressure, is "loaded by something else": the owner's log warning.
2. **Per recipe, not per node.** Keep each store's measured speed with its encoder and preset, so
   the easier-preset step of §1 knows what it buys.
3. **Exclude the first chunk of every run and any chunk whose read waited on a disk** from the
   window that decides a move down, as the startup already is.

**Recommendation (the author's):** 1 and 3; 2 when §1's preset step is built.

## 5. Sharing one encode among viewers

- **Jellyfin:** never. The output file is the MD5 of media path, user agent, device id and play
  session (`StreamingHelpers.GetOutputFilePath`), so two viewers of one film at one quality run two
  FFmpegs. Its live TV shares the *tuner's* input among consumers (`SharedHttpStream`,
  `ConsumerCount`), not a transcode.
- **Emby:** *"each transcoding session is unique and is customized by various user selections"*
  ([Emby team, 2017-09-10](https://emby.media/community/topic/50741-when-2-users-view-the-same-video);
  old).
- **Plex:** no shared transcode found; forum reports describe per-session decisions (**UNVERIFIED**).
- **Commercial on-demand packagers** (just-in-time origins behind a CDN) share by caching each
  segment by URL; this was not checked against a vendor's page (**UNVERIFIED**).
- **Orpheum** already shares: a store is a film's stream at one quality, its chunks made once and
  served to every player of that rung, runs judged by every player's interest (D309, D355), and a
  Watch Together group catches up from what is already being made (D250).

**What this means for Orpheum (options, not decisions).** A cap (§1, option 2) should count *runs*,
not viewers: ten viewers on one rung are one encode. The refused viewer's message can then say
"this server is busy converting for others" honestly, and a viewer asking for a rung already being
made is never refused for load.

## The facts that most change the design

1. **No self-hosted server responds to a transcoder falling behind**; Orpheum's speed watch already
   leads, and the owner's "easier preset first" has no precedent to copy, only Plex's and Emby's
   hardware-to-software fallback, which raises the CPU's load.
2. **No player can tell a slow core from a slow link by timing**: dash.js discards the delay, hls.js
   mostly counts it as slow transfer, Shaka entirely. The core has to say it.
3. **CMSD `mb` is the standard "lower your quality for my reasons"**, read by Shaka by default; the
   rest of what the owner wants carried (encode time, speed, queue) has no standard field and goes in
   prefixed custom keys.
4. **FFmpeg's `speed=` is an average since the run began**; deciding on it misjudges seeks, fresh
   runs and spun-down disks. Orpheum's windowed, chunk-based measure is the right base.
5. **Linux's pressure files** give the "loaded by something else" signal the owner wants logged,
   per machine and per container.
