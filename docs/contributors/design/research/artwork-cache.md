> **ABOUTME:** How competitors bound and evict their image caches, measured image sizes, the cost
> of a large library, and an eviction policy for a configurable ceiling. Input to the artwork
> plan.

# Artwork cache: ceiling, eviction and the high-water case

Research for `docs/contributors/design/plans/artwork.md` § Open, "Total cache ceiling, and what evicts".
Date 2026-09-25. Unless a line says **[measured]**, **[source]** or **[code]**, treat it as analysis.
**[est]** marks an estimate. **[unverified]** marks something I could not confirm.

Local copies of what I fetched: `/yoloai/cache/http/*.summary.md` (WebFetch summaries),
`/yoloai/cache/tmdb-img/` (the images I measured), `/yoloai/cache/jellyfin`,
`/yoloai/cache/jellyfin-web`, `/yoloai/cache/xbmc` (source clones, Sept 2026 HEADs).

---

## TL;DR

- **Nobody in this space enforces a size ceiling on image caches.** Jellyfin and Plex delete
  cache files by **age** (30 days, by mtime or creation time), whether or not the files are
  used. Kodi deletes images that have gone **unused for 30 days *and* that no library item
  references any more**. Emby deletes an item's metadata folder when the item is removed. None
  has a byte limit. The requests for one ("Customisable max dirsize", Plex; "a simple size
  limit", Jellyfin #3929) went unanswered. Only generic HTTP caches (nginx `max_size` +
  `min_free`, LRU) enforce a ceiling.
- **Their caches grow because the set of sizes is open.** Plex users report PhotoTranscoder
  caches of 42–577 GB. Jellyfin's cache key includes the client's exact pixel width × DPR, the
  quality, *percent played* and *unplayed count*, so each window size, each device and each
  change in watch state makes a new file. Orpheum's closed size set already removes the cause.
  With it, the numbers are small.
- **Real numbers [measured from TMDB's CDN, WebP served by TMDB's own encoder; n=6 films, 16 episodes]:**
  poster 2000×3000 ≈ 750–810 KiB WebP. Backdrop 3840×2160 ≈ 480–490 KiB. Episode still
  1920×1080 ≈ 130 KiB. A poster at grid size (w342) ≈ 38–43 KiB. For **5,000 films + 500 series
  + ~2,000 seasons + 20,000 episodes**: base images at provider-original resolution come to
  **≈10.5 GiB**. Three derived sizes add **≈2.6 GiB**. With the base capped at the largest
  size actually served, base is **≈4.7 GiB [est]**. A second poster language adds
  ≈5.5 GiB (originals) or ≈2.3 GiB (capped).
- **Derived sizes are about 20% of the bytes.** Purging all of them buys little, which is why
  the owner's high-water case is really "the bases don't fit".
- **Recommendation (detail in §3–§4):** evict in **cost order, not only recency order**:
  1. tombstoned items;
  2. large derived sizes, least recently served first;
  3. bases that can be rebuilt locally (sidecar or embedded art);
  4. provider-fetched bases, least recently served first.
  Never evict the grid-size derivative of a live item. Record recency **per item, at day
  granularity, from an in-memory set flushed in batches**. Nothing is written on the request
  path and filesystem atime is never used. Enforce the limit with **two watermarks** (sweep
  when over the ceiling, stop at 90% of it) **plus a minimum-free-space floor**. In the
  high-water case **keep serving**, fall back to on-demand fetch, and raise a **persistent,
  hysteretic admin health item** that gives the numbers and a recommended ceiling.

---

## 1. How competitors bound and evict

### Jellyfin

- **Where:** resized images go to `<cache>/images/resized-images/` **[code]**
  (`src/Jellyfin.Drawing/ImageProcessor.cs:77`, `ImageCachePath => Path.Combine(CachePath, "images")`
  in `BaseApplicationPaths.cs:55`). Downloaded provider originals are *not* in the cache.
  They go next to the media when "save artwork into media folders" is on, otherwise to the
  internal metadata path (`ImageSaver.cs:93`, `BaseItem.GetInternalMetadataPath`) **[code]**.
  The Clean Cache task never touches them.
- **Cleanup:** scheduled task **"Clean Cache Directory"** (`DeleteCacheFileTask`, key
  `DeleteCacheFiles`), which runs **every 24 h** **[code]**
  (`Emby.Server.Implementations/ScheduledTasks/Tasks/DeleteCacheFileTask.cs`). It deletes every
  file under the cache path whose **last-write time is older than 30 days**. It deletes temp
  files older than 1 day. It has **no size limit and no access tracking**. A cache hit does not
  touch the file (`if (!File.Exists(cacheFilePath)) encode…; return cacheFilePath`), so a
  poster viewed every day is still deleted on day 31 and re-encoded.
- **Why it grows:** the cache filename is built from source path, **quality, datemodified,
  format, width, height, maxwidth, maxheight, fillwidth, fillheight, percentPlayed,
  unwatchedCount, blur, background colour, foreground layer** **[code]**
  (`ImageProcessor.GetCacheFilePath`). jellyfin-web asks for
  `fillWidth = ceil(cssWidth × devicePixelRatio)` at **quality 96** **[code]**
  (`jellyfin-web/src/components/cardbuilder/utils/url.ts:165-167`). So every layout width, DPR
  and watch-state change makes a new file.
- **Complaints:**
  - Discussion #16808 (2026-05-09): WebP files in `resized-images` **over 15 MB each** "because
    of the 4K clients". The user's recompression script is undone when Jellyfin regenerates
    the files. No reply. [source](https://github.com/orgs/jellyfin/discussions/16808)
  - Issue #9069: people thumbnails stored unscaled from TMDB, "over 1MB" each, "hundreds of
    MB". Closed as not planned. [source](https://github.com/jellyfin/jellyfin/issues/9069)
  - Issue #3929 (2020): "a simple size limit" for the transcode cache. Closed with no visible
    maintainer reply. [source](https://github.com/jellyfin/jellyfin/issues/3929)
  - Issue #15036 (10.11.0, 2025-10-20): Jellyfin **refuses to start** when data, cache or temp
    has under 2 GB free ("The path `/tmp/jellyfin` has insufficient free space. Required: at
    least 2GB."). This is its only "cache too big" signal, and it is fatal.
    [source](https://github.com/jellyfin/jellyfin/issues/15036)

### Plex

- **Where:** `…/Plex Media Server/Cache/PhotoTranscoder`. It holds every resized image for every
  client resolution, for video artwork as well as photos. The provider originals live in
  `Metadata/` bundles.
- **Cleanup:** weekly scheduled maintenance. "This task cleans up cache files that were created
  more than a month ago." **Clean Bundles** (weekly) deletes bundles of removed items, which is
  the tombstone case. **Optimize Database** (weekly) is SQLite only.
  [Plexopedia](https://www.plexopedia.com/plex-media-server/general/scheduled-tasks/). Plex's own
  support page returned 403 to me **[unverified directly]**. **There is no size setting.**
- **Complaints** ([thread 631581](https://forums.plex.tv/t/huge-size-of-phototranscoder-folder/631581)):
  - 55 GB, 170 GB, 183 GB, 42 GB (PMS 1.20.3.3483), 300 GB.
  - **248 GB on a 500 GB boot drive**, which regrew to 113 GB within 12 days of deletion.
  - 577 GB in a linked thread title.
  - Staff (drzoidberg33, Sept 2020): "We should be clearing cache files older than 30 days
    every week". That cleanup was broken on some platforms, and a fix followed. Staff also said
    manual deletion is "perfectly safe" and mentioned exploring a configurable expiry.
  - Feature request
    ["Cache/PhotoTranscoder Folder Size Sanity Check"](https://forums.plex.tv/t/cache-phototranscoder-folder-size-sanity-check/639837):
    "Customisable max dirsize would be a nice option". Sizes 4 GB to ~100 GB. No staff reply.
  - Also ["6.5 GB, should I delete?"](https://forums.plex.tv/t/cache-phototranscoder-contains-over-6-5-gb-of-data-should-i-delete/773817)
    (not opened).
- Third-party tools exist only to clean Plex images: Kometa
  [ImageMaid](https://github.com/Kometa-Team/ImageMaid) (not read in detail).

### Emby

- Its option "Keep a cached copy of images in the server's metadata folder" produced **145 GB**
  in `metadata/library` on a 256 GB device (Feb 2024). The fix was to delete every `imagecache`
  and `thumbnailcache` directory by hand. 4.9.0.32 (Jul 2025) purges ImageCache on refresh when
  the option is off.
  [source](https://emby.media/community/topic/126733-image-cache-and-storage-problems/)
- Tombstones: "When an item is removed from the database, so too is it's folder under the
  server's metadata folder" (Luke, 2023-11-01). Orphans still leak through permission errors,
  and there is a "scan metadata folder" task.
  [source](https://emby.media/community/topic/122806-cleanup-the-metadata-of-items-no-longer-in-the-library/)
- I found no size limit and no documented age-based cleanup task for Emby **[unverified; not exhaustive]**.

### Kodi (the most thoughtful design here)

**[code, xbmc HEAD 2026-09-24]**

- **Storage:** `Thumbnails/` plus `Textures13.db`. Each texture has `usecount` and
  `lastusetime` per size row (`TextureDatabase.cpp:113`).
- **Recency is batched:** a use is appended to an in-memory vector. Every **100 uses** one job
  writes them all in a single transaction (`TextureCache::IncrementUseCount`,
  `CTextureUseCountJob`). The request path never does a per-hit write.
- **Eviction is "orphaned and idle":** `GetOldestCachedImages` selects images with
  `lastusetime < now − 30 days` whose `lastlibrarycheck` is stale.
  `CImageCacheCleaner::ScanOldestCache` then asks the video, music and add-on databases which
  of them are **still referenced**. The referenced ones get `lastlibrarycheck` stamped and are
  **kept**. Only unreferenced ones are deleted. The job runs about 1,000 images per pass, every
  6–48 h, and re-checks "used since" before deleting (`TextureCache::ScanOldestCache`). That is
  "tombstoned first" with an idle guard, **and nothing beyond it**: Kodi never evicts a
  referenced image, and it has no ceiling.
- **Bounding by resolution instead of by count:** `advancedsettings.xml` `<imageres>` (default
  **720**) and `<fanartres>` (default **1080**), both 0–9999 (`AdvancedSettings.cpp:367-368,
  1175-1176`). The wiki's "reduce disk space" advice is to lower them. It stores JPEG (PNG when
  there is alpha), with `imagequalityjpeg` defaulting to 4 on a 0–21 scale.
  [Kodi wiki Artwork/Cache](https://kodi.wiki/view/Artwork/Cache) is behind Cloudflare, so
  **I read the defaults from source, not from the wiki**.
- Complaint: ["20 GB of thumbnails: remove them?"](https://forum.kodi.tv/showthread.php?tid=367372) (title only; not opened).

### Infuse

- It has a per-share "Pre-Cache Images" option (off means artwork is fetched on demand) and a
  "Library Cache" size display with a clear button
  ([Settings Overview](https://support.firecore.com/hc/en-us/articles/360015608854-Settings-Overview), via search snippet).
- On Apple TV **the OS evicts it**: "by the time you get the message in Infuse the storage has
  already been wiped (this is done by tvOS, not Infuse)". One user reported a ~500 MB metadata
  cache for a 4 TB library.
  [source](https://community.firecore.com/t/infuse-removed-metadata-due-to-lack-of-space/36681)
- The lesson: a client cache that can vanish at any time is normal, and Infuse treats it as
  refetchable.

### Generic prior art (for the policy only)

- **nginx `proxy_cache_path`:** `max_size` plus **`min_free`** (1.19.1): "When the size is
  exceeded or there is not enough free space, it removes the least recently used data". It
  evicts in bounded batches (`manager_files` 100, `manager_threshold` 200 ms, `manager_sleep`
  50 ms), and `inactive` removes idle entries regardless of size.
  [docs](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
- **SIEVE / S3-FIFO** (NSDI'24 / SOSP'23): on web traces, "quick demotion" of one-hit wonders
  and "lazy promotion" (a visited bit rather than moving the entry on each hit) beat LRU on
  miss ratio and are cheaper per hit.
  [SIEVE paper](https://www.usenix.org/system/files/nsdi24-zhang-yazhuo.pdf),
  [;login: summary](https://www.usenix.org/publications/loginonline/sieve-cache-eviction-can-be-simple-effective-and-scalable)
- **ARC / 2Q / W-TinyLFU** (Caffeine) are scan-resistant and adapt between recency and
  frequency. They are built for in-memory caches with millions of operations per second, which
  is far more than Orpheum needs **[from general knowledge; not re-fetched]**.
- **Linux `relatime`:** atime is updated only if older than mtime/ctime, or "more than 1 day
  old". [mount(8)](https://man7.org/linux/man-pages/man8/mount.8.html). It is prior art for
  "record recency at day granularity", and a reason **not** to rely on atime: with `noatime`
  it is never written.

---

## 2. Real numbers

### Per image [measured]

Source: `image.tmdb.org`, the same image fetched as JPEG (default) and as WebP (`Accept:
image/webp`; TMDB's CDN negotiates). Sample: posters and backdrops of 6 films and 16
episode stills from a season each of two series. The sample is small, so read these as ±30%. The WebP files come from TMDB's encoder at unknown quality,
but they are real deliverable bytes.

| Kind | Size (px) | JPEG median / mean | WebP median / mean | WebP/JPEG (median) |
|---|---|---|---|---|
| Poster w92 | 92×138 | 6.0 / 8.9 KiB | 4.8 / 8.8 KiB | 0.78 |
| Poster w154 | 154×231 | 14.1 / 18.3 | 11.2 / 15.4 | 0.83 |
| Poster w185 | 185×278 | 17.4 / 22.3 | 15.9 / 16.6 | 0.86 |
| Poster w342 | 342×513 | 51.5 / 50.9 | 37.8 / 43.1 | 0.75 |
| Poster w500 | 500×750 | 102.9 / 110.7 | 75.2 / 80.1 | 0.73 |
| Poster w780 | 780×1170 | 226.2 / 234.5 | 193.4 / 191.9 | 0.86 |
| Poster original | 2000×3000 | 985 / 873 | 809 / 751 | 0.95 |
| Backdrop w300 | 300×169 | 11.8 / 11.9 | 6.5 / 7.4 | 0.66 |
| Backdrop w780 | 780×439 | 47.5 / 54.0 | 28.7 / 37.7 | 0.66 |
| Backdrop w1280 | 1280×720 | 111.3 / 118.2 | 73.2 / 93.5 | 0.70 |
| Backdrop original | 3840×2160 | 671 / 727 | 477 / 490 | 0.69 |
| Still w300 | 300×169 | 13.8 / 13.9 | 9.7 / 9.6 | ~0.70 |
| Still original | 1920×1080 | 223 / 246 | 128 / 133 | ~0.57 |

For comparison, Google's WebP study reports "25%–34% compression gains compared to JPEG at equal
or slightly better SSIM" ([study](https://developers.google.com/speed/webp/docs/webp_study)).
That matches the 0.66–0.75 ratios above. The 0.95 for original posters suggests TMDB re-encodes
large posters at high quality. A WebP base encoded at q≈80 by Orpheum would probably land nearer
0.7× JPEG **[est]**.

Other data points:
- The owner's own library has only 8 local `folder.jpg`/`poster.jpg` files: 54–261 KB, with one
  poster at 3.4 MB. Local artwork can be far larger than TMDB's **[measured, n=8]**.
- Jellyfin #16808 reports resized WebP >15 MB (4K client, quality 96).

### A large library

Counts: 5,000 films, 500 series, **~2,000 seasons [est: 4 per series]**, 20,000 episodes.
Per film and per series: 1 poster + 1 backdrop + 1 logo. Per season: 1 poster. Per episode:
1 still. Logos ≈ 30 KiB **[est]**. People photos are excluded: roughly 30k people, ≈0.3–1 GiB
depending on the size kept **[est]**. WebP means are from the table.

| | Count | Base at provider original | Base capped at largest served size **[est]** | Derived set **[est choice]** |
|---|---|---|---|---|
| Posters (film+series+season) | 7,500 | 751 KiB → **5.4 GiB** | 1000×1500 ≈ 310 KiB → **2.2 GiB** | w185+w342+w500 ≈ 140 KiB → **1.0 GiB** |
| Backdrops | 5,500 | 490 KiB → **2.6 GiB** | 1920×1080 ≈ 210 KiB → **1.1 GiB** | w300+w780+w1280 ≈ 139 KiB → **0.73 GiB** |
| Episode stills | 20,000 | 133 KiB → **2.5 GiB** | 1280×720 ≈ 65 KiB → **1.2 GiB** | w300+w780 ≈ 48 KiB → **0.9 GiB** |
| Logos | 5,500 | ~30 KiB → **0.16 GiB** | same → 0.16 GiB | ~0.05 GiB |
| **Total** | 38,500 images | **≈10.6 GiB** | **≈4.7 GiB** | **≈2.7 GiB** |
| + 2nd poster/logo language | | +5.5 GiB | +2.3 GiB | +1.0 GiB |

The "capped" column scales the measured w780 and w1280 sizes by pixel count, which overstates
the size a little, because bytes grow sub-linearly with pixels.

What follows:
- **Derived sizes are ~20% of the total.** Purging every derived size frees only ~2.7 GiB of
  ~13 GiB.
- **The grid-size derivatives of the whole library are tiny.** w342 posters, w300 backdrops and
  w300 stills together come to ≈0.55 GiB. Keeping them resident always is cheap.
- **The biggest single lever is the base resolution, not eviction.** A 3840×2160 backdrop or a
  2000×3000 poster serves no screen Orpheum targets that 1920×1080 or 1000×1500 would not.
  Capping the base at the largest served size halves the base. This is an open design point
  (plan § Open, "How many sizes"), so it is **for the owner to decide**, not for me to assume.
- Jellyfin and Plex reach 50–500 GB because their size set is open (per-client width × DPR ×
  quality × watch-state), not because the libraries are large. Orpheum's closed set of sizes
  (artwork.md "A closed set of sizes") bounds the cache at a few GiB per 40k images. A sensible
  default ceiling is probably **"measured need + headroom"** rather than a fixed number.

---

## 3. Eviction policy assessment

### The workload

- **Poster wall:** a burst of 50–100 grid-size requests per page, and a full scroll touches
  every item once. A scan like this is exactly what flushes a plain LRU.
- **Detail page:** one large poster, one backdrop and many stills, for a small set of items.
- **Long tail:** most of a 5,000-film library is seldom opened.
- **Immutable, content-addressed URLs** (artwork.md) mean clients cache hard. The server sees
  roughly **one request per client per image**, not one per view. Server-side recency therefore
  measures "a client first needed it recently", which is a weak and lumpy signal.
- **Miss costs differ by an order of magnitude:**
  - A derived miss is a local node re-derive from the base (milliseconds of CPU).
  - A base miss is a **network fetch from TMDB**: slow, possibly rate-limited or offline, and
    the provider may have **replaced the image**. The hash then changes, so the same URL cannot
    be rebuilt byte-identically.
  - A base from sidecar or embedded art is a local re-read by the node.
- **Tombstoned items have zero future value.** The one exception is an item that comes back,
  such as a disk re-attached.

### Options

| Policy | Fit | Problem |
|---|---|---|
| Age-only (Jellyfin, Plex: 30 days by mtime or creation) | Simple, no tracking | Evicts hot images and keeps nothing bounded. It is the cause of their complaints. |
| Orphan-only (Kodi) | Perfect for tombstones | No ceiling |
| Plain LRU, exact | Standard | Needs a write per hit, a full-wall scroll pollutes it, and it ignores miss cost and size |
| LFU | Keeps the home-page regulars | A newly added film starts at zero and is evicted first, the worst possible outcome for "recently added". It needs ageing. |
| ARC / 2Q / W-TinyLFU | Scan-resistant | Overkill. They assume millions of operations per second and uniform miss cost. |
| CLOCK / SIEVE (visited bit) | Cheap and scan-tolerant | Uniform-cost assumption, but it fits well **within one class** |
| **Cost-class order + coarse recency within a class** | Matches the asymmetric miss costs | Slightly more logic, all of it in a background sweep |

### Is "tombstoned first, then LRU" sound?

**Mostly yes. It is Kodi's orphan rule plus nginx's LRU.** It has three gaps:

1. **It treats all bytes as equal.** Under pressure it will evict a base (a network re-fetch,
   possibly a *different* image) before a large derived size of a more recently used item.
   Order by **rebuild cost first**, then recency.
2. **It needs a recency record, and per-request writes are the wrong cost.** Kodi's answer is
   the one to copy: record in memory and flush in batches. With immutable URLs the signal is
   coarse anyway, so **day granularity per item is enough**.
3. **The grid size must be protected.** One wall scroll is 50 misses on one page, which is the
   most visible failure there is. Grid derivatives for every live item come to ~0.5 GiB. Make
   them non-evictable while the item is live.

### Recommended policy

**Units and state:**
- Evict **per image file**, but track recency **per item** (a film, series, season or episode).
  One `last_served_day` integer per item lives in the store.
- The core keeps an **in-memory set of item IDs served** since the last flush. A background task
  flushes it every hour, or at shutdown, as one batched `UPDATE`. That is Kodi's batching with
  relatime's granularity.
- **No write on the request path, and no filesystem atime.** Losing up to an hour of recency on
  a crash only makes eviction slightly less accurate. That is harmless in a cache that is safe
  to delete.
- Size per file is known at write time, so keep a running total. Do not `du` the tree.

**Trigger:**
- A sweep runs when `total > ceiling` **or** `free space < min_free`. It evicts down to a
  **low watermark** (for example 90% of the ceiling), so the sweep does not thrash at the edge.
- Deletion is batched and bounded like nginx (N files or M ms per step) and runs on the side
  that owns the cache (D128: core-side posters and thumbnails, node-side trickplay and overlays).
- A daily sweep also removes tombstoned items' images, whatever the size (Kodi / Plex "Clean
  Bundles" behaviour). Delete **after a grace period** (for example 7–30 days), so a disk that
  comes back does not re-fetch everything. This should follow the existing tombstone lifecycle
  of the removal and gone-disk work rather than a new clock.

**Eviction order** (each class drained oldest `last_served_day` first before the next class
starts):
1. **Tombstoned items**, every file, oldest tombstone first.
2. **Large derived sizes** (detail and backdrop sizes: w500+ posters, w780+/w1280 backdrops and
   stills) of live items. The next request re-derives them from the base.
3. **Bases that are locally re-obtainable** (sidecar `poster.jpg`, embedded art) of live items.
   Re-reading them costs node I/O and no network.
4. **Provider-fetched bases** of live items, least recently served first. Re-fetching needs the
   network and may produce a changed image.
5. **Never:** the grid-size derivative of a live item. User-chosen art is outside `cache/`
   already (artwork.md).

This ordering is size-aware in effect: classes 2–4 hold the large files. Order within a class by
recency, not by size. Within a class, size-based policies (GDSF) mostly punish backdrops, which
are needed exactly when one is opened.

**A dependency to flag:** artwork.md says "Derived sizes are produced at scan time, in the node"
and "The core never parses an image at all". Evicting a derived size is only safe if there is an
**on-demand derive path**: core miss → ask node → node derives from base (or refetches) → upload.
Otherwise an evicted image stays broken until the next scan. The plan does not say what the core
returns while that happens. Options: a 503 with `Retry-After`, or a short-lived redirect to
another cached size of the same image. A redirect must not be `immutable`. **This is a design
question for the owner (rule 1), not something to invent.**

---

## 4. The high-water case

### What others do

**Nothing graceful.**
- Plex and Jellyfin grow until the disk is full. The Plex reports include the boot drive filling
  and the server crashing (248 GB on 500 GB).
- Jellyfin's only guard is a **startup refusal** below 2 GB free (#15036), which turns a cache
  problem into an outage.
- tvOS silently wipes Infuse's cache.
- nginx silently LRUs.
- No product I found **tells the admin the configured cache is too small for the library**. The
  Plex "sanity check" request asked for exactly that and was never answered.

### Recommendation

**Definition.** Count the **required set**: grid-size derivatives plus bases for every live item.
The high-water state is required set > ceiling, measured after tombstones and large derived sizes
have been purged. This is the owner's definition, made precise.

**The server keeps serving. It never refuses to start and never refuses to scan over cache size.**
Cache is always safe to delete (overview.md), so a small cache costs latency, never correctness.

**What to drop, in order:**
1. Continue down class 3, then class 4 of §3, least recently served first.
2. Keep grid derivatives for every live item, so the poster wall stays intact.
3. The items whose bases were dropped fetch or derive on demand when opened.
4. When fetching an evicted base only to derive one size, derive and **don't keep the base**
   (stream-through). Otherwise a detail-page visit evicts another item's base, and the cache
   churns.

**Guard against thrash:**
- Count "evicted then needed again within 7 days".
- If that rate is high, eviction is costing more than the ceiling saves. Report it. Do not tune
  anything automatically.

**Also respect `min_free`,** as nginx does. The cache usually shares a disk with the database.
The Plex boot-drive incidents show the ceiling alone is not enough. When `min_free` is breached,
evict as for high water and alert too, since the disk, not the setting, is the constraint.

**What to tell the admin:** a **persistent health item**, not a log line, with hysteresis:
- Raise it after the state holds for about 24 h. Clear it below about 90% of the ceiling.
- Example: *"The artwork cache limit is 2 GiB, and this library needs about 4.7 GiB to keep every
  item's artwork ready. 1,830 items' artwork is fetched when first opened, which is slower and
  needs internet access. Raise the limit to at least 5.5 GiB."*
- The recommended figure is `required_set × 1.15`, rounded up **[est headroom]**.
- Include the thrash count and the free-space figure when they are the cause.
- Expose the same numbers in the API, and in `/metrics` if one exists: bytes by class, required
  set, ceiling, evictions by class, refetches, thrash.

**Pre-flight:**
- When the admin **sets** the ceiling, show the current required-set figure next to it, so the
  mistake is visible before it happens.
- After a scan adds many items, recompute and warn if the required set now exceeds the ceiling.

### What the owner should settle

These are open or frozen design points.

- **Base resolution.** Keep provider originals, or cap at the largest served size? Capping is
  the largest single saving, about 2.2×.
- **The core's behaviour on a derived miss**, and the on-demand node derive path.
- The **default ceiling**. One option is "auto = measured required set × 1.5, bounded by a
  fraction of free space", which would make high water rare by default. The other is a fixed
  number.
- **The grace period** before a tombstoned item's art is deleted. Ideally it matches the
  existing removal and tombstone lifecycle.
