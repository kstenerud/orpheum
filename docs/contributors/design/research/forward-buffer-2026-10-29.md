> **ABOUTME:** How far ahead the chunked player should load: what hls.js, Shaka, dash.js, Media3,
> AVPlayer, Netflix, YouTube and Puffer load ahead and why; the trade-off between fewer stalls and
> slower visible quality changes, wasted bytes, browser memory limits and on-demand encoding; when
> to throw away loaded low quality on a step up; and proposals for the cap. 2026-10-29.

# How far ahead to load

- **Date:** 2026-10-29. Asked by the owner: *"We should research this"*, after a playback held at
  360p filled 450 s ahead with 360p, so a later step up showed only minutes later (staging,
  2026-10-29). The lead then capped hls.js at 60 s ahead at every quality (`loadsAheadAtMostS`),
  pending this research.
- **Builds on:** [abr-adaptation-2026-10-27.md](abr-adaptation-2026-10-27.md) (the evidence behind
  [D352](../../decisions/working-notes.md#d352--the-chunked-player-trusts-its-link-as-far-as-the-evidence-goes-its-first-seconds-latch-nothing-it-climbs-as-far-as-consistent-samples-carry-and-it-grows-steadier-when-it-swings-or-its-link-flakes))
  and [bandwidth-sampling-2026-10-29.md](bandwidth-sampling-2026-10-29.md) (the estimator). Those
  cover how the player chooses a quality. This one covers how much of it to load. It does not
  repeat their sources on estimation.
- **Code read** (at `main` an earlier commit): `web/packages/player/src/chunked.ts` (`chunkedHlsConfig`,
  `loadsAheadAtMostS`, `keptBehindS`), `abr.ts` (`holdsQualityAheadS`), `step-up.ts`
  (`stepUpBufferMs`, `nextCap`), `steadiness.ts` (`holdBeforeStepUpMsAt`), `hls.ts`
  (`keepRunwayWhenTold`), `audio-shift.ts` (its `BUFFER_FLUSHING`), `web/apps/web/src/watching.ts`
  (`preloadAheadS`), `internal/core/settings/registry.go` and `registry_sweep.go`
  (`chunk_qualities`, `chunk_sound_kbps`, `chunk_lookahead_seconds`, `chunk_kept_ahead_seconds`),
  and hls.js 1.7.3 as installed (`web/node_modules/hls.js/dist/hls.mjs`).
- **Cache:** `/yoloai/cache/forward-buffer/` (Media3 `DefaultLoadControl.java` and both release
  notes, Shaka `player_configuration.js` and `externs/shaka/player.js`, dash.js `Settings.js`,
  hls.js `config.ts`, Chromium's `demuxer_memory_limit*.cc/.h`, `source_buffer_stream.cc`,
  `SysUtils.java` and `sys_info.cc`, and the ETH study as text). The earlier papers and notes are in
  `/yoloai/cache/abr-research/`.
- **Labels:** **[1P]** read at the source this session or in the cached source; **[S]** a search
  engine's or fetch tool's summary only; **[I]** inference or my own arithmetic; **[G]** a guess,
  to be measured.
- **Not reached:** Apple's own page for `preferredForwardBufferDuration` (a summary only); any
  published buffer size from Netflix after 2014, from Prime Video, or from YouTube itself (only
  outside measurements); what Silk on the Fire TV Stick really uses as its memory limit (§3.3:
  the source says one thing, a staging log hints another).

---

## Summary

**What the others do.** No two players agree, but they cluster. When a player can't swap loaded
chunks for better ones, it keeps **10–60 s** ahead. Shaka keeps 10 s. dash.js keeps 18 s below its
top quality, and 30 s at the top (60 s for a film over 10 minutes). Media3 keeps 50 s, and also
stops at about 131 MB of video. hls.js keeps 30 s, or longer if 60 MB lasts longer, up to 600 s.
Only players that do swap, or whose ladders are steady, go deeper. YouTube settles near 80 s and
re-downloads low chunks as better ones. Netflix's browser player held 240 s in 2014. Puffer keeps
15 s. **dash.js's rule, more ahead at the top than below it, is the one built for our problem.**
Below the top, loaded low quality is what delays the step up. At the top, there is nothing better
to wait for, so more ahead costs only memory. (§2)

**Why 60 s at every quality is wrong at both ends.**
- **Below the top it is too long.** The step up needs 20 s loaded and a hold of at least 30 s
  (D352 §3). Then everything already loaded must play out before the better picture shows. At 60 s
  ahead, that is **up to 90 s from the decision to the change on screen**, at best. The 450 s of
  360p was the same fault, just bigger. (§1.2)
- **At the top it can be too much for the device.** Chromium keeps at most **150 MiB** of video
  per page on a desktop, but only **30 MiB** on an Android device with 1 GB of RAM or less. A
  Chromecast playing H.264 also gets 30 MiB. At 9 Mb/s, 30 MiB is **28 s**, counting what is kept
  behind. The film's own picture at 20 Mb/s fills 150 MiB in about 60 s. Past the limit, appends
  fail. hls.js then shrinks its own cap and fetches the chunk again. That is wasted bytes, and a
  stall risk on exactly the weak devices. (§3.3)

**Bytes or seconds?** Both. The step-up and hold rules are in seconds, so the target is in
seconds. The memory limit is in bytes, so the cap is in bytes. Today, hls.js's `maxBufferSize`
does the opposite: it is a byte **floor** that makes low qualities load *further* ahead. (§1.1, §3.4)

**Replace loaded low quality on a step up?** Yes, from about 15 s ahead of the playhead: one and
a half of our 10 s chunks. That is dash.js's fast switching (on by default since 2018), and
YouTube does the same. In dash.js's tests, the better picture showed about **50 s sooner**. Only
**6%** of bytes were thrown away (median, on 4G). For us the bytes are small: 15 s of 360p is
about 1 MB. The real costs are elsewhere. First, the server may have to start an encode at the new
quality. Second, Watch Together counts what is loaded. So replace only when the new quality's
next chunk is already made, or is the copied original. (§4)

**Proposals for the owner** (§5):
- **P1.** Below the top quality, load **30 s** ahead: the step-up's 20 s plus one chunk. At the
  top quality, load **60 s** ahead.
- **P2.** Cap each by bytes: at most about a third of the browser's video limit. That limit is
  150 MiB on desktop Chrome. Use 30 MiB on a 1 GB Android device or a Chromecast, about 100 MiB on
  Firefox, and about 100 MB on Safari. Never go below 20 s.
- **P3.** Replace loaded low quality on a step up. Flush from 15 s past the playhead, using the
  `BUFFER_FLUSHING` event the sound shift already uses. Do this only when the new chunks are ready
  or copied, and not while a Watch Together group holds a runway.
- **P4.** Keep `chunk_lookahead_seconds` (60 s) as it is. It is already about what the top quality
  loads.
- **P5.** No separate setting for the LAN. A LAN plays at the top, where P1 already loads more.
- **P6.** Log five things:
  - the target in force at each change, in seconds and bytes;
  - each append that failed for quota, and hls.js's own cap cuts;
  - the time from a step up to the first frame of the new quality;
  - the bytes thrown away by each replacement;
  - the device's `navigator.deviceMemory`.

Five tests (§5.7) check each of these on staging and on the Fire TV.

---

## 1. What the code does today

### 1.1 hls.js's own rule

hls.js loads ahead as far as **max(maxBufferLength, 8 × maxBufferSize ÷ level bitrate)**, never
past **maxMaxBufferLength** (`getMaxBufferLength`, hls.mjs). The defaults are 30 s, 60 MB and
600 s. The bitrate is the level's `maxBitrate`: its BANDWIDTH, the peak. **[1P]** Rob Walch, an
hls.js maintainer, explained why it counts bytes in 2021: "the browser's buffer eviction algorithm
is starting after the browser detects that video buffer size reaches a limit in bytes"
([hls.js #4003](https://github.com/video-dev/hls.js/issues/4003)). **[S]**

Note the shape. The 60 MB is a **floor**, not a cap. It stretches the buffer at low bitrates
(60 MB of 360p is over ten minutes), and never shortens it at high ones. Only `maxMaxBufferLength`
caps. **[1P]**

### 1.2 Ours, worked through

`chunkedHlsConfig` keeps `maxBufferLength` at 30 s and sets `maxMaxBufferLength` to
`loadsAheadAtMostS = 60`. It sets `backBufferLength` to `keptBehindS = 30`. **[1P]** The ladder is
`chunk_qualities` (3000k@720p down to 400k@360p, peaks capped at 1.5×). The sound is
`chunk_sound_kbps` 96 kb/s. The top is usually the film's own picture, copied. Worked through
(**[I]**, taking BANDWIDTH as 1.5 × the average plus the sound):

| Quality | BANDWIDTH ≈ | hls.js alone | With the 60 s cap | Bytes ahead at 60 s |
| --- | --- | --- | --- | --- |
| 400k@360p | 0.7 Mb/s | 600 s (686 s, capped) | 60 s | ≈ 3.7 MB |
| 1200k@480p | 1.9 Mb/s | 253 s | 60 s | ≈ 10 MB |
| 3000k@720p | 4.6 Mb/s | 104 s | 60 s | ≈ 23 MB |
| own picture, 9 Mb/s | 9 Mb/s | 53 s | 53 s | ≈ 60 MB |
| own picture, 20 Mb/s | 20 Mb/s | 30 s (the floor) | 30 s | ≈ 75 MB |

So today the cap is about 53–60 s at almost every quality, and 30 s for a heavy film's own
picture.

**The time from a drop to a visible step up.** The ladder steps up only after these conditions,
in order:
1. A hold of 30, 60, 120 or 240 s, rising with how often the link has swung
   (`holdBeforeStepUpMsAt`, D352 §3).
2. At least 20 s loaded ahead (`stepUpBufferMs`).
3. Everything loaded at the old quality has played. hls.js changes only what it loads next
   (`nextAutoLevel`); our code never flushes the picture. **[1P]**

The third adds the whole cap. At 60 s, the change shows **at least 30 + 60 = 90 s** after the link
recovered, or more on a swinging link. Under hls.js's 600 s it was about ten minutes. The
staging report matches that. **[I]**

### 1.3 The other knobs that touch it

- **The film page's preload loads 10 s** (`preloadAheadS`), one chunk of the ten-second grid.
  That matches Media3's preload and Shaka's goal, and is not in question here. **[1P]**
- **Watch Together's runway** (`keepRunwayWhenTold`, D285 §2) lowers both hls.js values while
  someone in the group is loading. Afterwards it restores the values captured at attach time.
  **[1P]** If hls.js has lowered its own `maxMaxBufferLength` in the meantime, after a quota error
  (§3.3), the restore undoes that cut. **[I]** Harmless on a desktop. It is a loop on a device that
  really hits its limit.
- **The server encodes ahead too.** It encodes `chunk_lookahead_seconds = 60` past the chunk the
  player asked for, in runs of up to 300 s (D247 §10). **[1P]** So the encoder runs about
  *player's buffer + 60 s* ahead of the playhead. That is about 120 s today. A paused or departed
  player gets `chunk_kept_ahead_seconds = 30` more (D309 §7). **[1P]**
- **iPhone decides for itself.** ManagedMediaSource stops the page loading at about 30 s and
  resumes below 10 s ([idiosyncrasies.md](../../idiosyncrasies.md)). Our cap there is moot.
  **[1P]**

---

## 2. What the main players load ahead, and why

| Player | Ahead | When it starts or resumes | Behind | By bytes? | **Strength** |
| --- | --- | --- | --- | --- | --- |
| hls.js 1.7.3 | max(30 s, 60 MB ÷ rate) ≤ 600 s | after the first fragment | ∞ by default | a floor | [1P] code |
| Shaka (main) | `bufferingGoal` **10 s** | `rebufferingGoal` 0 s | `bufferBehind` 30 s | no | [1P] code |
| dash.js (development) | **18 s** below top (`bufferTimeDefault`); **30 s** at top; **60 s** at top for content over 600 s | — | `bufferToKeep` 20 s | no | [1P] code |
| Media3 `DefaultLoadControl` | min = max = **50 s** | 1 s; 2 s after a stall | 0 | **cap**: video 131 MB, audio 13 MB | [1P] code |
| AVPlayer | `preferredForwardBufferDuration` 0 = the system chooses | — | — | — | [S] |
| Netflix browser (2014) | **240 s** (BBA: reservoir 90 s, cushion 126 s, upper reservoir 24 s) | — | — | — | [1P] paper |
| YouTube (2019 measurement) | settles near **80 s**, re-downloads low chunks | — | — | — | [1P] paper |
| Puffer (Stanford, live TV) | **15 s** maximum | — | — | — | [1P] paper |
| Jellyfin web (open PR #7455) | 50 MiB ÷ rate, clamped 6–60 s; today 30 s, or 6 s for high rates | — | — | — | [S] |

What each one's reasons are, as far as they are written down:

- **Shaka, 10 s.** Shaka is built for live as much as for VOD, with low-latency mode on by default
  in its config. It may jump quality every 8 s (`switchInterval`). `minBufferAheadToUpgrade`
  defaults to 0. A short goal keeps its switches visible at once. It also has `clearBufferSwitch`
  (off by default), which clears the buffer on a switch, keeping `safeMarginSwitch` seconds. The
  docs say "on some device it's better to clear buffer". **[1P]**
- **dash.js, 18 / 30 / 60 s.** The docs say: "If there are multiple bitrates in your adaptation,
  and the media is playing at the highest bitrate, then we try to build a larger buffer at the top
  quality to increase stability and to maintain media quality." Below the top, 18 s (once called
  `stableBufferTime`, 12 s). Its BOLA switches in from 12 s buffered. **[1P]** This is the only
  open player whose target depends on the quality. The reason is ours: below the top, loaded
  seconds are seconds you cannot improve.
- **Media3, 50 s, with a byte cap.** ExoPlayer used to load from 15 s up to 50 s, in bursts. In
  2.10 (2019) it set min = max = 50 s for video, and in 2.12.0 (2020-09-11) treated audio the same
  (`RELEASENOTES.md` **[1P]**). Akamai's write-up of the reason (Tamas-Selicean, Greve, 2019):
  "the ExoPlayer team conducted experiments that found setting minBufferMs to the same value as
  maxBufferMs (and thus changing the buffering behavior from bursty to drip-style) reduced
  rebuffering events significantly, while increasing battery usage only slightly." **[S]** The byte
  cap (`DEFAULT_VIDEO_BUFFER_SIZE = 2000 × 64 KiB`) wins over the time unless
  `prioritizeTimeOverSizeThresholds` is set. Even then, Media3 1.11 (2026-08) falls back to bytes
  "if available heap memory seems tight". **[1P]** At 50 s, the bytes bind only above about 21
  Mb/s. *For us:* a well-tested player on the same TV-class hardware keeps 50 s, drips rather than
  bursts, and lets memory override time. **[I]**
- **Netflix, 240 s.** BBA's paper (Huang et al., SIGCOMM 2014) ran in Netflix's browser player,
  with a 240 s buffer of 4 s chunks. Its rate map runs from a 90 s reservoir to 216 s. **[1P]** The
  deep buffer works because BBA chooses by the buffer: every second loaded is margin against the
  traces' swings (17 Mb/s to 500 kb/s). Its stated cost is quality at start-up, "filling the
  reservoir" at the lowest rate. abr-adaptation §3 rejects BBA for us for that reason.
- **YouTube, up to about 80 s, with re-downloads.** Licciardello, Grüner, Singla (ETH Zürich),
  "Understanding video streaming algorithms in the wild", PAM 2020, measured ten platforms. "some,
  like YouTube, settle towards as much as 80 seconds of buffer, while others like Fandom operate
  with a much smaller buffer of around 20 seconds"; RobustMPC on the same traces sat at 20 s.
  **[1P]** Their reading is the crux of this report: "for approaches that allow redownloads
  (including YouTube), larger buffers are a reasonable choice: any chunks that were downloaded at
  low quality can later be replaced … However, for approaches that do not use redownloads, a larger
  buffer implies that all its content must be played out at whatever quality it was downloaded at
  … Thus operating with a smaller buffer of higher-quality chunks may be preferable." **[1P]** Arte
  threw away 23% of its bytes re-downloading; YouTube, SRF and Vimeo "milder". **[1P]**
- **Puffer, 15 s.** Puffer is live TV with 2 s chunks, so it cannot buffer far. Its randomised
  trial (Yan et al., NSDI 2020) compared algorithms *at* that buffer, not buffer sizes. **[1P]**
- **AVPlayer.** The property defaults to 0, "the player will choose an appropriate level of
  buffering". Setting it low raises stalls; setting it high "will increase demand on system
  resources". Reportedly it is honoured only with `automaticallyWaitsToMinimizeStalling` off.
  **[S]** Not ours: Safari runs hls.js on ManagedMediaSource, which decides itself (§1.3).
- **The academic testbeds** used 20–60 s: MPC (Yin, SIGCOMM 2015) and Pensieve 60 s, BOLA's VOD
  tests 25 s. A 30 s buffer filled in the first minute costs half that minute's bitrate (Sammy's
  appendix). **[1P]** (`/yoloai/cache/abr-research/notes-*.txt`)

**Where they agree.** For VOD, **30–60 s** is the mainstream, with 10–20 s at the short end.
Deeper buffers come with either a buffer-based chooser (Netflix) or re-downloads (YouTube). Only
Media3 caps by bytes; hls.js's bytes stretch. Only dash.js varies the target with the quality.

---

## 3. The trade-off

### 3.1 Fewer stalls

A buffer covers a dip as long as it lasts. 60 s ahead survives a minute with no link at all; 30 s,
half a minute. abr-adaptation §4 found that stalls are what viewers mind most. A stall for 1% of a
film's length cuts viewing by 5% (Krishnan, Sitaraman, IMC 2012). Engagement falls with buffering
up to 0.3 events a minute (Balachandran, SIGCOMM 2013). **[1P]** Our links are mostly a home LAN
and a home upload. Their dips are seconds: Wi-Fi, a relay's congestion, another household member.
They are rarely a minute with no link (network-profiles.md). **[I]** Past 20–30 s, more buffer buys
little against our dips. Media3's experiment is the one data point for more, and it compared 15 s
with 50 s. **[I]**

Two more costs of a small buffer:
- **The ON-OFF pattern.** A full buffer makes the player idle between chunks, and an idle
  connection measures low (Huang, IMC 2012). A drip like Media3's keeps the connection warm.
  (abr-adaptation §2.)
- **A stall needs refill time.** The start gate waits for 150% of the next chunk's expected load
  (`ready_ahead_of_next_chunk_percent`). That is independent of the cap.

### 3.2 Slower visible quality changes

Every second loaded below the top is a second the better picture waits (§1.2). ETH's reading
(§2) and dash.js's design agree on this. The cost falls only on playbacks below the top, which
for us are the slow-link and relayed ones. A step down is the other way round: dash.js appends
the lower quality *at the end*, "to preserve the higher quality media for as long as possible".
**[1P]** Ours already does that by not flushing.

### 3.3 Memory: the browser's limits are in bytes, and on weak devices they are small

Chromium's limits, read in its source (`media/base/demuxer_memory_limit*.cc`, main, 2026-10-29):
**[1P]**

| Where | Video | Audio |
| --- | --- | --- |
| Desktop (Linux, Windows, macOS) | 150 MiB | 12 MiB |
| Desktop marked low-end | 30 MiB | 2 MiB |
| Android, ordinary | 150 MiB | 12 MiB |
| Android, "partial low-end" (3–6 GB, a feature flag) | 80 MiB | 5 MiB |
| **Android, low-end: ≤ 1024 MB of RAM** (`SysUtils.LOW_MEMORY_DEVICE_THRESHOLD_MB`) | **30 MiB** | **2 MiB** |
| Android, ≤ 512 MB | 15 MiB | 1 MiB |
| **Chromecast (Cast build), MSE, H.264** | **30 MiB** | 2 MiB (stereo AAC); 5 MiB (AC-3, 5.1) |
| Chromecast, VP9 / HEVC | 80 MiB | as above |

Other engines: Firefox about 100 MiB video and 30 MiB audio (Mozilla bug 1760529) **[S]**; WebKit
on iOS about 105 MB per SourceBuffer, enough for "approximately five minutes of 1080p" by WebKit's
own reckoning **[S]**.

**What happens at the limit** (Chromium `SourceBufferStream::GarbageCollectIfNeeded`) **[1P]**:
1. Chromium frees what has been played, up to the playhead.
2. Then it frees from the far end, back to the newest append.
3. If that is not enough, the append fails with `QuotaExceededError`.

hls.js 1.7.3 then:
1. Frees its own back buffer and retries.
2. If that fails, reports `BUFFER_FULL_ERROR`.
3. Lowers `maxMaxBufferLength` to what is loaded less three fragments, but not below half of it
   (`reduceMaxBufferLength`).
4. Throws the fragment away and loads it again. **[1P]**

So a buffer larger than the limit costs a wasted download per overflow and a lowered cap, which
our runway restore may undo (§1.3).

**The Fire TV Stick.** The owner's stick is an AFTSSS: Fire TV Stick 3rd gen, **1 GB RAM**, Fire
OS 7 on Android 9, Silk 138 ([Amazon device
specifications](https://developer.amazon.com/docs/device-specs/device-specifications-fire-tv-streaming-media-player.html)).
**[S]** If Silk follows Chromium's Android code, it is low-end, and gets **30 MiB of video and
2 MiB of sound**. **[I]** Worked through:
- **The picture.** At 9 Mb/s, 30 MiB is 28 s in all, behind and ahead together. Our 30 s kept
  behind plus 53 s ahead would be about 93 MB, three times the limit.
- **The sound.** 2 MiB of our 96 kb/s AAC is 175 s, so that is fine. 2 MiB of a copied 640 kb/s
  AC-3 is 26 s, which is not.

But staging's VQ116 log has the stick at **37.3 s loaded ahead** of the film's own picture. If
that picture averaged its declared 9 Mb/s, that is about 42 MB, over 30 MiB. **[1P]** for the
log, **[I]** for the sum. So either Silk does not take the low-end limit, or the picture averaged
less. A test settles it (§5.7, T4). The keptBehindS comment ("about 150 MB of video" on a 1 GB
stick) assumes the desktop limit. Until T4, that is not shown.

**Chromecast** (the lab's receiver runs `playChunked`, `apps/cast-receiver/src/lab.ts`) **[1P]**:
the Cast build's 30 MiB for H.264 is in the source, not inferred. At 9 Mb/s, the receiver can hold
about 28 s in all. Our 30 s behind would take half of that. **[I]**

### 3.4 Seconds or bytes

- **The target belongs in seconds.** Everything that reads it is in seconds: the step-up's 20 s,
  the quality hold's 20 s, the stall margin, a group's runway. A byte target makes a low quality
  load far ahead, which is the fault we hit. A seconds target is what Shaka, dash.js, Media3 and
  Puffer use. **[I]**
- **The cap belongs in bytes.** The browser's limit is in bytes (§3.3), and so is Media3's cap.
  hls.js uses bytes for the right reason (§1.1) but in the wrong direction, as a floor.
- **Combine them as Media3 does:** load to the seconds target unless the bytes say stop. **[I]**

### 3.5 Wasted bytes when people seek or leave

Everything loaded ahead of a seek or a departure is waste. Earlier measurements put **25–39%** of
YouTube's bytes as wasted this way on PCs, and up to 48% on phones (summarised in a 2022
short-video paper, arXiv 2206.09839). **[S]** For us, waste is linear in the target. Leaving 60 s
ahead of the top wastes up to about 60 MB; 30 s of 360p, about 2 MB. **[I]** Seeking within the
loaded range wastes nothing. A far seek wastes it all. A Watch Together jump too. Waste costs the
owner's upload, which the household shares, more than it costs the viewer.

### 3.6 Server load: on-demand encoding

The core encodes every quality except the copied top, on demand, 60 s past the player's request
(§1.3). So a deep buffer at a low quality is **encoder work spent at the wrong quality**. At 600 s,
the 360p run was ten minutes ahead. At 60 s, it is still up to two minutes ahead (buffer + look
ahead). When the player steps up, the remainder is either thrown away or kept for a later play
(D309 §8). **[I]** A shorter target below the top means less of this. A replacement (§4) to the
copied top costs no encode. A replacement to a middle quality starts a new run from the flush
point, whose first chunk takes `firstChunkMs` (logged by the node per run). **[1P]** That cost
decides P3's condition.

### 3.7 Should the cap depend on the link's steadiness, the quality, the LAN?

- **On the quality: yes.** dash.js's rule (§2) and ETH's reasoning both say so, and our problem is
  exactly the one it solves. **[1P]/[I]**
- **On the link's steadiness: not yet.** In BBA's world a swinging link earns more buffer. Media3
  keeps 50 s for everyone, and its experiment found the deeper drip helped. Ours already answers
  swings with longer holds (D352 §3: 30 s up to 240 s). That holds the quality steadier, not the
  buffer deeper. With P1, a swinging link at the top gets 60 s, and one below it gets 30 s, which
  already covers a dip of 30 s. Revisit if the stall logs show a playback below the top running
  dry at 30 s. **[I]**
- **On being on a LAN: no.** A LAN playback plays at the top, so P1 already gives it 60 s. It
  refills in seconds, so it needs less, but loading less buys it nothing either: the bytes are
  local, and the memory cap (P2) applies anyway. One more condition is one more thing to test.
  **[I]**
- **On the device's memory: yes, as a cap** (P2). Media3 does the same with its heap check.
  **[1P]**

---

## 4. Replacing loaded low quality on a step up

### 4.1 What the players offer

- **hls.js** has three setters (`docs/API.md`; `set currentLevel`, `nextLevel`, `loadLevel`,
  hls.mjs). **[1P]**
  - `currentLevel` flushes **everything** and reloads at the playhead. The docs say playback "will
    interrupt at least shortly to re-buffer".
  - `nextLevel` runs `nextLevelSwitch`. That estimates how long the next fragment takes to fetch:
    1 s + TTFB + its size at the estimate. It keeps what is loaded up to the fragment playing
    then. It flushes from a quarter to half into the following fragment, to the end.
  - `loadLevel` changes only what loads next.

  `nextLevel` and `currentLevel` set `manualLevel`, which turns hls.js's automatic choice off, so
  our controller cannot use them as they are. hls.js itself calls `nextLevelSwitch` when its cap
  rises for a larger player or for dropped frames. Our code steps up through `nextAutoLevel`, the
  `loadLevel` behaviour. **[1P]** The public `BUFFER_FLUSHING` event (`{startOffset, endOffset,
  type}`) flushes a range without touching the choice. `audio-shift.ts` already uses it for the
  sound. **[1P]**
- **dash.js** "fast switching" (`fastSwitchEnabled`, null = on for everything except low
  latency): "after an ABR up-switch in quality, instead of requesting and appending the next
  fragment at the end of the current buffer range it is requested and appended closer to the
  current time … The maximum time to render a higher quality is current time + (1.5 * fragment
  duration)." It does not do this when less than one fragment is loaded, or after an abandoned
  request. A down-switch appends at the end. **[1P]**
- **Shaka** `clearBufferSwitch` plus `safeMarginSwitch`, off by default. **[1P]**
- **YouTube** re-downloads (ETH, §2). **[1P]**

### 4.2 The evidence for it

Spiteri, Sitaraman, Sparacio, "From Theory to Practice", ACM TOMM 2019 (doi:10.1145/3336497),
measured dash.js's fast switching, replacing from 1.5 segments past the playhead, earliest deadline
first. They found:
- the better picture showed **about 50 s sooner**;
- **about 45% higher median bitrate** on traces where the link stepped up;
- **6% of bytes discarded**, median, on 4G traces.

**[1P]** (`notes-bola-tomm.txt`; testbed, **medium**.) ETH's 23% for Arte is the bad end
(§2). Both agree the cost is bytes, and it is largest where the re-download is eager.

### 4.3 When it is worth it for us

It helps when **a step up lands with a lot loaded below it**. With P1's 30 s below the top, the
step up always lands with 20–30 s loaded (the step-up needs 20). Without replacement, the
change shows 20–30 s later. With it, 15 s later, or about one chunk. **[I]** The gain is
10–15 s per step up: real, but much smaller than the 60 s cap's gain from P1 alone. So **P1
comes first**, and P3 is worth it if its conditions are cheap to state:

- **Bytes:** 15 s of 360p thrown away is about 1 MB; of 720p, about 6 MB. Small. **[I]**
- **Encoding:** a flush to a middle quality whose chunks are not made starts a run at the flush
  point. Its first chunk takes `firstChunkMs`, seconds on a software node. If that exceeds the
  15 s margin, the viewer stalls. **Replace only when the new quality is the copied top, or its
  chunk at the flush point is already made** (the core knows; the player could be told, or could
  ask with a HEAD). **[I]**
- **The link:** the step up already needs 1.5× headroom (`stepUpHeadroom`), so a 10 s chunk
  arrives in at most about 6.7 s. Add the wait for its first byte, now estimated separately
  (bandwidth-sampling P6). The margin must exceed twice that sum, as hls.js's own rule does with
  its 1 s safety. **[1P]/[I]**
- **Sound:** flush the picture only. A change of sound with the quality threw the loaded sound
  away and stalled the Fire TV (VQ116, D368). **[1P]**
- **Groups:** while a Watch Together group holds a runway, the "loaded ahead" the group reads would
  drop under it. Do not replace then. **[I]**
- **Keyframes:** our chunks cut on keyframes on a ten-second grid. Flush from a chunk boundary
  at or after playhead + 15 s, and hls.js's fragment tracker reloads from there. **[I]** Test it
  on Silk: dash.js's own docs list devices (WebKit embedded) where pruning the range being played
  stutters (`avoidCurrentTimeRangePruning`). Ours never prunes that range. **[1P]**

---

## 5. Recommendation for Orpheum: proposals for the owner

### 5.1 P1: a seconds target that depends on the quality

- **Below the top quality: 30 s.** The step-up needs 20 s loaded (`stepUpBufferMs`), and the next
  chunk needs room to load: one 10 s chunk, so 20 + 10 = 30. Less, and the step-up could never
  open. More only delays the change on screen. dash.js uses 18 s with its BOLA from 12 s. We use
  more because our gate is 20. **[I]**
- **At the top: 60 s**, the cap the lead set: dash.js's long-form 60 s, and Media3's 50 s.
  Nothing better can come, so loading ahead is pure insurance, bounded by P2. **[1P]/[I]**
- **Built how:** set `maxBufferLength = maxMaxBufferLength` to the target, and leave
  `maxBufferSize`'s floor below it (for example, `maxBufferSize: 0`), so the bytes never stretch it
  again. Change both on `LEVEL_SWITCHING`, as `keepRunwayWhenTold` already changes them live. Keep
  `keepRunwayWhenTold` taking the lower of its runway and P1's target. Make it restore P1's target
  rather than the values it captured at attach. **[I]**
- **Its cost:** a playback below the top has 30 s of cover instead of 60. Its stalls are the
  number to watch (T2).

### 5.2 P2: a byte cap from the device

Forward bytes = **at most about a third of the browser's video limit**, so the limit also covers
the 30 s kept behind and a chunk being appended. Never below **20 s** (the hold): if the bytes say
less, the device cannot hold the top at that rate, which is a ladder question, not a buffer one.
**[I]** In numbers:

| Device | Limit | Third | 9 Mb/s ahead | 20 Mb/s ahead |
| --- | --- | --- | --- | --- |
| desktop Chrome, Android ordinary | 150 MiB | 52 MB | 46 s | 21 s |
| Firefox | ≈ 100 MiB | 35 MB | 31 s | 20 s (floor) |
| ≤ 1 GB Android (if Silk applies it), Chromecast H.264 | 30 MiB | 10 MB | 20 s (floor) | 20 s (floor) |

How the player knows:
- **Most reliable:** count `QuotaExceededError` and lower the cap for the rest of the session.
  hls.js already does this. P2's job is not to undo it (§1.3).
- **Ahead of time:** `navigator.deviceMemory`, which reports in buckets: 1 GB or less suggests
  the low-end limit. The Cast receiver can assume 30 MiB from the source.

T4 decides which of these the Fire TV needs. **[G]** until T4.

**Also lower `keptBehindS` where the limit is 30 MiB:** 30 s behind at 9 Mb/s is 34 MB, the whole
limit. Chromium frees what was played first, so the cost is a step back that reloads, not a
failure. Still, 10 s behind would keep one step back (`seekStepMs`). That is the owner's call. **[I]**

### 5.3 P3: replace loaded low quality on a step up, under conditions

On a step up, flush the **picture only**, from the first chunk boundary at or after **playhead +
15 s** to the end, with `BUFFER_FLUSHING`. Only when all four hold:
1. the new quality is the copied top, or its chunk at the flush point is made;
2. 15 s is at least twice the expected time for that chunk (its wait plus its bytes at the
   confident estimate);
3. no group runway is in force;
4. at least 15 s plus one chunk is loaded.

Expected: the change shows about 15 s after the step up, not 20–30 s. About 1–6 MB discarded per
step up. **[I]** Build it after P1, and only if T3 shows the remaining delay matters to the owner.

### 5.4 P4: the server's look-ahead stays at 60 s

`chunk_lookahead_seconds` is 60 s past what the player asks for. It exists so a player "never asks
for a chunk no run is making" (D247 §10). With P1, the player asks at most 30–60 s ahead, so the
encoder runs 90–120 s ahead at the top (copied, cheap) and 90 s below it. Halving it to 30 s
below the top would save encoder time at the cost of the "never asks" margin. Nothing seen yet
asks for that. **[I]**

### 5.5 P5: no LAN rule, no steadiness rule, for now

See §3.7. Both are one more condition with no failure seen that they would fix.

### 5.6 P6: what to log

Each line goes in the existing play reports or playback log.

- **`target`:** at each change, the seconds target, the byte cap, which bound (quality, bytes,
  runway, hls.js's own cut), the quality and its BANDWIDTH.
- **`quota`:** each `QuotaExceededError` and `BUFFER_FULL_ERROR`, with what was loaded behind and
  ahead in seconds and bytes, and hls.js's "Reduce max buffer length to …" with its new value.
- **`steppedUpShown`:** from the step up's decision to the first frame of the new quality
  (`LEVEL_SWITCHED`), in ms, with what was loaded at the decision. This is the number the owner
  saw as "minutes".
- **`replaced`:** for P3, the flush range, the bytes discarded, the time to the new quality's
  first frame, and whether the playhead got within 2 s of the flush point (a near-stall).
- **`deviceMemory`:** once per playback, `navigator.deviceMemory` and the user agent.
- **On leaving or seeking far:** the bytes loaded ahead that are being thrown away (the waste of
  §3.5).

### 5.7 Tests

Each runs against the logs above. The emulated links are D252's and D263's.

- **T1, the delay (staging, headless Chromium).** Throttle to 0.6 Mb/s for 60 s, then lift to
  20 Mb/s. Passes with P1 when `steppedUpShown` is at most hold + 30 s + one chunk: ≤ 70 s at
  steadiness 0. Today's 60 s cap reads ≤ 100 s; hls.js's default read about 450 s. Red if P1 is
  reverted.
- **T2, the cover (emulated).** A link that drops to zero for 20 s every 2 minutes, with the
  playback held below the top. Passes when there is no stall. A 40 s drop is expected to stall at
  P1's 30 s, so record it as the cost, not a failure.
- **T3, replacement (P3, staging).** As T1, but stepping up to the copied top. Passes when
  `steppedUpShown` ≤ hold + 20 s, `replaced` shows ≤ 6 MB discarded, and there is no near-stall.
  Then the same to a middle quality with its chunks not made: passes when P3 declines to replace.
- **T4, the Fire TV's limit (lab, Silk on the AFTSSS).** Play the film's own picture of a film at
  ≥ 9 Mb/s with the cap lifted to 120 s for the test. Log `quota`, `deviceMemory`, and the
  bytes held when the first QuotaExceededError comes. That byte figure is the stick's limit:
  about 31 MB means Chromium's low-end path, 157 MB the ordinary one. The same on the Chromecast
  receiver (`apps/cast-receiver`) checks the Cast build's 30 MiB.
- **T5, nothing breaks (Chrome, Firefox, Safari, Fire TV).** Two minutes of a film at the top,
  with a step back and a far seek. Passes when there is no `quota` line and no stall, and when
  the bytes held stay under P2's cap plus one chunk.

---

## 6. What the evidence does not settle

- **The best seconds below the top.** dash.js says 18, ETH's RobustMPC sat at 20, ours derives 30
  from our own gate. No study compares 20 with 30 with 60 *without* re-downloads on links like
  ours. T2 is the measurement.
- **Silk's memory limit** (§3.3). The source and a staging log disagree. T4.
- **Whether drip beats burst for us.** Media3 found drip (min = max) cut stalls. hls.js already
  drips (it loads whenever below the target). Our WebRTC path's idle decay (abr-adaptation §2)
  says drip is right. Not measured here.
- **Netflix's and YouTube's current values.** Both published numbers are old (2014) or measured
  from outside (2019). Neither is a design statement.
- **The waste figures** (§3.5) come from short-video and YouTube studies through a summary.
  Household film watching may leave far less often mid-film.

---

## Sources

Papers, with strength:

- Licciardello, Grüner, Singla (ETH Zürich), "Understanding video streaming algorithms in the
  wild", PAM 2020, https://arxiv.org/abs/2001.02951 (read via
  https://ar5iv.labs.arxiv.org/html/2001.02951). Ten platforms, emulated traces: **strong** for
  what they do, **medium** for why.
- Huang, Johari, McKeown, Trunnell, Watson, "A Buffer-Based Approach to Rate Adaptation: Evidence
  from a Large Video Streaming Service", ACM SIGCOMM 2014, doi:10.1145/2619239.2626296,
  https://yuba.stanford.edu/~huangty/sigc040-huang.pdf. Netflix production: **strong**, for 2014.
- Spiteri, Sitaraman, Sparacio, "From Theory to Practice: Improving Bitrate Adaptation in the DASH
  Reference Player", ACM TOMM 15(2s), 2019, doi:10.1145/3336497. Testbed: **medium**.
- Yan et al., "Learning in situ: a randomized experiment in video streaming", USENIX NSDI 2020,
  https://www.usenix.org/system/files/nsdi20-paper-yan.pdf. **Strong**, for its 15 s live
  setting.
- Krishnan, Sitaraman, IMC 2012; Balachandran et al., SIGCOMM 2013; Huang et al., IMC 2012; Yin et
  al., SIGCOMM 2015; Spang et al., SIGCOMM 2023: as cited in
  [abr-adaptation-2026-10-27.md](abr-adaptation-2026-10-27.md).
- "Bandwidth-Efficient Multi-video Prefetching for Short Video Streaming", arXiv 2022,
  https://arxiv.org/abs/2206.09839, for the 25–39% / 48% waste figures it cites. **Weak** (a
  summary of a citation).

Players' code and documentation (**strong** for what each does, not for whether it is right):

- hls.js 1.7.3 `hls.mjs` (installed) and `docs/API.md`, https://github.com/video-dev/hls.js;
  Rob Walch on the buffer settings, 2021, https://github.com/video-dev/hls.js/issues/4003 (**[S]**).
- Shaka Player `lib/util/player_configuration.js` and `externs/shaka/player.js`, main,
  https://github.com/shaka-project/shaka-player.
- dash.js `src/core/Settings.js`, development, https://github.com/Dash-Industry-Forum/dash.js.
- Media3 `DefaultLoadControl.java` and `RELEASENOTES.md`, release branch,
  https://github.com/androidx/media; ExoPlayer `RELEASENOTES.md`, release-v2,
  https://github.com/google/ExoPlayer.
- Tamas-Selicean, Greve, "Enhancing video streaming quality for ExoPlayer, Part 2", Akamai blog,
  2019-10-23,
  https://www.akamai.com/blog/performance/enhancing-video-streaming-quality-for-exoplayer-part-2-exoplayers-buffering-strategy-how-to-lower.
  **Medium** (reports Google's experiment, not its data).
- Chromium `media/base/demuxer_memory_limit.h`, `demuxer_memory_limit_default.cc`,
  `demuxer_memory_limit_android.cc`, `demuxer_memory_limit_cast.cc`,
  `media/filters/source_buffer_stream.cc`, `base/android/java/src/org/chromium/base/SysUtils.java`,
  `base/system/sys_info.cc`, main, 2026-10-29, https://github.com/chromium/chromium. **Strong**
  for Chrome; for Silk, only as far as Silk keeps Chromium's defaults.
- Mozilla bug 1760529 (Firefox's 100 MiB), https://bugzilla.mozilla.org/show_bug.cgi?id=1760529.
  **[S]**, **medium**.
- WebKit's iOS SourceBuffer limit, via https://github.com/alexeychikk/webtor/pull/28. **[S]**,
  **weak**.
- Apple, `AVPlayerItem.preferredForwardBufferDuration`, via
  https://developer.apple.com/forums/thread/63435. **[S]**, **weak**.
- Jellyfin web PR #7455 (open), https://github.com/jellyfin/jellyfin-web/pull/7455. **[S]**,
  **weak** (a proposal, not shipped).
- Amazon, "Device Specifications: Fire TV Streaming Media Player",
  https://developer.amazon.com/docs/device-specs/device-specifications-fire-tv-streaming-media-player.html
  (AFTSSS: 1 GB). **[S]**, **strong** source.
