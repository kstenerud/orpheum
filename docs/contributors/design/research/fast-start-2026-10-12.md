> **ABOUTME:** Why a device on an encoded quality waits ~9 s for its first chunk after a jump, what
> it costs to make one on this machine (measured), how Jellyfin, Kyoo, Plex, Emby, Stremio, ErsatzTV
> and Tunarr cut and serve their segments, what HLS and Safari allow when qualities' chunks do not
> line up, and how ready hls.js is to play a chunk while it is made. Ends in options ranked, those
> that change the design marked. Follows seek-resync-2026-10-12.md; feeds D284 §1.

# Fast start on an encoded quality after a jump

Researched **2026-10-12**, after the owner, on the options for D284 §1: *"do some research to see if
anyone else has tried these things, or other things to deal with this issue."* The question: an
iPhone on the 720p we encode waited ~9 s after a jump for its first 10 s chunk, where the devices on
the film's own picture were ready in ~3 s.

Clones, pages and benchmark output are cached under `/yoloai/cache/fast-start-research/` (Kyoo
`b55a25d`, ErsatzTV `16fd997`, Tunarr `74d1a58`, stremio-video, jellyfin-ffmpeg; RFC 8216, draft
8216bis-19, Apple's authoring spec, scaleplex SEEK.md; `notes.md`, `bench/run-*.txt`); hls.js and
Jellyfin from `/yoloai/cache/seek-research/`. **[V]** is read in a source or measured, **[I]**
inferred.

## 0. Measured on this machine

FFmpeg 8.1.2 from the runtime image, 8 CPUs, staging idle; the 720p rung's own arguments
(`encoder.go`: 3000k, `-g 100000`, `-sc_threshold 0`), each run starting on a source keyframe after
`-ss`.

| Source, CPUs | First frame after `-ss` | 2 s veryfast | 10 s veryfast | 10 s superfast | 10 s ultrafast | 2 s ultrafast |
|---|---|---|---|---|---|---|
| Star Wren II (H.264), 8 | 0.8 s | 2.4 s | 5.7 s | 4.5 s | 3.4 s | 1.5 s |
| Star Wren II, 3 CPUs | 1.3 s | 2.8 s | 6.7 s | 4.8 s | 3.9 s | 1.9 s |
| Conan (AV1), 3 CPUs | 0.3 s | 1.35 s | 6.8 s | 5.1 s | 4.1 s | 0.95 s |

- A run costs ~0.3–1.3 s to start, then ~0.5–0.65 s per second of film. `-tune zerolatency` saves
  under 0.2 s; keyframes every 2 s inside a 10 s chunk cost no time. **[V]**
- Staging's 8.8–10.3 s is 3–4 s more than this: the sound's encode, copied-picture runs, the core's
  polling and publishing sharing the machine. **[I]**

**What a shorter keyframe interval costs**, 120 s of each film at 720p, veryfast, CRF 23, file size
as the proxy:

| Keyframe every | Star Wren II (grainy) | Copperfield (2019) | Bram |
|---|---|---|---|
| 6 s | +4.1% | +1.7% | +1.7% |
| 4 s | +8.6% | +4.4% | +3.7% |
| **2 s** | **+23.0%** | **+12.1%** | **+8.5%** |

seek-resync §6 had guessed 3 to 5% for 2 s; it is 8.5 to 23%. **[V, one stretch per film]** Streaming
Learning Center found 5.57% between 1 s and 10 s on Big Buck Bunny
([source](https://streaminglearningcenter.com/learning/choosing-the-optimal-segment-duration.html)).

## 1. How others cut and serve

- **Jellyfin.** Encoded output in 3 s segments, a copy in 6 s, cut at the film's keyframes only for a
  remux (`StreamState.cs:73-104`; `DynamicHlsPlaylistGenerator.cs:38-47,155-186`: *"For video
  transcodes it is sufficient with equal length segments as ffmpeg will create new keyframes"*).
  Forces `expr:gte(t,n_forced*len)` on libx264 and VAAPI; a fixed GOP on QSV, NVENC and AMF, which
  cannot force (`EncodingHelper.cs:2031-2091`). A segment is served once the next exists or the job
  ends (`DynamicHlsController.cs:1916-1987`). A seek kills FFmpeg and starts it at the target.
  **It never switches quality between a copy and an encode**: its ladder is not offered when the
  picture is copied (`DynamicHlsHelper.cs:638-665`). Seeks of 5–10 s were called expected
  ([jellyfin#2664](https://github.com/jellyfin/jellyfin/issues/2664), hardware "no meaningful
  improvement"). **[V]**
- **polyfin**, a Jellyfin fork, ends the first segment at the first keyframe ≥2 s, then aims at 4 s:
  1080p encoded starts 5.15 → 3.07 s, remux 3.15 → 1.85 s
  ([PR #138](https://github.com/moodiness/polyfin/pull/138), 2026-10-06). **[V]**
- **Kyoo** cuts every quality, copied or encoded, at every source keyframe, no minimum or maximum
  (`videostream.go:117`, `stream.go:308-329`): one grid, as ours, with our problem on sparse
  keyframes. A seek starts a new encoder head one segment early. Preset `fast`. **[V]**
- **Plex** forces a keyframe every 8 s; a seek starts a new transcoder at the offset
  ([scaleplex SEEK.md](https://github.com/Varashi/scaleplex/blob/main/docs/SEEK.md)). No first-segment
  trick found. **[V, third party]** **Emby:** 3 s segments, 6–8 s seeks reported. **[weak]**
- **Stremio** ([stremio-server-go](https://github.com/M0Rf30/stremio-server-go)): 4 s segments, one
  FFmpeg per segment with a 10 s run-up; 5.2 s per 720p segment, 3.8 s with 5 s
  ([#30](https://github.com/M0Rf30/stremio-server-go/issues/30)). Ours start on keyframes, no run-up.
  **[V]**
- **ErsatzTV, Tunarr** (live channels): 4 s segments; ErsatzTV a keyframe every 2 s. **[V]**
- **No one** found switches quality between a copy and encodes on different grids, or serves a
  film's segment while it is encoded. **[V]**

## 2. Chunks that do not line up across qualities

- **RFC 8216 §6.2.4** requires matching content, timestamps, discontinuities and target duration,
  not aligned boundaries; clients place segments by the playlist timeline (§6.3.2). Apple's spec:
  boundaries at the same points SHOULD (8.22, for interoperability), discontinuities MUST (8.15),
  segments start with an IDR MUST (7.4), IDRs every 2 s SHOULD (1.13). **[V]**
- **hls.js 1.7.3** loads, after a switch, the fragment holding the buffer's end
  (`base-stream-controller.ts:1874-1960`), appending its overlap over what was buffered. Unaligned
  switches are otherwise untested; one old hang report ([#1905](https://github.com/video-dev/hls.js/issues/1905),
  0.11, stale). **[V]**
- **WebKit** flushes, and stutters, when an append replaces a frame already sent to the decoder
  ([WebKit PR #68088](https://github.com/WebKit/WebKit/pull/68088), open): an overlap near the
  playhead stutters on an iPhone, one far ahead does not. **[V; I for where]**

## 3. Serving a chunk while it is made

- **hls.js `progressive`** is "experimental" and forces the fetch loader, so our `xhrSetup` headers
  (pace, sought) would move to `fetchSetup`. **A chunk of several fragments sticks in progressive
  mode since 1.4.2** ([#5697](https://github.com/video-dev/hls.js/issues/5697),
  [#6764](https://github.com/video-dev/hls.js/issues/6764)); the fix,
  [#8058](https://github.com/video-dev/hls.js/pull/8058), merged 2026-09-29, is **not in 1.7.3**,
  and may be incomplete. **[V]**
- A chunk streamed as it is made measures the encode, not the link: hls.js's estimate, and our D265
  controller, would read a slow link; dash.js works round it for low-latency DASH by measuring per
  moof. **[V]**
- LL-HLS parts are for a live edge, removed three target durations back (8216bis-19); no one lists
  them through a whole film. **[V; I]** Nothing found either way on ManagedMediaSource.

## 4. Other ways

- **A faster preset for the first chunk:** ultrafast ~40% less time than veryfast, superfast ~20%,
  a softer picture at the same rate. **[V for time]**
- **Contention:** ~3–4 s of staging's 9 s. **[I]**
- **Hardware encoding:** no start-up numbers published; Jellyfin saw little gain. **[V]**
- **Player:** `startFragPrefetch` and `testBandwidth` act only at start-up, not at a seek. A first
  chunk at 480p (~0.45× 720p's pixels) is dash.js's open TODO. A still from the timeline (Jellyfin
  trickplay, Apple I-frame playlists, Netflix's 10 s thumbnails) shortens nothing but how the wait
  feels. **[V]**

## 5. Options

| | What | iPhone after a jump | Cost | Risk | Design |
|---|---|---|---|---|---|
| A | A jump's first chunk first: other runs lowered and their threads capped while it is made | ~9 → ~6 s | small | none worth naming | no |
| B | A jump's run makes its first chunk at superfast or ultrafast, the rest at veryfast from the second in parallel | ~6 → ~4–5 s with A | small | that chunk is softer, and kept | no |
| C | Encoded chunks split inside the copied grid, ~3 s each, IDR at each: every copied boundary an encoded one | first part ~1.5–3 s; ready ~3–5 s | +4–9% bits, 3–5× requests on encoded qualities; the grid and runs | moving up to the copied picture re-loads part of a chunk, far ahead of the playhead; needs one iPhone test | D247 §10 |
| 1 | Encoded qualities on their own 2 s grid | as C | +8–23% bits | overlaps both ways, the drop to a lower quality near the playhead, where WebKit stutters | D247 §10 |
| 2 | 10 s chunks of 2 s fragments, served as made, hls.js `progressive` | ~1–2 s, in theory | streaming in node, relay and player; fetch loader; ABR measured per fragment | broken in 1.7.3 for chunks of several fragments; experimental; untested on ManagedMediaSource | chunk format |
| D | A jump's first chunk at 480p, stepping up after | modest | D265's 120 s hold excepted | softer for a while | ABR |
| E | A still while waiting | felt only | UI and thumbnails | none | M5 UI |

**Recommended:** A and B now, which change no design and help whatever grid follows; for the grid,
C over 1, after one iPhone test of hls.js moving from an encoded quality up to the copied picture in
the middle of a chunk; 2 set aside until hls.js releases #8058 and the controller measures per
fragment.
