> **ABOUTME:** Which clients decode WebP and AVIF, from which version, and whether an Accept
> header can decide when to derive JPEG. Input to the artwork plan (D211's companion question,
> 2026-09-25).

# WebP / AVIF artwork compatibility matrix

Research date 2026-09-25. Question: artwork is stored as WebP and JPEG is made lazily only for
clients that cannot decode WebP. Which clients need JPEG, and how do we tell?

**How to read the evidence column.**
- **Primary**: a vendor spec page, caniuse data or MDN, read directly.
- **Derived**: worked out from a documented engine version plus caniuse. For example, a TV that
  documents "Chromium 94" gets Chrome 94's row.
- **Thin**: search results, user-agent databases, or what I remember. Treat these as unverified.

Pages fetched are cached under `/yoloai/cache/http/`. The facts from pages that only render with
JavaScript are in `_webfetch-notes-webp-matrix-2026-09-25.txt` there.

## Background facts used throughout

- **Chromium WebP support by version.**
  - Lossy only: Chrome 9 to 22.
  - Lossless and alpha: Chrome 23 and later.
  - Animated: Chrome 32 and later.
  - Sources: caniuse `webp.json` and the Google WebP FAQ.
- **Chromium AVIF support.** Chrome 85 and later (caniuse).
- **Android platform decoder** (`BitmapFactory`, used by native apps).
  - Lossy WebP: Android 4.0.
  - Lossless and transparent WebP: Android 4.2.1.
  - AVIF: listed as "Android 14+", mandatory from 14 (developer.android.com, "Supported media
    formats").
  - Animated WebP through `ImageDecoder`/`AnimatedImageDrawable`: API 28 (Android 9). This is
    from memory and I have not checked it against the API reference.
- **Apple ImageIO**, used by UIKit and SwiftUI in native apps.
  - WebP, including animated: iOS 14, iPadOS 14, tvOS 14, macOS 11 and watchOS 7. Sources are
    Apple's Safari 14 announcement coverage and SDWebImage issue #3041. I found no Apple
    developer doc page that states it.
  - AVIF: iOS 16 and macOS 13. For tvOS 16 the evidence is thin.

## Matrix

| Client | WebP (still) | Animated WebP | AVIF | Needs JPEG? | Evidence |
|---|---|---|---|---|---|
| **Chrome desktop** | Yes: full from 23, lossy-only 9–22 | 32+ | 85+ | No | Primary: caniuse, Google WebP FAQ |
| **Edge** | Yes: EdgeHTML 18, and every Chromium Edge (79+) | 18+ | 121+ (Jan 2024). Behind a flag before that | No | Primary: caniuse |
| **Firefox** | 65+ | 65+ | 93+ (still). Animated AVIF arrived later | No | Primary: caniuse, Google WebP FAQ |
| **Safari macOS 16+** | Yes | Yes | 16.4+. caniuse notes that 16.1–16.3 are still-only and need macOS 13 Ventura | No | Primary: caniuse |
| **Safari macOS 14.0–15.6** | Only on macOS 11 Big Sur or later. **None on Catalina (10.15) or earlier** | Same as still | No | **Yes, on Catalina or earlier** | Primary: caniuse note 3 |
| **Safari macOS ≤ 13** | No | No | No | **Yes** | Primary: caniuse |
| **iOS/iPadOS Safari, WKWebView, and every iOS browser** (all use WebKit) | iOS 14+ | iOS 14+ | iOS 16+ | Only on iOS ≤ 13 | Primary: caniuse `ios_saf`, Google WebP FAQ. That WKWebView matches Safari is derived: same WebKit and ImageIO |
| **Android Chrome** | 25+ | 32+ | 85+ (caniuse lists only the current version) | No | Primary: Google WebP FAQ. AVIF is derived |
| **Android System WebView** | Android 4.0+ lossy, 4.2+ full. Chromium-based since 4.4 and updated from Play since 5.0 | Chromium 32+ | Follows Chromium (85+). I have not checked this against WebView itself | No, in practice | Derived: caniuse `android` |
| **Chromecast, 1st gen (2013)** | Yes: launched on Chrome 31 (lossless and alpha OK). Last firmware 1.36 (Nov 2022), now unsupported | Not at launch (Chrome 31). Unknown at the last firmware | Unknown, assume no | No | Cast docs (primary) list WEBP as a supported Web Receiver image format with no per-device exceptions. The Chrome 31/32 user agents are thin (user-agents.net) |
| **Chromecast 2nd/3rd gen, Ultra** (CrKey) | Yes | Yes: user agents show Chrome 52 (Ultra, 2016) up to Chrome 90–92 (CrKey 1.56) | Probably yes on Chrome 85+ firmware, but not verified on the Cast platform | No | Primary: Cast "Supported Media" lists APNG, BMP, GIF, JPEG, PNG, WEBP. Chrome versions are thin (user-agent databases) |
| **Chromecast with Google TV (HD/4K), Google TV Streamer** | Yes | Yes | Likely: Android 12+/14 platform, and the user agent shows Chrome 140. Not verified for the Web Receiver | No | Primary: Cast Supported Media. Android version and user agent are thin (useragents.io) |
| **Cast Web Receiver, general** | Yes, on every device | — | Not listed | No | Primary: developers.google.com/cast/docs/media: "Image formats: APNG BMP GIF JPEG PNG WEBP". It also says images display at **720p max** (1280×720) |
| **Android TV / Google TV native apps** | Yes: platform decoder, and Android TV starts at Android 5 | API 28+ via ImageDecoder, or through the app's image library | Android 14+ guaranteed. Optional on 12–13 | No | Primary: Android supported formats |
| **Amazon Fire TV, Fire OS 5–8** (Android 5.1–11) | Yes: platform decoder, and Amazon WebView is Chromium | Native on Fire OS 7+ (Android 9). WebView yes | **Not native** (Fire OS 8 is Android 11). Amazon WebView version per device is undocumented | No | Derived from the Android table. Amazon WebView is documented as Chromium-derived, with release notes mentioning v84 and v87 |
| **Amazon Fire TV, Vega OS** (2025+, Linux) | Probably: Vega WebView is Chromium, version undocumented | ? | ? | Probably no | **Thin**: Amazon Vega docs say "Chromium engine" and give no version or format table |
| **Samsung Tizen 2015–2026** (Tizen 2.3–10) | **Yes, on every year**. Samsung's table marks "WebP image format" Yes from 2015 through 2026 | Chromium years (2017+, M47+) yes. 2015–16 WebKit unknown | Derived by engine: 2022+ (M85+) yes. 2021 (M76) and earlier **no** | No | Primary: Samsung "Web Engine Specifications", which has no AVIF row |
| **LG webOS 3.x–26** (2016–2026) | Yes | Chromium 38+ yes | Derived by engine: webOS 22 (C87)+ yes. webOS 6 (2021, C79) and earlier **no** | No | Primary: the webOS "Web API and Web Engine" table (webOS 3.x = Chromium 38 … webOS 26 = Chromium 132), and the image-format list (JPEG, GIF, PNG, BMP, **WebP**) on both the webOS 3.0 and 24 spec pages. No AVIF listed |
| **LG webOS 1.x/2.x** (2014–15, WebKit) | Unknown | Unknown | No | **Possibly** | Thin: the engine table says WebKit 537/538. Whether those builds included WebP is not documented |
| **Roku** (SceneGraph `Poster`) | **Roku OS 9.4+** (late 2020), alpha OK | Not documented, assume no | **No** (not listed) | **Yes, on Roku OS < 9.4** | Primary: Poster node doc: "supports JPEG, PNG and (since Roku OS 9.4) WebP". Roku tells apps to check the OS version before using WebP. Which legacy Roku models are stuck below 9.4 I did not verify |
| **Apple TV (tvOS) native** | tvOS 14+ via ImageIO | tvOS 14+ | tvOS 16+ | Only on tvOS ≤ 13 (HD 4th gen is on tvOS 17+ now, so effectively none) | Thin, but consistent with iOS: SDWebImage #3041 and Apple's WWDC 2020 coverage |
| **Kodi** | Yes: decodes WebP (an add-on client would get it) | Probably no | Unverified | No | Thin: Kodi wiki format list (search snippet, page 403) and xbmc#18531 ("webp format is well supported by kodi"). Artwork *file names* `*.webp` are not picked up by default, which does not matter to a server API |
| **Xbox** (Edge, Chromium, 2021+) | Yes | Yes | Edge 121+ | No | Derived: Edge is Chromium. The Xbox Edge version was not checked |
| **PlayStation 4/5 browser** | Unknown | Unknown | Unknown | Unknown | **No data.** The PS5 has only a hidden, WebKit-based browser and no public spec. Out of scope |

## Accept-header negotiation: who sends `image/webp`

Values for `<img>` and CSS image requests, from MDN "List of default Accept values":

| Sender | Image `Accept` value | Advertises WebP? |
|---|---|---|
| Chrome, and Edge 121+ | `image/avif,image/webp,image/apng,image/*,*/*;q=0.8` | Yes, and AVIF too |
| Firefox 65–91 | `image/webp,*/*` | Yes |
| Firefox 92–127 | `image/avif,image/webp,*/*` | Yes |
| Firefox 128+ | `image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5` | Yes |
| Safari on Big Sur+ (and iOS 14+) | `image/webp,image/png,image/svg+xml,image/*;q=0.8,video/*;q=0.8,*/*;q=0.5` | Yes. **AVIF is not advertised** in MDN's value, even though Safari 16.4 decodes it |
| Safari before Big Sur | `image/png,image/svg+xml,image/*;q=0.8,…` | No. This is correct, because it cannot decode WebP |
| Browser navigation, `fetch()`, XHR | `*/*` or the page default | **No**, whatever the browser can decode |
| Native HTTP stacks (Android OkHttp/Glide/Coil, iOS URLSession, Roku `roUrlTransfer`/Poster, Kodi curl) | `*/*`, `image/*` or no header, in my experience | **No**. Thin: I did not check each library |

**What others do.**
- **Cloudflare Polish** serves WebP "only when the `Accept` header from the browser includes
  WebP". It refuses origin responses that `Vary` on anything but `Accept-Encoding`, so the CDN
  does the negotiation. Source: Cloudflare docs.
- **Jellyfin** (`ImageController.GetClientSupportedFormats`, read from source) serves WebP when
  either:
  - `Accept` contains `image/webp`, or
  - a `?format=`/`accept` query parameter asks for it.

  Its one user-agent special case is Crosswalk on Android. Otherwise it serves JPEG. So its
  native clients that send no `Accept` get JPEG unless they ask for a format.
- The **Google WebP FAQ** recommends `Accept` negotiation for browsers, and `<picture>` or JS
  feature detection as client-side alternatives.

**How reliable `Accept` is.**
- **Browsers:** reliable for `<img>` and CSS loads. Every engine that decodes WebP sends
  `image/webp`, and every one that cannot (Safari before Big Sur) doesn't. A false positive is
  not a practical concern.
- **Not reliable for anything else.** `fetch()` loads, service-worker requests and every native
  client send `*/*` or nothing. If "no `image/webp`" means "JPEG", then:
  - WebP-capable native clients (Roku 9.4+, Android TV, tvOS, Kodi) get needlessly downgraded.
  - The server derives JPEG for them. That defeats the "lazily, only for incapable clients"
    goal.
- **`Vary: Accept` is expensive.** Content-addressed `immutable` URLs (artwork.md) and
  `Vary: Accept` fight each other. `Accept` strings vary a lot, so caches fragment on them, and
  some CDNs, like Polish, won't cache varied responses at all.

## What this means for us

**1. Which of our likely clients need JPEG at all?**
- **Cut A's clients never need it.** These are:
  - the web client in Chrome, Firefox and iPhone Safari (M3);
  - a remote TV's browser;
  - our Cast Web Receiver on any Chromecast or Google TV device (Google lists WEBP for the Web
    Receiver).

  The only exceptions are an iPhone or iPad on iOS ≤ 13, or a Mac on Catalina or older. Samsung
  2015+ and LG webOS 2016+ browsers and web apps decode WebP by vendor spec.
- **Outside Cut A**, JPEG is needed only for:
  - Roku devices on OS < 9.4;
  - LG webOS 1.x/2.x (2014–15), which is unconfirmed;
  - very old Apple OSes (iOS ≤ 13, macOS ≤ 10.15, tvOS ≤ 13).

  Each is a small, shrinking set. Everything built on Chromium 32+, Android 4.2+ or
  Apple 14-series OSes decodes still and animated WebP.
- **AVIF is not safe as the only format.** It fails on:
  - Samsung 2021 and earlier;
  - LG webOS 6 and earlier;
  - Roku (all);
  - Fire OS natively;
  - Safari/iOS before 16;
  - Android native before 14.

  This matches artwork.md's current lean.

**2. Is `Accept` negotiation enough to decide?**

It is enough for browsers only.
- **Browsers:** negotiating on `image/webp` in `Accept` works, and it is exactly how Cloudflare
  and Jellyfin decide.
- **The problems:**
  - Native clients and `fetch()` don't send it.
  - `Vary: Accept` undermines the immutable, content-addressed artwork URLs.
- **The better fit** is to put the format in the URL, like Jellyfin's `format` query parameter
  but part of the content address, so each variant is its own immutable URL. The client then
  picks the format:
  - **Web client and Cast receiver:** always `.webp`. Cut A needs no negotiation at all.
  - **Future native clients:** choose from a per-device capability. Roku reads its OS version
    (≥ 9.4 means WebP), and tvOS/iOS read theirs.
  - **An old-browser fallback:** either `<picture>` with a JPEG `<source>`, which makes the
    browser request the JPEG URL only when it can't decode WebP (and that request is the lazy
    trigger), or a one-time JS WebP probe. Both are driven by the client, not by `Accept`.
- **If we keep one URL with `Accept` negotiation**, treat a missing `image/webp` in `Accept` as
  "unknown", not "incapable". Otherwise every native client causes JPEG derivation.

## Gaps and unverified items
- **Chromecast firmware:** the Chrome version of each generation's current firmware, and AVIF on
  the Cast platform. Primary docs list WEBP only. Needs a device check, which fits
  playback-survey Phase 0 and the planned Chromecast purchase.
- **LG webOS 1.x/2.x:** WebP support.
- **Samsung 2015–16 (WebKit):** animated WebP.
- **Roku:**
  - which models are stuck below OS 9.4;
  - whether the Poster node animates WebP.
- **Amazon:**
  - the Amazon WebView Chromium version on current Fire OS;
  - Vega OS's engine version and formats.
- **Apple:** tvOS WebP and AVIF versions rest on secondary sources, not an Apple doc page.
- **Kodi:** WebP decoding version, and AVIF.
- **PlayStation:** no data.
- **Native HTTP libraries:** the default `Accept` values of OkHttp, Coil, Glide, URLSession and
  Roku were not checked one by one.
