> **ABOUTME:** Player engines and the client-platform matrix — where the real cost of a media
> server project lives, what each platform demands, and the one architecture that lets a small
> team cover several platforms without writing the player several times.

# Technology: Clients

**Clients are the project.** The survey evidence is unambiguous: people stay on Plex for its
apps, leave Jellyfin over its apps, and the Jellyfin clients they actually recommend are
third-party. A better server with a worse TV app loses. Budget accordingly.

## 1. The player engine question

Any client has to decide what decodes and renders the video. There are four real options and
the choice is per-platform, not global.

| Engine | Codec/container breadth | Subtitles | Where it fits |
| --- | --- | --- | --- |
| **libmpv** | Best in class — FFmpeg's full demuxer/decoder set | **libass native**, PGS/VobSub, embedded fonts | Desktop, and increasingly mobile |
| **libVLC** | Very broad | Very good, "legendary" for subtitles | Where mpv is awkward to embed; Swiftfin uses VLCKit |
| **ExoPlayer / Media3** | Android's platform set only | Weak for ASS; needs help for bitmap subs | Android when hardware decode and platform integration matter most |
| **AVPlayer** | Apple's set only (needs HLS/fMP4) | Limited | tvOS/iOS when you want the platform's PiP, AirPlay, Now Playing, and power efficiency |
| **`hls.js` / Shaka / native `<video>`** | Browser's set | Needs `jassub` for ASS | Web, and the HTML-based TV platforms |

**The important observation:** the engines with the best format coverage (mpv, VLC) are *not*
the ones with the best platform integration (ExoPlayer, AVPlayer). This is the real tradeoff,
and the incumbents split on it — Jellyfin's Android TV app uses ExoPlayer and its top-voted
issue is a playback rewrite; Swiftfin uses VLCKit "to maximize direct play"; Jellyfin's desktop
client is Qt WebEngine wrapping the web UI with **libmpv** underneath.

**Recommendation: support two engines per client and let the user choose**, with a native
platform engine as the default (best battery, best PiP/AirPlay/Now Playing/HDR passthrough)
and libmpv as the "play anything" fallback. Streamyfin already ships exactly this — its
`modules/` directory contains both an `exoplayer-player` and an `mpv-player` native module,
with `MPVLayerRenderer`, `PiPController` and `MPVNowPlayingManager` on the Apple side. That is
the reference implementation for this recommendation.

## 2. The client architecture that makes this affordable

**React Native for the UI shell; native modules for the player.**

This is the pattern that both Kyoo and Streamyfin independently arrived at, and it is the only
one that gets a small team onto phone, tablet, TV and web without writing the app four times:

```
┌──────────────────────────────────────────────────────┐
│ Shared TypeScript: API client (generated from        │
│ OpenAPI), library browsing, search, settings,        │
│ playback queue, offline state, focus/remote nav      │
├──────────────────────────────────────────────────────┤
│ Native player module per platform (Swift / Kotlin)   │
│   iOS/tvOS : AVPlayer + libmpv                       │
│   Android/TV: Media3 + libmpv                        │
│   Web       : hls.js + jassub                        │
└──────────────────────────────────────────────────────┘
```

Why the player must be native rather than shared: it needs the platform's hardware decoder,
surface/layer compositing, picture-in-picture, background audio, Now Playing / media session,
HDR passthrough, and audio-device routing. None of that is expressible in shared JavaScript,
and every attempt to abstract it produces the worst of both.

**TV specifics.** `react-native-tvos` maintains tvOS and Android TV support (latest release
tracks React Native 0.85; `@react-native-tvos/config-tv` is the Expo integration). It is a
community fork, not first-party React Native — a real risk to record. Streamyfin runs it in
production with an `EXPO_TV=1` build variant and TV-specific native modules
(`tv-recommendations`, `tv-search`, `tv-user-profile`, `top-shelf-cache`), which is good
evidence the path works. **Remote-control focus management is the hard part of a TV UI**, not
layout, and it is where a web-in-a-webview TV app feels wrong.

## 3. Platform matrix and what each actually costs

| Platform | Language / runtime | Cost | Verdict |
| --- | --- | --- | --- |
| **Web** | TypeScript, `hls.js` + `jassub` | Low | **v1.** Unavoidable; it is also the admin UI. |
| **Android TV / Google TV** | RN-tvos + Kotlin module (Media3/libmpv) | Medium | **v1.** The highest-value TV target: largest installed base among self-hosters, sideloadable, no store gatekeeping, and cheap Android TV sticks are the standard community answer to bad smart-TV platforms. |
| **Android phone/tablet** | Same codebase as Android TV | Low once TV exists | **v1.** Nearly free. |
| **iOS / iPadOS** | RN-tvos + Swift module (AVPlayer/libmpv) | Medium | **v2.** Requires an Apple developer account and App Store review. |
| **tvOS** | Same codebase as iOS + TV focus work | Medium | **v2.** High value — Apple TV is the enthusiast's box. |
| **Desktop** (Win/macOS/Linux) | Web UI in a shell + libmpv | Low–medium | **v2.** Jellyfin's Qt WebEngine + libmpv design is proven; Tauri is the modern equivalent. |
| **Samsung Tizen** | HTML/JS packaged app | Medium | **v3.** Named on HN as a dealbreaker gap for Jellyfin ("Emby has a native Samsung Tizen app and Jellyfin doesn't"). But: sideloading requires developer mode, Samsung's store process is hostile, and the runtime is an old browser. |
| **LG webOS** | HTML/JS packaged app | Medium | **v3.** Same shape as Tizen. Note the recurring complaint that Plex's own webOS app has been buggy for years — this platform is hard for everyone. |
| **Roku** | **BrightScript + SceneGraph** | High | **v3 or never.** A bespoke language with no code reuse, and Roku's decoder set is narrow — it cannot render bitmap subtitles at all, forcing burn-in. Large installed base, very poor cost/benefit. |
| **Chromecast** | Web receiver (HTML/JS) | Low | **v2.** Cheap because it is a web app. Note: registering a custom receiver requires a Google Cast developer account and a one-time $5 fee; the Default Media Receiver needs no registration but constrains what you can do. |
| **Kodi add-on** | Python | Low | **v3.** Cheap way to reach hardware we'd otherwise never support. |

**The honest summary:** covering web + Android(+TV) well is achievable. Adding Apple is
achievable. Everything after that is a long tail where Plex's 15-year head start is
insurmountable, and the correct strategy for that tail is **not to write those clients** —
see §5.

## 4. Non-negotiable client features

Each of these is on this list because its absence is a named complaint in
[sentiment.md](sentiment.md):

- **libass-quality ASS/SSA rendering with embedded fonts**, on every platform.
- **Subtitle offset/sync adjustment** in the player UI.
- **Manual override of the playback decision** — force direct play, force transcode, pick
  rendition — surfaced in the player, not buried in settings.
- **Visible "why is this transcoding?"** The server already computes a reason; show it.
- **Trickplay scrub previews.** Users notice their absence immediately.
- **Resume across devices**, correct to the second.
- **Offline download** with real progress and real error messages.
- **Remote-control-first navigation** on TV, with no dead ends.

## 5. The client cold-start problem, and the cheapest way out

A new server has no clients, and nobody writes a client for a server with no users. Three ways
through, in increasing order of leverage:

1. **Write the two clients that matter** (web, Android/TV) and accept a narrow start.
2. **Publish an excellent OpenAPI document and generate SDKs**, so third parties can. Necessary
   but slow.
3. **Serve a Jellyfin-compatible API surface alongside our own.** Navidrome does exactly this
   in reverse — it implements a Jellyfin-compatible API (`server/jellyfin/`) so Jellyfin music
   clients work against it unmodified, *and* the Subsonic API for the same reason. If a
   compatibility shim over our model makes Findroid, Swiftfin, Infuse and Streamyfin work on
   day one, it is worth more than any client we could write in the same time.

Option 3 has real costs — it constrains our model to what the Jellyfin API can express, and
those clients will expose our features only as fast as they expose Jellyfin's. It should be a
**bridge, not a foundation**: our own clients speak our own API; the shim exists to make the
first year survivable. This needs its own plan and its own decision.

## 6. What this settles

1. **React Native shell + native player modules per platform.** Proven by Streamyfin and Kyoo.
2. **Two engines per client** — platform-native by default, libmpv as the "play anything"
   fallback, user-switchable.
3. **v1 clients: web and Android/Android TV.** v2: Apple, desktop, Chromecast. v3: Tizen/webOS
   /Kodi. Roku is probably never.
4. **A Jellyfin-compatible API shim is the highest-leverage answer to the cold-start problem**
   and deserves evaluation before we write a second client.
5. **Subtitle quality and playback-decision override are client requirements**, not
   nice-to-haves.
