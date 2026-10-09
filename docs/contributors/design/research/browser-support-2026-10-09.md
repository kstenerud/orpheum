> **ABOUTME:** Which browsers the web client should support and from which version: what
> media-server users report problems in, what Plex, Jellyfin, Emby, Netflix and YouTube state,
> market and version shares (September 2026), the features and media bugs that set each floor, the
> Cast receiver's old Chromium, and what each extra engine costs the tests. Feeds ui-architecture §6 Q3.

# Browser support: the set, the floors and the cost of testing more

Researched **2026-10-09**. Builds on [safari-versions.md](safari-versions.md) (Safari and iOS shares,
MSE/MMS per version, WebKit bugs that touch sync), on
[ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) §3.1 and §6 Q3 (which CSS features the
floor allows), and on the browser entries in [idiosyncrasies.md](../../idiosyncrasies.md). It does not
repeat them. The iPhone floor is already decided: iOS and iPadOS 17.1 for Watch Together, with native
HLS below it for watching alone (D250 §7). The owner's standing position is that policies may differ per
browser, chosen by feature detection, and that parity never comes.

Labels:

- **[sourced]**: a data set, release note, bug, document or issue says it.
- **[measured]**: I ran it here (the Playwright WebKit probe of §4.2).
- **[inferred]**: my reasoning. **Judgement** marks a recommendation that is an opinion.

Cache: `/yoloai/cache/browser-support/`. This holds:

- StatCounter CSVs for September 2026, worldwide, Germany, the US, the UK and France, each beside a
  `.url` file, in `sc/`;
- the GitHub searches over jellyfin-web, and the parsed "Browser" field of its bug form, in `ghsearch/`;
- Plex forum JSON in `plex/` and Emby forum pages in `emby/`;
- the Netflix, YouTube and Jellyfin pages;
- the WebKit preference sources;
- the WebKit probe and its output, in `probe/`.

Clones:

- MDN browser-compat-data 8.1.5 (2026-10-08), caniuse and Playwright
 , in `/yoloai/cache/git/`;
- jellyfin-web `dd6d1d66`, reused from earlier research.

> **Correction to the brief.** The browser tier does not run WebKit. `web/e2e/playwright.config.ts` has
> two projects, `chromium` and `firefox`, and `make browsers` installs only those two (`Makefile:331`).
> The M4 acceptance runs a Chrome host and a Firefox guest (`web/e2e/acceptance.config.ts`).

---

## 1. Which browsers media-server users care about

### 1.1 What users report: jellyfin-web's issues [sourced]

The bug form in jellyfin-web has a required **Browser** field
(`.github/ISSUE_TEMPLATE/1-bug-report.yml:112`). I fetched every issue whose body contains
"### Browser", 1,725 issues in all. Of these, 390 were filed through the current form, from 2024-05 to
2026-10. I read the field from each of the 390. One issue can name several browsers.

| Browser named in the field | Issues (of 390) |
| --- | --- |
| Firefox (including LibreWolf and Zen) | 182 (47 %) |
| Chrome or Chromium | 134 (34 %) |
| Edge | 34 |
| Safari (macOS or iOS) | 33 |
| Brave | 28 |
| Jellyfin Media Player (a desktop app, Chromium inside) | 23 |
| webOS / LG | 18 |
| Android app (a WebView) | 16 |
| Vivaldi | 8 |
| Tizen / Samsung TV | 7 |
| Opera | 5 |
| Xbox | 2 |
| Samsung Internet | 1 |

GitHub search counts for the same repository (`repo:jellyfin/jellyfin-web is:issue in:title <term>`,
open and closed, run 2026-10-09) give the same order in titles:

- Firefox 71, webOS 47, Chrome 36, LG 33, iOS 27, Safari 26, Tizen 17, Edge 15, Chromium 12.
- iPad 5, PlayStation 5, Xbox 4.
- Brave 1, Opera 1, iPhone 1.
- Samsung Internet 2, Vivaldi 0.

jellyfin-web labels platforms only (`p: ios`, `p: tizen`, `p: webos`, `p: android`, `p:xbox`). It has no
labels for desktop browsers.

- **[inferred]** Firefox's lead is partly selection. Firefox users meet more codec gaps (HEVC, AC-3),
  and they are likelier to file issues on GitHub. Firefox's share among self-hosters is still clearly
  far above its 6 % of the desktop market.
- The TV rows (webOS, Tizen) are Jellyfin's own TV apps, which are web apps wrapped for the TV. They are
  not people browsing in the TV's browser.

### 1.2 Plex and Emby forums [sourced]

- **Plex** forum titles since 2023-01-01, by the Discourse search API: Safari 25, Firefox 23, Edge 2,
  Brave 1. "Samsung Internet" matched only threads about the Samsung TV app. Typical threads:
  - **Firefox 149 regression:** HLS started only after a manual seek (Mozilla bug 2026875), fixed in 150
    ([thread](https://forums.plex.tv/t/firefox-149-0-hls-streaming-bug/937687), 2026-04).
  - **Firefox 147:** direct-played H.265 showed one frame every 4 s
    ([thread](https://forums.plex.tv/t/problems-with-direct-play-h265-since-firefox-147-0-0/936103)).
  - **Safari 26.3:** would not open a local server over HTTP
    ([thread](https://forums.plex.tv/t/plex-will-not-open-in-safari-26-3-http-v-https/936588)).
  - **Edge:** its "not secure" warning blocked login
    ([thread](https://forums.plex.tv/t/microsoft-edge-preventing-access-to-server-login/904724), 28 posts).
- **Emby** has a Web Client forum
  ([forum 146](https://emby.media/community/index.php?/forum/146-web-client/)). Its search returned 403,
  so I scanned the first 10 pages, 251 topics. Titles mention:
  - Firefox 13, Chrome 7, iOS/iPad 5, Edge 4, Safari 1, Vivaldi 1.
  - Tesla's in-car browser 2.
  - Brave, Opera and Samsung 0.

  The Firefox threads are about HEVC and ASS subtitles, for example `topic/124258-firefox-120-and-hevc`.

**What the forums show [inferred]:** the trouble is in **Firefox, Safari and Chrome**, roughly in that
order. Edge and Brave come up now and then. Opera, Vivaldi and Samsung Internet barely appear. Neither
TV browsers nor Samsung Internet shows up as a browser people use for a media server.

### 1.3 What the products state [sourced]

| Product | Browsers named | Minimums stated |
| --- | --- | --- |
| Jellyfin ([clients](https://jellyfin.org/docs/general/clients/)) | Firefox, Firefox ESR, Chrome, Chrome for Android, Safari on macOS and iOS, Edge | "the two most recent versions"; "Older browsers may be supported … full functionality is not guaranteed". The `package.json` browserslist also pins Chrome 27–63 and Edge 18, probably for TV engines **[inferred]** |
| Plex Web ([overview](https://support.plex.tv/articles/200289388-overview/)) | Chrome, Edge, Firefox, Safari | "the most recent release versions" |
| Emby ([web client](https://emby.media/support/articles/Web-Client.html)) | none named | none |
| Netflix ([23742](https://help.netflix.com/en/node/23742)) | Chrome, Edge, Firefox, Opera, Safari | Chrome 117, Edge 118, Firefox 129, Opera 92, Safari 14 (macOS 11+), iPadOS 14. Live events need Safari 15+ |
| YouTube ([78358](https://support.google.com/youtube/answer/78358), [175292](https://support.google.com/youtube/answer/175292)) | Chrome, Firefox, Safari, Edge, Opera | "Newest version" |
| Disney+, Prime Video | not read: the help pages are rendered by script, or returned 404 | — |

**The field agrees on four: Chrome, Edge, Firefox and Safari.** Only Netflix names numbers. Its
numbers are about three years behind current: Chrome 117 is from 2023-09.

### 1.4 The market, September 2026 [sourced: StatCounter CSVs in `sc/`]

| Share, % | Chrome | Safari | Edge | Firefox | Samsung Internet | Opera | Other |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Desktop, worldwide | 65.1 | 8.1 | 14.6 | 6.1 | 0.6 | 2.4 | 3.1 (Brave 1.1) |
| Desktop, Germany | 41.8 | 10.3 | 19.7 | **18.3** | 1.2 | 6.3 | 2.5 |
| Desktop, US | 60.9 | 14.0 | 14.8 | 5.6 | 0.3 | 1.7 | 2.6 |
| Mobile, worldwide | 67.8 | 24.6 | 0.4 | 0.7 | 3.5 | 1.4 | 1.7 |
| Mobile, Germany | 67.5 | 21.1 | 0.2 | 1.9 | **6.7** | 1.3 | 1.3 |
| Tablet, worldwide | 53.1 | 30.5 | 1.0 | 0.0 | 1.5 | 1.0 | 13.0 (Android WebView 12.2) |
| All platforms, Germany | 57.6 | 16.9 | 7.7 | 8.2 | 4.5 | 3.2 | 2.0 |
| Console, worldwide | 0.2 | — | 73.1 (Xbox) | — | — | — | PS4 26.5 |

StatCounter has no TV category: `device=tv` silently returns the all-platforms figures.

---

## 2. Is anything beyond the four compelling?

| Browser | Engine | Covered by testing the four? | Compelling on its own? |
| --- | --- | --- | --- |
| Chrome on Android | Blink, current | Mostly: Chrome desktop shares the engine, but not Android's audio routing, the call mode of echo cancellation, or the touch layout (idiosyncrasies "Chrome and Android") | **Yes.** 62 % of mobile traffic. Already in D190's device list (the S23) |
| Brave, Opera, Vivaldi | Blink, close to current | **Yes for rendering and media.** Not for their privacy layers: Brave's shields and the microphone refusal on Android (idiosyncrasies "Brave on Android refused the microphone"), Opera's VPN | No (judgement). Brave alone is 1.1 % of the desktop market but 7 % of Jellyfin's bug reports; worth one manual voice check when Watch Together ships |
| Chrome, Firefox and Edge on iPhone/iPad | WebKit of the installed iOS | **Yes, by iOS Safari**, except that whether ManagedMediaSource is exposed in Chrome's WKWebView is unconfirmed (safari-versions T12) | No: T12 settles it |
| Samsung Internet | Blink, **lagging**: 28→Chromium 130, 29→136, 30→143, against Chrome 155 ([Wikipedia](https://en.wikipedia.org/wiki/Samsung_Browser)) | Partly. It meets the feature floor of §3 from version 25 (Chromium 121). Its own UI layers (forced dark mode, a video-assistant overlay, permission prompts) are untested **[inferred]** | **The one candidate.** 6.7 % of German mobile traffic, preinstalled on Samsung phones, and 39 % of German mobile devices are Samsung. 10.9 % of German Samsung Internet traffic is below version 27. But it has 1 of 390 Jellyfin reports. Judgement: best-effort, not tested, until someone uses it |
| Firefox on Android | Gecko (GeckoView) | Engine yes. Not its audio: it makes a "call" of gain control and noise suppression (idiosyncrasies) | No (0.7 % of mobile worldwide, 1.9 % in Germany). VQ25 already touches it |
| Edge on Xbox | Blink | Engine yes | No: console traffic is a sliver of all traffic |
| webOS and Tizen TV browsers | Old Blink/WebKit, frozen per model | No | No. Media-server users reach TVs through apps, which for Jellyfin are its own web apps in wrappers. Cut A reaches the TV through the Cast receiver (D124) |
| **The Cast receiver** | Cast firmware's Chromium, frozen per device | **No**. See below | **Yes, but as its own target in M6**, not as a browser |

### The Cast receiver's Chromium [sourced thinly; inferred]

- Google's support page lists every Cast-firmware device on firmware **1.56.x**: the 2nd and 3rd gen
  Chromecast, the Ultra, and Cast for Android TV
  ([support 7124014](https://support.google.com/chromecast/answer/7124014), updated 2026-06-23).
  - The 1st gen is frozen at 1.36 and no longer supported.
  - The Chromecast with Google TV and the Google TV Streamer run Google TV builds.
- No Google document gives the receiver's Chromium version. User-agent fixtures in the
  [Matomo device-detector](https://github.com/matomo-org/device-detector/tree/master/Tests/fixtures)
  and [uap-core](https://github.com/ua-parser/uap-core/tree/master/tests) tests give:
  - `Chrome/90.0.4430.225 … CrKey/1.56` for Chromecast;
  - `Chrome/92` for Android TV;
  - `Chrome/70` for the 1st gen at 1.36.
- **[inferred] If a Chromecast receiver really is Chromium 90:**
  - none of container queries (105), `:has()` (105), popover (114), CSS nesting (112/120) or
    `color-mix()` (111) is there;
  - nor are `Array.prototype.at` (92), `findLast` (97), or ES2022 class static blocks (94).
  - `web/apps/cast-receiver/vite.config.ts` builds for `es2023`, which passes such syntax through
    untranspiled.
  - The packages the receiver will import already use `.at(` and `findLast`
    (`packages/player/src/abr-choice.ts`, `cache-hit.ts`, `packages/sdk/src/together-follow.ts`).
  - The receiver is a stub until M6 (its README), so nothing is broken today. **Whatever M6 builds must
    target the oldest receiver it supports, not the web client's floor.** A device check settles the
    version: read `navigator.userAgent` on the lab's Cast device.

---

## 3. Version floors

### 3.1 What the client uses and wants [sourced: BCD 8.1.5 unless marked]

**Today's code** (grep of `web/packages` and `web/apps`, 2026-10-09):

- ES2023 syntax, untranspiled (Vite `build.target: "es2023"`);
- `findLast` and `.at`;
- `color-mix()`;
- ManagedMediaSource, the MMS path of hls.js 1.7.3;
- `setCodecPreferences`, guarded by an `in` check;
- `encodings[].priority` and `networkPriority`;
- `preservesPitch`, unprefixed;
- `AudioContext`, `createMediaStreamSource`, `IntersectionObserver`.

It does not yet use `<dialog>`, popover, `inert`, `:has()`, container queries, `@scope`, cascade layers,
`navigator.mediaSession`, `requestVideoFrameCallback` or `structuredClone`. `crypto.randomUUID` is
banned (idiosyncrasies "Secure-context APIs").

| Feature | Chrome / Edge | Firefox | Safari (macOS = iOS unless noted) | Used? |
| --- | --- | --- | --- | --- |
| ES2023 syntax as built (static blocks 94, `findLast` 97) | 97 | 104 | 16.4 | yes |
| `color-mix()` | 111 | 113 | 16.2 | yes |
| ManagedMediaSource | — | — | macOS 17.0, iOS 17.1 | yes (iPhone) |
| `preservesPitch` unprefixed | 86 | 101 | **17.2** (`webkit`-prefixed since 4) | yes |
| `setCodecPreferences` | 76 | **128** | 13.1 | guarded |
| `RTCRtpReceiver.getCapabilities` | 59 | 113 | 12.1 | yes |
| `jitterBufferTarget` (as a setting) | 124 | 115 | **27** | read from stats only |
| `requestVideoFrameCallback` | 83 | **132** | 15.4 | proposed (safari-versions §4) |
| Media Session with `setPositionState` | 81 | 82 | 15 | wanted (lock screen, keys) |
| Container size queries | 105 | 110 | 16 | wanted (M5) |
| Cascade layers | 99 | 97 | 15.4 | wanted (R1) |
| `:has()` | 105 | 121 | 15.4 | wanted |
| `inert`, `<dialog>` | 102, 37 | 112, 98 | 15.5, 15.4 | wanted |
| Popover | 114 | 125 | 17 | wanted |
| CSS nesting, relaxed syntax (the `&` form earlier) | 120 (112) | 117 | 17.2 (16.5) | wanted |
| `@scope` | 118 | **146** (2025-12) | **26.4** (2026-03), after removal and partial support in 26.0–26.3 | proposed (R1) |
| View transitions, same-document | 111 | 144 | 18 | not planned |
| Anchor positioning | 125 | 147 | 26 | not planned |
| `focusgroup` | developer trial (M150, [chromestatus](https://chromestatus.com/)) | — | — | not usable |
| Element fullscreen | 71 | 64 | 16.4; **iPad only, never iPhone** | yes (D254) |

hls.js 1.7.3 needs far less: Chrome 47, Firefox 51, Safari 10, iOS 17.1 through MMS (its README,
"Compatibility"). React 19 states no browser matrix, and its bundle uses no syntax past ES2015
**[inferred from the bundle]**. **So the floor is set by our own build target and the CSS we want, not
by a library.**

### 3.2 Media facts that bear on floors [sourced, per tag]

These are about codecs in MSE (fMP4), the path the player takes.

- **HEVC:**
  - **Chrome** 107 (2022-10). Hardware only on Windows, and on Linux only through VAAPI
    ([chromestatus](https://chromestatus.com/feature/5186511939567616); Chromium `supported_types.cc`).
  - **Edge** needs the paid HEVC Video Extension on Windows.
  - **Firefox** 134 on Windows, 136 on macOS, 137 on Linux, where it goes through the system's FFmpeg
    ([Firefox release notes](https://www.mozilla.org/en-US/firefox/137.0/releasenotes/)).
  - macOS Firefox ≥154 drops mid-film HEVC chunks that start on a CRA (idiosyncrasies; D266 handles it).
  - Whether Firefox plays 10-bit HEVC is unverified.
- **AV1:**
  - Chrome 70 and Firefox 67.
  - Safari 17, **hardware only** (M3 Macs, iPhone 15 Pro and later)
    ([Safari 17 notes](https://developer.apple.com/documentation/safari-release-notes/safari-17-release-notes)).
- **AC-3 / E-AC-3:**
  - **not in desktop Chrome**: Chromium enables them only for Cast and tvOS builds (`media/media_options.gni`);
  - not in Firefox;
  - Safari and Edge on Windows and macOS **[inferred, not sourced here]**.
  - The server already transcodes sound (D3, D178), so this sets no floor.
- **Opus in MP4:**
  - Chrome 70 and Firefox;
  - Safari **17, stereo only**
    ([Safari 17 notes](https://developer.apple.com/documentation/safari-release-notes/safari-17-release-notes)).
- **FLAC in MP4:**
  - Chrome 62, but it may claim `fLaC` and then fail (crbug 1422728, cited in hls.js `utils/codecs.ts`);
  - Firefox;
  - Safari unverified.
- **Safari's MSE bugs by version:** listed in safari-versions §2. The relevant ones for a floor are:
  - `seeked` never fired on 17.0, fixed in 17.1;
  - seeks could stick until 18.4.
- **Firefox regressions:** they are short-lived because Firefox ships every two weeks (Firefox 149's HLS
  start, fixed in 150; 147's H.265 stutter). A version floor does not help there. Testing the current
  release does.

**Conclusion [inferred]:** no media fact sets a floor above what §3.1's features set, given that the
server prepares H.264 and AAC for any browser that cannot play the original. Codec support is
negotiated per browser by `isTypeSupported` (D3), not assumed from a version.

### 3.3 Shares below candidate floors, September 2026 [sourced data, computed by me]

Each figure is the percentage of that browser's own versioned traffic in the StatCounter CSVs. The CSVs
truncate the long tail, so these are slight underestimates. StatCounter gives no versions for Chrome on
Android or for iPhone Safari. iPhone Safari is taken from the iOS version (see safari-versions §1 for the
frozen-UA caveat).

| Browser, desktop | Floor | Worldwide | Germany | US |
| --- | --- | --- | --- | --- |
| Chrome | < 111 | 5.5 | 2.8 | 2.6 |
| Chrome | **< 120** | **9.4** | **4.0** | **4.0** |
| Chrome | < 125 | 11.6 | 4.8 | 4.9 |
| Edge | **< 120** | **3.0** | **1.0** | **1.0** |
| Firefox | < 128 | 24.2 | 6.2 | 11.7 |
| Firefox | **< 140** | **25.3** | **8.6** | **13.6** |
| Firefox | < 146 (`@scope`) | — | — | — |
| Safari (macOS) | < 16.4 | 4.4 | 3.6 | 4.0 |
| Safari (macOS) | **< 17.1** | **9.1** | **6.8** | **9.3** |
| Safari (macOS) | < 18.0 | 22.1 | 13.8 | 27.1 |
| Safari (macOS) | < 26 (`@scope` needs 26.4) | 34.5 | 26.0 | 39.0 |

| iOS / iPadOS | below 16.4 | below 17.0 | below 17.4 | below 18.0 | below 26 |
| --- | --- | --- | --- | --- | --- |
| iPhone, worldwide | 2.3 | 4.2 | 4.6 | 6.1 | 23.0 |
| iPhone, Germany | 1.9 | 3.6 | 4.0 | 5.3 | 27.3 |
| iPad, worldwide | 11.6 | 14.8 | 15.4 | 21.4 | 34.8 |

Notes:

- **Chrome below 120** is mostly **Chrome 109**, frozen on Windows 7 and 8.1: 1.53 % of all desktop
  traffic worldwide, about 2.5 % of Chrome.
- **Firefox worldwide** has implausible spikes at 121 (0.66 % of all desktop traffic) and 120 (0.33 %).
  These are probably automated clients with a fixed User-Agent **[inferred]**. Germany's figure (92 % of
  Firefox on 140 or later) is the more believable one.
- **Firefox ESR:**
  - ESR 140 reaches end of life on **2026-10-13**, and ESR 153 replaces it.
  - ESR 115 is extended to March 2027, but only for Windows 7–8.1 and macOS 10.12–10.14
    ([whattrainisitnow](https://whattrainisitnow.com/release/?version=esr)).
- **Devices that can no longer update** ([Apple security releases](https://support.apple.com/en-us/100100)):
  - iPhone 6s/7/SE 1 are stuck at iOS 15.8.
  - iPhone 8/X are stuck at iOS 16.7.
  - iPhone XS/XR are stuck at **iOS 18.7** (iOS 26 dropped the A12).
  - iOS 27 dropped no iPhone.
  - On macOS:
    - Monterey is stuck at Safari 17.6;
    - Ventura at 18.6;
    - Sonoma at 26.6.1.
  - Apple ships new Safari to the two previous macOS versions.
  - macOS 27 is Apple-silicon only, so **Intel Macs stop at Tahoe and Safari 26.x**
    ([Wikipedia, macOS 27](https://en.wikipedia.org/wiki/MacOS_Golden_Gate)).
- **Current versions:**
  - Chrome 155, stable 2026-10-06
    ([chromiumdash](https://chromiumdash.appspot.com/fetch_milestone_schedule?offset=-4&n=8));
  - Edge 155, 2026-10-08;
  - Firefox 157, 2026-09-29;
  - Safari 27.0.1.

### 3.4 Proposed floors (judgement)

| Browser | Floor | Released | Users below (desktop, DE / worldwide) | Why this number |
| --- | --- | --- | --- | --- |
| **Chrome** (desktop and Android) | **120** | 2023-12 | 4.0 % / 9.4 % | The first with every CSS feature M5 wants except anchor positioning: relaxed nesting 120, popover 114, `@scope` 118, `:has()` and container queries 105. It drops Chrome 109 on Windows 7, a system that gets no security fixes |
| **Edge** | **120** | 2023-12 | 1.0 % / 3.0 % | Same engine and number as Chrome, so one rule |
| **Firefox** | **140** (the ESR of 2025-06) | 2025-06 | 8.6 % / 25 % (worldwide distorted, §3.3) | Covers popover 125, `:has()` 121, `setCodecPreferences` 128, rVFC 132, HEVC 137. Nothing the client wants lies between 140 and 153, and from 2026-10-13 140 is the last ESR Mozilla has retired, so 140 costs no feature and keeps the laggards: 18.9 % of German desktop Firefox is below 153. Debian 13 already ships `firefox-esr` 153.4 **[measured: `apt-cache policy` in the dev image]** |
| **Safari, macOS** | **17.1** | 2023-10 | 6.8 % / 9.1 % | The same number as iOS (D250), so one Safari rule. It keeps Monterey Macs (17.6). It has popover (17) and container queries (16) |
| **Safari, iOS and iPadOS** | **17.1** (decided, D250 §7) | 2023-10 | iPhone about 4.4 %; **iPad about 15 %** | ManagedMediaSource. iPads below 17 fall back to native HLS for solo playback, as decided |

Consequences for the M5 UI (ui-architecture §6 Q3) **[inferred from the table]**:

- **Container queries, `:has()`, `inert`, `<dialog>`, cascade layers and popover: usable** above these
  floors. Popover needs Safari 17, Firefox 125 and Chrome 114, all met.
- **`@scope`: not usable.** It needs Safari 26.4 and Firefox 146. That would cut off about 26–35 % of
  macOS Safari and every Intel Mac on Sonoma. Use layers and the R5 rule ("a component owns its look")
  without `@scope`, or feature-detect it as an enhancement only.
- **CSS nesting:** write only the `&`-prefixed form. Safari 17.1 has nesting (16.5) but not the relaxed
  syntax (17.2), and the floor is 17.1.
- **Unprefixed `preservesPitch` arrives in Safari 17.2.** On iOS 17.1 the follower's
  `media.preservesPitch = …` (`packages/sdk/src/together-follow.ts:539`) only sets a plain property, and
  pitch stays kept. If the "no pitch-keeping on nudges" setting matters on 17.1, set `webkitPreservesPitch`
  too **[inferred; this is a code question for the owner, not done here]**.
- **Vite's `build.target`** should name the floors (`chrome120`, `edge120`, `firefox140`, `safari17.1`,
  `ios17.1`) rather than `es2023`. Then the build, not a reviewer, keeps syntax inside the floor.
  `vite.config.ts` is not a gate file, but the change is a design choice for the owner.
- **A browser below the floor:** the D250 pattern of telling the browser to update, decided by feature
  detection, extends naturally. For example, a check at start-up for the features the build assumes.
  This is judgement, and it would be new design.

---

## 4. Testing cost

### 4.1 What each engine adds today [sourced from the repo; inferred estimates]

The figures come from the test audit ([test-audit-2026-10-08.md](test-audit-2026-10-08.md) §1.2):

- `make check-pr` took **278 s** against a **300 s** budget.
- The browser tier's 13 tests, run one worker at a time over two engines, took about **57 s**.
- Its film fixtures took another 30–50 s.
- Since the M4 acceptance's latest change, it runs four network profiles of 30 minutes
  each, about **2.5 h**, Chrome host with Firefox guest.

| Adding | Cost per check-pr | Other cost |
| --- | --- | --- |
| **WebKit (Playwright)** in the browser tier | About +25–30 s **[inferred: the tier's 57 s over two engines; the fixtures are shared]**. That is about a third of the 78 s the test audit's fixes would win back | **[measured]** in the dev image (Debian 13): `playwright install-deps webkit` adds 185 packages, **355 MB**, 39 s; the WebKit build is 270 MB. A change to `Makefile:331` and the image, both owner's gates |
| **WebKit as a guest** in the acceptance | +2.5 h if run as a further pairing | Or rotate the guest's engine per profile, at no added time (judgement) |
| **Edge** (`channel: "msedge"`) | About +25 s | The same Blink as Chrome. It would catch little but Edge's own codecs on Windows, which a Linux run cannot show. Not worth it (judgement) |
| **Branded Chrome** instead of Chrome for Testing | 0 | Playwright's docs say Chromium lacks some proprietary codecs ([browsers.md](https://github.com/microsoft/playwright/blob/main/docs/src/browsers.md), "Media codecs"). The tier already plays H.264/AAC, so this is not a gap today |
| **A real iPhone or Mac** | Not automatable here | Device time, as the verification queue and the device tests of safari-versions §6 |

### 4.2 What Playwright's WebKit does and does not tell you [measured, sourced]

I ran Playwright 1.63's WebKit (WebKit **26.6**, the Linux WPE port) in a throwaway copy of the
development image. The probe is `/yoloai/cache/browser-support/probe/probe.mjs`, and its output is beside
it. It found:

- **Both `MediaSource` and `ManagedMediaSource` exist**, even with an iPhone User-Agent, `isMobile` and
  touch.
  - WebKit enables MMS by default on Cocoa, GTK and WPE (`defaultManagedMediaSourceEnabled`,
    `Source/WebKit/Shared/WebPreferencesDefaultValues.cpp`).
  - A real iPhone has MMS only (safari-versions §2).
  - So hls.js in Playwright's "iPhone" takes the **classic MSE path**, not the iPhone's.
  - Deleting `window.MediaSource` in an init script would push hls.js onto MMS **[inferred, not tried]**.
    But MMS's streaming decisions there are the GStreamer port's, not iOS's.
- **No native HLS:** `canPlayType("application/vnd.apple.mpegurl")` is `""`. Real Safari plays HLS
  natively. The solo fallback below iOS 17.1 cannot be tested here.
- **Every codec reports supported**: HEVC, 10-bit HEVC, AV1, VP9, AC-3, E-AC-3, Opus and FLAC in MP4,
  for both MSE and MMS. The answers come from the host's GStreamer and libav. They say nothing about
  Apple's decoders (AV1 hardware-only, Opus stereo-only, `hvc1` versus `hev1`).
- **WebRTC works for tests:**
  - RED is listed first in the audio capabilities;
  - `setCodecPreferences` and `jitterBufferTarget` exist;
  - with `permissions: ["microphone"]` on an `http://localhost` page, `getUserMedia` returns a
    "Mock audio device 1".
  - So the voice test could run in WebKit. Whether its timing resembles Safari's is unknown.
- **CSS and JavaScript features:** popover, container queries, `:has()`, `@scope`, nesting and anchor
  positioning are present. This is a fair check that the UI renders in WebKit's layout engine. But it is
  WebKit 26.6, not the floor's 17.1, so it **cannot show what old Safari lacks**.
- **Not represented at all** (Cocoa or iOS only):
  - the GPU-process time estimator behind "`currentTime` stands still for up to about 250 ms"
    (idiosyncrasies);
  - AVFoundation's decoders;
  - iPhone's gesture rules for loading and sound;
  - the read-only volume;
  - the ring/silent switch;
  - the earpiece routing of call audio (WebKit 230902);
  - `webkitEnterFullscreen`;
  - Low Power Mode;
  - MMS's real thresholds.

  Playwright says so itself: "for the closest-to-Safari experience you should run WebKit on mac, for
  example if you do video playback" (browsers.md, "WebKit").

**Verdict (judgement):** Playwright WebKit on Linux is a **layout and API smoke test of the WebKit
engine**. It is not a Safari test, and least of all an iPhone test. Its value is catching WebKit
rendering or script breakage before a person does, which is cheap insurance for the M5 UI. Every
playback, sync and voice behaviour that has bitten us on Safari would still pass in it.

### 4.3 What stays manual

- [verification-queue.md](../../verification-queue.md) has 27 items. **8 of them name an iPhone or
  Safari** (VQ12–VQ15, VQ18, VQ20, VQ22, VQ25, VQ27). That is the standing proof that the Apple side is
  verified by hand.
- The device tests T1–T13 of safari-versions §6 (time granularity, rate nudges, seeks, paused buffering,
  AirPlay, lock screen, Chrome on iPhone).
- New with this research:
  - a **floor check** on the oldest supported Apple devices: an iPhone on 17.x or 18.x (an XS or XR is
    stuck at 18.7) and a Mac on Safari 17.x, opening each M5 screen once;
  - the **Cast receiver's real Chromium version** (§2).
- Firefox on macOS is not covered by the Linux tier. The HEVC-on-CRA freeze is macOS only, so a person
  with a Mac checks Firefox there at each milestone.

---

## 5. Recommendation

**Supported (judgement):**

- **Chrome 120+** on desktop and Android;
- **Edge 120+**;
- **Firefox 140+** on desktop (raise it only when a wanted feature needs a later one);
- **Safari 17.1+** on macOS, iOS and iPadOS.

Below a floor, the client says so, and watching alone may still work (D250's pattern). The reasons:

1. These are the four that every competitor names and that media-server users file problems in
   (Firefox and Chrome first, Safari and Edge next, §1).
2. Each floor is the lowest version that has every feature the M5 UI wants except `@scope` (§3.1).
3. The floors leave out mostly frozen systems: Windows 7's Chrome 109, iPhones before the XS, and Macs
   on Big Sur or older.
4. Below 17.1, an iPhone cannot run the player that Watch Together needs.

**Covered without extra tests:** Brave, Opera and Vivaldi on desktop (Blink), and every iOS browser
(WebKit, pending T12).

**Not supported but likely to work:** Samsung Internet 25+, Firefox on Android, and Edge on Xbox.

**Testing (judgement):**

- Keep Chromium and Firefox in the browser tier.
- Add Playwright WebKit as a rendering smoke test once M5's UI lands, if check-pr has the time: about
  +25–30 s and 625 MB in the image.
- Keep every playback and sync question about Safari on real devices.
- Do not add Edge to the tier.

**What would make another browser compelling:**

- Samsung Internet: a user in the household or a recurring report.
- Firefox on Android: Watch Together voice being used there.
- Any browser: a failure in it that the four do not reproduce.
- **The Cast receiver is already compelling** for M6, and needs its own floor set by the real device's
  Chromium, which may be about 90.

---

## Where the evidence is weak

- **The Cast receiver's Chromium version** comes from user-agent test fixtures, not from Google, and not
  from a device. It may be newer, especially on Google TV devices, where the system WebView updates.
- **StatCounter:**
  - truncates long tails;
  - counts by User-Agent, which Safari 26 freezes on iOS and macOS freezes at 10.15;
  - has no versions for Chrome on Android or for iPhone Safari;
  - shows worldwide Firefox spikes (120, 121) that look like bots.
  - Its "general web" denominator is not self-hosters. No data set of media-server users' browsers
    exists, so I used jellyfin-web's bug form instead, which counts *problems*, not *users*, and favours
    people who file on GitHub.
- **Samsung Internet:** its share of Samsung-phone traffic (at most ~17 % in Germany) is my ratio of two
  StatCounter series. Whether it is the default on Samsung phones in the EU, given the browser choice
  screen, is unconfirmed.
- **AC-3/E-AC-3 in Edge and Safari, and FLAC-in-MP4 in Safari** are not sourced here. The server
  transcodes regardless.
- **The +25–30 s for a WebKit project** is a share of a measured total, not a run.
- **Plex's and Emby's counts** are from title searches and a ten-page scan, not full corpora.

## Not tried

- **A WebKit run of the browser tier itself.** Only the probe ran. How many of the 13 specs pass in
  WebKit unchanged is unknown.
- Deleting `window.MediaSource` in Playwright WebKit to force the MMS path.
- Any real device: no iPhone, Mac, Samsung phone or Cast device was available to this research.
- **Disney+ and Prime Video** supported-browser pages: the first is rendered by script, the second's URL
  returned 404.
- **Emby's forum search** returned 403.
- **The web search engine** was unavailable: the session's budget was spent, and DuckDuckGo and Bing
  served captchas. Everything here came from known URLs, APIs and clones, so threads that only a search
  would find (on Reddit, for instance) were not read.
- Chrome on Android version shares, and Samsung Internet's own media quirks (forced dark mode,
  video assistant), beyond noting that they exist.
- Whether Firefox 140–157 plays 10-bit HEVC through MSE.
