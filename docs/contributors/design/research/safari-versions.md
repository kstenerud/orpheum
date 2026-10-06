> **ABOUTME:** Which Safari versions Watch Together can support, by market share and by what each
> version's player can do: ManagedMediaSource from iOS 17.1, native HLS below it, the WebKit bugs
> per version that touch sync, and how others sync Safari. Corrects playback-in-practice.md §1.
> Ends in a recommended floor, per-range rules and device tests.

# Safari versions for Watch Together: market share, MSE/MMS, native HLS, and what the sync must work around

Researched 2026-10-06. Builds on [playback-in-practice.md](playback-in-practice.md) §1, and on the iPhone round behind it
("research-ios" below, whose findings that section holds), and does not repeat them except where this research corrects them.
Labels:

- **[sourced]**: a release note, commit, bug, spec, data set or issue says it.
- **[source-read]**: I read it in source code (WebKit release branches, hls.js, jellyfin-web, SyncLounge).
  Nobody has written it up.
- **[inferred]**: my reasoning. Not confirmed on a device.

Cache: `/yoloai/cache/research-safari/` (StatCounter CSVs, all Safari 15–27 release notes as JSON,
WebKit sources from the `safari-7623`, `safari-7624.*` and `safari-7625` release branches, WebKit commits,
Bugzilla bugs, MDN BCD). The clones in `/yoloai/cache/git/` were reused (hls.js `3a2e737`,
jellyfin-web `dd6d1d6`, SyncLounge `322159e`).

> **Correction to research-ios.md and playback-in-practice.md §1.** Both say Safari 27's fixes for
> "currentTime updating only at fixed intervals" (170115677) and "currentTime stale after the rate goes
> from 0 to non-zero" (177046564) mean those bugs were "present on her iOS 26.6". The WebKit commits say
> otherwise **[sourced]**:
>
> - The two bugs were in `AudioVideoRendererRemote`, a new MSE/WebM renderer added 2025-10-01
>   ([commit 6069cb8](https://github.com/WebKit/WebKit/commit/6069cb8422)). It was "behind a preference,
>   disabled by default".
> - 177046564 was a regression introduced by the fix for 170115677 itself (311867@main, 2026-04-23)
>   ([commit 5a1e303](https://github.com/WebKit/WebKit/commit/5a1e30384382df1fbfedb3c0429b94d6c9cbf456)).
> - In every Safari 26 release branch I checked (`safari-7623-branch`, `safari-7624.1.16-branch`,
>   `safari-7624.5.1.11-branch`), the preference `MediaSourceUseRemoteAudioVideoRenderer` is still
>   `default: false`. It is gone, meaning always on, only in `safari-7625-branch` (Safari 27) **[source-read]**.
>
> So the 250 ms stepping and the stale-after-play reading were **Safari 27 beta regressions, fixed before
> 27.0**. They were most likely **not** on iOS 26.6 **[inferred; the branch→version mapping is mine]**. A
> milder, older version of the same effect does exist on 18.4–26.x. It is described in §4.
>
> The other one: 175774587 ("currentTime on iOS to update more frequently") is about
> `MediaPlayerPrivateWirelessPlayback`, i.e. **AirPlay** routes, not local playback
> ([commit 3b6d172](https://github.com/WebKit/WebKit/commit/3b6d172541dd55e39042c9f972abafc91fe892b0)).
> And Safari 17.4's "Fixed the always empty video.buffered attribute" was **WebM only**
> ([commit 1021b95](https://github.com/WebKit/WebKit/commit/1021b9500accef619ee22b690c8b7ce7a8582baf)).

---

## 1. Market share (2026)

### iPhone: StatCounter, worldwide, September 2026 [sourced]

From StatCounter's CSV export (`gs.statcounter.com/chart.php?...statType_hidden=ios_version&device_hidden=mobile`,
cached as `sc-ios_version-mobile-202609.csv`). I summed the minor versions:

| iOS (iPhone traffic, all browsers) | Share | Cumulative from newest |
| --- | --- | --- |
| 27.x | 4.5 % | 4.5 % |
| 26.x | 72.8 % | 77.3 % |
| 18.x | 16.5 % | 93.8 % |
| 17.1–17.7 | 1.8 % | **95.6 %** |
| 17.0 | 0.07 % | 95.7 % |
| 16.x (mostly 16.7) | 2.5 % | 98.2 % |
| 15.x (mostly 15.8) | 1.4 % | 99.6 % |
| ≤ 14 | 0.3 % | 99.9 % |

- **Caveat on the UA freeze [sourced].** Safari 26 freezes the OS token in its user agent at
  `iPhone OS 18_6`/`18_7`, while `Version/26.x` still tracks the real version. StatCounter miscounted iOS
  26 as 18.7/18.6 until a patch on 2026-01-19 (the note on StatCounter's page;
  [uaparser.dev](https://docs.uaparser.dev/guides/how-to-detect-ios-26-using-javascript.html);
  [AppleInsider 2026-01-12](https://appleinsider.com/articles/26/01/12/ios-26-adoption-isnt-record-breaking-but-reports-of-extremely-low-rates-are-flawed)).
  The 10.2 % "iOS 18.7" in September may still contain some iOS 26. If so, that only *understates* new
  versions, so the cumulative column above is conservative.
- **For Orpheum:** never pick a code path from the UA's OS number. Detect features
  (`'ManagedMediaSource' in window`, `window.MediaSource`) **[inferred]**.
- **The tail below 17.1 is hardware-capped [inferred from the shares].** It is almost entirely
  iOS 16.7.x (1.56 %) and 15.8.x (1.04 %). Those are the final releases for iPhone 8/X and for iPhone
  6s/7/SE (1st gen) respectively. These phones will never get MMS.
- **All iOS browsers use WebKit [sourced].** Chrome for iPhone (5.6 % of world mobile browser traffic,
  `sc-browser_version-mobile-202609.csv`) runs the same engine version as the iOS it is on. So the iOS
  version is the engine version. Whether MMS is exposed inside Chrome's WKWebView I could not confirm.
  See device test T12.

### iPhone and iPad: Apple's own figures, App Store transactions, 2026-06-07 [sourced]

([9to5Mac 2026-06-10](https://9to5mac.com/2026/06/10/ios-26-adoption-grows-but-still-lags-slightly-behind-ios-18/),
[AppleInsider 2026-06-10](https://appleinsider.com/articles/26/06/10/fewer-iphone-users-are-updating-to-ios-26-than-they-did-with-ios-18).)

| | 26 | 18 | earlier |
| --- | --- | --- | --- |
| iPhone, all | 79 % | 14 % | 7 % |
| iPhone, last 4 years | 86 % | 11 % | 3 % |
| iPad, all | 68 % | 17 % | 15 % |
| iPad, last 4 years | 79 % | 16 % | 5 % |

Apple's "earlier" for iPhone (7 %) includes 17.x. StatCounter puts 17.x at about 1.9 % and below-17.1 at
about 4.3 %, which is consistent with Apple's figure.

### iPad (StatCounter tablet) [sourced, with a caveat]

- From StatCounter's tablet data (cached):
  - 26+27: 65 %.
  - 18: 13 %.
  - 17.1–17.7: 6.5 %.
  - 15–16: 7.7 %.
  - ≤14: 7.1 %. This is mostly iOS 12.5 (3.7 %) and 10.3 (1.4 %), which are old iPads stuck below 13.
- **Caveat [inferred]:** since iPadOS 13, Safari on iPad sends a desktop "Macintosh" UA by default. Modern
  iPads are therefore probably counted as macOS Safari, which overweights old iPads in the tablet
  numbers. Apple's 15 % "earlier than 18" is the better figure.

### macOS Safari: StatCounter desktop, September 2026 [sourced]

Safari is 8.0 % of desktop traffic. Within Safari:

| Safari | Share | Cumulative |
| --- | --- | --- |
| 27 | 3.9 % | 3.9 % |
| 26 | 60.5 % | 65.5 % |
| 18 | 12.4 % | 77.9 % |
| 17 | 13.2 % | 91.1 % |
| 16 | 5.3 % | 96.4 % |
| 15 | 2.4 % | 98.7 % |
| ≤ 14 | 1.3 % | 100 % |

- macOS Safari versions track the macOS release less tightly. For example, 17.6 is the last Safari for
  Monterey.
- Every macOS Safari since 8 has MSE.
- There may be some iPads in these numbers (previous caveat).

### What a floor covers

| Floor (iPhone) | iPhone traffic covered (StatCounter 2026-09) |
| --- | --- |
| iOS 18.0 | ~93.8 % |
| **iOS 17.1 (first MMS)** | **~95.6 %** |
| iOS 16.0 | ~98.2 % |
| iOS 15.0 | ~99.6 % |

**95 % of iPhone users is reached at 17.1.** That is also the first version where hls.js can run on an
iPhone at all, which is a convenient coincidence.

---

## 2. MSE / ManagedMediaSource per version, known bugs, hls.js minimums

### Availability [sourced: MDN BCD `api/MediaSource.json`, `api/ManagedMediaSource.json`; Safari release notes]

| Device | MediaSource (classic) | ManagedMediaSource | Otherwise |
| --- | --- | --- | --- |
| iPhone, any version incl. 27 | **never** ("Exposed in Mobile Safari on iPad but not on iPhone") | **17.1+** (17.0 had it "as a preview on iOS", Safari 17 notes 30320350; 17.1 notes 114270414) | native HLS only |
| iPad | **iPadOS 13+** ([WebKit 200147](https://webkit.org/b/200147)) | 17.0+ per Safari 17 notes ("macOS and iPadOS"); BCD says 17.1 | native HLS on iPadOS ≤ 12 |
| macOS Safari | 8+ | 17.0+ | — |

- **[source-read] hls.js defaults to `preferManagedMediaSource: false`** (`src/config.ts:453`).
  `getMediaSource()` returns MMS only when it is preferred or `window.MediaSource` is absent.
  - So with Orpheum's config, hls.js uses **MMS only on iPhone**.
  - On iPad and Mac it uses **classic MSE**: no `endstreaming` throttling, `maxBufferLength` is honoured,
    and hls.js does not set `disableRemotePlayback` there.
  - The MMS-specific behaviour in research-ios.md (10/30 s thresholds, `bufferingEnabled`) is therefore
    iPhone-only.
- **[sourced] MMS needs an AirPlay alternative or remote playback disabled** (Safari 17 notes 109130836).
  hls.js sets `disableRemotePlayback` itself (research-ios §1).
- **hls.js minimums:**
  - MMS support arrived with [PR #5542](https://github.com/video-dev/hls.js/issues/5542) (1.5.0).
  - Recovery for `endstreaming` without aborting requests: 1.6.0 (#6186).
  - iOS 26.2 cold-start recovery and the Safari bfcache/background restore (#7697, #7683): **1.7.0**.
  - Resume buffering on `seeking`: **1.7.1** (#7984).
  - Orpheum pins 1.7.3, which has all of them.

### WebKit MSE/MMS bugs by version that touch sync

Release notes are cached in `relnotes/`. All **[sourced]** unless marked.

| Fixed in | Bug | Effect on sync | Workaround if not fixed |
| --- | --- | --- | --- |
| 17.1 | "media element backed by MediaSource may not receive the seeked event" (114594559) | iPad/Mac 17.0 MSE: seek never "completes" | Moot above the floor. Otherwise a seek watchdog. |
| 18.0 | "allowing a video's currentTime to be further than the gap's start time" (124186726) | `currentTime` runs ahead of buffered data at a gap | Read rVFC `mediaTime` instead (§4) |
| 18.4 | "playback doesn't always resume after a seek" (140097993). MediaSource and WebM players compared MediaTime double vs rational exactly ([commit 4a58bf5](https://github.com/WebKit/WebKit/commit/4a58bf5dbacc3f73b940962c0f2d881b9178dbbb)). | **17.1–18.3 MMS:** a catch-up seek can leave the member stuck "seeking" | After `seeked`, if `currentTime` does not advance within ~1 s while `!paused`, issue `play()` or a tiny re-seek **[inferred]** |
| 18.4 | "playing video generating non-monotonic timeupdate events" (142275184). At start, the estimator ran ahead on `rateChanged` and then snapped back ([commit 0e4cfc6](https://github.com/WebKit/WebKit/commit/0e4cfc6b8f)). Affects all GPU-process players: MSE **and native HLS**. | **≤18.3:** `currentTime` briefly goes *backwards* right after `play()` | Discard drift samples for ~0.5 s after `playing` and `ratechange` **[inferred]** |
| 18.4 | `play()` during a seek now fires `play` (142517488) | Event ordering differs ≤18.3 | Don't key logic on `play` during a seek |
| 26.3 | Live: seek not completing after SourceBuffer remove/re-add (165628836) | VOD unaffected **[inferred]** | — |
| 26.4 | "ended event for MSE might never fire" (165430052) | End of film | hls.js emits `MEDIA_ENDED` |
| open | 26.2: MMS cold start after Safari force-quit ([WebKit 305712](https://bugs.webkit.org/show_bug.cgi?id=305712)) | First load fails | hls.js 1.7.0 recovers |
| open | **26.6/26.6.2:** after `SourceBuffer.remove(0, Infinity)` and an append from 0 *without a seek*, Safari presents every frame from the start of the new data up to `currentTime` in ~0.5 s ([WebKit 325251](https://bugs.webkit.org/show_bug.cgi?id=325251), 2026-09-25; also [shaka #10504](https://github.com/shaka-project/shaka-player/issues/10504)) | A full-buffer flush on a quality switch flashes a fast-forward | Prefer hls.js's smooth level switching (`nextLevel`/`loadLevel`, not `currentLevel`). Orpheum's `chunked.ts` switches by `autoLevelCapping`, hls.js's own smooth switch, so it does not flush (checked 2026-10-06). |
| 27.0 | currentTime fixed-interval / stale-after-rate (170115677, 177046564) | **27 betas only** (see the correction at the top) | — |
| 27.0 | MSE `readyState` not updated at once on a gap stall (176330683). Gap tolerance loosened (180439090). | ≤26: a gap stall shows late in `readyState`/`waiting` | Use hls.js stall events, not only `waiting` |
| open, **27.0.1** | MMS does not fire `startstreaming` after a seek outside the buffer while `streaming == false`. Seek never completes, `readyState` stays 4 ([WebKit 326390](https://bugs.webkit.org/show_bug.cgi?id=326390), 2026-10-05, reported from **Emby web on Safari 27.0.1**; earlier [321919](https://bugs.webkit.org/show_bug.cgi?id=321919)) | **Catch-up seeks on 27.x freeze forever** | hls.js ≥1.7.1 resumes buffering on `seeking`. Keep it. |
| open, 27 | `SourceBuffer.remove(buffered.start(0), …)` on audio leaves ~0.5 s ([WebKit 325128](https://bugs.webkit.org/show_bug.cgi?id=325128)) | Wrong next-audio position, stalls in "seeking" | **[source-read]** hls.js removes from `Math.max(0, start)`, usually 0 for flushes (`buffer-controller.ts` `removeExecutor`), so it is probably unaffected |
| open, 18.7 and 27 beta | A tab's audio goes silently dead while every API reports success ([WebKit 323104](https://bugs.webkit.org/show_bug.cgi?id=323104)) | A member hears nothing, nothing detectable | None. A "can you hear it?" UI hint. |

Searches of hls.js issues for "iOS 17"/"iOS 18" + ManagedMediaSource found nothing 17.x-specific. That is
absence of reports, not a clean bill.

---

## 3. Native HLS on iPhone/iPad, item by item, for what the sync needs

The source reading below is from WebKit's `safari-7624.5.1.11-branch` (a Safari 26.x branch):
`MediaPlayerPrivateAVFoundationObjC.mm`, `MediaPlayerPrivateRemote.cpp`, `RemoteMediaPlayerProxy.cpp`,
`HTMLMediaElement.cpp`, `MediaElementSession.cpp`, and `MediaSessionManagerCocoa.mm`.

### Position: `currentTime` accuracy and update granularity

- **[source-read]** Native HLS runs AVPlayer in the GPU process. The page reads `currentTime` from
  `MediaPlayerPrivateRemote::TimeProgressEstimator`, which works like this:
  - the estimate is `cachedTime + rate × (now − cachedAt)`;
  - it is clamped to be monotonic;
  - it is re-anchored when the GPU sends a time update;
  - AVPlayer's periodic observer runs every **0.1 s** (`addPeriodicTimeObserverForInterval: CMTimeMake(1,10)`);
  - the proxy's cached-state timer runs every **250 ms**, or every 2 s when the player can observe time
    changes itself (`RemoteMediaPlayerProxy::startUpdateCachedStateMessageTimer`).
  - So `currentTime` is smooth, not stepped. Its error is the estimator's drift since the last anchor.
- **[source-read + sourced] After any rate change, including `play()` and every `playbackRate` nudge,**
  `rateChanged` calls `forceUseOfCachedTimeUntilNextSetTime()`. `currentTime` then **freezes at the
  last anchor until the next GPU time update** arrives (≈ ≤100–250 ms) and then jumps. This is the 18.4
  fix in [commit 0e4cfc6](https://github.com/WebKit/WebKit/commit/0e4cfc6b8f) (Safari 18.4, 142275184).
  - Before 18.4 it was worse: values ran ahead and then went backwards.
  - Both are a mild form of the "stale after rate change" problem, and they apply to **18.4–26.x**
    native HLS and MSE.
- **[sourced]** Safari 15.5 fixed "HLS stream currentTime sometimes jumping backwards". iOS 15.0–15.4
  native has it.
- **[sourced]** A WebKit `<video>` sync engine measured that "correcting small drift with playbackRate
  raised WebKit's re-seeks from 6 to 28 per 10 s, because each rate change also freezes the element".
  It raised the WebKit drift tolerance from 0.18 s to 0.6 s instead
  ([lsegal/zvid PR #1112](https://github.com/lsegal/zvid/pull/1112), 2026-10-06; media type not stated).

### `playbackRate` on native HLS

- **Works for 0 < rate ≤ 2 on VOD [source-read + inferred].** WebKit calls `[AVPlayer setRate:]`. Rates
  above 2× and below 0 depend on `canPlayFastForward`/I-frame playlists. Negative rates on HLS fail
  silently ([Apple forum 102361](https://developer.apple.com/forums/thread/102361)). Planned ±10 % is
  well inside the range.
- **Audio pitch [source-read].** With `preservesPitch` true (the default) and rate ≠ 1, WebKit switches
  the item's `audioTimePitchAlgorithm` from **Varispeed** to **Spectral**. At rate 1 it switches back
  (`MediaSessionManagerCocoa::audioTimePitchAlgorithmForMediaPlayerPitchCorrectionAlgorithm`).
  - **[inferred]** Swapping the time-pitch algorithm on a playing item is a plausible cause of the
    "50–100 ms skip on each rate change" in
    [shaka #2823](https://github.com/shaka-project/shaka-player/issues/2823) (iOS 13).
  - A cheap experiment: set `video.preservesPitch = false` (unprefixed since Safari 17.2, `webkit`-prefixed
    since 4). The algorithm then stays Varispeed. At ±10 % that costs a pitch shift of ≈1.6 semitones,
    noticeable on music. Settle with device test T2.
- **Latency:** 100–500 ms, unpredictable (hls.js #7616, in research-ios). WebKit's 26.x `setPlayerRate`
  also toggles `automaticallyWaitsToMinimizeStalling` only for host-time play **[source-read]**. That
  path is reachable only through the Media Session coordinator, which is not on the web (§5).

### Seeking

- **Precise [source-read].** `currentTime = t` seeks with zero tolerance. `fastSeek(t)` allows landing
  short of the target (`HTMLMediaElement::fastSeek`). AVPlayer decodes from the preceding keyframe.
  Latency is AVPlayer's, unknown: device test T3.
- **Seek while paused before the first frame is dropped on iPhone [sourced].**
  - Reported on **iOS 18.3.1** ([video.js #9018](https://github.com/videojs/video.js/issues/9018), 2025-03,
    "does not occur on iPad") and **iOS 26.6.1** ([video.js PR #9239](https://github.com/videojs/video.js/pull/9239),
    2026-09-29, open).
  - It happens with plain `<video>`, MP4 and native HLS.
  - When paused with `readyState < HAVE_CURRENT_DATA`, a pending seek is lost when playback starts. The
    video plays from ~3 s, `currentTime` keeps reporting the target, `seeking` stays true, and later
    seeks are ignored.
  - **Workaround (the PR's):** hold the seek and apply it on `play` or `loadeddata`, whichever comes
    first. Or call `play()` first and seek right after.
  - Whether MMS is affected is not stated (T4).
- **Seek to unbuffered positions** works on native (AVPlayer loads the target segment) **[inferred]**.
  Cost depends on the segment length, as in research-ios §3.

### Does a paused element keep buffering ahead?

- **Preload before the first gesture is capped at `metadata` on iOS [source-read].**
  `MediaElementSession::effectivePreloadForElement` caps preload to MetaData while the
  `AutoPreloadingNotPermitted` restriction is set. That restriction is set when the document's
  `mediaDataLoadsAutomatically` is false, which is iOS's default **[inferred for the default]**.
  `removeBehaviorRestrictionsAfterFirstUserGesture` removes it. So a member who has never tapped play
  on *this element* buffers nothing past metadata.
- **After that, VOD keeps buffering while paused [inferred].**
  - Apple only documents a switch for *live* (`canUseNetworkResourcesForLiveStreamingWhilePaused`),
    which implies VOD keeps loading.
  - An Apple forum thread reports an HLS item continuing to buffer after pause
    ([forum 63435](https://developer.apple.com/forums/thread/63435)).
- **The amount cannot be set from a page [source-read].** WebKit does not set
  `preferredForwardBufferDuration`, so AVPlayer decides. The page can only observe progress through
  `video.buffered`, which comes from AVPlayerItem's `loadedTimeRanges`.
- **Consequence for "jump ahead and wait paused":** on native this *probably* works, but the page cannot
  ask for "30 s". It can only poll `buffered` and resume when enough is there, and it may never get
  that much. **Device test T5.**

### Events and `buffered`

- `waiting`, `stalled`, `canplaythrough` and `progress` are WebKit's generic `HTMLMediaElement` events,
  derived from `readyState`/`networkState`. These are fed by AVPlayer's `timeControlStatus` and
  `loadedTimeRanges` **[source-read, at a high level]**.
- 16.4 fixed duplicate `timeupdate`. 27 fixed `timeupdate` firing mid-seek (176861767) **[sourced]**.
- Their reliability on a throttled link is **not documented anywhere I found**. Device test T6.

### Quality control

- **[source-read]** WebKit's AVPlayer path sets **none** of `preferredPeakBitRate`,
  `preferredMaximumResolution` or `startsOnFirstEligibleVariant`.
- **[sourced]** Since iOS 13, AVPlayer "starts HLS playback by choosing an initial variant that
  optimizes the startup experience", not the first one listed. Only apps can opt back in, with
  `startsOnFirstEligibleVariant`
  ([Apple docs](https://developer.apple.com/documentation/avfoundation/avplayeritem/startsonfirsteligiblevariant)).
- **So the page's only levers are server-side [inferred]:**
  1. serve a per-session multivariant playlist that lists only the variants allowed;
  2. change `src` to switch. That is a full reload: a new load, then a seek. On a paused or not-started
     element it hits the #9239 bug.
- There is no per-segment load telemetry in the page:
  - AVPlayer's requests are made outside the WebContent process;
  - whether they appear in Resource Timing is not documented (device test T7).
  - **The server sees every segment request**, so a native member's link speed can be measured
    server-side **[inferred]**.

### Alternate audio renditions

- **[source-read]** WebKit builds `audioTracks` from AVFoundation's *audible media selection group*.
  For HLS, that is the `EXT-X-MEDIA TYPE=AUDIO` renditions
  (`MediaPlayerPrivateAVFoundationObjC::updateAudioTracks`, `safeMediaSelectionGroupForAudibleMedia`).
  `AudioTrack.enabled` is forwarded to the selection.
- Jellyfin relies on `audioTracks` on Safari, but only for direct play
  (`browserDeviceProfile.js` `canPlaySecondaryAudio`).
- Whether a switch on native HLS is seamless, and keeps A/V sync: device test T8.

### AirPlay

- **Native HLS:** AirPlay hands the URL to the receiver, which fetches it itself **[inferred]**. So
  tokens must ride in the URL (cookies do not travel).
- **On 26.x, `currentTime` during AirPlay is whatever the route reports.** Safari 27 added a 100 ms
  timebase for wireless playback because "there is no guarantee that currentPlaybackPosition will
  update at a reasonable interval" ([commit 3b6d172](https://github.com/WebKit/WebKit/commit/3b6d172541dd55e39042c9f972abafc91fe892b0))
  **[sourced]**. So sync during AirPlay is coarse on ≤26.
- **MMS/MSE:** no AirPlay unless a `<source>` m3u8 alternative is present (16.4 added "alternate URL
  for AirPlay"; 17.1 requirement) **[sourced]**. AirPlay of hls.js is still open
  ([hls.js #6482](https://github.com/video-dev/hls.js/issues/6482)).

### Fullscreen and inline

- **iPhone:** `playsinline` is needed, or `play()` enters native fullscreen (WebKit 2016 policies).
  Element `requestFullscreen` is **iPad only**: "Only available on iPad, not on iPhone"
  (BCD `Element.requestFullscreen`). On iPhone only `webkitEnterFullscreen()` on the video exists
  **[sourced]**.
- **[inferred]** In iPhone video fullscreen, page overlays (sync status, "waiting for group") are
  invisible. The page still runs, so corrections still apply.

### Autoplay and `play()` after a programmatic pause

- Covered in research-ios §3 **[source-read]**: after the first gesture-driven play on an element, the
  restrictions (rate change, Low Power Mode, thermal, preload) are removed **for that element**.
- A new element or a reload starts restricted. Low Power Mode refuses even muted autoplay.
- Keep one `<video>` for the session, including across a native `src` change **[inferred]**.

---

## 4. `requestVideoFrameCallback`, and the currentTime-interval bug

- **Availability [sourced]:** Safari 15.4+, iOS mirrors it (BCD).
- **What it reports on native HLS [source-read]** (`MediaPlayerPrivateAVFoundationObjC::updateLastPixelBuffer`):
  - `startVideoFrameMetadataGathering()` attaches an `AVPlayerItemVideoOutput`-based queue.
  - For each new pixel buffer, `mediaTime` = the frame's **display time (PTS)**.
  - `presentationTime` = `now − (currentTime − displayTime)`, computed in the GPU process from
    AVPlayer's own time.
  - So `mediaTime` is exact to the frame and **not subject to the WebContent estimator's freeze**.
    Its resolution is one frame (≈42 ms at 24 fps).
  - `position ≈ mediaTime + (performance.now() − presentationTime) × rate`. **[inferred]** this
    removes the after-rate-change freeze.
  - **[inferred]** Turning it on adds a video-output copy path, so it costs something on old phones.
    Device test T13.
- **Known rVFC problems on Safari [sourced]:**
  - DRM HLS + non-native text tracks broke on 15.4/15.5 when video.js started using rVFC
    ([video.js #7794](https://github.com/videojs/video.js/issues/7794)). Not our case.
  - On iOS 26.6 MMS, rVFC is what *revealed* the frame burst in WebKit 325251. It is reliable enough
    to diagnose with.
- **The "currentTime updates only at fixed intervals / stale after rate change" bug by version:**
  - **≤18.3:** estimator plus an extra 200 ms element-side cache, so it can go backwards at play start
    ([commit 0e4cfc6](https://github.com/WebKit/WebKit/commit/0e4cfc6b8f)) **[sourced]**.
  - **18.4–26.x:** monotonic and smooth, but **frozen after every play or rate change until the next GPU
    anchor** (≤ ~100–250 ms) **[source-read]**.
  - **27 betas:** 250 ms steps (MSE, new renderer), then a stale rate after 0→non-zero. Both were fixed
    before 27.0, which instead reads a shared-memory timebase published every 100 ms and on state
    changes ([commit beb0b21](https://github.com/WebKit/WebKit/commit/beb0b2162e),
    [1feeb18](https://github.com/WebKit/WebKit/commit/1feeb18419); `SharedTimebase` is present in
    `safari-7625-branch`) **[sourced/source-read]**.
  - **Workaround on all versions [inferred]:** ignore drift samples for ~300 ms after
    `ratechange`/`playing`/`seeked`. Better, take the position from rVFC as above. Require two
    consecutive out-of-band samples before acting.

---

## 5. How others sync native-HLS Safari

- **Jellyfin SyncPlay [source-read + sourced]:**
  - Safari on iOS always plays **native** HLS. `enableHlsJsPlayer()` returns false for `browser.iOS`,
    and false if `window.MediaSource == null`. macOS Safari also gets native for VOD
    (`canPlayNativeHls()` plus `runTimeTicks`) (`htmlMediaHelper.js`).
  - The sync engine has **no Safari or iOS branch**. The only trace is the comment "SpeedToSync is
    failing on Safari (Mojave); even if playbackRate is supported, some delay seems to exist"
    (`PlaybackCore.js:556`).
  - Defaults: SpeedToSync 60–3000 ms for 1 s, SkipToSync ≥400 ms.
  - **`enableSyncCorrection` defaults to false on every platform** (`PlaybackCore.js:69`). It was turned
    off first on mobile "because it causes constant stuttering"
    ([PR #2946](https://github.com/jellyfin/jellyfin-web/pull/2946), merged 2021-09-14), then everywhere
    ([issue #4972](https://github.com/jellyfin/jellyfin-web/issues/4972)).
  - So Jellyfin syncs Safari by **group play/pause/seek commands only, with no drift correction**.
  - A "V2" rework ([PR #7566](https://github.com/jellyfin/jellyfin-web/pull/7566)) was closed unmerged
    on 2026-03-16.
  - The other Safari special case: a "huge hack" pre-fetching the HLS playlist before handing the URL
    to Safari, for live transcodes (`plugin.js:472`).
- **SyncLounge [source-read]:** Shaka player. No Safari branch. Speed-sync at ±50 % rate, soft-seek
  threshold 200 ms (`config/defaults.js`). It inherits whatever Shaka picks: MMS on Apple from 4.8.0,
  so native only below 17.1.
- **Emby web [sourced, indirectly]:** WebKit 326390 was found "in the Emby web client in Safari on both
  macOS 27.0.1 and iOS 27.0.1" with hls.js. So Emby uses hls.js on MMS on iPhone, unlike Jellyfin.
- **Apple's coordinated playback on the web [source-read]:**
  - WebKit has `navigator.mediaSession.coordinator`. `HTMLMediaElement::playPlayer` can
    `playAtHostTime` (AVPlayer `setRate:time:atHostTime:`).
  - But `defaultMediaSessionCoordinatorEnabled()` is true only if the host process has the
    `com.apple.developer.group-session.urlactivity` entitlement (`WebPreferencesDefaultValues.cpp`,
    Safari 27 branch). That is a SharePlay app embedding WKWebView, not a web page in Safari.
  - **Not available to Orpheum.**
- **Teleparty / Scener [sourced, thin]:** Teleparty on iPhone is a **native App Store app** (iOS 17.6+,
  [App Store](https://apps.apple.com/app/id1620026075)), not iOS Safari. I found no evidence that either
  syncs a page in iOS Safari. Plex Watch Together survives only in Plex's web app
  ([Plex support](https://support.plex.tv/articles/watch-together/), via search; the page returned 403
  to me).
- **Published approaches:**
  - hls.js maintainer: rate and seek correction "should only be used on muted elements" (#7616).
  - zvid: drop rate nudges on WebKit, widen tolerance to 0.6 s (PR #1112).
  - I found nothing better published for native iOS.

---

## 6. Recommendation

### Support floor

- **Watch Together: iOS/iPadOS 17.1** (iPhone MMS). This reaches **~95.6 % of iPhone traffic**
  (StatCounter 2026-09). Every iPad on 13+ has classic MSE and every macOS Safari has MSE, so all
  supported Apple devices run the **same player: hls.js**. Only one sync tuning per engine generation
  is needed, not two players. **[sourced shares + inferred design]**
- **Solo playback: iOS 15** (~99.6 %). `hlsMethod()` already falls back to native where there is no MSE.
- **Below 17.1 in Watch Together: "best effort" native mode, or refuse with a clear message.**
  This is the **owner's call** (design is frozen; this is a product choice). Those ~4 % are
  hardware-capped iPhone 8/X/7/6s and pre-2013 iPads. **Decided 2026-10-06 (D250 §7): the floor
  is 17.1, and a browser below it is told to update before it joins.**

### Per range

| Range | Player | What the sync must do |
| --- | --- | --- |
| **iPhone iOS 15.0–17.0, iPad ≤ 12** | Native HLS (no choice) | **No rate nudges** (rate freezes `currentTime`, pitch-algorithm switch, Jellyfin's experience). Seek-only correction with a wide dead-band: ≥ 0.6–1 s **[inferred from zvid/Jellyfin; tune with T1/T3]**. Read position from rVFC (15.4+) or discard samples for ~0.3 s after `playing`/`seeked`. On 15.0–15.4, allow for backward jumps. **Join/catch-up: `play()` first, then seek, or hold the seek until `loadeddata`** (#9239; iPhone only). Wait-ahead: pause at the target and poll `buffered` until N s are ahead, with a timeout and no target guarantee (T5). Quality: per-session master playlist plus `src` change. Measure load **on the server**. One element, tap-to-start, `playsinline`. |
| **iPhone 17.1–18.3 (MMS)** | hls.js ≥1.7.1 on MMS | Everything in research-ios §5. Plus: **seek watchdog** (18.4 fixed "playback doesn't always resume after a seek"). Discard samples ~0.5 s after `playing` (non-monotonic start). Wait-ahead ≤ 30 s (MMS HighThreshold). `bufferingEnabled == false` is not a dead link. |
| **iPhone 18.4–26.x (MMS)** | hls.js on MMS | Rate nudges allowed but small (±10 %, as planned), at most one per few seconds, with **no drift sampling for ~300 ms after each `ratechange`**, or rVFC-based position. Dead-band ≥ 250–500 ms (#7616). Avoid flush-from-0 quality switches on 26.x (WebKit 325251). Test `preservesPitch=false` (T2). |
| **iPhone/iPad/Mac 27.x** | hls.js on MMS (iPhone) / MSE | Keep hls.js ≥1.7.1 for the open `startstreaming`-after-seek bug (326390, still in 27.0.1). `currentTime` is better (shared timebase); the dead-band may tighten after measuring T1. |
| **iPad 13–16** | hls.js on classic MSE | As 17.1–18.3 for the seek/start quirks. No MMS throttling: `maxBufferLength` applies. |
| **macOS Safari (all in use)** | hls.js on classic MSE | Same as iPad by version (seek watchdog ≤ 18.3, sample-discard after rate change ≤ 26). No AirPlay unless an m3u8 `<source>` is added. |
| **AirPlay (any)** | Native URL if we add it | Treat an AirPlaying member as "coarse": seek-only, wide dead-band, at least on ≤ 26. |

### Can native HLS be coaxed into what the sync needs?

| Need | On native | Basis |
| --- | --- | --- |
| Read position | **Yes**, with rVFC or a post-change blackout | source-read |
| Seek | **Yes**, with the iPhone paused-seek workaround | sourced |
| Rate nudges | **Unreliable**: skip them | sourced/inferred |
| Wait ahead | **Probably**, without control over how much | inferred; T5 decides |
| Quality control | **Only by reloading** with a restricted master playlist | source-read/inferred |
| Load measurement | **Only server-side** | inferred |

That is enough for a degraded mode, not for parity. **Making hls.js on (Managed)MediaSource work
properly** is the better investment for 17.1+. Concretely:

1. hls.js ≥1.7.1.
2. A Safari-aware follower: blackout after rate/seek/play, rVFC position, wider dead-band, nudge cap,
   seek watchdog.
3. Wait-ahead ≤ 30 s on MMS.
4. Count `bufferingEnabled == false` as not loading.
5. Avoid full flushes on 26.x.
6. Keep one element.

### What is well sourced, inferred, or open

- **Well sourced:**
  - the shares;
  - MSE/MMS availability;
  - each WebKit fix, its version and its commit;
  - the iPhone paused-seek bug on 18.3.1 and 26.6.1;
  - the open 27.0.1 MMS seek bug;
  - Jellyfin's native-only Safari choice and its correction being off by default;
  - coordinated playback being unavailable to pages.
- **Source-read, version mapping inferred:**
  - which WebKit branch is which Safari release (`7623` = 26.2/26.3, `7624.*` = 26.4–26.7, `7625` = 27);
  - therefore that the 250 ms stepping never shipped in 26.x.
- **Inferred:**
  - the pitch-algorithm switch as the cause of rate-change stutter;
  - paused VOD buffering on native;
  - server-side load measurement being the only native telemetry;
  - the per-range thresholds.
- **Only a device can settle:** everything in the list below.

### Device tests (real iPhones; run each on iOS 16.7, one 17.x, 18.x, 26.x and 27.x where available, native and MMS where both apply)

Shared harness: one page, one `<video playsinline>`, the same Orpheum film, a log uploaded to the server.
Each test records:

- `performance.now()`;
- `currentTime`, `paused`, `seeking`, `readyState`, `networkState`;
- `buffered` (all ranges);
- every media event;
- rVFC (`mediaTime`, `presentationTime`, `presentedFrames`);
- `getVideoPlaybackQuality()`;
- for MMS: `ms.streaming`, `endstreaming`/`startstreaming` and hls.js events;
- server-side: every segment request with its time and bytes.

For the throttled runs, use Settings ▸ Developer ▸ Network Link Conditioner ("3G", "Edge").

- **T1: currentTime granularity and freeze.**
  1. Tap play. Sample `currentTime` every rAF for 30 s.
  2. Set `playbackRate=1.1` for 5 s, then back to 1.0. Pause 5 s. `play()`.
  3. Repeat 10×.
  4. Report: the distribution of `currentTime` increments per rAF; the length of the flat run after each
     `ratechange`/`playing`; and |`currentTime` − rVFC-extrapolated position| over time.
  - *Settles the dead-band and the post-correction blackout per version.*
- **T2: rate-nudge cost.**
  1. Alternate rate 1.0 ↔ 1.1 ↔ 0.9 every 5 s for 2 min, with `preservesPitch` true, then false.
  2. Report: rVFC `presentedFrames` gaps > 2 frame durations, `droppedVideoFrames`, `waiting` count, the
     effective rate from the rVFC `mediaTime` slope, and audible glitches (headphones, by ear).
  - *Settles whether ±10 % nudges are usable at all, and whether `preservesPitch=false` helps.*
- **T3: seek latency and precision.**
  1. While playing, then while paused, seek to: +5 s (inside the buffer), +120 s (outside it), −30 s,
     an exact chunk start, and mid-chunk.
  2. Report: the time from assignment to `seeked`, the time to the first rVFC callback,
     first `mediaTime` − target, and the time to `playing`.
  - *Settles catch-up cost and whether to aim at chunk starts.*
- **T4: paused-before-first-frame seek (#9239).**
  1. Fresh load, `preload=auto`. On `loadedmetadata`, set `currentTime=600`.
  2. Wait 0.5 s / 3 s. Tap play.
  3. Report the first rVFC `mediaTime`.
  4. Repeat after a `src` change on an already-played element, and on MMS.
  - *Settles whether the join path needs the hold-seek workaround on each player.*
- **T5: paused buffering ahead.**
  1. Play 10 s, pause. Log `buffered` every 1 s for 3 min, plus server segment requests.
  2. Then, paused, seek +300 s and log the same.
  3. Do it once before any gesture on the element (`preload=auto`) and once after.
  4. For MMS, confirm the iOS `endstreaming`/`startstreaming` thresholds (10/30 s?).
  - *Settles whether "jump ahead and wait paused" fills on native, how far, and the real MMS thresholds.*
- **T6: event reliability under throttling.**
  1. Run T3 and T5 under "3G" and "Very Bad Network".
  2. Report which of `waiting`, `stalled`, `progress`, `canplaythrough`, `seeked` fire, and how late
     against server-side segment arrival.
  - *Settles which events the follower may trust per player.*
- **T7: quality control on native.**
  1. With a full ladder, see which variant AVPlayer fetches first (server log).
  2. Serve a per-session master with one variant, then switch `src` to another: measure the gap and
     whether position, play state and the gesture allowance survive.
  3. Check whether segment fetches appear in `performance.getEntriesByType('resource')`.
  - *Settles native quality control and in-page telemetry.*
- **T8: alternate audio on native.**
  1. Use a master with two `EXT-X-MEDIA` audio renditions. Check `audioTracks.length` and labels.
  2. Enable the second track while playing.
  3. Report time-to-switch, glitch, and A/V offset (clap test).
  - *Settles whether native can offer the audio choice.*
- **T9: AirPlay.**
  1. Native: AirPlay to an Apple TV. Sample `currentTime` at rAF. Apply a rate change and a seek.
  2. MMS: confirm the AirPlay button is absent; add an m3u8 `<source>` alternative and retry.
  - *Settles whether an AirPlaying member can stay in the group.*
- **T10: background and lock.**
  1. Play, lock the phone for 60 s, unlock.
  2. Report the `pause` event, the MMS `sourceclose`/errors, and whether a programmatic `play()` works
     without a tap. Repeat in Low Power Mode.
  - *Settles the rejoin path after a member's phone sleeps.*
- **T11: programmatic resume after a programmatic pause** (the wait-ahead).
  1. After one tap-to-play, `pause()` from code; 20 s later, `play()` from code.
  2. Normal, then Low Power Mode, then after a native `src` change.
  - *Settles whether wait-ahead can resume without a tap.*
- **T12: Chrome for iPhone.**
  1. Check `'ManagedMediaSource' in window` and `window.MediaSource`.
  2. Run T1 and T3.
  - *Settles whether 5.6 % of iPhone browsing goes the same path as Safari.*
- **T13: rVFC cost.**
  1. 10 min of 1080p native and MMS playback with and without an always-on rVFC loop.
  2. Report dropped frames and battery drain on the oldest supported phone (iPhone XS on 18.x).
  - *Settles whether the follower may keep rVFC on permanently.*

Device inventory that covers the ranges:

- an iPhone 8 or X (16.7.x, native only);
- an iPhone XS/XR (iOS 17.x and 18.x);
- a current iPhone on 26.x and 27.x;
- an iPad on iPadOS 15/16 (classic MSE);
- a Mac on Safari 17 and 26.
