> **ABOUTME:** What competitors, providers and platform guidelines size artwork at, per image type
> and screen, and a proposed initial closed set of sizes. Input to the artwork plan.

# Artwork sizes: competitor research and a proposed closed set

Research for `docs/contributors/design/plans/artwork.md` → *Open: "How many sizes, and whether
the TV's poster-wall size is the same as the phone's"*. Date: 2026-09-25.

Legend: **[src]** = read in source code (commit given) · **[doc]** = official documentation
fetched · **[3p]** = third-party / forum evidence · **[unverified]** = my inference or
recollection, not confirmed by a fetched source.

Cached sources: `/yoloai/cache/http/`, clones in `/yoloai/cache/{jellyfin,jellyfin-web,jellyfin-androidtv,xbmc}`.

---

## 1. TL;DR

- Nobody who ships a closed set of sizes uses more than about seven per image type. TMDB
  (w92…w780 + original) and Kodi (a 720-high cap, or 1080 for 16:9) are the two closed
  designs. Jellyfin, Emby and Plex take any size the client asks for. All three have paid for it
  in cache blow-ups and in regenerating images for one-pixel differences (§2.2, §2.3, §2.4).
- **Jellyfin has already half-closed its own set for backdrops.** jellyfin-web snaps backdrop
  requests to `[480, 720, 1280, 1440, 1920, 2560, 3840, 5120, 7680]` and rounds the card-grid
  window width down to the nearest 100 px. That is evidence that an open grammar was a mistake.
- Every client in the survey lands in the same few pixel bands. Most shapes a client can take
  are covered by a **doubling ladder** (240 → 480 → 960 wide for portrait, 480 → 960 → 1920 → 3840
  for landscape). Each step covers DPR 1× to 2× of the step below, so the client picks the
  smallest size that is ≥ CSS width × DPR.
- **Proposal: 9 named variants in 3 aspect classes, not per image type.** 3 portrait
  (2:3), 4 landscape (16:9), and 2 logo sizes that fit inside a box. Posters and person images
  share the portrait ladder. Backdrops, episode stills and thumbs share the landscape ladder.
  Never upscale.

---

## 2. What competitors produce and request

### 2.1 Metadata providers: the source sizes we will actually get

| Provider | Poster 2:3 | Backdrop / fanart 16:9 | Episode still | Logo | Other | Source |
|---|---|---|---|---|---|---|
| **TMDB** served sizes (closed set, widths) | w92, w154, w185, w342, w500, w780, original | w300, w780, w1280, original | w92, w185, w300, original | w45, w92, w154, w185, w300, w500, original (SVG only at original) | profile: w45, w185, h632, original | [doc] `/3/configuration` example payload, developer.themoviedb.org; image-basics page |
| **TheTVDB** upload spec (fixed) | 680×1000 | 1920×1080 ("Backgrounds") | 640×360 (16:9) / 640×480 (4:3) | ClearLogo 800×310 | Banner 758×140; ClearArt 1000×562; Icon 1024×1024; Actor 300×450 | [doc] support.thetvdb.com FAQ ids 1, 6, 8, 10, 11, 12, 95, 96 |
| **fanart.tv** | 1000×1426 | (1920×1080) [unverified] | — | clearlogo 800×310 (780×290 plus a 10 px gutter) | | [3p] search summary of fanart.tv rules / Kodi wiki |
| **Apple TV partners** (delivery) | min 2000×3000 | min 1920×1080, 3840×2160 preferred | | | | [doc] tvpartners.apple.com/support/3708 |
| **Amazon Prime Video** (delivery) | min 2000×3000 | min 1920×1080 | | | | [3p] search summary; page is JS-only, not read |
| **Infuse** recommended local art | 1000×1500 | 1920×1080 or 3840×2160 | | | | [doc] support.firecore.com 4405042929559 |
| **Plex** (forum, staff-adjacent) | 1000×1500 auto-downloaded; 2000×3000 common | | | | | [3p] forums.plex.tv/t/216841 |

Takeaways:
- Poster sources are usually **680–2000 px wide**. A tier above ~1000 wide is often an upscale
  and must alias down to the largest size that is really available.
- Backdrop sources are **1920 wide at the least, and often 3840**. TVDB episode stills are only
  **640×360**, so for TVDB-sourced episodes nothing above the 480 landscape tier is real.
- Logos are **~800×310 (≈2.58:1)** by convention, but TMDB logos vary in aspect ratio and
  can be SVG. Logos need a "fit inside a box" size, not a fixed aspect ratio.

### 2.2 Jellyfin (server, jellyfin-web, Android TV)

**Server API** [src, jellyfin@208c278]:
- `GET /Items/{id}/Images/{type}` takes `width, height, maxWidth, maxHeight, fillWidth,
  fillHeight, quality, blur, …` (`Jellyfin.Api/Controllers/ImageController.cs`).
- Resizing is done by SkiaSharp (`src/Jellyfin.Drawing.Skia/SkiaEncoder.cs`). **Every distinct
  parameter combination is a distinct cache file**: the cache key concatenates
  `quality, datemodified, format, width, height, maxwidth, maxheight, fillwidth, fillheight, pp,
  uc, blur, …` (`src/Jellyfin.Drawing/ImageProcessor.cs`, `GetCacheFilePath`), under
  `cache/images/resized-images`.
- `fill*` never upscales (`DrawingUtils.ResizeFill` "Clamp to current size").
- A scheduled task deletes cache files older than **30 days**
  (`Emby.Server.Implementations/ScheduledTasks/Tasks/DeleteCacheFileTask.cs`). That is eviction
  by age, because the key space is unbounded.

**jellyfin-web** [src, jellyfin-web@134e6ad]:
- Cards: `fillWidth/fillHeight = ceil(cssSize × devicePixelRatio)`, `quality=96`
  (`src/components/cardbuilder/utils/url.ts`). The requested size is therefore continuous in both
  window width and DPR.
- Card CSS width = `windowWidth / postersPerRow`. The window width is rounded down to a multiple
  of 100 when the window is resizable (`cardBuilder.js`). Posters per row
  (`utils/builder.ts`), with the resulting CSS width of each card:

  | Window (CSS px) | Portrait / row | ≈ card width (CSS) | Backdrop / row | ≈ card width |
  |---|---|---|---|---|
  | TV layout (any) | 6 | 1920→320 | 4 | 1920→480 |
  | ≥ 2200 | 10 | 220–256 | 6 (≥2500) | ≥ 416 |
  | ≥ 1920 | 9 | 213 | 5 | 384 |
  | ≥ 1600 | 8 | 200–240 | 5 | 320–384 |
  | ≥ 1200 | 6 | 200–233 | 4 | 300–350 |
  | ≥ 800 | 5 | 160–240 | 3 | 267–400 |
  | ≥ 500 (phone landscape / small tablet) | 3 | 167–267 | 2 | 250–385 |
  | < 500 (phone portrait) | 3 | ~120–166 | 1–2 | ~190–400 |

  Overflow rows (home screen): portrait cards are 42% of the width below 400 px, falling to 15%
  on wide screens. Backdrop cards are 72% of the width on phones, falling to 23.5% at ≥ 1800 px.
  On a 390 CSS px phone at DPR 3 a portrait overflow card is 0.315 × 390 × 3 ≈ **370 px** wide,
  and a backdrop card is 0.72 × 390 × 3 ≈ **840 px** wide.
- **Backdrops (full-screen and the detail-page hero):** `maxWidth = dom.getScreenWidth()`. That
  function **snaps to `standardWidths = [480, 720, 1280, 1440, 1920, 2560, 3840, 5120, 7680]`**
  (`src/utils/dom.ts`). It does not multiply by DPR. This is a closed set in all but name.
- Detail-page primary image: `width = innerWidth × 0.25`, times DPR, so ~480 CSS px on a
  1920 window.
- Card logo overlay: height 40 CSS px. Default quality when a size is given: 90
  (`ServerConnections.js`).

**jellyfin-androidtv** [src, jellyfin-androidtv@36ec2f4]:
- Cards request `maxWidth/maxHeight = card size in px`. The card is `staticHeight` dp, or by
  default 150 dp for portrait and 130 dp for landscape (`ui/presentation/CardPresenter.kt`). Home
  rows use 126–150 dp. Android TV lays out at 960×540 dp over 1080p (density 2), so a 150 dp
  poster is **~200×300 px** and a 130 dp 16:9 thumb is **~462×260 px**.
- Library grid ("Poster size" setting): 5 to 15 columns across the screen (MED = 11 portrait or
  7 thumb columns; X_LARGE = 5 or 3) (`ui/browsing/BrowseGridFragment.java`). At 1920 px, 5
  columns is ≈ 350 px per poster and 11 columns is ≈ 160 px.
- Logos: `maxWidth 600` on the detail page and `440` on the playback overlay
  (`FullDetailsFragment.java`, `CustomPlaybackOverlayFragment.java`).
  `MAX_PRIMARY_IMAGE_HEIGHT = 370` (`util/ImageHelper.kt`).
- **Backdrops are requested with no size at all**, i.e. the original
  (`data/service/BackgroundService.kt`). A TV app downloads multi-MB originals as backgrounds.

### 2.3 Emby

- Same API lineage as Jellyfin (Jellyfin forked from it): `Width, Height, MaxWidth, MaxHeight`.
  "Users will often download high resolution images. Steps should be taken to ensure that you
  are not retrieving these at full size." [doc] github.com/MediaBrowser/Emby/wiki/Images.
- Cache fragmentation in the field: *"thumbnails are generated by exact resolution. So if the
  browser size is changed by a couple of pixels … the server will go away and regenerate all
  of these again taking 1-5 seconds an image"*, with examples of requests one pixel apart,
  e.g. `maxHeight=212&maxWidth=282&quality=90` [3p] MediaBrowser/Emby issue #3626.
- Emby's current web client is closed source. Its sizes are not verified beyond the above
  [unverified].

### 2.4 Plex

- `GET /photo/:/transcode?width=&height=&minSize=0|1&upscale=0|1&url=…` (also `opacity`,
  `background`, `format`, `blur`) [3p] Arcanemagus/plex-api wiki "Image Resizing Service";
  plexapi.dev "Get a Resized Photo".
- Observed requests: `width=110&height=165` (wiki example) and **`width=240&height=360&minSize=1&upscale=1`**
  (122 of 122 transcode requests in one Plex server log, pastebin Qymqkcye; the client is not
  identified) [3p]. So Plex poster grids ask for about **240×360**, with upscaling on.
- Cache blow-up in the field: the PhotoTranscoder cache reached "over 55GB", "170GB" and "183GB",
  and Plex staff acknowledged a bug fix [3p] forums.plex.tv/t/631581.
- The sizes Plex Web and the TV apps request for backgrounds are not documented. I found no log
  evidence for them [unverified]. A 1080p-class TV app would plausibly ask for
  width=1920&height=1080.

### 2.5 Kodi (texture cache) and Estuary

- Kodi downscales every cached texture: **`imageres` = 720** (max height for images in general)
  and **`fanartres` = 1080** (max height for images within 1% of 16:9). Max width =
  height × 16/9 [src] `xbmc/settings/AdvancedSettings.cpp` L367–368, `xbmc/pictures/Picture.cpp`
  `CacheTexture` (xbmc@c89e1fd). In effect: **posters cap at 480×720, fanart at 1920×1080**,
  and everything else fits inside 1280×720. Two sizes per image, no DPR logic, and users
  raise them in `advancedsettings.xml` for 4K.
- Estuary lays out at 1920×1080 (other aspect ratios keep the 1080 height) [src]
  `addons/skin.estuary/addon.xml`. Poster wall item slot **300×445** (`View_500_Wall.xml`),
  InfoWall **320×445** (`View_54_InfoWall.xml`), Poster view item width 317, focused 490
  (`View_51_Poster.xml`). A 1080p poster wall needs **~300 px wide** posters. A focused or
  enlarged poster needs ~480.
- Kodi's texture cache is per client, not per server. It is included here because its caps are
  a proven "good enough on a 1080p TV" answer.

### 2.6 Infuse

- Documents only the recommended *source* sizes: poster 1000×1500 and landscape 1920×1080 or
  3840×2160 [doc] Firecore support. Its client cache is on the device; its display sizes are
  undocumented [unverified].

### 2.7 Platform design guidance (10-foot and mobile)

| Platform | Layout canvas | Card sizes given | Implication | Source |
|---|---|---|---|---|
| **tvOS HIG** | 1920×1080 pt; **@1× and @2×** (Apple TV 4K renders @2× = 3840×2160 px) | Grid unfocused widths: 2 col 860 pt, 3 col 560, 4 col 410, 5 col 320, 6 col 260, 7 col 217, 8 col 184, 9 col 160 | 16:9 4-col card = 410 pt → **820 px** on 4K. 2:3 poster in 6–7 cols = 217–260 pt → **434–520 px** on 4K, 217–260 px on HD | [doc] HIG Layout JSON, HIG Images ("macOS, tvOS @1x and @2x") |
| **Android TV / Google TV** | Design at 960×540 dp; "assets need to aim for 1080p" | 20 dp spacing. Card width: 1-card 844 dp, 2-card 412, 3-card 268, 4-card 196, 5-card 124 (sic, as published) | 16:9 4-card = 196 dp → **392 px** at 1080p. Most Android TV devices render UI at 1080p even with 4K output [unverified] | [doc] developer.android.com TV cards, TV layouts |
| **Roku** | FHD UI 1920×1080 max (4K Roku devices still use an FHD UI) | Title-safe 1534×866 | Nothing above 1920×1080 is ever useful on Roku | [doc] developer.roku.com/docs/specs/graphics.md (safe zones); FHD cap as commonly documented [unverified in fetched text; tables JS-rendered] |
| **Apple TV app (content partners)** | — | Delivery: 16:9 cover 3840×2160 preferred; 2:3 min 2000×3000; the iOS 26 Apple TV app moved to **2:3 portrait** tiles | Portrait is back as the primary browse shape on Apple platforms | [doc] tvpartners.apple.com; [3p] bendodson.com 2025-09-22 |
| **Netflix partner artwork** | — | Partner Help Center artwork page moved or 404s; Netflix GFX canvas ≥ UHD width | Not usable as a size source [unverified] | [3p] |

---

## 3. Device pixel ratio: what a layout needs

The pixel width needed = **CSS/pt/dp width × DPR**, and the requested tier is the smallest tier
≥ that value. A doubling ladder guarantees that no image is more than 2× oversampled.

| Client class | Typical DPR | Canvas (logical) | Poster card (logical → px) | 16:9 card (logical → px) | Full-screen backdrop px |
|---|---|---|---|---|---|
| Phone portrait | 3 (iPhone, flagship Android); 2–2.75 others | ~390×844 | grid 3/row ≈ 120 → **360**; overflow 31.5% ≈ 123 → 370 | 72% ≈ 280 → **840** | 1170–1290 wide as a width-fit 16:9 band → 1920 tier (see note) |
| Tablet | 2 | 820–1366 | 5–6/row ≈ 150–230 → **300–460** | 3–4/row ≈ 270–400 → **540–800** | 1640–2732 → 1920 or 3840 |
| Desktop web 1× (1080p monitor) | 1 | 1920 | 8–9/row ≈ 213 → **213** | 5/row ≈ 384 → **384** | 1920 |
| Desktop / laptop 2× (MacBook, 4K at 200%) | 2 | 1440–1920 | ≈ 200–240 → **400–480** | ≈ 300–400 → **600–800** | 2880–3840 |
| 10-ft UI, 1080p TV (Roku, Android TV, Kodi, tvOS HD) | 1 (1080p panel) / 2 (dp) | 1920×1080 px | 150 dp / 260 pt / Kodi 300 → **200–320** | 130–196 dp / 410 pt → **260–410** | 1920 |
| 10-ft UI, 4K TV (tvOS @2×, browsers on 4K TVs) | 2 | 1920×1080 pt | 217–320 pt → **434–640** | 410 pt → **820** | 3840 |
| Detail-page hero poster | varies | ~25% of width | 480 CSS @1× → 480; @2× → **960** | — | — |

Note on phones: a full-bleed backdrop in portrait orientation that uses cover-fit to the
screen height is cropped, not width-limited. At 844×3 = 2532 px tall it would want a ~4500 px
wide 16:9 image. No competitor does this: Jellyfin uses screen width snapped to a standard
width, and TV apps use 16:9 screens. A phone hero should be a width-fit 16:9 band (1170–1290 px
wide → 1920 tier).

Observations:
- **1080p TVs and 1× desktops need the same poster size (~200–320 px) as phones at 3× in a
  3-column grid (~360 px).** A 480-wide tier serves 4K TVs, 2× laptops and 3× phones. The
  answer to the plan's question "is the TV's poster-wall size the phone's?" is **yes, within one
  ladder step**.
- 4K matters only on tvOS (@2×) and in browsers on 4K or HiDPI displays. Roku caps at FHD and
  Android TV UIs are usually FHD [unverified]. A 3840 backdrop is therefore a niche tier and is
  worth making lazy or conditional (§4.3).

---

## 4. Recommendation

### 4.1 Principles

1. **Size classes by shape, not by image type.** Poster, season poster, collection poster and
   person photo are all portrait. Backdrop, episode still, thumb and chapter image are all
   landscape. A logo is "fit inside a box". This keeps the set at 9 instead of ~20.
2. **Doubling ladders.** Each step is 2× the one below, so any DPR from 1 to 3 is at most 2×
   oversampled. Clients choose `smallest tier ≥ cssWidth × dpr`, and the server never
   interprets a number.
3. **Never upscale.** A tier larger than the source is not produced. The manifest points it at
   the largest real tier (compare Plex `upscale=0` and Jellyfin's fill clamp). TVDB episode
   stills at 640×360 therefore get only the 480 tier.
4. **Fixed aspect ratio by crop for portrait and landscape; fit-inside for logos.** This matches
   jellyfin-web `fillWidth/fillHeight` (cover crop) and TMDB (width-only, aspect preserved).
   Cropping to exact 2:3 or 16:9 lets the client reserve boxes before load. Keep blurhash (or
   similar) as the placeholder, so no tiny tier is needed.

### 4.2 The proposed closed set (initial)

| Name | Class | Pixels (W×H) | Primary consumers | Competitor anchor |
|---|---|---|---|---|
| `portrait-s` | 2:3 | **240×360** | 1× desktop grids, 1080p TV rows and walls (Android TV ~200×300, Estuary ~300 slot, tvOS HD 217–260), 2× phone lists, search results | Plex's observed 240×360; TMDB w185/w342 bracket; Jellyfin 1× grids 200–256 |
| `portrait-m` | 2:3 | **480×720** | 3× phone grids (~360), 2× tablet/laptop grids (400–480), 4K tvOS walls (434–520), 1080p focused/enlarged posters, 1× detail page | Kodi `imageres` cap 480×720; TMDB w500 |
| `portrait-l` | 2:3 | **960×1440** | Detail-page hero poster on 2×/3× and 4K; tvOS 4K 5-column (640) at 1.5× | TMDB w780; Infuse/Plex 1000×1500 source recommendation; TVDB posters (680×1000) never reach this tier |
| `landscape-s` | 16:9 | **480×270** | 1× desktop thumb/"continue watching" cards (300–480), 1080p TV thumbs (Android TV 392–462, tvOS HD 410), episode lists | TMDB still w300 (next up); TVDB episode 640×360 ceiling |
| `landscape-m` | 16:9 | **960×540** | 3× phone cards (~840), 2× laptop/tablet cards (600–800), 4K tvOS 4-col cards (820), 1080p TV hero on low-memory devices | TMDB w780 backdrop; Jellyfin `standardWidths` 720/1280 bracket |
| `landscape-l` | 16:9 | **1920×1080** | Full-screen backdrop and detail hero on every 1080p device, phones, tablets, 1× desktop; the Roku ceiling; Android TV background | Kodi `fanartres` 1080; TVDB background 1920×1080; Jellyfin snapped 1920; TMDB w1280 (next down) |
| `landscape-xl` | 16:9 | **3840×2160** | 4K tvOS @2× backgrounds, HiDPI desktop full-screen. **Produce only if source ≥ 3840 wide** (else alias to `landscape-l`) | Apple TV partner 3840×2160 preferred; Jellyfin snapped 3840 |
| `logo-s` | fit in box | **≤ 400×155** | Card/row logo overlays (Jellyfin 40 CSS px tall → 80–120 px at 2–3×), playback overlay on 1080p (Android TV 440 wide) | fanart.tv/TVDB 800×310 at half size; TMDB w300/w500 bracket |
| `logo-m` | fit in box | **≤ 800×310** | Detail hero title treatment on every screen (Android TV 600 wide on details; ~1/3 of the width of a 1080p hero). Most sources are exactly this, so it is usually a re-encode | fanart.tv/TVDB ClearLogo 800×310 |

Notes:
- **Banner (758×140)**, **ClearArt** and **disc art** from D149's enumeration: store and serve
  only the **original, re-encoded** (one "orig-safe" variant) until a client shows it. Only
  Kodi-lineage skins use them.
- **Square (1:1)** is not needed for video in Cut A. If music or cast circles arrive, add
  `square-s 240×240` and `square-m 480×480` on the same ladder.
- Logos should keep alpha: WebP or PNG, never JPEG. Format is still open in `artwork.md`. See
  also the AVIF/WebP caniuse data already in the cache.
- **SVG logos (TMDB)**: rasterise in the node to `logo-m` and `logo-s`. Never pass SVG to
  clients, since SVG is attacker-controlled markup (fits D99's "core never parses an image").

### 4.3 Which tiers most clients share

| Tier | Share of clients that use it as their main size |
|---|---|
| `portrait-s` | every 1× desktop, every 1080p TV (Roku, Android TV, Kodi-like, tvOS HD) |
| `portrait-m` | every phone, every tablet, every 2× laptop, 4K tvOS. **This is the workhorse.** |
| `landscape-s` / `landscape-m` | the same split as portrait-s / -m, for thumbs and episodes |
| `landscape-l` | every device's full-screen background except 4K tvOS and 5K desktops |
| `portrait-l`, `landscape-xl` | detail heroes and 4K only: the fewest requests, the most bytes |

If the set has to shrink further: **drop `portrait-l`** (the detail hero upscales `portrait-m`
by 2× on 4K, as Kodi does by default), and **make `landscape-xl` lazy**: produced on first
request, or only for titles actually opened on a 4K client. That leaves 7 eager tiers.

### 4.4 Rough cache cost (estimate, [unverified] byte sizes)

Assumed average encoded sizes (WebP q≈80, photographic art): portrait-s 15 KB, -m 45 KB, -l
140 KB; landscape-s 25 KB, -m 80 KB, -l 250 KB, -xl 800 KB; logo-s 12 KB, -m 35 KB. **Measure
this in the device lab before relying on it.**

| Item | Tiers | ≈ bytes |
|---|---|---|
| Film or series (poster + backdrop + logo, all tiers) | 9 | ~1.4 MB |
| … without `landscape-xl` | 8 | ~0.6 MB |
| Episode (still only: landscape-s, -m; -l only if source ≥ 1920) | 2–3 | ~0.1–0.35 MB |
| Season poster / person photo | 3 portrait | ~0.2 MB |

Example: 2,000 films + 300 series × 2 languages for posters and logos (backdrops are usually
language-neutral), + 15,000 episodes, + 5,000 people ≈ 2.3k × (0.35 MB lang × 2 + 0.35 MB) +
15k × 0.2 MB + 5k × 0.2 MB ≈ **2.4 + 3 + 1 ≈ 6.5 GB without `-xl`**, **+ ~1.8 GB** if every
title also has `landscape-xl`. That is bounded and computable, unlike Plex's 55–183 GB reports.

### 4.5 Client selection rule (for the SDK)

```
need = ceil(boxWidthCss * devicePixelRatio)          // or dp * density on Android TV
tier = first tier in ladder with width >= need, else the largest available
```
With hysteresis (prefer the smaller tier when `need ≤ 1.15 × smaller.width`), a card that is a
few pixels too wide does not jump a tier. The server only ever sees one of 9 names.

---

## 5. Open points for the owner

1. Should the crop for 2:3 and 16:9 tiers be **exact (cover-crop)** or **fit-width with
   original aspect**? Exact is better for layout and cache keys; it slightly crops non-standard
   posters (e.g. 27×40 one-sheets at ~0.675, a ~1% crop).
2. Eager vs lazy for `portrait-l` / `landscape-xl` (§4.3). The plan says "produced at scan
   time, in the node". A lazy tier would need the node on the request path, which D128 avoids
   for posters.
3. Whether the TV client (tvOS @2×) is in Cut A. If not, `landscape-xl` can be deferred
   entirely.

---

## 6. Sources

Code (cloned into `/yoloai/cache/`):
- jellyfin/jellyfin @208c278: `src/Jellyfin.Drawing/ImageProcessor.cs`, `MediaBrowser.Model/Drawing/DrawingUtils.cs`, `Jellyfin.Api/Controllers/ImageController.cs`, `Emby.Server.Implementations/ScheduledTasks/Tasks/DeleteCacheFileTask.cs`
- jellyfin/jellyfin-web @134e6ad: `src/components/cardbuilder/utils/{url,builder}.ts`, `src/components/cardbuilder/cardBuilder.js`, `src/utils/dom.ts` (`standardWidths`), `src/components/backdrop/backdrop.js`, `src/apps/legacy/controllers/itemDetails/index.js`, `src/lib/jellyfin-apiclient/ServerConnections.js`
- jellyfin/jellyfin-androidtv @36ec2f4: `ui/presentation/CardPresenter.kt`, `ui/browsing/BrowseGridFragment.java`, `util/ImageHelper.kt`, `data/service/BackgroundService.kt`, `ui/itemdetail/FullDetailsFragment.java`, `ui/playback/CustomPlaybackOverlayFragment.java`
- xbmc/xbmc @c89e1fd: `xbmc/settings/AdvancedSettings.cpp`, `xbmc/pictures/Picture.cpp`, `addons/skin.estuary/{addon.xml, xml/View_500_Wall.xml, xml/View_51_Poster.xml, xml/View_54_InfoWall.xml}`

Web (cached under `/yoloai/cache/http/`):
- TMDB configuration reference: https://developer.themoviedb.org/reference/configuration-details ; image basics: https://developer.themoviedb.org/docs/image-basics
- TheTVDB artwork types: https://support.thetvdb.com/kb/faq.php?id=8 (posters), id=10 (backgrounds), id=6 (episodes), id=11 (banners), id=96 (clearlogos), id=95 (clearart), id=1 (actors), id=12 (icons)
- Apple HIG Layout (tvOS grids): https://developer.apple.com/design/human-interface-guidelines/layout ; Images (scale factors): https://developer.apple.com/design/human-interface-guidelines/images
- Apple TV partner artwork: https://tvpartners.apple.com/support/3708-artwork-requirements ; iOS 26 portrait shift: https://bendodson.com/weblog/2025/09/22/portrait-artwork-ios-26-apple-tv-app/
- Android TV cards: https://developer.android.com/design/ui/tv/guides/components/cards ; layouts: https://developer.android.com/design/ui/tv/guides/styles/layouts
- Roku graphics spec: https://developer.roku.com/docs/specs/graphics.md
- Plex photo transcoder: https://github.com/Arcanemagus/plex-api/wiki/Image-Resizing-Service ; https://plexapi.dev/api-reference/server/get-a-resized-photo ; log https://pastebin.com/Qymqkcye ; cache size https://forums.plex.tv/t/huge-size-of-phototranscoder-folder/631581 ; poster sizes https://forums.plex.tv/t/local-art-height-width-ratios/216841
- Emby: https://github.com/mediabrowser/emby/wiki/Images ; https://github.com/MediaBrowser/Emby/issues/3626
- Infuse: https://support.firecore.com/hc/en-us/articles/4405042929559-Overriding-Artwork-and-Metadata
- Not usable: Netflix artwork requirements page (404); Amazon Video Central artwork spec (JS-only, known only from a search summary).
