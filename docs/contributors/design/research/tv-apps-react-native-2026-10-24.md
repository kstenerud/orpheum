> **ABOUTME:** Whether apps for TVs, built in React Native, would meet Orpheum's needs: what
> react-native-tvos, react-native-video and react-native-webrtc can and cannot carry of the web
> player, what stays web on Samsung and LG, how comparable products build TV apps, and a spike
> plan. Read before choosing a TV client technology or starting the TV client.

# TV Apps in React Native

- **Date:** 2026-10-24.
- **Requested by** the owner, 2026-10-24: "If I decide to just go apps for TVs, would React Native
  be enough for our needs?" Prompted by the owner's TCL Android TV, where in the TV Bro browser
  every film's sound came about 0.5 s late, uncompensated, on every path, while YouTube in the
  same browser was in time ([idiosyncrasies.md](../../idiosyncrasies.md#a-tcl-tv-in-tv-bro-plays-a-pages-sound-about-half-a-second-late-and-the-browser-cannot-tell)).
  The companion [youtube-tv-browser-av-sync-2026-10-24.md](youtube-tv-browser-av-sync-2026-10-24.md),
  written the same day, finds the likeliest cause in our routing of the sound through Web Audio,
  which Chromium does not count (its §4.1). §5 below takes that into account.
- **Builds on:** [client-framework.md](client-framework.md), [tvos.md](tvos.md),
  [shared-core.md](shared-core.md), [tech-clients.md](tech-clients.md),
  [app-stores.md](app-stores.md), the TV client plan [tv-client.md](../plans/tv-client.md), and
  [D10](../../decisions/working-notes.md), [D28](../../decisions/working-notes.md),
  [D29](../../decisions/working-notes.md), [D30](../../decisions/working-notes.md),
  [D63](../../decisions/working-notes.md) and [D193](../../decisions/working-notes.md) §8 (the
  TV framework is chosen by a paired prototype, Compose for TV against react-native-tvos, when TV
  work starts). This document re-reads them against October 2026 sources; it decides nothing.
- **Cache:** pages under `/yoloai/cache/http/rn-tv/` (`<name>.txt`, with `<name>.url` beside it).
  Shallow clones in `/yoloai/cache/git/`:

  | Repository | Commit | Date |
  | --- | --- | --- |
  | react-native-tvos | `e724549f` | 2026-10-20 |
  | react-native-video, `main` (v7.0.0-beta.12) as `rnv` | `a3cde3b2` | 2026-10-08 |
  | react-native-video, tag v6.19.3 as `rnv-v6` | `88c178ff` | 2026-09-18 |
  | react-native-webrtc | `fb662c9d` | 2026-10-20 |
  | androidx/media (Media3 1.11.1) as `media3` | `8c6678b6` | 2026-09-08 |
  | expo (sparse: docs) | `22bfe837` | 2026-10-24 |
  | expo/config-plugins | `8c15065e` | 2026-10-19 |
  | streamyfin | `d22458bd` | 2026-10-22 |
  | jellyfin-androidtv | `2be72193` | 2026-10-24 |
  | jellyfin-tizen | `31408c12` | 2025-10-24 |
  | jellyfin-webos | `ab479404` | 2025-11-03 |
  | jellyfin-expo (now `jellyfin-ios`) | `ceca8eb1` | 2026-10-01 |
  | Swiftfin | `b0cb3127` | 2026-10-24 |
  | stremio-video | `fa1f3267` | 2026-10-20 |
  | libass-android | `14d7ca5e` | 2026-10-19 |
  | NuvioTV | `b36db808` | 2026-10-24 |
  | swift-ass-renderer | `28919f6b` | 2024-12-21 |

- **Labels:**
  - **[1P]** the vendor's own documentation, source code or staff post.
  - **[U]** a user report: forum thread, issue.
  - **[J]** a journalist's, reviewer's or consultancy's article.
  - **[S]** second-hand: a search engine's summary of a page that could not be read in full.
  - **[I]** my inference. **Unverified** marks a claim no source here confirms.
  - **Judgement** marks a recommendation that is an opinion.
- **Method:** three research passes in parallel (platforms and frameworks; react-native-video and
  Media3 source; comparable products and the web-based TVs), then spot checks of the load-bearing
  claims in the clones. Nothing was run on a device.

---

## 1. Summary

**Short answer: React Native is enough for the shell on Android TV, Google TV and Fire OS, and
not enough for the player.** The UI, the SDK and most of Watch Together carry over; the player
does not, on any TV. Our player's distinguishing work (our own ABR, the WebRTC transport,
loudness gain, the signed sound offset, ASS) has to be rebuilt natively, in Kotlin on Media3,
whatever framework draws the screens. react-native-video as published does not take all of it:
it has hooks for a custom data source and a wrapped media source, but none for the audio sink or
the track selector (§4). This is the shape [D10](../../decisions/working-notes.md) and
[tv-client.md](../plans/tv-client.md) already chose: a React Native shell over a native player
module of our own.

**It does not cover every TV.** Apple TV runs react-native-tvos, but react-native-webrtc does not
install there today and AVPlayer will not take media segments from anything but HTTP (§4.2, §6).
Samsung and LG apps are packaged web apps, so they stay our web client (§7). The newest Fire TV
sticks run Vega OS and need Amazon's own React Native fork (§3.3).

**On the TCL problem, an app is probably not needed.** The companion research finds that
Chromium tells an element routed through Web Audio that its sound has no output delay. On this
TV, that route accounts for about 420 of the roughly 500 ms
([youtube-tv-browser-av-sync-2026-10-24.md](youtube-tv-browser-av-sync-2026-10-24.md) §1, §4.1).
YouTube's page never routes its sound that way, and plays in time. If the element's own path
keeps time, the web client is fixed in the web.

If it does not, a native Android player is the escape. It times its picture against the audio
output's own timestamps, which Android requires to be accurate to ±2 ms (§5). Two tests, one of
them with no code, settle which case holds before any app is written (§10.2, steps 0a and 0b).

**Recommendation (judgement):** do not go apps-only. Go apps where they pay:
- **Android TV first:** a react-native-tvos shell with our own Kotlin Media3 player module.
- **Apple TV:** native Swift, cheap by [D30](../../decisions/working-notes.md).
- **Samsung, LG and the browser:** keep the web client.

Run the spike in §10.2 first. Its steps 0a and 0b cost an evening and may change the question.

### 1.1 Our needs, against React Native on each TV

"Web today" is `web/packages/player` and `web/packages/sdk` at `2cbda629`.

| Need | Web today | Android TV: RN + react-native-video v6 | Android TV: RN + our own Kotlin Media3 module | tvOS: RN or Swift, AVPlayer |
| --- | --- | --- | --- | --- |
| HLS, fMP4, separate audio renditions | hls.js | Yes: Media3 `HlsMediaSource` [1P] | Yes | Yes: AVPlayer's native HLS (unverified for our ladder) |
| **Our own ABR** (D265) | `abr.ts`, an hls.js `AbrController` | **No hook for the track selector.** Only `selectedVideoTrack` overrides and `maxBitRate` (§4.1) | Yes: a custom `ExoTrackSelection` [1P]. It is a port of `abr.ts` to Kotlin, not a reuse | **No.** Only `preferredPeakBitRate` and a resolution cap. AVPlayer owns ABR [1P] |
| **Requests over WebRTC data channels** | `rtc-transport.ts`, a custom hls.js loader | A custom `DataSource` via the plugin hook (§4.1). The WebRTC must be native, not react-native-webrtc's JS channel (§6.1) | Same, without the hook | **Only through an in-app HTTP server** (§4.2), and react-native-webrtc does not install (§6.2) |
| **Loudness gain** (D226 §4) | Web Audio `GainNode` (`gain.ts`) | **No audio-sink hook**: needs a fork or a patch (§4.1) | Yes: `GainProcessor` in `DefaultAudioSink` [1P] | `MTAudioProcessingTap` works with HLS from the 2026 OSes [1P] |
| **Signed sound offset** | tfdt shift of sound fragments (`audio-shift.ts`) | Yes: a wrapping `MediaSource`, which NuvioTV ships (§4.1) | Same | Unverified: a tap can delay, and moving sound earlier needs a resource-loader rewrite, which segments forbid (§4.2) |
| **ASS subtitles** | JASSUB (libass) | Media3's own SSA parser only (basic styles, `\pos`, `\move`, `\an`) [1P] | libass via `ass-media`, as jellyfin-androidtv does [1P] | swift-ass-renderer (tvOS 15+, last commit 2024-12) [1P] |
| **Watch Together sync** | SDK over WebSocket | The SDK runs as is [I] | Same | Same [I] |
| **Watch Together voice** | WebRTC in the page | Runs on Android [1P], but a TV has no microphone an app may use (§6.3) | Same | No: the library does not install on tvOS (§6.2) |
| **A/V sync on the TCL** | 0.5 s late in TV Bro | Unknown until steps 0a and 0b of §10.2 | Same | n/a |

---

## 2. What would be reused

- **The SDK** (`web/packages/sdk`): 9,080 hand-written lines and 8,935 generated ones (counted at
  `2cbda629`, tests excluded).
  - It is plain TypeScript over `fetch`, WebSocket and `RTCPeerConnection`. Five files touch
    `window`, `document` or `navigator`: `page-audio.ts`, `playable-formats.ts`, `rtc-media.ts`,
    `together-lead.ts` and `together-voice-browser.ts`.
  - So the API client, discovery, device code, path keeping and Watch Together logic would run
    in React Native (unverified until built).
  - The browser-only parts (codec probing, page audio) need native counterparts. They are
    platform-specific by nature ([shared-core.md](shared-core.md) §1).
  - Signalling for the transport and for voice could use react-native-webrtc's
    `RTCPeerConnection` on Android. The media bytes should not (§6.1).
- **The player** (`web/packages/player`, 4,301 lines) **is not reused on any native TV.**
  - It is built on hls.js and MSE, which React Native lacks. Its ABR, stall handling, step-up
    and diagnosis would be ported to Kotlin against Media3's interfaces.
  - The tests travel as specifications, not code [I].
  - The one exception is Vega OS, whose player is `HTMLMediaElement` + MSE (§3.3), where hls.js
    may run (unverified).
- **The web app** (`web/apps/web`, about 27,000 lines) is React DOM. Its screens are not reused
  by React Native; they are rewritten as TV screens. They would be rewritten anyway for a ten-foot
  layout [I].

---

## 3. React Native on TVs: react-native-tvos, Expo, Vega

### 3.1 react-native-tvos

**Platforms.** "Apple TV and Android TV support for React Native are maintained here … This is a
full fork of the main repository" (README at `e724549f`) [1P].
- **Fire OS and Google TV are Android to it.** `UiModeUtils.kt` detects TV by
  `UI_MODE_TYPE_TELEVISION`, "(Android TV / Fire TV)" [1P].
- Issue #1162 (2026-09-26) reproduces a bug on a Fire TV Stick 4K and a Fire TV Cube [U].
- **Vega OS is not a target:** the tree has no "Vega" or "Kepler" (§3.3).
- **Minimums:** tvOS 15.1, Android API 24 [1P].

**Cadence and health** (npm registry and GitHub API, 2026-10-24) [1P]:

| Minor | React Native | react-native-tvos |
| --- | --- | --- |
| 0.83.0 | 2025-12-10 | 2025-12-12 |
| 0.84.0 | 2026-02-11 | 2026-02-13 |
| 0.85.0 | 2026-04-07 | 2026-04-10 |
| 0.86.0 | 2026-06-09 | 2026-06-13 |
| 0.87.0 | 2026-08-11 | 2026-08-16 |
| 0.88.0 | 2026-10-15 | 2026-10-17 |

- Each minor arrives 2 to 5 days after upstream. [D10](../../decisions/working-notes.md)'s
  bail-out trigger ("two minor versions behind upstream for a quarter") is not close.
- 16 open issues, 1,424 stars, 48 npm versions in the year.
- **Every one of the last 60 GitHub releases was published by `douglowder`.** His profile reads
  "Currently at @expo. Ported the React Native framework to the Apple TV platform." The npm
  maintainers also include `brentvatne` (Expo) and `yusufyildirim` (XITE).
- **The fork is Expo-backed, and its bus factor is about one person** [I].
- **Plex maintains a fork of its own:** `plexinc/react-native-tvos` [1P].

**New Architecture and Hermes.** "React Native TV fully implements the new architecture,
identically to the core repo"; "Hermes is fully working on both Apple TV and Android TV, and is
enabled by default" (README) [1P].

**Focus and the D-pad** (README; `Libraries/Components/TV/`) [1P]:
- `Pressable` and the touchables get `onFocus`/`onBlur` and select-button presses.
- `TVFocusGuideView` wraps `UIFocusGuide` on tvOS and is implemented alike on Android. Props:
  `destinations`, `autoFocus`, `trapFocus*`, `onFocusEnter`/`onFocusLeave`.
- `nextFocus*` now works on tvOS too; `hasTVPreferredFocus`; `useTVEventHandler` for remote keys.
- `TVEventControl` on Apple TV only, for the menu key and the pan gesture.
- `VirtualizedList` is wrapped in a focus guide that traps focus.

**Known gaps** [1P/U]:
- On tvOS the APIs Apple lacks are suppressed: "web views, sliders, switches, status bar, etc."
- A Podfile takes either an iOS target or a tvOS target, not both.
- On Apple TV a `ScrollView` does not scroll without focusable items in it.
- Open focus regressions on Android TV:
  - #1087: focus navigation is bypassed inside ScrollView and FlatList since 0.80.
  - #1058: a disabled `Pressable` becomes unfocusable in 0.84.
  - #1162: focus oscillates at about 5 Hz on Android TV and Fire OS.
- #1131 (2026-06-23, no replies): on tvOS with Expo SDK 54 in bridgeless mode, "every autolinked
  third-party native module is `null` at runtime".
- **Focus is still where React Native on TV is weakest**, as
  [tv-client.md](../plans/tv-client.md) predicted. The prototype it asks for (D-pad press to
  selection moved, on the worst device) is still the test.

### 3.2 Expo

- Expo builds TV apps with `@react-native-tvos/config-tv`, applied when `EXPO_TV=1` or
  `isTV: true`, then `npx expo prebuild --clean`
  (`expo/docs/pages/guides/building-for-tv.mdx`, last changed 2026-10-15) [1P].
- From SDK 56, "upgrading your project to a newer SDK version also upgrades the TV repo
  dependency." SDK 58 is on React Native 0.88.0 [1P].
- **Works on TV** (Expo's list includes): Video, Audio, Image, SQLite, SecureStore, FileSystem,
  NetInfo, KeepAwake, Updates, Reanimated, FlashList, Svg, Skia and the dev client (from SDK 54)
  [1P].
- **Without `tvos` in its docs' platforms:** webview, camera, notifications, gesture-handler,
  clipboard, web-browser, auth-session, slider, picker and others [1P]. For a TV app the loss
  that matters is the web view [I].
- **EAS** builds TV profiles, but "tvOS does not use the same provisioning profiles as iOS …
  EAS tooling … only creates iOS provisioning profiles": the profile is made by hand [1P].
- The config-tv README still says the dev client and Expo Router are unsupported. The current
  Expo guide contradicts it, and there is a `with-router-tv` example [1P].

### 3.3 Fire TV is now two targets

- **Vega OS is Linux:** "We adopted Linux components as our foundation … We're using React
  Native 0.72" (Amazon developer blog, 2025-09-30) [1P].
- Its app frameworks are Amazon's own fork: "React Native (RN) for Vega is an out-of-tree fork …
  incorporates most of the React Native tvOS capabilities" (vega-rn-arch, page dated 2026-10-20)
  [1P].
- React Native 0.83 is "in early access" in SDK 0.24 (2026-10-20) [1P].
- **Vega's player is a W3C stack:** "The Vega Media Player supports HTMLMediaElement … MSE …
  EME … runs on top of the GStreamer framework". Its guide uses Shaka but allows "HLS.js,
  Dash.js" [1P].
- **No sideloading** ([app-stores.md](app-stores.md) §4).
- **Which devices run which:**
  - Vega: the Stick 4K Select (2025), the Stick HD (2026) and the refreshed Stick 4K (2026).
  - Fire OS: the Stick 4K Max and the Cube still run Fire OS (XDA, 2026-09-30) [J].
- **A react-native-tvos app reaches Fire OS devices only** [I]. Vega is a third port. Its MSE
  player may make it closer to our web player than to our Android app (unverified).

---

## 4. react-native-video

- **Versions:** npm `latest` is 6.19.3 (2026-09-18) and `beta` is 7.0.0-beta.12 (2026-10-06)
  [1P].
- **v7 is not stable:** the docs' status timeline marks "Stable" as upcoming. It is rebuilt on
  Nitro Modules [1P].
- **The paid add-ons** (offline SDK, chapters, uploader) touch none of our needs [1P].

### 4.1 Android (Media3)

- **Media3:** v6.19.3 defaults to Media3 1.8.0 (`android/gradle.properties:7`, overridable); v7
  defaults to 1.4.1 [1P].
- **HLS:** HLS goes through `HlsMediaSource.Factory(dataSourceFactory)`
  (`ReactExoplayerView.java:1166-1180`) [1P].
- **Tracks:**
  - `selectedAudioTrack` is supported. `selectedVideoTrack` is Android only, and is applied as a
    `TrackSelectionOverride` on a `DefaultTrackSelector` built with
    `AdaptiveTrackSelection.Factory()` (`:808-811`, `:2289-2475`).
  - Selection is ignored while `controls` is on (`:2292-2294`).
  - `maxBitRate` maps to `setMaxVideoBitrate`.
  - `bufferConfig` feeds a `DefaultLoadControl` [1P].

**The v6 plugin interface** (`RNVExoplayerPlugin.kt`) [1P] exposes:
- `overrideMediaDataSourceFactory` (`:39`): **our WebRTC transport fits here.**
- `overrideMediaSourceFactory` (`:49`): **our offset fits here**, as a wrapping `MediaSource`.
- `overrideMediaItemBuilder`, DRM hooks, `shouldDisableCache`.
- `onInstanceCreated(id, player: ExoPlayer)` (`:75`).
- The docs call it "an **experimental API**". "The player handle should be treated as
  **read-only**" (`docs/pages/other/plugin.md:21,24`).

**What v6 does not expose** [1P]:
- No hook for the `TrackSelector`, the `LoadControl`, the `RenderersFactory`/`AudioSink` or the
  `BandwidthMeter`. The player is built inside (`:842-846`); `ReactExoplayerConfig` supplies only
  a bandwidth meter, an initial bitrate and the load-error policy.
- Grepping both versions for `tunnel`, `AudioProcessor` and `AudioSink` finds nothing.
- **So our ABR and our gain need a fork, a patch or our own module** [I].
- libass-android's `ass-media` also needs `ExoPlayer.Builder.buildWithAssSupport(...)`, which
  replaces the renderers factory at build time [1P]. That is a third reason.

**v7** has `getMediaDataSourceFactory` and `getMediaSourceFactory`, and a public
`HybridVideoPlayer.player: ExoPlayer`. It still has no renderers or load-control hook
(`HybridVideoPlayer.kt:219-258`) [1P].
- Its native spec has no audio or video track selection: only `VideoPlayer.web.ts` has them.
- Issue #4867 ("[v7] Misisng track selectors") was closed "completed" on 2026-06-21, though no
  native implementation exists at `a3cde3b2` [1P/U].
- **v7 is weaker for us than v6 today** [I].

**Media3 itself can do everything we need** [1P]:
- **Transport:** a custom `DataSource`/`DataSource.Factory` passed to `HlsMediaSource.Factory`
  (Media3 customization guide).
- **ABR:** an injected `TrackSelector` with a custom `ExoTrackSelection.Factory`.
- **Gain:** `DefaultRenderersFactory.buildAudioSink` overridden with
  `DefaultAudioSink.Builder.setAudioProcessors(...)` (`DefaultRenderersFactory.java:938`,
  `DefaultAudioSink.java:338`); `GainProcessor` ships in `libraries/common`.
- **Offset:** Media3 has no user A/V offset API. A react-native-video maintainer on #4128: "I
  don't see such possibility" [U].
  - NuvioTV, an Android TV app, does it the way our web player does: a `WrappingMediaSource`
    that shifts the sound's sample times, `buffer.timeUs -= audioDelayUsProvider()`
    (`AudioDelayMediaSource.kt:169`).
  - Its gain is a `buildAudioSink` override with `setAudioProcessors(arrayOf(gainAudioProcessor))`
    [1P].
  - Shifting samples on the device moves sound earlier as well as later, including for **direct
    play**, which our web player cannot do [I].

**Subtitles:**
- v6 side-loads SubRip, TTML and VTT; v7 maps `"ssa","ass"` to `MimeTypes.TEXT_SSA`
  (`MediaItemUtils.kt:70`) [1P].
- Media3's SSA parser handles basic styles and `\pos`, `\move` and `\an` only (`SsaStyle.java`)
  [1P]. That is not libass, and typeset anime ASS would render wrong [I].
- jellyfin-androidtv gets libass from `io.github.peerless2012:ass-media` 0.5.1 on Media3 1.11.1
  (`gradle/libs.versions.toml`) [1P].

### 4.2 iOS and tvOS (AVPlayer)

**v6:**
- The podspec declares `:tvos => "13.0"` [1P].
- `maxBitRate` sets `preferredPeakBitRate` (`RCTVideo.swift:961-968`).
- Audio is chosen by media selection; `selectedVideoTrack` is Android only.
- Side-loaded text tracks: "This feature does not work with HLS playlists (e.g., m3u8) on iOS"
  (`props.mdx:1192`) [1P].
- `RNVAVPlayerPlugin.overridePlayerAsset` lets a plugin hand over its own `AVURLAsset` with a
  resource-loader delegate [1P].

**v7:** its podspec declares **only `:ios`** (`ReactNativeVideo.podspec:16`), though its docs'
title names tvOS [1P]. tvOS support in v7 is unverified.

**Media segments cannot be fed from a resource-loader delegate.** An Apple Systems Engineer on
Apple developer forum thread 113063 [1P]:
- "The only response to an AVAssetResourceLoadingRequest for a segment (.ts) file that AVPlayer
  will accept is a redirect to an HTTP URL. respondWithData will be rejected".
- The same holds for "any media segment (TS, fmp4, packed audio, WebVTT, whatever)".
- Playlists can be rewritten that way; segments cannot.
- (This is a WebFetch summary; curl met a bot check.)

**So on Apple TV the WebRTC transport needs an in-app HTTP server on localhost** that carries
each request over the data channels [I]. The candidates are thin:
- GCDWebServer, archived since 2022.
- Telegraph, last push 2024-06.
- `@dr.pogodin/react-native-static-server`, which serves static files [1P metadata].
- A small server of our own on `Network.framework` is likely the answer (unverified).

**Gain on Apple:** "Instances of MTAudioProcessingTap can now process a mix of audio output from
any AVPlayerItem ... with the exception of FairPlay-protected audio" (Apple, "What's new in HLS",
WWDC 2026) [1P]. The OS versions this needs are unverified.
- An earlier sound offset is the hard one on Apple: AVPlayer owns the timing, and a tap can only
  delay [I].
- A playlist rewrite through the resource loader could apply an offset to the audio rendition's
  timeline only if AVPlayer honoured it per rendition, which nothing here confirms (unverified).

**ABR on Apple:** AVPlayer chooses the variant itself; the app sets a peak bitrate and a maximum
resolution [1P]. Our ABR does not run on AVPlayer. This is the same limit the web player meets
on Safari's native HLS (idiosyncrasies.md, "Native HLS gives the page no track choice …").

---

## 5. Why a native app might not have the TCL lag

**What we know** (owner, 2026-10-24):
- On the TCL in TV Bro the sound is about 0.5 s late, constantly, on the ladder and on direct
  play.
- The chunks are in step, and so are the browser's buffers.
- `requestVideoFrameCallback` puts frames within 10 to 20 ms of the element's sound.
- YouTube's page in the same browser is in time.
- The Web Audio context reports a base latency of 96 ms and an output latency of 328 ms, which
  the element's clock does not count.

**How Android's native players keep sync** [1P]:
- Android requires "The output timestamp returned by AudioTrack.getTimestamp … is accurate to
  +/- 2 ms" of any device that declares audio output (CDD §5.6, C-1-1).
- Media3's `AudioTrackPositionTracker` reads `getTimestamp` and `getLatency`.
- The video renderer releases each frame against that audio position, so whatever latency the
  TV's audio path reports is absorbed by holding the picture back [I, from the Media3 sources'
  design].
- In **tunneled** playback the TV's own hardware does this: it times frames against "an
  AudioTrack clock synchronized to audio presentation timestamps passed in by the app" (AOSP,
  multimedia tunneling) [1P].
- Media3 offers it as `setTunnelingEnabled`, warning it "is known to have many device specific
  issues and limitations" (`DefaultTrackSelector.java:740-743`) [1P].
- Audio processors are allowed in tunnel mode if they keep durations
  (`DefaultAudioSink.java:88-91`) [1P], so a gain does not rule it out.

**Why the browser path lags.** The companion research reads Chromium's
`WebAudioSourceProviderImpl`: an element routed through `createMediaElementSource` is rendered
with a delay of zero. Its clock then counts none of Web Audio's latency, while its sound waits
through all of it
([youtube-tv-browser-av-sync-2026-10-24.md](youtube-tv-browser-av-sync-2026-10-24.md) §4.1)
[1P there]. Without Web Audio, Chromium's own audio path uses the sink's reported delay, as a
native app would (same, §4.2) [S there].

**What is not known** [I]:
- Whether the element's own path keeps time on this TV. The only clip that measured without
  aliasing (the clip beeping every four seconds) went through Web Audio.
- **If that path keeps time, the web client is fixed by keeping the sound off Web Audio**:
  afcfe540 does this without levelling, and 2d3545ea counts the context's latency with it. A
  native app then buys nothing for sync.
- **If that path lags, the TV's audio HAL likely under-reports its latency** (unverified). A
  native ExoPlayer app on the same HAL might lag as well, and only the per-device offset helps.
  YouTube's page keeping time on the element's path argues against this case.
- Both are cheap measurements away from known: §10.2, steps 0a and 0b.

---

## 6. WebRTC: the transport and voice

### 6.1 The transport on Android: native, not through JavaScript

- react-native-webrtc 155.0.1 (2026-10-20) supports data channels on Android and iOS, not tvOS
  (README feature table) [1P].
- **Binary messages cross the bridge as base64:**
  - `RTCDataChannel.ts:160` sends `base64.fromByteArray(data)`.
  - `:207` decodes received data with `base64.toByteArray`.
  - `DataChannelWrapper.java:78` encodes received bytes with `Base64.encodeToString` [1P].
- A film carried that way would be base64-encoded per message, handed to JavaScript, decoded,
  and then handed back to a native `DataSource` for Media3 [I].
- Our transport already needs four peer connections to carry a film (`clientConnections`,
  `rtc-transport.ts`). **The media path should therefore be WebRTC in Kotlin, inside the
  `DataSource`** [I], on Google's `libwebrtc` for Android or a maintained build of it
  (unverified which).
- That means reimplementing the framing in `rtc-transport.ts` (text head, binary body in
  messages of at most 64 KiB, an empty message ending it, one channel per request) in Kotlin,
  against the core's `rtcapi`. Signalling through discovery could stay in the TypeScript SDK and
  pass the session to native, or move too (unverified which is simpler).

### 6.2 tvOS

- react-native-webrtc's podspec declares tvOS 16. But it depends on `JitsiWebRTC ~> 155.0.0`,
  whose podspecs declare only iOS and macOS [1P].
- Issue #1597 (opened 2024-07-22, open): "`JitsiWebRTC (124.0.0)`, which does not support
  `tvOS`". The maintainer, 2024-07-23: "The WebRTC framework that has tvOS support is not
  provided by us since the code is not upstream yet"; 2025-04-14: "No updates" [U/1P].
- Under `TARGET_OS_TV`, `getUserMedia` returns "getUserMedia is not supported on tvOS."
  (`WebRTCModule+RTCMediaStream.m`) [1P].
- **On Apple TV, any WebRTC means a WebRTC framework we build for tvOS ourselves** [I].
  [D62](../../decisions/working-notes.md) already keeps Watch Together's sync off data channels
  for this reason.

### 6.3 Microphones on TVs

**Android TV.** Android lists `android.hardware.microphone` among the features TVs do not
support (TV hardware guide, updated 2026-10-23) [1P]:
- "Some TV controllers have a microphone, which is not the same as the microphone hardware
  feature … The controller microphone is fully supported."
- Google's own Meet: "You can't use the Android TV remote for audio on Meet calls", with a USB
  microphone as the way (Meet help 13420160) [1P].
- A Philips Google TV user: "the remote's mic is reserved for Google Assistant" (Kellies-Voice
  #71, 2026-10-23) [U].
- The Expo WebRTC plugin adds `RECORD_AUDIO` and `CAMERA` without `required="false"`. A TV
  build must override the manifest or be hidden from TVs on Play [1P/I].

**Apple TV.** tvOS 17 added Continuity Camera and Mic for third-party apps
(`AVContinuityDevicePickerViewController`, `AVCaptureDevice` on tvOS 17+; WWDC23 session 10256:
"your app will need to adopt Device Discovery") [1P]:
- Apps may use an iPhone or iPad microphone, or a paired Bluetooth headset.
- None of the sources lists the Siri Remote's microphone as an input [S].

**This confirms [D63](../../decisions/working-notes.md):** on a TV, the phone is the voice
endpoint. An app changes nothing here except that a paired headset becomes possible on Apple TV
17+ [I].

---

## 7. What stays web: Samsung, LG, and the browsers

Samsung Tizen (`.wgt`) and LG webOS (`.ipk`) apps are packaged web apps [1P]. Their engines
by model year:

| Year | Samsung Tizen | Chromium | LG webOS | Chromium |
| --- | --- | --- | --- | --- |
| 2020 | 5.5 | M69 | 5.x | 68 |
| 2021 | 6.0 | M76 | 6.x | 79 |
| 2022 | 6.5 | M85 | 22 | 87 |
| 2023 | 7.0 | M94 | 23 | 94 |
| 2024 | 8.0 | M108 | 24 | 108 |
| 2025 | 9.0 | M120 | 25 | 120 |
| 2026 | 10.0 | M130 | 26 | 132 |

(Samsung web-engine specifications; LG web API and web engine page [1P].) A model keeps the
engine of its year [J, Dolby OptiView].

**Samsung:**
- MSE on every year; Web Audio from 2016; the WebRTC API "Yes" from 2020 (Tizen 5.5) [1P].
  Whether data channels work is unverified.
- AVPlay, an `<object>`-based native player, is Samsung's route "to play media content with
  features not supported in HTML5". It draws outside the DOM [1P]. Our hls.js player would run
  on MSE instead (unverified per model).
- jellyfin-web skips hls.js on Tizen and webOS, because "The native players on these devices
  support seeking live streams" (`htmlMediaHelper.js:54`) [1P].
- Shaka: "Tizen 2017 model is actively tested" [1P].

**LG:**
- **WebRTC is closed to ordinary apps.** LG staff, 2026-01-07, to someone wanting receive-only
  playback with no camera: "WebRTC is only available on contracted partner apps" (LG developer
  forum 28059) [U, LG staff].
- On LG, our WebRTC transport and voice cannot run in a packaged app. An LG app reaches a core
  only by HTTPS [I].
- "The webOS TV platform does not support cookies on packaged web apps (file scheme)" [1P].
- webOS is "expected to work, but … untested" by Shaka [1P].

**A/V sync on these:**
- Neither vendor documents A/V sync or audio latency for developers (searched 2026-10-24).
- Both have user-side lip-sync settings, such as LG's "AV Sync Adjustment" [S].
- **The TCL-style lag could therefore recur on these TVs with no native escape**, and only our
  per-device offset would answer it [I].

**React Native there:**
- Samsung's `react-native-tizen-dotnet` repository is gone; its last release was 0.1.14 (April
  2019) [S].
- The working routes are react-native-web builds (ReNative) [1P], or Callstack's consultancy
  rebuild, which reports TTI "15.27s → 10.9s" after a React Native Web rewrite with ">20s load
  time" failed (Callstack case study, 2025) [J, vendor].
- For us, a Tizen or webOS app is our web client packaged, as jellyfin-tizen and jellyfin-webos
  are ("a small wrapper around the web interface") [1P].
- Flutter is officially available for webOS TV 26 and later [1P, LG news headline].

---

## 8. How comparable products build TV apps

| Product | TV app technology | Player | Source |
| --- | --- | --- | --- |
| **Plex** (new experience) | React Native, "a whole new code base from the ground up"; reached Android TV stable as 2026.19.1 | A native "PlexPlayerSDK across Swift and Kotlin" | Lowpass 2025-01-06 [J]; Plex forum 943951, 2026-10-06 [U]; former Plex engineer's page [U]; `plexinc/react-native-tvos` [1P] |
| **Jellyfin, Android TV** | Kotlin, Leanback 1.2.0 with Compose 1.12.1 | Media3 1.11.1, FFmpeg decoder, libass via `ass-media` 0.5.1 | `jellyfin-androidtv` `libs.versions.toml` [1P] |
| **Jellyfin, Tizen/webOS** | jellyfin-web packaged | The TV's native `<video>`, no hls.js | `jellyfin-tizen`, `jellyfin-webos`, `htmlMediaHelper.js:54` [1P] |
| **Swiftfin** (tvOS) | Swift | VLC (recommended) or AVPlayer; VLC has a "known bug that results in a significant audio delay"; frame-rate matching only on AVPlayer | `Documentation/players.md` [1P] |
| **Streamyfin** | Expo 57 + `react-native-tvos@0.86.0-2`, with `:tv` build scripts | Native modules: `mpv-player` (libmpv) and `exoplayer-player` (Media3) | `package.json`, `modules/` [1P]; no TV store listing seen |
| **jellyfin-expo** | Expo 46, React Native 0.69, a web view around jellyfin-web (now `jellyfin-ios`) | The web's | repository [1P] |
| **Emby** | The dedicated Android TV app is no longer developed; the universal Android app gains a TV UI | not checked | Emby forum [U] |
| **Infuse** | Native, Apple only | Its own, "sits on top of … FFmpeg", since 2010 | Firecore staff, 2022 [U] |
| **Kodi** | C++ | Its own VideoPlayer, MediaCodec on Android | [S] |
| **Stremio** | Rust core (`stremio-core`); web backends for Tizen (`webapis.avplay`) and webOS (`luna://com.webos.media`); the Android TV app's technology not verified | Per platform | `stremio-video` [1P] |
| **YouTube** | Cobalt: "a lightweight application container … compatible with a subset of the W3C HTML5" | Cobalt's own | `youtube/cobalt` README, 25.lts [1P] |

**Others using React Native on TV in production:**
- Zattoo moved from native to React Native (Callstack webinar, 2026-04-27) [J].
- TF1 kept memory under 200 MB on its target device after it had reached 360–401 MB and crashed:
  "TV users never kill their application" (React Native Connection 2024) [J].
- Amazon measured "~1000ms improvement" from the New Architecture on a Fire TV Stick HD (dev.to,
  2025-02-06) [1P-ish].
- No credible source was found for Max, Paramount, BBC, F1 TV or Tubi.

**The pattern** [I]:
- The products that care most about playback (Plex, Streamyfin, Jellyfin, Infuse) put a native
  player of their own under whatever UI.
- Plex, the largest, chose React Native for the UI on every TV but the web ones, and wrote its
  player natively.
- None of them uses the stock react-native-video as its player.

---

## 9. Alternatives, and what each reuses of the TypeScript

| Option | TVs reached | Reuse of `web/packages/sdk` | Player | State, 2026-10 |
| --- | --- | --- | --- | --- |
| **react-native-tvos + our Kotlin/Swift player modules** | Android TV, Google TV, Fire OS, Apple TV | **Direct**, apart from the browser-only parts (§2) | Native per platform | §3.1 |
| **Kotlin + Compose for TV + Media3** | Android TV, Google TV, Fire OS | None at run time: the API client generated in Kotlin (D29); the together and transport logic ported | Media3 directly, no bridge | tv-material 1.1.0 and tv-foundation 1.0.0 (2026-05-06); Leanback: "deprecated. Use Compose for TV instead." [1P] |
| **Swift + SwiftUI/AVKit** | Apple TV (and iOS for about 3% more, D30) | None: generated Swift client, ported logic | AVPlayer, or libmpv/VLCKit | [tvos.md](tvos.md) |
| **Kotlin Multiplatform** (logic) + native UI | All native targets | None: the SDK would be rewritten in Kotlin once for Android and Apple. Kotlin/Native's `tvosArm64` is Tier 2 [1P] | Native per platform | KMP for logic only; Compose Multiplatform has no tvOS: JetBrains closed a tvOS PR on 2026-10-09, "We are currently out of capacity to support additional platforms" [1P] |
| **Flutter** | Android TV with gaps ("tooling, I/O, navigation, focus, core design language", flutter/flutter #180542) [1P]; tvOS only by a community fork; webOS by LG's SDK | None | Plugins over ExoPlayer/AVPlayer | Not a fit, as [D28](../../decisions/working-notes.md) found |
| **Packaged web client** | Tizen, webOS, and Vega's WebView | **All of it, player included** | hls.js on MSE | §7 |

**Reuse is the deciding difference** [I]:
- React Native keeps the TypeScript SDK running as is, including Watch Together's state
  machines, roughly half of the 9,080 hand-written SDK lines by file count (unverified by line).
- Compose for TV and Swift rewrite it per platform, on a generated API client.
- Neither reuses the player, which is the hard part.
- [D29](../../decisions/working-notes.md)'s argument (most of the shared logic is generated or
  server-side) predates Watch Together, which put a large client-side state machine into the
  SDK. That makes React Native's reuse worth more now than when D29 was written.

---

## 10. Recommendation

### 10.1 Answer and reasoning (judgement)

**Is React Native enough?**
- **For the screens on Android-family TVs, yes.** react-native-tvos tracks upstream within days
  and has Expo behind it. Plex shipped exactly this to Android TV this month. Our SDK runs in it.
- **For the player, no:** neither React Native nor react-native-video carries our player.
  Whatever framework is chosen, the TV client's cost is a Kotlin Media3 module of our own:
  - a track selector running our ABR
  - a `DataSource` carrying requests over native WebRTC
  - a `GainProcessor` for levelling
  - a wrapping `MediaSource` for the offset
  - libass via `ass-media`
- react-native-video is fine for the first spike and should not be the product's player: it has
  no hook for three of those five [I].

**Apps only, everywhere, is not reachable with React Native:**
- **Apple TV:** react-native-tvos works there, but its player needs an in-app HTTP server for the
  transport, and WebRTC needs a self-built framework. Writing the Apple player in Swift is
  required anyway, and by [D30](../../decisions/working-notes.md) the Apple TV UI in Swift is
  nearly free beside it. **React Native or SwiftUI on tvOS is then a UI choice that can wait for
  the Android result.**
- **Samsung and LG:** apps there are our web client packaged. LG gives such apps no WebRTC.
  Their store certification is the "app store hell" the owner deferred
  ([milestone-reassessment-2026-10-18.md](milestone-reassessment-2026-10-18.md), owner decision
  4).
- **Vega Fire TV sticks:** a separate React Native fork with an MSE player.

**So the sound plan is an Android TV app, sideloaded first** ([D108](../../decisions/working-notes.md)'s
versioned APKs; Google's developer verification still applies, [app-stores.md](app-stores.md)
§4), beside the web client, which stays the answer for every other TV.

**The TCL's lag should not be the reason for it.** If step 0a shows the web keeps time without
Web Audio, the lag is fixed in the web. If step 0b shows a native player lags too, no app fixes
it. An app is worth building for what the web cannot do well on a TV: a launcher entry, the
remote's media keys, frame-rate matching and HDR, and Fire OS sticks whose browsers are weak.
Those reasons are judgement, not measured here.

[D193](../../decisions/working-notes.md) §8's paired prototype (Compose for TV against
react-native-tvos) still holds. The player module is the same Kotlin in both, so the pairing
compares only focus and screens, and can be small.

### 10.2 Spike plan

Each step names what it must show. Steps 0a, 0b and 1 need the owner's TCL.

- **Step 0a: the web without Web Audio** (minutes, on the next TV visit). Play either
  sync-test clip in TV Bro: offset 0, levelling off, in the bare player and on the ladder after afcfe540.
  - This is the test of
    [youtube-tv-browser-av-sync-2026-10-24.md](youtube-tv-browser-av-sync-2026-10-24.md) §6.3.
  - **In time:** the web is fixed in the web, and the rest of this plan is about an app's other
    merits.
- **Step 0b: zero code: does native play in time on this TV?** (one evening)
  - Play "the sync-test clip (2026).mp4" and the same title's ladder (`master.m3u8` over plain HTTP at
    home) in an ExoPlayer-based app on the TCL. Candidates, all unverified: the Media3 demo app
    built from the `media3` clone, opened with
    `adb shell am start -a androidx.media3.demo.main.action.VIEW -d <url>`; or Jellyfin for
    Android TV playing the file from a Jellyfin library.
  - Also compare YouTube's own app with YouTube in TV Bro, and play a progressive H.264+AAC MP4
    in TV Bro (already owed).
  - **Pass:** the native app is in time to within what the owner can see.
  - **Fail:** it lags like TV Bro. Then the TV's audio path likely under-reports (unverified), an
    app does not help, and the offset is the fix on every path.
- **Step 1: react-native-tvos + Expo + react-native-video v6, plain HTTP.** (2–3 days, rough)
  - An Expo SDK 58 app with `EXPO_TV=1`, sideloaded by `adb`. Sign in with a token pasted in,
    then play the ladder from the production NAS.
  - Check: separate audio renditions and the `-raw` sound are chosen; video steps through the
    ladder; A/V offset on the sync-test clip; and [tv-client.md](../plans/tv-client.md)'s number, D-pad
    press to selection moved, on a poster grid on the TCL and on the oldest Fire TV stick
    running Fire OS.
  - Try `setTunnelingEnabled` through the `onInstanceCreated` handle as a variant (unverified
    that it takes effect after the player is built).
- **Step 2: the WebRTC transport, native.** (4–7 days, rough)
  - A react-native-video v6 plugin whose `overrideMediaDataSourceFactory` returns a `DataSource`
    carrying each request over data channels from Kotlin libwebrtc, framed as `rtc-transport.ts`
    frames it, to the core's `rtcapi`.
  - Signalling comes from the TypeScript SDK through react-native-webrtc or a small native
    bridge.
  - **Pass:** a film plays from staging with no HTTPS address reachable, at the bitrates the web
    transport reaches ([reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md)
    §9.4 has the web's numbers).
- **Step 3: own the player.** (5–8 days, rough)
  - Replace react-native-video with our own Kotlin module: an Expo module, or a Nitro or Turbo
    module, with no React Native types at its core ([D28](../../decisions/working-notes.md)
    rule 1).
  - It adds a `GainProcessor` fed by the core's gain, the NuvioTV-style wrapping `MediaSource`
    for the signed offset, `ass-media` for an ASS film, and a first `ExoTrackSelection` that
    applies `abr-choice.ts`'s rules.
  - **Pass:** levelled and offset playback of a typeset ASS film on the TCL.
- **Step 4: decide.** The owner chooses React Native or Compose for TV for the screens, from
  steps 1 and 3 and a Compose for TV poster grid on the same devices (2 days, rough). The player module
  carries over either way.

### 10.3 Effort, if the spike passes

All rough, in focused weeks of the owner with agents, **unverified estimates** by analogy with
the web client's history:

| Piece | Estimate |
| --- | --- |
| Android TV app at the web client's Cut A feature level: browse, play, resume, subtitles, levelling, offset, ABR port, TV sign-in by code (D93), Watch Together following (no voice on the TV) | 6–10 weeks |
| Store listing for Google Play and Amazon (Fire OS), with the reviewer demo server tv-client.md requires | 1–2 weeks of work, plus review time |
| Apple TV: Swift AVPlayer module, local HTTP bridge for the transport, a tvOS WebRTC build, UI in react-native-tvos or SwiftUI | 5–9 weeks |
| Tizen and webOS packaging of the web client, per platform | 1–2 weeks each, plus certification measured in months for Jellyfin ([app-stores.md](app-stores.md) §3) |
| Vega OS | not estimated: a separate React Native fork, MSE player unverified |

---

## 11. Not tried

- **Nothing was run on a device**, the TCL included. Every playback, sync and focus claim is from
  documents and source.
- Whether Media3 on this TCL plays in time; whether the TCL's HAL reports its output latency
  honestly; tunneled playback on it. All are steps 0a, 0b or 1.
- Separate EXT-X-MEDIA audio renditions of our ladder in Media3 and AVPlayer: not played.
- react-native-video v6 with Media3 1.11; v7 on tvOS; `setTunnelingEnabled` after construction.
- react-native-webrtc data channel throughput on Android; Kotlin libwebrtc builds and their
  maintenance; a tvOS WebRTC framework build.
- The `MTAudioProcessingTap` with HLS on 2026 OSes, and whether an earlier sound is possible on
  AVPlayer at all.
- Samsung WebRTC data channels; hls.js on Tizen and webOS model years; Vega's MSE with hls.js.
- Plex's player internals, Emby's TV stack, and the Stremio Android TV app's technology:
  unverified.
- Memory and focus latency of a react-native-tvos app on a low-end Fire OS stick.
- The effort estimates are analogies, not measurements.
