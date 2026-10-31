> **ABOUTME:** How others cut the time from pressing play to the first frame: predicting the title,
> warming the stream, starting low and stepping up. Gathered 2026-10-07 for the owner's question
> about pre-buffering. Feeds D256–D260.

# Cold start: getting from "play" to the first frame

Research for the owner's question (2026-10-07): *could we check the viewer's connectivity and
pre-buffer or pre-generate the best-matched stream before they press play? Has anyone written
about that, or about other ways to cut cold-start time?*

Marks: **[unverified]** = from memory or a secondary source I could not open this session;
*(inferred)* = my reasoning, not a source's; **measured here** = run in this sandbox today. The web
search budget for the session ran out at the start, so the sources are pages fetched directly,
cached research from earlier sessions (`/yoloai/cache/research-*`), and the source trees of
hls.js, Jellyfin and Kyoo. Fetched pages are cached in `/yoloai/cache/http/coldstart/`
(`notes.md` there lists what each says).

---

## 0. The short answer

- **Yes, people do this, and it has a name.** Netflix calls it **predictive caching**: "predicting
  what a user will play in order to cache (part of) it on the device before the user hits play,
  enabling the video to start faster and/or at a higher quality… We have seen substantial
  reductions in the time spent waiting for video to start"
  ([Netflix tech blog, 2018](https://netflixtechblog.com/using-machine-learning-to-improve-streaming-quality-at-netflix-9651263ef09f)).
  Apple told developers in 2016 to "do some small amount of network IO speculatively when the user
  has identified the content they probably want to play before they make it official"
  ([WWDC16 503](https://www.asciiwwdc.com/2016/sessions/503)). Shaka Player has had a
  `preload()` API for exactly this since 4.8 (2024), and hls.js has `startFragPrefetch`.
- **The connectivity check is the part that does not work as a separate step.** A standalone speed
  test is the path the self-hosted servers took and their own maintainers advise against;
  the browser's network API is Chrome-only and blind to a home server (slow-links.md §2, §7). What
  works is **remembering the last speed measured on real video, per device and server**, which is
  what Plex does ("It will save the last known bandwidth from that particular Plex Media Server for
  future use", [Plex support](https://support.plex.tv/articles/quality-suggestions/)).
- **Pre-generating is cheap for a remux and moderate for one low rung.** Measured here: FFmpeg
  writes a remux's first segment 0.15–0.3 s after starting, and a 480p x264 rung's first 2 s
  segment in 0.4–0.5 s (§5). So FFmpeg is very unlikely to be where a minute goes. *(inferred)*
  The minute is elsewhere: in Orpheum, most probably in waiting for a whole-file read before chunks
  can be cut, in a first segment as long as the source's GOP (10 s in one sample) and as heavy as
  the original's bitrate, and in the time to download that over a slow link. **Measure first**
  (§6); the instrumenting is the first recommendation.

---

## 1. Why it matters, in numbers

- **Krishnan & Sitaraman (Akamai, 23 M views, IMC 2012 / IEEE ToN 2013):** abandonment "is near
  zero for the first 2 s, but starts to rise rapidly"; "abandonment rate increases by 5.8% for each
  1-s increase in startup delay". Viewers of long videos (>30 min, i.e. films) are more patient
  than viewers of short clips; better-connected viewers abandon sooner (fiber soonest, mobile
  latest). ([paper](https://groups.cs.umass.edu/ramesh/wp-content/uploads/sites/3/2019/12/Video-stream-quality-impacts-viewer-behavior-inferring-causality-using-quasi-experimental-designs.pdf);
  quotes from the ToN version cached as `research-bandwidth/krishnan2013-ton.txt`.)
- **Dobrian et al. (Conviva, SIGCOMM 2011):** join time "seems non-critical at the view-level" but
  "becomes more critical for determining viewer-level engagement" (whether people come back);
  tolerance "drops beyond a certain point (around 15 seconds for JoinTime)". Rebuffering matters
  most within a view. ([paper](http://conferences.sigcomm.org/sigcomm/2011/papers/sigcomm/p362.pdf))
- **Definition used by the industry** (Mux): video startup time is "the time it takes for the
  first frame of video to appear after a user hits 'play'"
  ([Mux](https://www.mux.com/blog/the-four-elements-of-video-performance)). Mux gives no target.
- *(inferred)* For two people watching their own films, the abandonment numbers mean less than the
  ratio: under ~2 s feels instant, 5–10 s feels slow, a minute feels broken. A film audience is
  the most patient one measured, so the target is "a few seconds", not the sub-second of
  short-video apps.

---

## 2. Commercial streamers and players

### Netflix

- **Predictive caching** (quote in §0): they model the likelihood that a title will be played
  (e.g. the next unwatched episode of a series being watched), "while respecting constraints
  around resource usage coming from the cache size and available bandwidth"
  ([blog](https://netflixtechblog.com/using-machine-learning-to-improve-streaming-quality-at-netflix-9651263ef09f)).
  The same post lists the inputs to the **initial quality** choice: network characterisation from
  "longer-term historical information about the network and device", because throughput is
  "quite noisy".
- **Start low, ramp using a throughput estimate, then go buffer-based** (Netflix's BBA paper,
  Huang et al., SIGCOMM 2014): "capacity estimation is unnecessary in steady state; however using
  simple capacity estimation (based on immediate past throughput) is important during the startup
  phase, when the buffer itself is growing from empty". Their pure buffer-based BBA-0 always
  started at the lowest rate and lost 700 kb/s of average quality in the first 60 s, which is why
  the shipped version ramps on throughput during startup
  ([paper](https://yuba.stanford.edu/~nickm/papers/sigcomm2014-video.pdf)).
- **Autoplay previews on browse pages** exist on Netflix's TV and web UI [unverified: product
  behaviour, no engineering source found]. Whether they warm the main title's stream was not found.

### Apple (AVFoundation, HLS authoring)

- **Speculative I/O before "play"**: HLS start is four serialised steps (master playlist, key,
  media playlist, first segment); start them "when the user has identified the content they
  probably want to play", e.g. by asking the asset for its duration
  ([WWDC16 503](https://www.asciiwwdc.com/2016/sessions/503)).
- **The first segment's bitrate is the start time**: "A higher bitrate first segment takes longer to
  download. And that means it will take longer to start." AVPlayer starts with the **first variant
  listed**; apps rewrite the playlist to put the one they want first, or set
  `preferredPeakBitRate` (same session).
- **Which first variant**: "For Wi-Fi delivery, the default video variant SHOULD be the 2000 kbit/s
  … For cellular delivery … the 730 kbit/s" (HLS Authoring Spec 1.32a/b); target segment duration
  6 s; IDR every 2 s (1.13)
  ([Apple](https://developer.apple.com/documentation/http-live-streaming/hls-authoring-specification-for-apple-devices);
  cached `research-bandwidth/apple-hls-authoring.txt`).
- **`automaticallyWaitsToMinimizeStalling`** (default since iOS 10): play at once and let
  AVFoundation wait until it judges playback "likely to keep up" (WWDC16 503). *(inferred)* On a
  link slower than the variant, that judgement can take a long time, which matches the owner's
  "the original's high bitrate stalls before falling back".
- **Low-Latency HLS** (partial segments, blocking playlist reload, `EXT-X-PRELOAD-HINT`) is for
  live: the hint lets a client request a resource before it exists and "the server will hold onto
  the request ('block') until it can respond"
  ([draft-pantos-hls-rfc8216bis-22](https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis)).
  The *blocking* idea transfers to our 425 loop (§4.4); partial segments do not matter for VOD.

### YouTube, Twitch

- YouTube plays videos inline on hover in its home feed [unverified: product behaviour; the help
  page could not be fetched]. Google's QUIC paper reports lower YouTube video latency and rebuffer
  rates from QUIC ([SIGCOMM 2017](https://research.google/pubs/the-quic-transport-protocol-design-and-internet-scale-deployment/))
  [unverified: numbers not extracted from the PDF this session].
- Twitch: not researched this session. Its techniques are for live latency and do not transfer.

### Web players' start-up knobs (source read)

| Player | Start quality | Speculative loading |
| --- | --- | --- |
| hls.js | `startLevel` (else first listed); `abrEwmaDefaultEstimate` 500 kbps raised to the first variant's bitrate, capped by `abrEwmaDefaultEstimateMax` 5 Mbps; `testBandwidth` fetches one lowest-level fragment first when `startLevel = -1`; `maxLoadingDelay` 4 s bounds the auto start choice | `startFragPrefetch`: "Start prefetching start fragment although media not attached yet"; `autoStartLoad:false` + `startLoad(pos)` (hls.js `docs/API.md`; its interstitials controller uses `startFragPrefetch:true` to preload ad assets) |
| Shaka | 1 Mbps default estimate | `player.preload(uri, startTime)` returns a `PreloadManager` that "will load the manifest… as well as the initialization segment… intended for reducing start-time latency"; added in 4.8.0 (2024-04-26) (`research-bandwidth/shaka-player.js`, `shaka-CHANGELOG.md`) |
| dash.js | the last bitrate it saved, else 1 Mbps | — |
| Media3 | a table by country and network type | — |
| AVPlayer / Safari native | first variant listed | browser `preload` only (see §4.2) |

hls.js appends a fragment only when it has all of it unless `progressive` (experimental) is on
(`docs/API.md`), so **the first fragment's size divided by the link speed is a floor on start
time** *(inferred from the default)*.

### Speculation in browsers generally

Chrome's Speculation Rules give a well-tested policy for "start work on intent": at `moderate`
eagerness it speculates "if you hold the pointer over a link for 200 milliseconds (or on the
pointerdown event if that is sooner, and on mobile where there is no hover event)", and keeps at
most **2** such speculations, FIFO, cancelling the oldest
([Chrome docs](https://developer.chrome.com/docs/web-platform/prerender-pages)). That is a good
template for when to start and when to cancel a speculative prepare.

---

## 3. Self-hosted servers

### Plex

- **Direct Stream** = remux on the fly ("The Plex Media Server repackages the content on-the-fly
  before streaming it", [Plex](https://support.plex.tv/articles/200250387-streaming-media-direct-play-and-direct-stream/)).
- **Optimized Versions**: pre-transcoded copies (Mobile 4 Mbps 720p, TV 8 Mbps 1080p, Original
  Quality = MP4 remux), so a constrained client direct-plays instead of waiting for a transcoder
  ([Plexopedia](https://www.plexopedia.com/plex-media-server/general/optimized-versions/)). This is
  "pre-generate everything", the expensive end of the spectrum.
- **Remembered bandwidth, not a test**: "the client will track the bandwidth of the video as it
  loads… It will save the last known bandwidth from that particular Plex Media Server for future
  use"; after **3 consecutive buffering events** it *asks* whether to lower quality, once per
  session ([Plex quality suggestions](https://support.plex.tv/articles/quality-suggestions/)).
  The older "Automatically adjust quality" (2017) started "at the quality that you have selected"
  and adapted from there ([forum](https://forums.plex.tv/t/try-the-new-auto-quality-feature-now-on-web-android-ios/193160)).
- **Transcoder settings**: "Transcoder default throttle buffer — amount in seconds to buffer before
  throttling back"; "Prefer higher speed encoding"; background transcodes use x264 `veryfast` by
  default ([Plex transcoder settings](https://support.plex.tv/articles/transcoder/)).

### Jellyfin (source at `00bb05e2`, 2026-10-04)

- **Playlist up front, segments on demand.** The VOD HLS playlist is computed before FFmpeg runs:
  equal-length segments for transcodes, and for **MKV remuxes the keyframes are read from the
  container's Cues index on demand** (`EncodingOptions.AllowOnDemandMetadataBasedKeyframeExtractionForExtensions = ["mkv"]`;
  `MatroskaKeyframeExtractor` seeks to `CuesPosition` and reads `CuePoint`s). No whole-file read is
  needed to cut a remux at keyframes.
- **Seek placement**: `-ss` goes before `-i` (input seek to the previous seek point), with **+0.5 s
  added for HLS remux** "to get the exact keyframe on most videos" (`EncodingHelper.GetFastSeekCommandLineParameter`).
- **Throttling is off by default now**: a 2025 migration sets `EnableThrottling=false` "since it
  wasn't used before and may introduce issues" (`20250420050000_DisableTranscodingThrottling.cs`).
- **A remux is not the slow part**: a user measured Remux/DirectStream jobs at "1,000–3,500×
  real-time speed", a 1.5 GB film's segments written in about 3 s
  ([jellyfin#16608](https://github.com/jellyfin/jellyfin/issues/16608)).
- **A TTFF patch that was rejected, and why** ([jellyfin#17661](https://github.com/jellyfin/jellyfin/pull/17661), Aug 2026):
  `-hls_init_time 1` (short first segment), `-analyzeduration 1.5s`, `-max_delay 0.5`. A
  maintainer: `max_delay` "does NOT make the stream to start faster" and shifts timestamps;
  a short `analyzeduration` "is unsafe… some files… miss its audio channels, as some files will
  only have its first audio channel after a few seconds". Closed. **Lesson: do not shave probing
  globally.**
- **Slow direct-play start on NAS libraries, 5–15 s**: root cause was .NET file locking over NFSv3
  without the lock service; fixed by mounting `nolock` or using NFSv4
  ([jellyfin#873](https://github.com/jellyfin/jellyfin/issues/873)). Another commenter traced a
  10–15 s start to waiting for segment files written over NFS (inotify) when using rffmpeg (same
  thread). *(inferred)* Go's `os.Open` takes no lock, so the first cause should not hit us; the
  second shape (a waiter that does not see a file written on another host promptly) could.
- Its pre-play **bitrate test** and the "auto" pitfalls are in slow-links.md §2.

### Kyoo (the closest design to ours)

Kyoo aligns transcoded rungs on the original's keyframes, so it needs every keyframe time.
"On slow HDD, keyframe extraction can take up to 30 seconds", so "Kyoo actually start[s]
transcoding before every keyframes could be retrieved". Reading packet flags without decoding is
"20 times faster" than `-skip_frame nokey`
([Zoe's blog](https://zoriya.dev/blogs/transcoder/), cached `research-bandwidth/kyoo-blog.txt`).

### Emby, Infuse, Kodi

- **Emby** web: `navigator.connection.downlink`, else a fixed 4 Mbps; no adaptation during
  playback ([forum](https://emby.media/community/topic/138130-setting-streaming-bitrate-to-either-fixed-or-better-choose-auto/)).
- **Infuse** direct-plays everything with its own engine and has no server-side transcode
  control; users on slow links ask for one because high-bitrate files "keep buffering"
  ([Firecore forum](https://community.firecore.com/t/transcoding-options-for-plex-emby-jellyfin/18164)).
  Fast to start on a LAN, no answer for a slow link.
- **Kodi** exposes its read-ahead cache (memory size, read factor) as GUI settings since v21
  ([Kodi wiki](https://kodi.wiki/view/Advancedsettings.xml)). Not relevant to a browser client.

---

## 4. The techniques, applied to Orpheum

### 4.1 Speculative preparation (details page or hover)

**What it would do.** When a film's details page opens (or the play button is hovered/pressed
down), ask the server to *warm* the playback it would choose: for a remux, start the prepare; for
a slow device, make the first chunk(s) of the rung it would start on; for direct play, open the
file and read its first megabytes (and wake the disk).

**Cost.** Measured here: a remux's first segment costs 0.15–0.3 s of FFmpeg; a 480p rung's first
2 s costs 0.4–0.5 s on 8 cores (§5). The real cost is **reads from the NAS** (a remux left
running reads the whole film: 1–60 GB) and **disk wake-ups** *(inferred)*. A speculative prepare
should therefore be **bounded**: the first N segments, or the first minute, then pause until a
real play arrives.

**Cancellation.** Chrome's policy is a good default: start after 200 ms of hover or on
pointerdown/page open; keep at most 2 speculations, FIFO; a new one cancels the oldest
([Chrome](https://developer.chrome.com/docs/web-platform/prerender-pages)). Add an idle timeout
(e.g. leave the details page → cancel within seconds) *(inferred)*.

**Precedents.** Netflix predictive caching; Apple's speculative I/O; Shaka `preload()`;
hls.js `startFragPrefetch`; web.dev's "pre-cache multiple first segments" with the Cache API
([web.dev](https://web.dev/articles/fast-playback-with-preload)). None of the self-hosted servers
does it [unverified: not found in Jellyfin/Plex docs or source searched].

**Client side, separately from the server.** Even with the server warm, the client can fetch the
master playlist, init segment and first segment into memory while the user is on the details page
(hls.js with `autoStartLoad:false` / `startFragPrefetch`, then `attachMedia` on play). On iOS Safari
with *native* HLS, page-initiated `preload` is capped at `metadata` until the first user gesture
on the page (WebKit: `MediaDataLoadsAutomatically` is false on iOS, and the restriction
`AutoPreloadingNotPermitted` is lifted by `removeBehaviorRestrictionsAfterFirstUserGesture`;
`research-ios/wk-HTMLMediaElement.cpp`, `wk-UnifiedWebPreferences.yaml`). A plain `fetch()` of the
first segment is not subject to that, but warms only the HTTP cache *(inferred)*.

### 4.2 Probing connectivity before play

| Method | Verdict | Why |
| --- | --- | --- |
| `navigator.connection.downlink/effectiveType/rtt` | **No** | Chrome/Edge only; not in Safari (desktop or iOS) and `downlink` not in Firefox (MDN browser-compat data, cached `bcd-api-NetworkInformation.json`); "excluding connections made to private address space" ([WICG netinfo](https://wicg.github.io/netinfo/)), so blind to a home server on the LAN or a Tailscale 100.x address *(inferred for CGNAT range)*; Chrome caps it at 10 Mbps (slow-links.md) |
| A dedicated speed-test download | **No** | Jellyfin's test was mis-timed for years, skewed by the app's own traffic; maintainers advise turning it off (slow-links.md §2) |
| Timing artwork/thumbnail loads | **Weak** | Small responses are dominated by time-to-first-byte and caches (slow-links.md §2 "Too small", "Cached"); artwork may come from a CDN or cache, not the node's path *(inferred)* |
| Remembering the last video throughput per device × server | **Yes** | Plex does it; dash.js saves its last bitrate; Netflix uses longer-term per-network history. Store in `localStorage` with a timestamp and an identity of the network path if one is visible (e.g. whether the server URL is LAN or remote) *(inferred)* |
| Measuring the speculative first-segment fetch from §4.1 | **Yes, free** | If the details page prefetched a real segment, its throughput (excluding TTFB, ≥ 128 kB) is a fresh sample on exactly the right path *(inferred)* |

### 4.3 Start low and switch up

- hls.js does this natively with a multi-variant master playlist: set `startLevel` from the
  remembered estimate (Orpheum's `chunked.ts` already does, from the server's `startAtBps`),
  `testBandwidth:false` when the estimate is fresh.
- Safari's native player starts with the **first listed variant**; order the master playlist so
  the variant that fits the remembered link comes first (WWDC16 503; authoring spec 1.32).
- **Switching up within one HLS ladder is seamless** when variants share the keyframe grid
  (Orpheum's chunk design, Kyoo). **Switching from the original file to an HLS rung is not**: a
  Plex developer called it something that "can never be done a seamless manner" (slow-links.md §7).
  *(inferred)* So the decision "direct play the original or start on a rung" must be made **before**
  play, and that is where the remembered estimate pays off: if the link is known to be below the
  original's peak bitrate, start on the ladder instead of trying the original and falling back.

### 4.4 Shorter first segments, faststart, range starts

- **Short first segment**: FFmpeg's HLS muxer has `-hls_init_time` ("Set the initial target
  segment length… After the initial playlist is filled, ffmpeg will cut segments at duration equal
  to hls_time", [FFmpeg formats](https://ffmpeg.org/ffmpeg-formats.html)). **It only helps where
  keyframes exist**: measured here, a copied HEVC film with keyframes every 10.4 s still gave a
  10.43 s first segment with `-hls_init_time 1 -hls_time 2`; an encoded rung with forced
  keyframes gave 2.0 s (§5). For a remux the first segment is as long as the source's first GOP,
  and its size is the original's bitrate × that GOP *(inferred: a 30 Mbit/s Blu-ray remux with a
  10 s GOP is ~37 MB before the first frame; ~15 s on a 20 Mbit/s link)*.
- RFC 8216 requires every segment ≤ the target duration but not equal lengths, so a short first
  segment is legal *(inferred from RFC 8216 §4.3.3.1)*. Apple recommends 6 s nominal.
- **The 425 loop vs. a held request.** Today a prepared playlist not yet written answers 425 with
  `Retry-After: 1` and the SDK retries (`internal/core/api/playback.go`, `web/packages/sdk/src/ready.ts`).
  That adds up to 1 s per wait. LL-HLS's blocking model (hold the request until the resource
  exists) removes it; the chunk path already holds up to 20 s. Small gain, low effort *(inferred)*.
- **faststart for direct-play MP4**: `-movflags +faststart` moves the moov index to the front;
  without it the browser needs an extra range request to the end before it can start
  ([FFmpeg formats](https://ffmpeg.org/ffmpeg-formats.html)). Cost: one round trip — noticeable
  only on high-latency paths (Private Relay, DERP) *(inferred)*. Orpheum does not rewrite
  originals, so this is "detect at scan, and prefer a remux when the moov is at the end and the
  link is high-latency" at most.
- **MKV by range request**: browsers other than Chrome do not play Matroska in `<video>`
  [unverified this session], which is why MKV goes through a remux.
- **MSE first bytes**: web.dev shows fetching the first range of a file and appending it with MSE
  so play starts instantly ([web.dev](https://web.dev/articles/fast-playback-with-preload)); it also
  notes Chrome forces `preload=metadata` on cellular, and `<link rel=preload as=video>` "is not
  currently supported in Chrome nor Safari".

### 4.5 What dominates a remux's start, from a NAS

- **Probing**: FFmpeg reads up to `probesize` (5 MB default) and `analyzeduration` (5 s default)
  of input before writing ([FFmpeg formats](https://ffmpeg.org/ffmpeg-formats.html)). From local
  disk it is negligible (§5); over a NAS it is a few round trips plus 5 MB *(inferred)*. Shrinking
  it globally drops late-starting audio tracks (jellyfin#17661). Orpheum already probes at scan
  time; *(inferred)* passing the known stream layout does not remove FFmpeg's own probe, so leave
  it.
- **Seek**: `-ss` before `-i` seeks to the previous seek point using the container's index, and a
  stream copy keeps everything from there ([FFmpeg](https://ffmpeg.org/ffmpeg.html)). For MKV the
  index (Cues) is usually at the end of the file, so the first seek costs one far read
  [unverified: muxer-dependent].
- **The disk**: a NAS disk spun down takes several seconds to wake [unverified: typical 5–15 s;
  no datasheet fetched]. Orpheum's audit already notes spin-down as an unpriced cost
  (`audit-2026-09-19.md` §16). Speculative warming on the details page hides it *(inferred)*.
- **Whole-file reads before the first chunk**: see §6; this is the one that can take minutes.

---

## 5. Measured here (2026-10-07)

Rig: this sandbox (8 cores), FFmpeg 8.1.2 from `orpheum-runtime:staging`, sample films on local
disk, **warm page cache** (not a NAS, not cold). Scripts in the scratchpad `ff/` directory. The
command mirrors the prepare's: video copied, first audio track to AAC stereo 192k, fMP4 HLS,
`-hls_time 2`, EVENT playlist; time = from launch until the playlist lists `seg00000`.

| Film | What | First segment listed | First segment |
| --- | --- | --- | --- |
| Flow (2024), 1080p HEVC 10-bit MKV, ~2 Mbit/s | remux | 278–299 ms | 10.43 s, 1.84 MB |
| Flow | remux, `-probesize 1M -analyzeduration 1M` | 298 ms | same |
| Flow | remux, `-hls_init_time 1` | 277 ms | still 10.43 s (keyframes every 10.4 s) |
| Flow | 480p x264 veryfast 1 Mbit/s, keyframe every 2 s, `-hls_init_time 1` | 413–499 ms | 2.00 s, 0.30 MB |
| Film A, 1080p x264 MP4 | remux | 150–191 ms | 9.97 s, 0.45 MB |
| Film B, 1080p HEVC MP4 | remux | 149–236 ms | 10.26 s, 0.75 MB |
| Flow (1.27 GB) | `ffprobe` packet scan of the whole file for keyframes (Kyoo's method) | 15.8 s | 649 keyframes |

Readings *(inferred)*: FFmpeg start-up is sub-second and probing options do not matter here; all
three samples have ~10 s GOPs, so **a remux's first segment is ~10 s of the original, whatever
`hls_time` says**; a whole-file keyframe scan of a 1.3 GB film takes ~16 s even from page cache,
so for a 20–60 GB remux-quality film from a NAS it is minutes.

---

## 6. Where Orpheum's minute probably goes, and how to see it

From the code (read, not run): a prepared playback waits for its playlist through 425 +
`Retry-After: 1` (`internal/core/api/playback.go` `relayHLSFile`; SDK `whenReady`, up to 600
attempts). The chunked ladder needs the film's keyframe **grid**, which comes from the node's
**read-through** — one FFmpeg reading the *whole* file for loudness and the picture's timeline
(`internal/node/readthrough/doc.go`); before it exists, chunk requests answer 425 "the film is
being read for where it can be cut" (`internal/core/api/chunks.go`). The prepare's segment length
starts at 2 s (`nodeproto.StartingSegmentLengthSeconds`), but a copied picture can only be cut at
the source's keyframes.

Candidate causes, most likely first *(all inferred until measured)*:

1. **A film not yet read through cannot be chunked** until the whole file has been read (§5: ~16 s
   per 1.3 GB from cache; minutes for a large film from a NAS, more if the read-through is queued
   behind a scan). Any fallback to chunks on such a film waits for that.
2. **The first segment of an original-quality remux is ~10 s of the original** (§5). Over a link
   near or below the original's bitrate, downloading it takes 10 s or more, and AVPlayer/hls.js
   will not start until it has it (hls.js appends whole fragments by default).
3. **Starting at the original on a slow link, then falling back** costs the failed attempt plus the
   restart, because original → ladder is not seamless (§4.3).
4. Disk wake-up, dispatch to the node, and the 1 s 425 granularity: seconds, not a minute.

**The time-to-first-frame breakdown to log** (one line per playback, client and server joined by
the playback token):

| Mark | Where | Meaning |
| --- | --- | --- |
| t0 | client | play pressed (or details page opened, if speculative) |
| t1 | core | playback decision answered (which path: direct / remux / transcode / chunked, and why) |
| t2 | node | job received; t2b job started (slot and device reader obtained) |
| t3 | node | FFmpeg spawned; t3b first byte read from the source (disk wake shows here) |
| t4 | node | first segment and playlist written |
| t5 | client | first non-425 playlist answer (count of 425s) |
| t6 | client | first segment: TTFB, bytes, duration of transfer (→ throughput sample) |
| t7 | client | `canplay`; t8 `playing` / first `requestVideoFrameCallback` |
| — | client | for direct play: time to `loadedmetadata`, range requests made, whether moov was at the end |
| — | both | whether the grid existed; whether the read-through was queued, running or done |

This follows the project's own rule that a report needing digging should add the logging that
would have said it.

---

## 7. Recommendations, ranked by value for effort (two people, self-hosted)

1. **Instrument TTFF (§6)** — *low effort, needed before anything else.* Every later item is a
   guess until the breakdown says where the time goes.
2. **Get the keyframe grid without reading the whole film** — *medium effort, likely the largest
   win for the chunked path.* Read keyframes from the container's index (Matroska Cues, as
   Jellyfin does on demand for MKV; MP4's sync-sample table) at scan time or on first play, and keep
   the read-through for loudness and per-second bytes. Where the index is missing or sparse, do as
   Kyoo: start cutting from the keyframes known so far and extend the grid as the read proceeds.
   (A design change touching D247 §10's grid source: owner's call.)
3. **Remember the link per device × server and decide the start path before play** —
   *low–medium effort.* Store the last measured video throughput (the existing link meter) with a
   timestamp; if it is below the original's peak need, start on the chunked ladder at the fitting
   rung instead of trying the original. Order the master playlist so Safari's first variant is that
   rung. Without a remembered value, start one rung below the top that fits Apple's 2 Mbit/s Wi-Fi
   default. This is what Plex, dash.js and Netflix do; it replaces any pre-play speed test.
4. **Speculative warm-up on the details page** — *medium effort.* On page open (or 200 ms hover /
   pointerdown on play), ask the server to warm the path it would choose: remux → start the
   prepare and pause after the first minute; chunked → make the first two chunks of the start rung;
   direct → open and read the first few MB (wakes the disk). At most 2 warm-ups per viewer, FIFO,
   cancelled on leaving the page. On the client, prefetch playlist + init + first segment and use
   that transfer as a fresh link sample (feeds item 3). The speculative work is background-priority
   and must yield to any real play (the agent's play-driven/background split already exists).
5. **Short first segment for encoded rungs** — *low effort.* `-hls_init_time 1` plus a forced
   keyframe at 1 s (and the rung's normal grid after) makes the first download small. Does nothing
   for copied video. Only do it if the grid alignment rules allow an extra cut point in the first
   chunk (RFC 8216 does not require aligned boundaries; slow-links / playback-in-practice note
   unaligned switches were not tested).
6. **Hold instead of 425 for the first playlist/segment of a prepare** — *low effort, ≤1 s.*
   Hold the request until the file exists (bounded, as chunks already do), so the client does not
   sleep a full second after it is ready.
7. **Leave FFmpeg's probing alone** — FFmpeg is not the bottleneck (§5), and shrinking
   `analyzeduration` loses late audio tracks (jellyfin#17661).
8. **Do not add** a pre-play speed test, `navigator.connection`, or artwork-based speed guesses
   (§4.2).

---

## 8. Not confirmed, and what would confirm it

- Where Orpheum's reported minute actually goes: needs item 1's log on staging/devnas, cold and warm.
- NAS cold-read and spin-up times on the owner's hardware: time `t3b` with the disk asleep.
- How often Matroska Cues list every video keyframe (mkvmerge vs. other muxers), i.e. whether
  index-derived grids match the read-through's: compare both on the sample library.
- Netflix/YouTube hover-preview mechanics, Twitch, the QUIC paper's YouTube numbers: not opened
  this session.
- Whether iOS Safari lifts the preload restriction after a tap on a details page *before* the video
  element exists (WebKit source suggests per-element, after a gesture): test on the iPhone.

## Sources

Netflix ML blog: https://netflixtechblog.com/using-machine-learning-to-improve-streaming-quality-at-netflix-9651263ef09f ·
Huang et al. 2014 (BBA): https://yuba.stanford.edu/~nickm/papers/sigcomm2014-video.pdf ·
Krishnan & Sitaraman: https://groups.cs.umass.edu/ramesh/wp-content/uploads/sites/3/2019/12/Video-stream-quality-impacts-viewer-behavior-inferring-causality-using-quasi-experimental-designs.pdf ·
Dobrian et al.: http://conferences.sigcomm.org/sigcomm/2011/papers/sigcomm/p362.pdf ·
WWDC16 503: https://www.asciiwwdc.com/2016/sessions/503 ·
Apple HLS authoring spec: https://developer.apple.com/documentation/http-live-streaming/hls-authoring-specification-for-apple-devices ·
HLS bis draft: https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis ·
Mux: https://www.mux.com/blog/the-four-elements-of-video-performance ·
Chrome speculation rules: https://developer.chrome.com/docs/web-platform/prerender-pages ·
web.dev preload: https://web.dev/articles/fast-playback-with-preload ·
WICG netinfo: https://wicg.github.io/netinfo/ ·
FFmpeg formats/ffmpeg docs: https://ffmpeg.org/ffmpeg-formats.html, https://ffmpeg.org/ffmpeg.html ·
Plex: https://support.plex.tv/articles/quality-suggestions/, https://support.plex.tv/articles/transcoder/, https://support.plex.tv/articles/200250387-streaming-media-direct-play-and-direct-stream/, https://www.plexopedia.com/plex-media-server/general/optimized-versions/, https://forums.plex.tv/t/try-the-new-auto-quality-feature-now-on-web-android-ios/193160 ·
Jellyfin: https://github.com/jellyfin/jellyfin/pull/17661, https://github.com/jellyfin/jellyfin/issues/873, https://github.com/jellyfin/jellyfin/issues/16608, source tree at commit 00bb05e2 ·
Kyoo: https://zoriya.dev/blogs/transcoder/ ·
Emby: https://emby.media/community/topic/138130-setting-streaming-bitrate-to-either-fixed-or-better-choose-auto/ ·
Infuse: https://community.firecore.com/t/transcoding-options-for-plex-emby-jellyfin/18164 ·
Kodi: https://kodi.wiki/view/Advancedsettings.xml ·
QUIC paper: https://research.google/pubs/the-quic-transport-protocol-design-and-internet-scale-deployment/ ·
hls.js `docs/API.md` and Shaka `lib/player.js` / `CHANGELOG.md` (cached source) ·
WebKit `HTMLMediaElement.cpp`, `MediaElementSession.cpp`, `UnifiedWebPreferences.yaml` (cached source).
