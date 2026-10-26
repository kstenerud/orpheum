> **ABOUTME:** Which playback problems on unusual devices and browsers are worth a Diagnostics
> page's automatic checks and guided tests, ranked by how often each really happens; what today's
> media servers ask a user for when one is reported; which of it a browser can already tell by
> itself; and what a single synthetic test clip should add, in what order, versus what needs its
> own separate file. Read before designing the Diagnostics page or the synthetic clip it plays.

# Playback diagnostics: what to test, and why

- **Date:** 2026-10-26. Requested by the owner: a user-facing Diagnostics page, usable on any
  device (smart-TV browsers, Fire TV Silk, game consoles, Android TV boxes, old iPads, Safari,
  Chromecast), running automatic checks and guided visual/audible tests against a built-in
  synthetic clip, ending in a report to the household's admin.
- **Builds on:** [youtube-tv-browser-av-sync-2026-10-24.md](youtube-tv-browser-av-sync-2026-10-24.md)
  (what a browser can and cannot tell about its own output latency), the idiosyncrasies entry
  [A TCL TV in TV Bro plays a page's sound about half a second
  late](../../idiosyncrasies.md#a-tcl-tv-in-tv-bro-plays-a-pages-sound-about-half-a-second-late-and-the-browser-cannot-tell),
  [agent-failures A43](../../agent-failures.md) (a periodic test signal is ambiguous within its
  own period — the reason the owner's current clip design uses four colours, four rising tones, a
  counter and a filling clock instead of a once-a-second flash), and
  [device-testing.md](device-testing.md) (Jitsi's lip-sync check — a green flash with a beep — and
  the industry habit of having the device report on itself rather than watching it). Adjacent
  plans not answered here but relevant to building the page: [device-lab.md](../plans/device-lab.md)
  (the device-report pattern and its pass numbers), [bug-reports.md](../plans/bug-reports.md) (the
  pseudonymised report format a Diagnostics page's "send to the admin" button should reuse, not
  reinvent), and [playback-survey.md](../plans/playback-survey.md) (`canDisplayType`,
  `MediaSource.isTypeSupported` already used there for Chromecast's own direct-play survey).
- **Cache:** `/yoloai/cache/research-diagnostics/` (empty — every source below was read through
  `WebSearch`'s summaries or fetched live with `WebFetch`; nothing was large enough, or re-fetched
  often enough, to need caching, and the GitHub search API's counts change with every search, so a
  cached copy would mislead rather than help).
- **Labels:**
  - **[1P]** read at the source: a vendor's own documentation, or a search queried directly
    against GitHub's API (not summarised).
  - **[S]** a search engine's summary of a forum thread, GitHub issue or article, not read in
    full. Most of §1 and §3 is this label; treat the counts as a rough proxy, not a census.
  - **[I]** my inference.
- **Not reached:** Plex's and Emby's own crash/telemetry dashboards (private); any single
  authoritative ranking of playback complaints by volume (none of the four projects publishes
  one); Fire TV Silk-specific or games-console-specific forums (searched for, found little beyond
  the general Android/Chromecast material already covered); a working build or measurement of any
  of §4's proposed clip — this is a research pass only, no code or clip was written.

---

## 1. Summary: ranked by how often it is real, and how solid that signal is

No project publishes a ranked list, so this rank is built from the weight of forum and issue
volume across Plex, Jellyfin, Kodi (xbmc) and Emby, judged against how well each is suited to a
browser-run synthetic-clip test. GitHub counts are keyword searches against one repo's full issue
history, in the thousands to tens of thousands of total issues — a crude proxy for "comes up a
lot", not a clean rate. **[S]** throughout unless marked **[1P]** (the GitHub counts themselves,
queried directly).

| Rank | Problem group | Frequency signal | How solid |
| --- | --- | --- | --- |
| 1 | **A/V sync (lip sync)** | Recurring on every project's forums; Orpheum hit it directly on a real TV (A43, the YouTube research doc). `xbmc/xbmc` issues matching "out of sync": **80** [1P]. `jellyfin-web` issues matching "sync": **246** [1P] (noisy — includes non-AV meanings of "sync"). | Strong. Also the best-understood group: we already know which parts a clip can and cannot resolve (§2, §4). |
| 2 | **Stalls / buffering / freezes** | The single most common WebOS/Tizen complaint found: "constantly buffering", freezes every 3–5 minutes, high-bitrate transcodes freezing the server. `xbmc/xbmc` "dropped frames"/judder: **94** [1P]. | Strong, but mostly a network/hardware/server symptom, not something a short local clip reproduces well (§4 notes this as a separate, longer-running check). |
| 3 | **Codec/container unsupported (won't direct-play)** | The reason dedicated tools exist outside the servers themselves (`hwatest`, `jellyprobe`, the jell.yfish.us bitrate-test files); a Jellyfin moderator's standard ask is "post your ffprobe/mediainfo output" because the container and codec names alone don't say enough. | Strong, and the most automatable of all twelve groups (MediaCapabilities, §2). |
| 4 | **Audio: no sound / 5.1 downmix eats dialogue / passthrough** | Emby's own forum names this precisely: users hear "sirens, gunshots, birds" but no dialogue, because the centre channel never reaches two speakers. Reported on Android TV and Fire TV specifically. | Strong and high-impact (unwatchable, not just imperfect), but entirely a listening test — no web API confirms a speaker played anything (§2). |
| 5 | **Aspect ratio / stretched or cropped picture** | Multiple fresh (2024–2026) Plex forum threads: a rewritten Android app stretching everything vertically, a "Zoom" mode behaving like "Stretch", letterboxing that drops its bars partway through playback. | Strong, recent, and the easiest of all groups to turn into one unambiguous visual (a circle is round, or it isn't; §3, §4). |
| 6 | **HDR washed out / tone mapping** | A specific, well-described failure on Android's MediaCodec Surface path: tone mapping happens inside a vendor black box Kodi cannot see or influence, reported as "washed out", "bleak", "overexposed" on Android TV / Fire TV sticks (`xbmc/xbmc#20695`). | Strong where it applies, but narrower: only HDR-mastered content on HDR-capable displays, and the browser cannot see or correct the device's own tone-mapping path either (§2, §4). |
| 7 | **Colour range crushed blacks / washed-out greys (limited vs. full range mismatch)** | A generic, extremely common HDMI-chain complaint (TV's black level setting vs. source's RGB range) — well documented for consoles and PCs; not found as an Orpheum- or media-server-specific thread, because it is almost always a display/output setting, not the app. | Moderate: common in general, but the fix is usually outside any app's reach. Still worth one PLUGE-style check, since a self-hosted project cannot phone a TV's support line. |
| 8 | **Subtitle timing / rendering** | Recurring Jellyfin complaint, concentrated around transcoding (`.srt` not burned in) and around seek/resume (desync after a skip). GitHub "subtitle"+"sync" in titles only: **2** [1P] (title-only search undercounts; body text has far more). | Moderate. Specific and fixable causes (seek, transcode) rather than one universal failure. |
| 9 | **Seeking problems** | `jellyfin/jellyfin` "seek" in title: **41** [1P]. dash.js's own functional-test bar (a seek must land within 0.5 s) implies the industry treats this as real but secondary. | Moderate, and the single most automatable group after codec support — the browser can check this against itself with no person needed (§2). |
| 10 | **Frame-rate judder / dropped frames** | Well-understood TV behaviour (3:2 pulldown mismatching 24p content to a 60 Hz panel), not usually described as a bug; `getVideoPlaybackQuality` numbers exist for every player (Media3's ≤1%/≤10-consecutive bar, device-testing.md §2). | Moderate signal, but mostly a TV/display property the project cannot fix — worth reporting, not worth building much test around. |
| 11 | **Casting (Chromecast) disconnects / black screen** | Common across every cast-capable app (Spotify, YouTube, Netflix, Plex), not specific to self-hosted servers; "cuts out every 10–15 minutes", black screen with sound continuing. | Moderate but diffuse — mostly Wi-Fi and receiver-app issues outside a web page's reach; `playback-survey.md` already uses the receiver's own `canDisplayType`/`isTypeSupported` for the one part that is reachable. |
| 12 | **Remote/controller input** | `xbmc/xbmc` "remote" + "not working" in title: **36** [1P]. Little Plex/Jellyfin/Emby-specific signal found; most complaints are IR/CEC/pairing, one layer below a web page. | Weaker signal specific to a browser page, but still necessary for a usable TV Diagnostics page — covered as a guided test (§2, §4), not ranked on complaint volume. |

**Read as:** groups 1–5 are where most of the user-visible pain actually is, well attested across
projects, and each maps to a distinct part of one clip. 6–7 (HDR, colour range) are real but
narrower and largely outside the project's own reach — worth one pattern each, not much
engineering. 8–10 are secondary papercuts with their own fixable causes. 11–12 matter for
usability but are thin on complaint evidence specific to a self-hosted media server; build them
because a TV Diagnostics page needs them, not because the forums demand them.

---

## 2. How each is diagnosed today, and what a browser can tell by itself

### 2.1 What support people ask for today

- **Plex:** reproduce the problem, then *Settings → About → Download Logs*; separate article per
  platform (Roku, Apple TV, desktop) for where the log lives. **[S]**
- **Jellyfin:** a moderator's standard reply is "recreate the issue, then post your Jellyfin and
  ffmpeg logs (Dashboard → Logs) to a pastebin, plus `mediainfo` or `ffprobe` for the file" —
  explicitly because "the container and codec don't tell the whole issue." **[S]**
- **Common to both, read across many threads:** device/app model and version, direct-play vs.
  transcode, container/codec/profile/level, audio channel layout, network type (wired vs. Wi-Fi),
  and — for sync specifically — which output stage the sound takes (HDMI passthrough, TV speakers,
  a separate soundbar/AVR). **[S]/[I]**
- Orpheum already has a design for the *shape* of a report: [bug-reports.md](../plans/bug-reports.md)
  keeps exactly this list (container, codecs, profiles, channel layouts, track counts) while
  dropping titles, paths and usernames. A Diagnostics page's "send to the admin" step is this same
  pseudonymised report, scoped to one playback attempt, not a new mechanism. **[I]**

### 2.2 What the browser can already tell, with no person needed

| Signal | API | What it reports | Caveat |
| --- | --- | --- | --- |
| Will this codec/container/profile decode at all, smoothly, power-efficiently | `MediaCapabilities.decodingInfo()` — `supported`/`smooth`/`powerEfficient` **[1P]** | A real answer, not a guess, once the device has played enough to have stats | Chrome/Edge/Firefox/WebView since 2020; `smooth`/`powerEfficient` default true until the device has recorded playback stats **[S]** |
| Will MSE accept this mime+codecs string | `MediaSource.isTypeSupported()` **[I]**, `HTMLMediaElement.canPlayType()` **[I]** | Yes/no/maybe, no quality signal | Already Orpheum's own tool for Chromecast in `playback-survey.md` |
| Frames actually dropped | `HTMLVideoElement.getVideoPlaybackQuality()` — `droppedVideoFrames`/`totalVideoFrames`/`corruptedVideoFrames` **[1P]** | A number, comparable to Media3's ≤1%/≤10-consecutive bar (device-testing.md) | Says nothing about *why* (network, decode, compositor) |
| Exactly when a frame was composited, and what media time it carries | `requestVideoFrameCallback()` — `mediaTime`, `presentationTime`, `expectedDisplayTime`, `processingDuration` **[1P]** | Lets the page correlate its own clock to what it showed, within the page — this is how the YouTube research doc could prove our chunks agree within ~20 ms | Says nothing about when the *speaker* played the matching sound, or when the *panel* actually lit the pixel (idiosyncrasies: 16 ms to compositor, then an unmeasured TV-side delay) |
| Does this display claim HDR | `matchMedia('(dynamic-range: high)')`, `matchMedia('(video-dynamic-range: high)')` **[1P]** | A claim from the OS/browser, not a confirmation that tone mapping looks right | Supported Chrome 98+, Firefox 100+, Safari 13.1+ **[S]** |
| A gamepad/handset is present and which buttons it has | `navigator.getGamepads()`, `gamepadconnected`/`gamepaddisconnected` **[1P]** | Confirms a controller is recognised at all | A pad already connected before the page loads stays invisible until a button is pressed; most TV remotes arrive as synthetic `keydown` events, not Gamepad objects, so both paths are needed **[S]** |
| Audio context's own output latency | `AudioContext.baseLatency`/`outputLatency`/`getOutputTimestamp()` **[1P]** (already used in `gain.ts`) | Only ever describes *that context's* path | Exactly the trap A43/the YouTube doc found: an element's sound that never goes through Web Audio has no equivalent API at all — the element's own sink latency is simply unknown to the page |
| A seek landed where asked | compare the value passed to `currentTime =` against the time read after the `seeked` event fires **[I]** | Exact, no ambiguity | None found |
| Cast receiver is reachable, and what it can decode | the Cast Web Sender SDK / `RemoteDisplay`, and the receiver's own `canDisplayType`/`isTypeSupported` (`playback-survey.md`) **[1P]** | Confirms a receiver exists and claims support | Says nothing about whether the receiver actually renders it correctly |

### 2.3 What only a person can confirm

- **A/V lag's existence and direction**, beyond what the page's own clock says — the whole reason
  the owner's clip design exists (A43: the instrument itself can lie).
- **Whether a speaker actually produced sound**, and on which channel — no API confirms sound
  left a transducer; `AudioContext.state` and track presence only confirm the page tried.
- **HDR tone mapping and colour range**, which happen in the TV's own pipeline, invisible to any
  page (§2.2's `dynamic-range` query only says the OS *claims* HDR capability, never whether the
  rendered picture looks right).
- **Overscan/aspect ratio**, which is the TV cropping or stretching a correctly-rendered frame
  after it leaves the browser entirely.
- **Subtitle rendering correctness** (position, font fallback, colour) — cue timing can be checked
  automatically (§2.2's seek logic extends to this), but what it looks like cannot.
- **Remote/controller usability** — that a button *press* register is automatic; that the
  highlighted control is where the person expects it, and that D-pad focus order makes sense, is
  not.

---

## 3. Existing test clips and patterns, and what is worth copying

| Pattern | What it does | Good for | Usable by a non-expert filming a TV with a phone |
| --- | --- | --- | --- |
| **Sync-One2** (hardware tool, Harkwood Services) | A clip of light/dark screen changes paired with a beep at each change; the device's microphone and light sensor measure the gap directly, in ms, signed (sound ahead or behind). **[S]** | The direct ancestor of Orpheum's own design — same idea, but the current clip replaces the "every second" period (ambiguous beyond half a period — A43) with distinct colours/tones over a 4 s cycle. | The *hardware* readout is not phone-usable, but the *clip* (flash + beep) is exactly what a phone can film and a person can judge by eye/ear, which is what Orpheum already does. |
| **Jitsi's lip-sync check** (green flash + beep) | The same pattern, used to self-test a video call's own sync. **[S]**, via device-testing.md | Confirms the pattern is already the industry's cheap default for exactly this problem, not an invention. | Yes — this is the simplest possible version of what Orpheum's clip already does. |
| **Blur Busters TestUFO "frame skipping"** | A fast-moving box; a long-exposure (≥1/10 s) photo turns missed refreshes into visible gaps in the motion blur. **[S]** | Makes a dropped-frame/refresh problem *provable with a photo*, not just a feeling — a good idea for an optional advanced test. | Needs a deliberate long exposure, which a phone's default camera app does not do automatically — usable, but needs an instruction, not a glance. |
| **AVS HD 709 / Spears & Munsil** (calibration Blu-rays) | PLUGE bars (near-black steps, "can you see bar 17 but not 16?"), 100 %/0 % white and colour bars, overscan/safe-area markers, sharpness patterns, RGB-vs-YCbCr comparison. **[S]** | The PLUGE idea — a simple "can you distinguish this bar from pure black/white?" question — is exactly the right shape for a non-expert; overscan markers are a similarly simple "can you see all four corners?" check. | Yes for PLUGE and overscan markers (a yes/no question about a static frame); not for the full calibration suite, which assumes a controlled room and a meter. |
| **Channel-identification test files** (Fraunhofer multichannel samples, and the generic pattern seen across "surround sound test" sites) | Each channel speaks its own name ("front left", "centre", "side right"...) in turn. **[S]** | The single best non-expert test in this whole survey: no equipment, no visual judgement, just "did you hear the word 'centre' come from the middle?" — directly catches the Emby-forum failure (ranked group 4) where the centre channel silently vanishes. | Yes, with no caveats — this is the pattern most worth copying outright. |
| **Jellyfin's own test files** (`jell.yfish.us`, `hwatest`, `jellyprobe`) | Fixed H.264/H.265 clips at known bitrates, and automated profile-probing tools. **[S]** | Confirms codec/container support is treated industry-wide as a solved, automatable problem (matches §2.2) — not a pattern for the clip itself, since it is a battery of separate files, not one clip. | N/A — these are machine-read, not watched. |

**Read as:** the spoken channel-ID pattern and the PLUGE/overscan-marker idea are the two clearest
additions a non-expert can use without instruction; TestUFO's long-exposure trick is worth offering
as an optional "advanced" step, not a default one, because it needs a deliberate camera setting the
others do not.

---

## 4. What one synthetic clip should add, and what should be a separate variant

**Everything below is inference** — proposing how to extend the owner's existing design (per-second
background colour + a circle filling clockwise once a second, a beep at each second start, four
rising tones over a 4 s cycle so a lag up to ±2 s reads unambiguously, a centre seconds counter, a
note symbol rising one step per tone) to cover the next several problem groups from §1, in one
deterministic FFmpeg-filter clip (`geq`, `aevalsrc`, `drawtext`, `lavfi`-generated, no footage).

### 4.1 Sections to add to the one clip, in order

1. **A short PLUGE-style leader, first, while attention is freshest.** Near-black steps the
   viewer is asked "can you still tell this bar from pure black?", and a 100 %/0 % white step for
   the matching clipping question. Static, no sound needed, filmable or just looked at. Covers
   ranked group 7 (colour range) and contributes to group 6 (HDR) by pairing the question with
   whatever `matchMedia('(dynamic-range: high)')` already told the page — if the OS claims HDR but
   the bars still look crushed, that gap is the report.
2. **The existing sync segment** (colour cycle, filling circle, beep, four rising tones, counter,
   note symbol), unchanged in its own timing logic, run for several 4 s cycles — group 1. While it
   plays, the page logs its own `requestVideoFrameCallback` timestamps and `mediaTime` alongside
   what the person reports seeing/hearing; the *gap between the two* (as the YouTube research
   found: our own chunks agreed within ~20 ms while the TV's output differed by ~500 ms) is itself
   diagnostic of whether the lag is in the page or downstream of it — worth recording, not just
   the person's raw report.
3. **A spoken channel-identification segment**, immediately after — short, one word per channel in
   turn through the full layout the file carries. The single highest-value non-expert test found
   (§3); directly covers group 4 (no sound / downmix eats dialogue) with no ambiguity about what a
   "pass" sounds like.
4. **A brief moving-object segment** (a bar or UFO-style shape sweeping the frame for a few
   seconds) to let judder/stutter be seen, with `getVideoPlaybackQuality` logged underneath for the
   automatic half of group 10 — kept short, since most of what it reveals (3:2 pulldown on a 24p
   file over a 60 Hz panel) is the TV's own known behaviour, not a bug to chase.
5. **An aspect-ratio/overscan grid**, near the end: a full-frame grid with corner markers and a
   circle. "Are all four corners visible? Is the circle round?" catches both overscan (group 7's
   sibling, cropping) and stretching (group 5) in one static frame, and is the easiest section in
   the whole clip to photograph, since it does not need to be caught mid-motion.
6. **Subtitles rendered throughout, tied to the existing counter** (e.g. a cue reading "subtitle
   7" exactly at second 7 of each cycle), rather than a separate section — costs no extra clip
   time, and lets a misalignment be read directly against the clock already on screen (group 8).
7. **A short interactive close**, last: "press OK/select now" for the guided remote/controller
   test (group 12, via Gamepad events and a `keydown` listener for D-pad-as-keys), and an on-page
   seek target the viewer is asked to jump to, which the page checks automatically against the
   `seeked` event (group 9) — no person needs to judge this one, just trigger it.

### 4.2 What should be a separate clip (variant) instead, and why

These need to be *properties of the file itself*, not an overlay inside one SDR/H.264 clip:

- **Codec/container variants** (H.264+AAC, H.265/HEVC, AV1, VP9+Opus, in MP4/WebM/MKV as
  relevant) — the point of the test is whether *that* codec and container play at all; a single
  clip cannot test a codec it is not encoded in. Pairs with `MediaCapabilities.decodingInfo()`
  (§2.2) run first, so a device that already reports "not supported" need not download the file
  to confirm it.
- **HDR variants** (HDR10/PQ, HLG, plain SDR) — correct tone mapping depends on the file's actual
  mastering metadata, which cannot be faked inside an SDR container; needs its own properly
  tagged file per the `zscale`/`tonemap` pipeline found in §3.
- **Frame-rate variants** (23.976p, 25p, 29.97p, 50p, 60p) — judder depends on the *relationship*
  between the file's frame rate and the display's refresh rate, which only shows up if the content
  is genuinely at that rate.
- **Channel-layout variants** (5.1/7.1 vs. plain stereo) beyond what one file's channel-ID segment
  already demonstrates — useful if the admin wants to separately confirm that a *stereo-only* file
  still sounds right, isolating "this device mishandles 5.1" from "this device mishandles audio in
  general".
- **A longer, real-bitrate variant** for the stalls/buffering group (group 2) — that group is
  dominated by sustained network/decode behaviour over minutes, which a short deterministic clip
  cannot reproduce; this is better served by `playback-survey.md`'s existing direct-play/remux
  measurement than by anything in the Diagnostics clip itself.

---

## Sources

Read live via `WebFetch` against GitHub's search API (counts current as of 2026-10-26, and will
drift):
- [`jellyfin/jellyfin-web` issues matching "sync"](https://api.github.com/search/issues?q=repo:jellyfin/jellyfin-web+sync+in:title,body) — 246
- [`xbmc/xbmc` issues matching "tone mapping"](https://api.github.com/search/issues?q=repo:xbmc/xbmc+%22tone+mapping%22+in:title,body) — 67
- [`xbmc/xbmc` issues matching "out of sync"](https://api.github.com/search/issues?q=repo:xbmc/xbmc+%22out+of+sync%22+in:title,body) — 80
- [`jellyfin/jellyfin-web` issues matching "no sound"/"no audio" in title](https://api.github.com/search/issues?q=repo:jellyfin/jellyfin-web+%22no+sound%22+OR+%22no+audio%22+in:title) — 5
- [`jellyfin/jellyfin` issues matching "subtitle"+"sync" in title](https://api.github.com/search/issues?q=repo:jellyfin/jellyfin+%22subtitle%22+%22sync%22+in:title) — 2
- [`jellyfin/jellyfin` issues matching "seek" in title](https://api.github.com/search/issues?q=repo:jellyfin/jellyfin+%22seek%22+in:title) — 41
- [`jellyfin/jellyfin` issues matching "transcod"+"fail" in title](https://api.github.com/search/issues?q=repo:jellyfin/jellyfin+%22transcod%22+%22fail%22+in:title) — 0
- [`jellyfin/jellyfin` issues matching "not supported"+codec](https://api.github.com/search/issues?q=repo:jellyfin/jellyfin+%22not+supported%22+codec+in:title,body) — 94
- [`xbmc/xbmc` issues matching "dropped frames" or judder](https://api.github.com/search/issues?q=repo:xbmc/xbmc+%22dropped+frames%22+OR+judder+in:title,body) — 36
- [`xbmc/xbmc` issues matching "remote"+"not working" in title](https://api.github.com/search/issues?q=repo:xbmc/xbmc+%22remote%22+%22not+working%22+in:title) — 9

Forum, documentation and article sources (summarised via `WebSearch`, not read in full, unless
marked otherwise):

- [Jellyfin forum: WebOS/Tizen buffering and freezing threads](https://forum.jellyfin.org/t-jellyfin-really-slow-constantly-buffering-on-webos), [playback freezes every 3–5 minutes](https://forum.jellyfin.org/t-playback-freezes-for-5-10-15-seconds-every-3-5-minutes), [`jellyfin-webos#315`](https://github.com/jellyfin/jellyfin-webos/issues/315), [`jellyfin-tizen#289`](https://github.com/jellyfin/jellyfin-tizen/issues/289)
- [Kodi forum: HDR tone mapping and washed-out colour threads](https://forum.kodi.tv/showthread.php?tid=359129), [`xbmc/xbmc#20695` — washed-out HDR on Fire TV Stick 4K Max](https://github.com/xbmc/xbmc/issues/20695)
- [Emby community: 5.1/downmix and silent-dialogue threads](https://emby.media/community/topic/80967-no-dialogue-but-background-audio-plays-fine/), [downmix to stereo](https://emby.media/community/topic/132755-downmix-71-or-51-audio-to-stereo/)
- [Plex forum: audio/video sync threads](https://forums.plex.tv/t/plex-audio-video-out-of-sync-when-settings-video-and-audio-match-content-match-frame-rate-on/838484), [aspect ratio/stretching threads](https://forums.plex.tv/t/the-new-app-messes-up-the-aspect-ratio/941661), [anamorphic aspect ratio](https://forums.plex.tv/t/anamorphic-content-plays-with-wrong-aspect-ratio/915588), [buffering support article](https://support.plex.tv/articles/201575036-why-is-my-video-stream-buffering/)
- [Jellyfin forum: subtitle desync threads](https://forum.jellyfin.org/t-subtitles-out-of-sync), [`jellyfin/jellyfin#11825` — SRT desync while transcoding](https://github.com/jellyfin/jellyfin/issues/11825), [`jellyfin/jellyfin#2547` — subtitles out of sync on resume](https://github.com/jellyfin/jellyfin/issues/2547)
- [Google Nest/Chromecast community: black-screen and disconnect threads](https://www.googlenestcommunity.com/t5/Streaming/Chromecast-broadcasting-black-screen-intermittently/td-p/444129)
- [RTINGS: Stutter vs. Judder vs. Micro-Judder](https://www.rtings.com/tv/learn/research/motion-cadence) — 3:2 pulldown explained
- [Harkwood Sync-One2 support: test-file design](https://sync-one2.harkwood.co.uk/support/test-files/) — beep-on-flash pattern, signed ms readout
- [Blur Busters: TestUFO frame-skipping test](https://testufo.com/frameskipping) and [forum explainer](https://forums.blurbusters.com/viewtopic.php?t=3450) — long-exposure photo proof of dropped refreshes
- [`w6rz.net` AVS HD 709 patterns manual (PDF)](https://w6rz.net/avshd709/Patterns-Manual.pdf) — PLUGE, overscan/safe-area markers
- [Spears & Munsil: PLUGE pattern help text](https://spearsandmunsil.com/portfolio-item/setting-the-brightness-control-2/)
- [Fraunhofer IIS: HTML5 multichannel playback tests](https://www2.iis.fraunhofer.de/AAC/multichannel.html) — spoken per-channel identification
- [MDN: `MediaCapabilities.decodingInfo()`](https://developer.mozilla.org/en-US/docs/Web/API/MediaCapabilities/decodingInfo)
- [MDN: `VideoPlaybackQuality` (`droppedVideoFrames`/`totalVideoFrames`)](https://developer.mozilla.org/en-US/docs/Web/API/VideoPlaybackQuality)
- [MDN/web.dev: `requestVideoFrameCallback()`](https://web.dev/articles/requestvideoframecallback-rvfc)
- [Chromium `dynamic-range`/`video-dynamic-range` media query intent](https://groups.google.com/a/chromium.org/g/blink-dev/c/OpUsOWnnN6c)
- [MDN: Gamepad API, `gamepadconnected`](https://developer.mozilla.org/en-US/docs/Web/API/Gamepad_API/Using_the_Gamepad_API)
- Device-testing pass criteria (Media3's ≤1 %/≤10-consecutive dropped-frame bar, dash.js's 0.5 s
  seek tolerance): already sourced in [device-testing.md](device-testing.md), not re-fetched here.
