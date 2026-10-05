> **ABOUTME:** What others do when a viewer's link is slower than the film: measuring the link,
> making smaller copies, switching between them, and what a watch party does about its slowest
> member. Gathered after the owner's first Watch Together test, so that the design avoids the
> paths others have already found bad.

# Slow links

Researched **2026-10-05**, after the owner's first Watch Together test, in four parts run in
parallel: home media servers, how streaming players choose a bitrate, watch-party products, and
how lower bitrates are produced. Sources are primary where one exists: the source code of
Jellyfin, jellyfin-web, hls.js, Shaka, dash.js, Media3, Syncplay, SyncLounge, Kyoo and Tailscale,
read from clones; Apple's SDK headers and specifications; papers; and issue threads with a
developer's reply. A claim marked *(inferred)* is our reading, not a source's words.

## 0. What happened, and what the research changes

The owner hosted from a MacBook running Firefox, tethered to a phone on mobile data, reaching the
server through Tailscale. The film was *The Big Parade*, a 2.4 Mbps H.264 file, served as the
`remux` prepared rendition because its sound needed converting. Played alone, it paused often.
In Watch Together, the group played one frame and froze, over and over.

Five findings, each detailed below:

1. **The link was not a Tailscale relay.** Tailscale's relays carry about 2.2 Mbit/s (§1), which
   would explain it, but the owner's `tailscale ping` answered directly, in 413 ms. The house uploads at
   43.7 Mbit/s, so the limit is most likely the phone's mobile data. *(inferred)* A round trip of
   413 ms usually means a full queue: the link was saturated.
2. **A film's average bitrate is not what it needs.** Peaks run 3–4× the average; Plex budgets 2×
   the average until it has analysed a file (§3). A "2.4 Mbps" film needs about 5 Mbps.
3. **No production player runs a separate speed test before playback.** Every one measures the
   film's own segments as they arrive, starting from a cautious default or the last value it
   remembered (§2). The servers that do run a test (Jellyfin, Emby) have had bugs in it for
   years, and their maintainers advise turning it off.
4. **The one-frame loop is a known bad path.** Jellyfin's SyncPlay counts a member as ready at its
   first frame, and has the same loop. The group must also stop waiting at some point, and must
   not wait again straight away for the same person (§5).
5. **Our prepared rendition is already HLS played by hls.js, which already switches between
   variants on its own** (cut-a.md §6; `web/packages/player/src/hls.ts`). With one variant it has
   nothing to switch to. Lower variants, aligned on the source's keyframes, are what is missing
   (§4).

## 1. The link: Tailscale's relays

- Tailscale says DERP relays "have limited quality of service (QoS) characteristics, so they are
  generally slower than direct connections and may offer lower maximum throughput"
  ([connection types](https://tailscale.com/kb/1257/connection-types)). They are "shared
  infrastructure… subject to throughput limits that ensure fair access for everyone". Tailscale's
  own iperf through one measured "~2.2 Mbits/sec sustained", with a "wildly variable sender
  bitrate", against 27.7 Mbit/s through a peer relay
  ([peer relays blog](https://tailscale.com/blog/peer-relays-international-networks)).
- The open-source relay has an optional per-client token bucket
  (`RateConfig.PerClientRateLimitBytesPerSec`, `derp/derpserver/derpserver.go`). The value
  Tailscale runs with is not published.
- DERP is carried over TCP over HTTPS, so throughput is uneven *(inferred: TCP inside TCP)*.
- **Getting a direct path:** opening UDP 41641 "ensures a direct connection from any peer device,
  where possible"; blocked UDP and hard NAT are "the most common scenarios that prevent direct
  connections" (same page). `tailscale ping <address>` says whether a path is direct or "via
  DERP". A **peer relay** (Tailscale 1.86+) runs on a device the owner controls, over one UDP port
  ([peer relays](https://tailscale.com/kb/1591/peer-relays)).
- *(inferred)* The house is DS-Lite with native IPv6 (no public IPv4), and the router is the
  tailnet node. A direct path over IPv6, or a peer relay, may lift the ceiling more than any
  bitrate change. This is the cheapest thing to try first. **Checked 2026-10-05:** the owner's
  path was direct (`pong … via 11.22.33.144:7853 in 413ms`), so the relay was not the cause.

## 2. Measuring the link

### What players do

| Player | Before any measurement | How it measures | Trusts the estimate after |
| --- | --- | --- | --- |
| hls.js | 500 kbps, raised to the first variant's bitrate, at most 5 Mbps (`config.ts`, `level-controller.ts`) | min of two EWMAs, half-lives 3 s and 9 s; time-to-first-byte excluded | can fetch one lowest-variant fragment first (`testBandwidth`) |
| Shaka | 1 Mbps, "may be too high" for slow links (`player_configuration.js`) | min of EWMAs, 2 s and 5 s | 128 kB; samples under 16 kB ignored |
| dash.js | the last bitrate it saved, else 1 Mbps (`AbrController.js`) | EWMA 3 s / 8 s, dead time excluded | — |
| Media3 | a table by country and network type, else 1 Mbps (`DefaultBandwidthMeter.java`) | weighted median of samples | 2 s or 512 KiB |
| AVPlayer | the first variant listed; Apple's spec says 2 Mbps on Wi-Fi, 730 kbps on cellular | not published | not published |

Safety factors on the estimate: 0.7–0.95 (hls.js 0.95 to stay, 0.7 to step up; Media3 0.7;
dash.js 0.9). Shaka and dash.js drop a response that arrives in under 5–10 ms as a cache hit.

**dash.js's default is the hybrid Netflix's paper recommends** (D86): throughput while the buffer
is below 6 s, buffer-based (BOLA) above 12 s, with the gap between as hysteresis. Its authors:
"throughput-based ABR performs better in situations such as startup and seek when the buffer is
low or empty. And BOLA performs better when the buffer levels are sufficient[ly] large"
([Spiteri et al., ACM TOMM 2019](https://groups.cs.umass.edu/wp-content/uploads/sites/3/2019/12/From-theory-to-practice-Improving-bitrate-adaptation-in-the-DASH-reference-player.pdf)).
The Puffer trial (8 months, 637k streams) found "simple buffer-based control (BBA) performs
surprisingly well", and that links under 6 Mbit/s were 14% of viewing time and **83% of stalls**
([Yan et al., NSDI'20](https://www.usenix.org/system/files/nsdi20-paper-yan.pdf)).

**An emergency abort** is standard: when a segment in flight will finish after the buffer runs
dry, hls.js and dash.js abandon it, fetch a lower variant, and reset the estimate down.

### The separate speed test, and why nobody trusts it

- **Jellyfin** (`jellyfin-web/src/utils/bitrateTest.ts`) downloads 500 kB, then 1 MB, then 3 MB of
  random bytes from `/Playback/BitrateTest`, stopping early below a threshold, with a 5 s timeout
  each, then cuts the result by 30% and caches it for an hour. On a 2 Mbps link the 1 MB probe
  takes about 4 s, near its timeout *(inferred arithmetic)*.
  - For years the timer stopped when the headers arrived, not the body
    ([jellyfin-web#4314](https://github.com/jellyfin/jellyfin-web/issues/4314)).
  - A cap from the browser's `downlinkMax` divides by 8 by mistake, "from the very beginning",
    clamping Auto to 175 kbps–8.75 Mbps; still open
    ([jellyfin-web#8554](https://github.com/jellyfin/jellyfin-web/issues/8554)).
  - A Jellyfin member: "I don't recommend using the auto option… at that same time the app tries
    to load other stuff from the server which skews with the speed test results"
    ([jellyfin-androidtv#2055](https://github.com/jellyfin/jellyfin-androidtv/issues/2055)).
- **Emby**: only the Fire TV and Android TV apps test. The web app uses
  `navigator.connection.downlink`, else a fixed 4 Mbps; "Emby does not test during playback or
  make dynamic changes"
  ([forum 138130](https://emby.media/community/topic/138130-setting-streaming-bitrate-to-either-fixed-or-better-choose-auto/)).
- **The browser's own estimate is not usable.** `navigator.connection.downlink` exists only in
  Chrome; Chrome caps it at 10 Mbps; the specification excludes "connections made to private
  address space", so it cannot see a home server at all
  ([WICG netinfo](https://wicg.github.io/netinfo/)). Firefox and Safari do not have it.
- **A trustworthy standalone test is expensive.** fast.com uses 25 MB files and several
  connections, and must discard its ramp-up and decide when to stop
  ([Netflix tech blog](https://netflixtechblog.com/building-fast-com-4857fe0f8adb)). Several
  connections also overstate what one media fetch gets. *(inferred)* A test good enough to trust
  costs about what the film's first segments cost, so measure those.

### Ways a measurement goes wrong

- **Too small:** time-to-first-byte dominates and the link reads slow (hls.js #3563, fixed).
- **Cached:** a throttled 1 Mb/s link read as about 404 MB/s
  ([hls.js#5826](https://github.com/video-dev/hls.js/issues/5826), open).
- **Idle gaps between segments:** TCP restarts slow start after each gap; under competition the
  player spiralled down to the lowest rate in 91% of runs. Larger segments fixed it
  ([Huang et al., IMC'12](http://yuba.stanford.edu/~nickm/papers/Confused_Timid_and_Unstable_Picking_a_Video_Streaming_Rate_is_Hard.pdf)).
  *(inferred)* `tcp_slow_start_after_idle=0` on the server is the matching knob.
- **A shaped relay** reads its burst and overestimates *(inferred from the DERP log's swings)*.

## 3. Sizing a film to a link

- Plex: "It isn't uncommon for the spikes for these scenes to require 3x, 4x, or even more the
  'average' bitrate of the file." A 3.5 Mbps average needs about 5.4 Mbps with a large player
  buffer and 8.4 Mbps with a small one. Before its nightly Deep Analysis has run, the server
  "will assume a [2 x average bitrate] value". Server caps use only 80% of the stated upload
  ([Plex support 227715247](https://support.plex.tv/articles/227715247-server-settings-bandwidth-and-transcoding-limits/)).
- Apple's authoring spec caps a variant's peak at 200% of its average.
- *(inferred)* Our scan already reads each file's packets for loudness; a peak profile (the
  highest bitrate over a window the player's buffer covers) could be measured once, alongside.

## 4. Lower copies

### What exists today in our design

The `remux` profile copies the video into fMP4 HLS segments of about 2 s, cut at the source's
keyframes, and `h264-8m` re-encodes at about 8 Mbps. `h264-8m` is chosen for a remote client
only when the source is above about 10 Mbps (cut-a.md §6). Both are one-variant HLS, played by
hls.js except in Safari. A 2.4 Mbps film on a 2 Mbps link gets the remux and nothing below it.
Cut A left out live transcoding (cut-a.md §1); transcoding.md §(b) and D86 hold the design for
it.

### Three ways to make one

| | Pre-encoded copy | Live whole-file transcode | Segments encoded just ahead of need |
| --- | --- | --- | --- |
| Who | Plex Optimized Versions | Jellyfin, Plex, Channels DVR | Kyoo |
| Switch mid-play | no: a new file and a seek | no: one bitrate fixed at start | yes, with aligned segments |
| Cost | disk; encode time per film | an encoder per viewer; seek restarts it | encoder time near playback; a segment cache |
| Known trouble | Plex's lowest preset (4 Mbps 720p) is above a 2 Mbps link | seek restarts ffmpeg; segment drift; A/V desync on resume | ffmpeg options from a fork; variable segment lengths |

- **Plex Optimized Versions** exist for "limited upstream bandwidth" so that remote viewers need
  no transcode ([Plex 214079318](https://support.plex.tv/articles/214079318-media-optimizer-overview/)).
  Disk cost is about 450 MB per hour per Mbps *(arithmetic)*: a 1.2 Mbps rung is about 0.55 GB/h,
  a fifth of an 8 Mbps copy.
- **Jellyfin's live transcode** restarts ffmpeg when a seek lands more than 24 s from the encoder
  (`DynamicHlsController`). After the restart its playlist and ffmpeg's actual cuts drift apart
  by one segment, and native HLS players stall
  ([jellyfin#17966](https://github.com/jellyfin/jellyfin/issues/17966), open, September 2026).
  Resuming with copied video and re-encoded audio started the audio seconds early
  ([jellyfin#14194](https://github.com/jellyfin/jellyfin/issues/14194); the fix elsewhere was
  `-copypriorss:a 0`). Its throttle "is just that fragile", and maintainers consider removing it
  ([jellyfin#11465](https://github.com/jellyfin/jellyfin/issues/11465)). Plex shipped "don't
  throttle hardware transcodes" in 1.41.4 and asked users to downgrade within weeks when transcode
  disks filled
  ([Plex forum](https://forums.plex.tv/t/transcoding-throttle-removal-problems-an-example/903554)).
- **Channels DVR** fixed slow seeks by "keeping the encoded segments around in a size-limited
  cache instead of deleting the transcoded content as soon as the player has stopped"
  ([Channels forum](https://community.getchannels.com/t/testing-the-new-experimental-transcoder-for-improved-remote-viewing-of-recorded-content/20044)).
- **Kyoo** ([design write-up](https://zoriya.dev/blogs/transcoder/)) is the worked-out version of
  transcoding.md §(b):
  - Read the source's keyframe times once from packet flags (`ffprobe -show_entries
    packet=pts_time,flags`, about "20 times faster" than decoding), and cache them.
  - Use them as the segment boundaries of **every** variant; force IDR frames at exactly those
    times in the encoded ones (`-forced-idr 1`: "some hardware encoders uses others i-frames").
    The stream-copied original is then one of the variants, and switching is seamless.
  - Each encoder runs forward from the requested segment, stops when it reaches segments already
    made, and a new one starts only for a request more than about 60 s from any running one.
  - It builds against its own ffmpeg; `-hls_times` and `absolute_tfdt` may not be upstream
    *(unchecked)*.

### A ladder for 1–3 Mbps

Apple's authoring spec, H.264 (HEVC in brackets): 432p at 730 and 1100 kbps (300); 540p at
2000 (600 / 900 / 1600); 720p at 3000 and 4500 (2400 / 3400); keyframes every 2 s; 6 s segments
([Apple HLS authoring spec](https://developer.apple.com/documentation/http-live-streaming/hls-authoring-specification-for-apple-devices)).
Netflix's old fixed ladder put 480p at 1750 kbps and 720p at 2350; per-title encoding showed some
films reach 1080p at 1540 kbps and others need 8000 for the same quality
([Netflix per-title](https://netflixtechblog.com/per-title-encode-optimization-7e99442b62a2)).

*(inferred)* Below the remux and `h264-8m`: about 0.8 Mbps (432p/480p), 1.2 Mbps (480p/540p) and
2.2 Mbps (540p/720p), H.264, since every browser decodes it. HEVC saves about 35% but decodes in
Chrome and Firefox only with GPU support, so it needs a capability check; AV1 decodes in Safari
only on M3 Macs and iPhone 15 Pro onward.

**Cost on the NAS:** Quick Sync on an N100 encodes 1080p H.264 at about 270 fps; x264 at its
default preset manages about 26 fps
([benchmark](https://randomnamer.github.io/StaticBlog/en/2024/03/25/jellyfinTranscodeBench/)).
A 2-hour film at 720p is roughly 10–15 minutes of Quick Sync *(inferred)*, ten times faster than
it plays. Quick Sync needs 10–25% more bitrate than x264 for equal quality, more at low bitrates
([goughlui.com](https://goughlui.com/2023/12/28/video-codec-round-up-2023-part-6-h264_qsv-h-264-intel-quick-sync-video/)).

**Switching mid-play** needs Media Source Extensions or native HLS: a progressive file can only
be swapped and sought. hls.js uses MSE. iPhone Safari has only `ManagedMediaSource` (iOS 17.1+),
which works only with an AirPlay alternative or remote playback disabled
([WebKit 17.1](https://webkit.org/blog/14735/webkit-features-in-safari-17-1/)), and plays HLS
natively anyway, with its own switching.

## 5. A watch party's slowest member

| Product | Waits for a stalled member? | For how long | Then |
| --- | --- | --- | --- |
| Jellyfin SyncPlay | yes; "ready" is the first `playing` event | forever until 12.1; 30 s since | others play; the next stall pauses everyone again |
| Silo-Server (self-hosted, Go) | once | 10 s | the member catches up alone; offered a lower quality after 2 stalls in 5 min |
| Apple SharePlay | no, by default; opt-in per reason | the player decides | the member rejoins at the group's current position |
| Syncplay | no pause; everyone is pulled back to the slowest | — | rewind above 4 s, slow to 0.95× above 1.5 s |
| SyncLounge | only for the host | — | guests catch up by rate up to 10 s, else seek |
| Watch2Gether | stopped relaying members' own play/pause | — | — |

- **Jellyfin.** jellyfin-web's `_onPlaying = () => { … self.onReady(); }` makes a member ready at
  its first frame, and it reports a stall only after 3 s. Before 12.1 one stuck client froze the
  group forever ([jellyfin#17864](https://github.com/jellyfin/jellyfin/issues/17864)); 12.1 added
  a 30 s limit with no cooldown, and "The server waits 30 seconds, gives up, and force-pauses the
  entire group. This loops indefinitely"
  ([jellyfin#18121](https://github.com/jellyfin/jellyfin/issues/18121), open). An open proposal
  separates the deadlines: "a generous timeout (60 s) while a member is still preparing playback
  for the first time vs. a tight timeout (10 s) once it has already caught up and is merely
  re-buffering", and names who is buffering
  ([jellyfin#17349](https://github.com/jellyfin/jellyfin/pull/17349)).
- **Silo-Server** had our problem and redesigned on 2026-09-23
  ([issue #1307](https://github.com/Silo-Server/silo-server/issues/1307),
  [PR #1320](https://github.com/Silo-Server/silo-server/pull/1320)): "A stall while the room plays
  pauses it only if the viewer hasn't stalled in the last 5 minutes, the room hasn't paused for
  buffering in the last minute, and the viewer isn't already catching up. Otherwise the viewer is
  marked catching up and the room keeps playing." Also:
  - a stalled host no longer drags the room back to its frozen position;
  - a member still buffering gets no position corrections, because each one rebuilt its stream;
  - a catch-up seek aims ahead by the last load time (at most 10 s), with retries backing off from
    10 s to 60 s;
  - "Continuing without <name>. They'll catch up.";
  - rejected: "never pausing for guest buffering, which makes every hiccup skip content in a
    two-person party".
- **Apple** ([WWDC21 10225](https://developer.apple.com/videos/play/wwdc2021/10225/)): "Forwarding
  everyone's little pauses to the whole group would just cause too many annoyances." Waiting is
  opt-in per suspension reason (`suspensionReasonsThatTriggerWaiting`), and can be limited by
  group size. A player is ready when it is "likely to keep up", which accounts for the link rate
  ([`automaticallyWaitsToMinimizeStalling`](https://developer.apple.com/documentation/avfoundation/avplayer/automaticallywaitstominimizestalling)).

## 6. How much to load before resuming

| Player | Before starting | After a stall |
| --- | --- | --- |
| Media3 (since 1.6) | 1 s | 2 s |
| ExoPlayer v2 | 2.5 s | 5 s |
| Shaka | was 2 s; now left to the browser | the same |
| hls.js, dash.js | left to the browser | the same |
| AVPlayer | "likely to keep up" | the same |

No player documents why it chose its numbers. *(inferred)* A small fixed resume threshold on a
link slower than the film gives repeated short stalls; only a lower variant, or a resume rule
that accounts for the link rate, avoids them. Our 10 s refill (2026-10-05) is a larger fixed
threshold: it turns the loop into longer runs with pauses, not into smooth playback.

**What viewers mind most** ([Dobrian et al., SIGCOMM'11](http://conferences.sigcomm.org/sigcomm/2011/papers/sigcomm/p362.pdf);
[Krishnan & Sitaraman, IMC'12](https://groups.cs.umass.edu/ramesh/wp-content/uploads/sites/3/2019/12/Video-stream-quality-impacts-viewer-behavior-inferring-causality-using-quasi-experimental-designs.pdf)):
time spent stalled has "the largest impact on the user engagement"; bitrate matters "significantly
higher… on live content than on VoD"; abandonment starts after about 2 s of startup delay.
Stalls cost far more than a softer picture.

## 7. Known bad paths

1. **A one-shot speed test before playback** (Jellyfin, Emby): skewed by the app's own startup
   traffic, buggy for years, too short to see a shaped link. Maintainers advise turning it off.
2. **The browser's network estimate** (`navigator.connection`): Chrome only, capped at 10 Mbps,
   blind to private-address servers.
3. **An "Auto" that silently means a fixed low number** (Jellyfin's 1.5 Mbps fallback, Emby's
   4 Mbps). People read "Auto" as "the best my link allows".
4. **Deciding local or remote from the client's address**: wrong behind proxies, tunnels and VPNs
   (Jellyfin #15017, #2682). Tailscale is one of those *(inferred)*.
5. **Sizing to the average bitrate.** Peaks are 3–4× it.
6. **Switching between the original file and a transcode mid-play**: "can never be done a
   seamless manner" (a Plex developer). Plex's server-side automatic quality forced a transcode
   even on the LAN, froze video on iOS, and is "deprecated and will not be fixed".
7. **Restarting the encoder on seek without aligned segments**: drift, desync, two encoders at
   once on weak hardware, waits that "felt like it had hung forever".
8. **Throttling the encoder by the client's reported position** (Jellyfin, Plex).
9. **Ready at the first frame** in a group (Jellyfin SyncPlay; ours until 2026-10-05).
10. **Waiting for a member forever, or with a deadline and no cooldown** (Jellyfin before and after
    12.1; Silo before #1320).
11. **Correcting a member that is still stalled**, each correction restarting its stream (Silo).
12. **One deadline for a first load and a re-buffer** (Jellyfin #17349).

## 8. What has worked

1. **Fix the path first.** A direct Tailscale path or a peer relay, not DERP.
2. **Measure the film's own segments**, excluding time-to-first-byte, small and cached responses;
   remember the last estimate per server and device; start cautiously without one.
3. **Throughput at low buffer, buffer occupancy at high buffer** (dash.js DYNAMIC; Huang 2014;
   D86), with an emergency abort of a segment that will arrive too late.
4. **Variants aligned on the source's keyframes**, so the untouched original is one of them and a
   switch is seamless (Kyoo; transcoding.md §(b)).
5. **A few low rungs, encoded ahead of playback where the encoder is fast enough**, and kept in a
   size-limited cache across seeks and stops (Channels, Plex Optimized Versions).
6. **The person in the loop**: Plex now *asks* whether to lower the quality after 3 stalls, once
   per session, and says what "Auto" chose ("Now: 1.5 Mbps, 480p").
7. **In a group: wait once, then let the slow member catch up alone**, with a cooldown, separate
   deadlines for a first load and a re-buffer, catch-up seeks aimed ahead, no corrections while a
   member is still loading, and the group told who is holding it up (Silo, Apple, Jellyfin #17349).
8. **Only the slow member drops quality**; the group never does (D123; Silo).

## 9. Where this meets our design

- **D86 holds.** Its buffer-driven re-picking is what dash.js and Puffer confirm, and its note that
  startup needs a simple capacity estimate is what every player does. What the research adds:
  the estimate comes from the film's own segments, not a test.
- **hls.js already implements most of §8.3** (throughput EWMAs, abort, switching). With one variant
  it has nothing to do; lower variants in the same master playlist would let it act today.
- **The prepare job is the encoder.** It already writes fMP4 HLS segments while the film plays,
  into a playlist that grows. At Quick Sync's speed, a low rung can be written ahead of the viewer, which is close to
  Kyoo's model without per-request encoders *(inferred; it must be measured on devnas)*.
- **The variants must share the source's keyframe grid**, as transcoding.md §(b) and D86 already
  require; the `remux` rung's segments are cut on the source's own keyframes, so the encoded
  rungs must force IDRs at those times.
- **D246's "everyone waits" (Q2)** is the policy Jellyfin and Silo both moved away from. The 10 s
  refill fixes the loop; it does not bound the wait, and a member who stalls every minute pauses
  everyone every minute.
- **Cut A excludes live transcoding**, and the `h264-8m` rule only fires above about 10 Mbps. The
  owner has said Cut A was a guess to be corrected by use (2026-10-05).
