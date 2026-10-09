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

**Gaps filled 2026-10-09, in a second pass**, after the owner decided the floors (D271 §1): Disney+
and Prime Video's browser pages, rendered; Emby's threads; issue trackers in place of Reddit, which
could not be read; Samsung Internet's forced dark mode and Video Assistant; Firefox's 10-bit HEVC by
operating system; and Chrome on Android's versions and update speed. "Not tried" lists what is still
open.

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

The second pass's pages are in `/yoloai/cache/http/browser-support/`: the rendered Disney+ and Prime
Video pages (`.txt` beside the HTML), Emby topics, GitHub and Bugzilla JSON, Firefox decoder sources
(`ff-*.cpp`), the chromiumdash schedule and Wikimedia's browser tables (`wikimedia/`).

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
- **The one Samsung Internet report is a user-agent lesson**
  ([jellyfin-web #8435](https://github.com/jellyfin/jellyfin-web/issues/8435), 2026-09-08, closed
  2026-09-21). jellyfin-web 12.0 treats any User-Agent containing "samsungbrowser" as a television
  (`src/scripts/browser.js`), so on a Galaxy Z Fold 8 the interface was *"comically large and chopped
  off"*. The phone sent `SamsungBrowser/30.0 Chrome/143.0.0.0 Mobile`. **[inferred]** Deciding a layout
  from the User-Agent's brand is the trap. Choosing by feature detection and by the window's size, not
  by the browser's name, avoids it.

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

  Topic pages fetch fine; only the forum's search returns 403. The second pass read these:
  - **Firefox 145+ claims Matroska, so Emby direct-plays it, and the start takes minutes**
    ([topic 145428](https://emby.media/community/topic/145428-firefox-takes-5-to-10-minutes-to-start-playing-and-is-jittery/),
    2026-01 to 2026-02, Firefox 146 on Kubuntu). Emby's lead developer: *"firefox is now reporting that
    it supports mkv so the web app is trying to direct play them"*. The user's cure was
    `media.mkv.enabled=false`. The Mozilla side is in §3.2.
  - **iOS 26.5 Safari stopped accepting AC-3 and E-AC-3**, "no compatible streams"
    ([topic 147919](https://emby.media/community/topic/147919-ios-265-broke-safari-web-playback/),
    2026-05 to 2026-09). Clearing the cache brought AC-3 back but not E-AC-3; the reporter says 26.6
    fixed it, and a second user still sees E-AC-3 transcoded on iOS 26.6 and on Safari for macOS 12.7.6.
    Emby chose to wait for Apple rather than force it on.
  - **iOS needs two taps to start** (topic 119710, 2023): *"due to the browser auto play policy"*.
  - Emby states no browser list: its page says only *"Emby runs right in your browser"*; staff in 2018
    called the usual four *"modern browsers ... Chrome will generally be the most advanced"*
    (topic 56059).

- **Reddit could not be read.** Its JSON API returned 403, a headless Chromium was blocked, old.reddit
  now needs a login, and a site-restricted web search returned no Reddit pages. The issue trackers stood
  in for it:
  - [jellyfin #16604](https://github.com/jellyfin/jellyfin/issues/16604) (2026-04), Firefox 149 on Arch
    Linux: H.264 **High 10** fails, HEVC **Main 10** 1080p SDR plays directly.
  - [Mozilla bug 2071317](https://bugzilla.mozilla.org/show_bug.cgi?id=2071317) (2026-09-11, NEW):
    Jellyfin's 1080p HEVC on macOS loads slowly, freezes after about a minute, and the picture freezes
    after a seek while sound runs; 720p HEVC and H.264 are fine, and so is Safari. Possibly the same
    family as the CRA freeze in idiosyncrasies (bug 2065822) **[inferred]**.
  - [webcompat #234781](https://github.com/webcompat/web-bugs/issues/234781) (2026-09-13): Plex's HEVC
    failed on Firefox 155 on Windows 10; the triager could not reproduce it on 156.
  - [jellyfin #18163](https://github.com/jellyfin/jellyfin/issues/18163) (2026-09-22): Jellyfin 12.1
    refused direct hvc1 HEVC in Safari on iOS 17 and Firefox on Windows; a server profile bug, fixed
    **[inferred]**.
  - **Safari 26.x, fixed for a while by clearing the cache**, three times over: Plex on iOS 26.2 (media
    error s3014, [thread 934847](https://forums.plex.tv/t/934847), 2025-12, came back later); Emby on
    iOS 26.5 (above); and video stuck on its first frame when a page or web app was reopened on iOS
    26.0–26.0.2 (Apple discussions 256152107, 2025-09; the reporter says 26.2 fixed it). **[inferred]**
    Worth one line in idiosyncrasies if Orpheum meets it: "clear Safari's website data" is the first
    thing to ask a 26.x user to try.

**What the forums show [inferred]:** the trouble is in **Firefox, Safari and Chrome**, roughly in that
order. Edge and Brave come up now and then. Opera, Vivaldi and Samsung Internet barely appear. Neither
TV browsers nor Samsung Internet shows up as a browser people use for a media server.

### 1.3 What the products state [sourced]

| Product | Browsers named | Minimums stated |
| --- | --- | --- |
| Jellyfin ([clients](https://jellyfin.org/docs/general/clients/)) | Firefox, Firefox ESR, Chrome, Chrome for Android, Safari on macOS and iOS, Edge | "the two most recent versions"; "Older browsers may be supported … full functionality is not guaranteed". The `package.json` browserslist also pins Chrome 27–63 and Edge 18, probably for TV engines **[inferred]** |
| Plex Web ([overview](https://support.plex.tv/articles/200289388-overview/)) | Chrome, Edge, Firefox, Safari | "the most recent release versions" |
| Emby ([web client](https://emby.media/support/articles/Web-Client.html)) | none named: *"Emby runs right in your browser"* | none |
| Netflix ([23742](https://help.netflix.com/en/node/23742)) | Chrome, Edge, Firefox, Opera, Safari | Chrome 117, Edge 118, Firefox 129, Opera 92, Safari 14 (macOS 11+), iPadOS 14. Live events need Safari 15+ |
| YouTube ([78358](https://support.google.com/youtube/answer/78358), [175292](https://support.google.com/youtube/answer/175292)) | Chrome, Firefox, Safari, Edge, Opera | "Newest version" |
| Disney+ ([requirements](https://help.disneyplus.com/article/disneyplus-computer-browser-requirements), rendered by script; read in a headless Chromium) | Windows 10+: Chrome, Firefox, Edge, Opera. macOS 11+: Chrome, Firefox, Edge, Safari. ChromeOS: Chrome | *"the latest versions"*; no numbers, no Linux, no mobile browsers |
| Prime Video ([system requirements](https://www.amazon.com/gp/help/customer/display.html?nodeId=GUVGB3QMQRYRERYW), rendered the same way) | Chrome, Firefox, Edge, Safari, Opera, on Windows, macOS, ChromeOS or Linux | *"the latest version"*. Outside Windows and macOS, standard definition only; browsers get *"up to HD"* and stereo |

**The field agrees on four: Chrome, Edge, Firefox and Safari.** Of the six products, only Netflix
names numbers; Disney+ and Prime Video say "latest", like Plex and YouTube. Both add Opera, Disney+
only on Windows, and neither names a mobile browser. No product names Samsung Internet. Netflix's
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
| Chrome on Android | Blink, current | Mostly: Chrome desktop shares the engine, but not Android's audio routing, the call mode of echo cancellation, or the touch layout (idiosyncrasies "Chrome and Android") | **Yes.** 62 % of mobile traffic. Already in D190's device list (the S23). It updates fast: a new stable every two weeks since 153, and about half its users move within two weeks (§3.3) |
| Brave, Opera, Vivaldi | Blink, close to current | **Yes for rendering and media.** Not for their privacy layers: Brave's shields and the microphone refusal on Android (idiosyncrasies "Brave on Android refused the microphone"), Opera's VPN | No (judgement). Brave alone is 1.1 % of the desktop market but 7 % of Jellyfin's bug reports; worth one manual voice check when Watch Together ships |
| Chrome, Firefox and Edge on iPhone/iPad | WebKit of the installed iOS | **Yes, by iOS Safari**, except that whether ManagedMediaSource is exposed in Chrome's WKWebView is unconfirmed (safari-versions T12) | No: T12 settles it |
| Samsung Internet | Blink, **lagging**: 28→Chromium 130, 29→136, 30→143, against Chrome 155 ([Wikipedia](https://en.wikipedia.org/wiki/Samsung_Browser)) | Partly. It meets the feature floor of §3 from version 25 (Chromium 121), and about 99 % of its users are on 25 or later (§3.3). Its own layers are not covered: forced dark mode repaints dark pages, and Video Assistant may offer its own full-screen controls (below) | **The one candidate.** 6.7 % of German mobile traffic, preinstalled on Samsung phones, and 39 % of German mobile devices are Samsung. 10.9 % of German Samsung Internet traffic is below version 27. But it has 1 of 390 Jellyfin reports. Judgement: best-effort, not tested, until someone uses it |
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

### Samsung Internet's own layers [sourced; inferred where marked]

**Forced dark mode.**

- Samsung's own account (Daniel Appelquist,
  [Samsung Internet blog, 2020-12-15](https://developer.samsung.com/internet/blog/en-us/2020/12/15/dark-mode-in-samsung-internet)):
  13.2 beta added `prefers-color-scheme` and the `color-scheme` meta, but behind a flag with three
  settings. Only "Prefer Media Query Over Force Dark" honours a page's
  `<meta name="color-scheme" content="light dark">`.
- MDN browser-compat-data [issue 17408](https://github.com/mdn/browser-compat-data/issues/17408), open
  since 2022: *"By default in Samsung Internet neither color-scheme nor @media prefers-color-scheme do
  anything"*. [Issue 29347](https://github.com/mdn/browser-compat-data/issues/29347) (2026-03, open):
  the meta still does not work. The cure offered is the user turning off "Force Dark mode for web
  content"; one user says even that did not help.
- Samsung has an experimental "Enable Adaptive Force Dark" flag (beta 26+) that respects the page's
  scheme ([v01.io, 2024](https://www.v01.io/posts/2024/04/samsung-internet-dark-mode/)); whether it is now on by default was not found.
- **A dark page is darkened a second time**
  ([herdr-web-ui #451](https://github.com/devswha/herdr-web-ui/issues/451), 2026-10-05, Galaxy Tab S7+):
  a page that declares `color-scheme` was repainted, *"similar shades ran together"* and meter bars
  looked empty. Only the user's setting fixed it. Chrome leaves such pages alone.
- Whether video is dimmed is disputed: ctrl.blog (2020, version 11) found that *"videos and canvas ...
  aren't color filtered"*; v01.io (2024) found *"Videos, Images, and SVGs are toned down"*.
- **For Orpheum [inferred]:** the app is dark throughout (D271 §3), and no page-side opt-out is known
  to work in current Samsung Internet. Expect low-contrast pieces to merge: progress bars, buffered
  ranges, disabled states. The poster, and possibly the picture, may be dimmed. Only a Samsung device
  shows how much. The known remedy is a help line for users: turn off "Force Dark mode for web
  content".

**Video Assistant.**

- It offers a pop-up player, its own full screen with extra controls, and casting by Smart View; users
  can turn it off under Settings › Useful features ([SamMobile, 2019](https://www.sammobile.com/news/video-assistant-returns-butchered-in-latest-samsung-internet-beta/)).
- The pop-up player needs a plain video URL. Samsung support, quoted in
  [video.js #7656](https://github.com/videojs/video.js/issues/7656) (2022): *"you can not use videos of
  MSE type."*
- **For Orpheum [inferred, unverified]:** playback goes through MSE (hls.js), so the pop-up player
  cannot take over the video. Video Assistant's own full screen may still be offered, and it would bypass
  Orpheum's controls. If it drives the media element directly, it fires the same media events, so a
  Watch Together follower's own seeks might still be reported. No site opt-out was found.

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
  - **Firefox and 10-bit HEVC (Main 10), by operating system** [sourced: Bugzilla and Firefox's source;
    inferred where marked]. The meta bug ([1842838](https://bugzilla.mozilla.org/show_bug.cgi?id=1842838)) gives the HEVC versions above.
    - **Windows:** 10-bit HEVC and AV1 through the WMF decoder since **132** ([1866265](https://bugzilla.mozilla.org/show_bug.cgi?id=1866265),
      with 8- and 10-bit tests). It still needs the HEVC Video Extension, which is not installed on every
      Windows ([1963910](https://bugzilla.mozilla.org/show_bug.cgi?id=1963910), open). HDR is not right: *"We support HDR on macOS not on
      Windows"* (a Mozilla engineer, 2025, in [1949088](https://bugzilla.mozilla.org/show_bug.cgi?id=1949088), open: 10-bit HEVC "washed out" on
      Firefox 137).
    - **macOS:** VideoToolbox since 136 ([1924066](https://bugzilla.mozilla.org/show_bug.cgi?id=1924066)). Today's source picks a 10-bit pixel
      format for a 10-bit track and accepts any HEVC when the pref is on (`AppleVTDecoder.cpp`,
      `AppleDecoderModule.cpp`), so 10-bit is plumbed. When that landed was not established, so
      whether 140 has it is not proven.
    - **Linux:** VA-API and FFmpeg since 137 ([1894818](https://bugzilla.mozilla.org/show_bug.cgi?id=1894818)); `FFmpegVideoDecoder.cpp` maps
      `HEVCMain10` and handles 10-bit output. A user reports HEVC Main 10 1080p SDR playing directly on
      Firefox 149 ([jellyfin #16604](https://github.com/jellyfin/jellyfin/issues/16604)). On Wayland
      with compositor HDR, 10-bit SDR video was oversaturated until 157 ([2066333](https://bugzilla.mozilla.org/show_bug.cgi?id=2066333)).
    - **Android:** fixed in 137 ([1946553](https://bugzilla.mozilla.org/show_bug.cgi?id=1946553)).
    - No Mozilla test or statement covers 10-bit HEVC **through MSE** on each system. Firefox uses the
      same platform decoder for MSE and file playback **[inferred]**, so the above should carry over.
    - **For Orpheum [inferred]:** at the floor of 140, every desktop system decodes 10-bit HEVC,
      Windows only with the extension. SDR 10-bit is the safe case; HDR renders correctly only on
      macOS. `isTypeSupported` (D3) cannot catch "plays but washed out", so an HDR original should
      not go to Firefox on Windows on its answer alone.
    - Also: Firefox fails 10-bit **H.264** (High 10) on Windows ([1711812](https://bugzilla.mozilla.org/show_bug.cgi?id=1711812), open), and a
      user reported it failing on Linux 149. The server's H.264 renditions are 8-bit, so this matters
      only for an original in High 10 **[inferred]**.
- **AV1:**
  - Chrome 70 and Firefox 67.
  - Safari 17, **hardware only** (M3 Macs, iPhone 15 Pro and later)
    ([Safari 17 notes](https://developer.apple.com/documentation/safari-release-notes/safari-17-release-notes)).
- **AC-3 / E-AC-3:**
  - **not in desktop Chrome**: Chromium enables them only for Cast and tvOS builds (`media/media_options.gni`);
  - not in Firefox;
  - Safari and Edge on Windows and macOS **[inferred, not sourced here]**.
  - The server already transcodes sound (D3, D178), so this sets no floor.
  - iOS 26.5 Safari stopped accepting AC-3 and E-AC-3 for a while (Emby, §1.2), which is one more
    reason not to rely on them.
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
- **Firefox claims Matroska from 145 on** ([1991752](https://bugzilla.mozilla.org/show_bug.cgi?id=1991752)), and file playback of Matroska wanted
  to buffer the whole file until **158** ([2000420](https://bugzilla.mozilla.org/show_bug.cgi?id=2000420), fixed 2026-09-23), so 145–157 are
  affected. HEVC in Matroska also froze after a seek until 158 ([2006226](https://bugzilla.mozilla.org/show_bug.cgi?id=2006226)) and stuttered
  until 159 ([2069252](https://bugzilla.mozilla.org/show_bug.cgi?id=2069252)). Emby trusted the claim and direct-played, and starts took minutes
  (§1.2). Orpheum remuxes to fMP4 for MSE (idiosyncrasies, "Browsers other than Chrome play no
  Matroska"), so this does not touch the player today. **[inferred]** If a direct-file path is ever
  added, a browser's "yes" to Matroska is not enough: Firefox said yes from 145 to 157 while it
  could not stream it well.
- **Safari 26.x, fixed for a while by clearing the cache:** three reports, on 26.0, 26.2 and 26.5 (§1.2).
  No version floor answers it; it is a support step.

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
  - **Chrome now ships every two weeks**: 148 to 153 came four weeks apart, then 153 (09-08), 154
    (09-22), 155 (10-06) and 156 (10-20) two weeks apart (chromiumdash's schedule, fetched 2026-10-09).

**Chrome on Android, from Wikimedia's data** [sourced data, computed by me]. StatCounter gives no
Android versions; Wikimedia's weekly page views to its mobile sites by browser and major version do
([mobile_site_by_browser_family_and_major.tsv](https://analytics.wikimedia.org/published/datasets/periodic/reports/metrics/browser/mobile_site_by_browser_family_and_major.tsv),
cached under `wikimedia/`). "Chrome Mobile" is 43 % of those views, and includes browsers that send its
User-Agent.

- **Adoption is fast:** about half of users are on a new version two weeks after its release, about
  80 % after three. 151 went from 9 % to 43 % to 80 % in three weekly steps; 154 reached 45 % in its first
  full week.
- **Week of 2026-09-27:** 154 45 %, 153 35 %, 152 5 %, 151 5 %. **Below 120: 1.3 %; below 140: 4.0 %;
  below 150: 8 %.** Below 120 ran at 1.2–3.3 % across the past year, with an unexplained spike of
  3.3–5.9 % in July 2026.
- **The frozen tail:** 138 holds 1.39 %, the last Chrome for Android 8 and 9 (139 needs Android 10,
  as 9to5Google reported on 2025-06-26); 119 holds 0.13 % (probably the last for Android 7
  **[inferred]**).
- **Samsung Internet** (3.1 % of the views): 30 is 90 %, 29 3.5 %, 28 2.1 %, 27 0.8 %, so about 99 % are on
  25 or later (Chromium 121+).
- Firefox for Android: 156 is 62 %, 157 21 %. Chrome on iOS is the app's version, not the engine's, and
  says nothing about a floor.
- **[inferred]** The Chrome 120 floor costs about 1.3 % of Chrome on Android, nearly all phones on
  Android 7 or older. Android 8 and 9 phones sit on 138, above it. A floor of 140 would cut them too,
  about 2.7 % more. Wikimedia's readers are not media-server users, but this is the only versioned
  Android data found.

### 3.4 Proposed floors (judgement)

| Browser | Floor | Released | Users below (desktop, DE / worldwide) | Why this number |
| --- | --- | --- | --- | --- |
| **Chrome** (desktop and Android) | **120** | 2023-12 | 4.0 % / 9.4 %; on Android about 1.3 % (Wikimedia, §3.3) | The first with every CSS feature M5 wants except anchor positioning: relaxed nesting 120, popover 114, `@scope` 118, `:has()` and container queries 105. It drops Chrome 109 on Windows 7, a system that gets no security fixes, and on Android only phones on Android 7 or older |
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

**Supported (judgement; decided as written by D271 §1, Samsung Internet as best effort):**

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

The second pass found nothing that moves a floor. It adds three things to the best-effort browsers
and to support **[inferred]**: Samsung Internet's forced dark mode may darken the already dark app
(§2), so a help line should say how to turn it off; Firefox 140–157's Matroska claim matters only if
a direct-file path is added (§3.2); and a Safari 26.x user's first step is clearing website data
(§1.2).

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
- **Chrome on Android's versions** come from Wikimedia's readers, not media-server users, and its
  "Chrome Mobile" family includes other browsers that send Chrome's User-Agent.
- **Samsung Internet's dark mode:** whether it dims video is disputed between two write-ups (2020 and
  2024), and whether the adaptive flag is now on by default was not found. The Video Assistant
  conclusions are inferred from a 2022 support answer, not seen on a device.
- **Firefox's 10-bit HEVC** is read from bugs and source, not played through MSE; when the macOS
  10-bit path landed is not established.
- **The Safari 26.x cache reports** are three users on three forums, each helped by clearing the cache.

## Not tried

What is still open after the second pass, and why:

- **A WebKit run of the browser tier itself.** Only the probe ran. How many of the 13 specs pass in
  WebKit unchanged is unknown.
- Deleting `window.MediaSource` in Playwright WebKit to force the MMS path.
- **Any real device:** no iPhone, Mac, Samsung phone or Cast device was available. A Samsung phone would
  settle forced dark mode and Video Assistant; a Firefox on each system would settle 10-bit HEVC
  through MSE (`MediaSource.isTypeSupported('video/mp4; codecs="hvc1.2.4.L120.90"')`, then play).
- **Reddit:** its API returned 403, a headless browser was blocked, old.reddit needs a login, and a
  site-restricted web search returned nothing. Issue trackers stood in for it (§1.2).
- **Emby's forum search** returned 403; topic pages were read instead. Emby's old supported-browser blog
  post now returns 404.
- **Disney+'s old version numbers** exist only in third-party copies; every archived capture of its page
  is the same script shell.
- **Samsung Internet's adaptive dark flag:** whether it is on by default, and in which version.
- **When Firefox's macOS decoder gained 10-bit output**, so whether 140 has it.
