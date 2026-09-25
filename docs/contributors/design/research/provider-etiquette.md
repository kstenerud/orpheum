> **ABOUTME:** What TMDB, TheTVDB, fanart.tv, OMDb and MusicBrainz ask of clients, how competitors
> refresh metadata and handle dead images, and what a polite 404 and retry policy would cost Orpheum.

# Provider etiquette: refreshes, dead images and retries

Research for the artwork job's handling of a 404 from an image server. The question: when
TMDB's image CDN answers 404 for a path our stored response names, should we refetch the title
once? And how should transient failures be retried without troubling the providers
([D170](../../decisions/working-notes.md#d170--one-source-per-title-fetched-once-a-users-correction-is-permanent-and-every-refetch-is-an-act-of-the-user),
[D212](../../decisions/working-notes.md#d212--artwork-every-image-re-encoded-by-the-node-a-webp-base-kept-by-the-core-nine-sizes-jpeg-cut-by-the-core-and-a-ceiling-that-evicts-what-can-be-remade)).
Date 2026-09-25.

**[source]** marks a published page (linked). **[code]** marks a repository path, read in a
local clone. **[measured]** marks something I observed myself. **[est]** marks an estimate.
**[unverified]** marks something I could not confirm. Anything unmarked is analysis. §1–§4 are
fact. §5 is analysis.

Local copies: `/yoloai/cache/http/*.summary.md` (WebFetch summaries, plus the full TMDB terms
and Servarr FAQ). Clones, all at their Sept 2026 HEADs: `/yoloai/cache/jellyfin`,
`/yoloai/cache/jellyfin-web`, `/yoloai/cache/xbmc`, `/yoloai/cache/sonarr`,
`/yoloai/cache/radarr`, `/yoloai/cache/tinymediamanager`, `/yoloai/cache/mediaelch`. No API key
was used. The measurements are four requests to `image.tmdb.org` and one unauthenticated request
to `api.themoviedb.org`.

---

## TL;DR

- **TMDB's terms forbid caching "any information obtained through or from TMDB" for longer
  than 6 months** (API Terms of Use §1.C, last updated 2023-10-20). D170's "fetched once, kept,
  never refreshed unasked" is in tension with that clause. Radarr refreshes a movie at most every
  **180 days**, and that is the only competitor whose policy matches it. Jellyfin's default
  ("Never"), Kodi and Infuse all keep the data indefinitely. The same terms forbid an application
  that "cloak[s] or conceal[s]" its identity. **Orpheum sends Go's default User-Agent today.**
- **TMDB image paths look immutable, and a replaced image gets a new path.** The CDN (BunnyCDN)
  serves an image with `Cache-Control: max-age=31919000` (about 370 days), an `ETag` and
  `Last-Modified`, and answers conditional requests with 304 **[measured]**. A 404 is cached for
  **12 hours** (`max-age=43200`) **[measured]**. A TMDB moderator describes two kinds of
  deletion: *unlinked* (the file still serves) and *permanently deleted* (404). So a 404 almost
  always means **our stored response is stale**. It is sometimes a CDN fault, which TMDB staff
  fixed once by purging the file.
- **Competitors almost never retry a dead image on their own.** Jellyfin moves to the next
  candidate on a 404 or 403 and gives up on any other error. Radarr and Sonarr log a warning and
  try again at the next scheduled refresh. tinyMediaManager retries a 5xx 5 times, 200 ms apart,
  and never retries a 404. Kodi records only successes, so a failing URL is tried again each time
  it is displayed. None does exponential back-off on images.
- **Refresh policy splits in two.** The *arr tools refresh on a schedule: Sonarr every 12 h,
  Radarr every 24 h, each gated by an age-and-status rule. They do it **through their own
  caching proxies** (Skyhook, `api.radarr.video`). Radarr uses its proxy's "changed since" feed
  when its last run is under **14 days** old, which is TMDB's changes-feed window. The media
  servers do not refresh by default. Jellyfin and Emby offer 30/60/90 days, with Never as the
  default. Plex refreshes periodically for music (and TV, per Plexopedia). Kodi, tinyMediaManager
  and MediaElch only refresh by hand. **Infuse removed auto-refresh because TMDB's daily change
  volume meant "thousands of updates to fetch".**
- **What goes wrong for providers is volume and anonymity, not per-title refetches.** fanart.tv
  was told by Cloudflare to pay $2,000/month for its image bandwidth. Built-in keys shared with
  piracy add-ons were revoked after a DMCA notice. MediaElch's built-in TMDB key was revoked in
  May 2026. Medusa hit TMDB 429s while updating many shows at once. TheTVDB moved to licensed or
  paid-per-user v4 keys (2020–2022).
- **Published limits:** TMDB about 40 req/s (legacy 40/10 s disabled in 2019), and its CDN
  allows 20 simultaneous connections per IP. A 304 still counts against the limit. MusicBrainz
  1 req/s and a meaningful User-Agent. OMDb 1,000/day on a free key. fanart.tv shows images 7
  days late on a project key and 2 days late on a personal key. TheTVDB recommends a copy kept
  in sync from `/updates`, or a caching proxy.
- **TMDB's changes feed** lists IDs changed in a window (24 h by default, 14 days at most), 100
  per page. Staff say every edit is listed, and the per-item example shows `images` changes. For
  a library our size, **the feed probably costs more requests per day than refreshing every title
  once every 6 months** (about 31 requests a day) **[est]**.
- **Suggestion (§5):** on a 404, refetch the title's source record **once**. That is one API
  request, and one season listing for an episode still. Retry the image only if the new record
  names a different path, and otherwise record it missing, stamped with the record's fetch time.
  Treat 5xx, timeouts and 429 as transient: capped exponential back-off with jitter, honour
  `Retry-After`, and a **per-host circuit breaker**. A 6-month rolling refresh is the smallest
  change that meets TMDB's terms. It costs about 31 API requests a day at reference scale. It
  would reverse part of D170, so it is the owner's call.

---

## 1. What the providers ask for

### TMDB

- **Rate limit.** "As of December 16, 2019, we have disabled the original API rate limiting
  (40 requests every 10 seconds.)". Upper limits remain "somewhere in the 40 requests per second
  range", "could change at any time", and clients should "respect the `429` if you receive one".
  [source](https://developer.themoviedb.org/docs/rate-limiting)
- **The CDN's own limit.** Travis Bell (TMDB staff), 2022-07-08: "One of our CDN providers
  enforces some base level rate limiting to help prevent DDOS attacks. I believe it's a maximum
  of 50 requests per second and 20 connections per IP." On 2022-08-21: "The limit is only a
  *simultaneous connection* limit on *image.tmdb.org*."
  [source](https://www.themoviedb.org/talk/62c7c1b258361b005fd2e747)
- **Terms of Use** (last updated 2023-10-20)
  [source](https://www.themoviedb.org/api-terms-of-use), §1.C "You must not":
  - "Attempt to cloak or conceal Your identity, or the identity of any … Application … that
    accesses TMDB or the TMDB APIs".
  - "Use, or create any Application that, in Our sole discretion, (i) uses an excessive amount of
    bandwidth, (ii) degrade or impairs … access to, or use of, TMDB … or (iii) otherwise adversely
    impacts the stability of TMDB".
  - **"Cache, for longer than 6 months, any information obtained through or from TMDB or the
    TMDB APIs."**
  - "Use TMDB as an image hosting service".
  - §1.D: "TMDB may terminate the license at any time for any reason."
  - §3 requires the TMDB logo and the notice "This [product] uses TMDB and the TMDB APIs but is
    not endorsed, certified, or otherwise approved by TMDB."
  - The terms say nothing about refresh frequency.
- **Conditional requests on the API.** Travis Bell, 2018-02-23: ETags were "restored", and
  `If-None-Match` returns `304 Not Modified`. But "I don't believe our rate limiting is
  sophisticated enough to not make that request count … A 200, 404 or HEAD, are all the same to
  it." Endpoints on the "Lua stack" (people, discover, search, lists, v4) had no ETags then.
  [source](https://www.themoviedb.org/talk/5a8e4a269251416c8400b303). "It will count towards the
  rate limit. The benefit is the saved transfer". Do not send `If-Modified-Since` together with
  `If-None-Match`. [source](https://www.themoviedb.org/talk/528b04d9760ee3343c018049). Whether
  the API still sends ETags in 2026 I could not check without a key **[unverified]**.
  Unauthenticated, `api.themoviedb.org` answers through CloudFront with
  `Cache-Control: public, max-age=300` **[measured]**.
- **Image CDN behaviour [measured, 2026-09-25]:**
  - `GET https://image.tmdb.org/t/p/w342/<path>.jpg` → `200`, `server: BunnyCDN-DE1-…`,
    `cache-control: public, max-age=31919000` (about 370 days), `etag: "67f0b8df-9401"`,
    `last-modified: Sat, 05 Apr 2025`.
  - The same URL with `If-None-Match`, or with `If-Modified-Since` → `304`.
  - A path that does not exist → `404`, `cache-control: max-age=43200` (12 h). The edge caches
    the 404 itself.
  - Image paths are random 27-character names. A year-long `max-age` on a public cache is only
    safe if the bytes behind a path never change, so **a replaced poster gets a new path**
    **[inference from the headers; TMDB does not document it]**.
- **What a 404 means.** TMDB moderator ticao2, 2021-02-19, on a poster path that 404ed: there
  are two kinds of deleted image, "those that were deleted, unlinked, from a movie", which
  continue to exist on the servers, and "those that have been permanently deleted from TMDb
  Servers", which 404. The example was "deleted on 2020-04-09 … probably in a maintenance
  operation that took place in early 2020".
  [source](https://www.themoviedb.org/talk/602f5684f90b190040704be2)
  - So an unlinked image keeps serving at its old path, and our copy stays valid but is no
    longer the title's chosen image. A purged one 404s.
- **A 404 that was a CDN fault.** 2022-10-05: a poster served on `www.themoviedb.org` but 404ed
  on `image.tmdb.org`. The moderator re-uploaded it under new paths. Travis Bell: "I just
  manually purged that file from our CDN which I believe should have fixed it."
  [source](https://www.themoviedb.org/talk/633c90b720e6a5007c38c7f7)
- **Changes feed.** `GET /movie/changes` (also `/tv/changes`, `/person/changes`) is "a list of
  all of the movie ids that have been changed in the past 24 hours". "You can query this method
  up to 14 days at a time". "100 items are returned per page"
  [source](https://developer.themoviedb.org/reference/changes-movie-list),
  [source](https://developer.themoviedb.org/docs/tracking-content-changes).
  - The per-item `GET /movie/{id}/changes` returns field-level changes. Its documented example
    shows an `images` key with poster additions and file paths.
    [source](https://developer.themoviedb.org/reference/movie-changes)
  - Staff: "All entries that get changed will be listed there". "You could poll the
    /movie/changes method on some schedule (daily, weekly, etc...) and look for the IDs you're
    interested in" (2019). [source](https://www.themoviedb.org/talk/5ca4f43dc3a3685ab00ce231).
    Also "All of the edits are tracked".
    [source](https://www.themoviedb.org/talk/5c88f2fdc3a36864650b3426)
  - The global feed cannot be filtered by field or by ID (2019, same thread).
  - How many IDs it lists a day is not published **[unverified]**. Firecore says "thousands"
    (§2, Infuse).
- **Daily ID exports** run at about 07:00–08:00 UTC and are kept for 3 months: valid IDs plus
  adult, video and popularity. [source](https://developer.themoviedb.org/docs/daily-id-exports)
- **Getting a replacement image.** `GET /movie/{id}/images` (and the TV and episode equivalents)
  lists every image of an item. `append_to_response=images` adds the same list to a details
  call, at the cost of one request either way
  ([source](https://developer.themoviedb.org/docs/append-to-response); the per-call cost is my
  reading **[unverified]**). Orpheum appends only
  `credits,external_ids` today **[code]** (`internal/core/identify/tmdb/works.go:117`). The stored
  response therefore holds one path per image kind and no fallbacks.

### TheTVDB (v4)

- **Licensing.** "Starting in November 2020 … v4 API keys are now per-project, and not for use
  by individual users. … All API keys are required to have either a commercial license, or have
  subscriptions enabled for end users." [source](https://support.thetvdb.com/kb/faq.php?id=62).
  End users of a "user-supported" project buy a subscription and enter a PIN
  ([source](https://support.thetvdb.com/kb/faq.php?id=82)).
  - Pricing is by the licensee's revenue. Under $50k/yr is free "with attribution".
    [source](https://thetvdb.com/api-information)
  - Kodi, Jellyfin and Plex users had to move to v4 scrapers before the legacy APIs were turned
    off (end of 2022) [source, via search summary of kodi.tv/article/thetvdb; the page returned
    403 to me].
- **Caching guidance.** The official repo lists three access strategies:
  - keep a full copy and watch "the /updates endpoint and update your database with any
    changes";
  - run a caching proxy and "set time to live for each endpoint based on the needs of your
    users";
  - direct access by end users, which needs the subscription model or a contract.

  Static lookups (genres, artwork types, languages …) are "heavily cacheable".
  [source](https://github.com/thetvdb/v4-api)
- **Rate limit.** None published that I could find **[unverified]**. Sonarr's FAQ says TVDB's
  API has a 1–3 h cache plus up to 1 h of CDN cache (§2).

### fanart.tv

- **Keys.** A project key is mandatory. A personal key is optional. Project key: images approved
  **7 days** ago. Personal key: **48 hours**. Personal key of a VIP member: everything approved.
  Per the official client, "Retries up to 3 times when rate limited" and "Waits the duration
  specified by the server". [source](https://github.com/fanart-tv/fanart.tv-api), and the wiki
  and Medium posts on personal keys, which returned 403 to me (text from search summaries).
- **Cost pressure.** Around September 2020, Cloudflare told fanart.tv it was "on the wrong
  package for their bandwidth usage" and should move to an enterprise plan at "$2000/mo". That
  was more than its donations, so it built its own Varnish cache for image assets. Keys used by
  piracy add-ons were deactivated after a DMCA notice, which broke Kodi's Artwork Beef for a time
  [source, via search summaries of fanart.tv/recent-api-issues/ and
  github.com/rmrector/script.artwork.beef/issues/31; the fanart.tv page returned 403 to me].
- **A November 2025 incident.** The API briefly returned bare filenames instead of URLs. On
  2025-11-17, "301 redirect loops" on some movie IDs caused "endless API requests" from Emby.
  Emby keeps fanart.tv responses in `cache/fanart-movies` and `cache/fanart-tv`.
  [source](https://emby.media/community/topic/143934-fanarttv-api-changes/). That is the
  clients' side of the problem: a loop with no cap floods the provider.

### OMDb

- Free key: **1,000 requests a day**. More comes through Patreon.
  [source](https://omdbapi.com/apikey.aspx)

### MusicBrainz (the reference point for community databases)

- "Currently that rate is (on average) 1 request per second" per IP. Over it, you get `503`.
  A client must identify itself as "Application name/<version> ( contact-url )" or with a
  contact email. Blank and generic agents (Java, Python-urllib …) share a throttled pool of
  about 50 req/s. Global cap about 300 req/s.
  [source](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting)
- Jellyfin's MusicBrainz client adds `(team@jellyfin.org)` to its product token and applies a
  configurable delay between requests **[code]**: `Jellyfin.Server/Startup.cs:110-113`,
  `Emby.Server.Implementations/ApplicationHost.cs:259`,
  `MediaBrowser.Providers/Plugins/MusicBrainz/Plugin.cs:46`.

### User-Agent, generally

- Wikimedia's policy is the most explicit: `CoolBot/0.0 (https://example.org/coolbot/;
  coolbot@example.org) generic-library/0.0`. Clients without a descriptive agent "may be blocked
  without notice", and copying a browser's agent "will be assumed malicious".
  [source](https://foundation.wikimedia.org/wiki/Policy:Wikimedia_Foundation_User-Agent_Policy)
- Sonarr and Radarr send `<AppName>/<version> (<os> <osversion>)` **[code]**
  (`radarr/src/NzbDrone.Common/Http/UserAgentBuilder.cs:36`). Jellyfin sends its product name
  and version **[code]** (`Jellyfin.Server/Startup.cs:103`).
- **Orpheum sets no User-Agent.** Its outbound client is `http.DefaultTransport` with a timeout,
  and neither TMDB call sets one, so Go sends `Go-http-client/2.0` (or `/1.1`) **[code]**:
  `internal/platform/outbound/client.go`, `internal/core/identify/tmdb/client.go:60-86`,
  `internal/core/identify/tmdb/images.go:52`.

---

## 2. How competitors refresh, and what they do with a dead image

### Jellyfin **[code, jellyfin HEAD 2026-09-22]**

- **Library scan** ("Scan Media Library", `RefreshMediaLibraryTask`) every **12 h**
  (`Emby.Server.Implementations/ScheduledTasks/Tasks/RefreshMediaLibraryTask.cs`).
- **A scan does not refetch known items from the internet.** Remote metadata providers run on
  an item's first refresh, on a full refresh, or when `AutomaticRefreshIntervalDays` has passed
  since `DateLastRefreshed` (`MediaBrowser.Providers/Manager/MetadataService.cs:99`).
  - The interval's choices are **Never (default), 30, 60 or 90 days**
    (`jellyfin-web/src/components/libraryoptionseditor/libraryoptionseditor.js:54-61`). The
    field carries the warning "Enabling this option may result in significantly longer library
    scans."
  - Remote *image* providers run only when the refresh mode is Full, or the item has never been
    refreshed (`MetadataService.GetNonLocalImageProviders`, line 717 on).
- **Missing images.** On a manual refresh, an image type the item lacks is fetched even in the
  default mode (`ItemImageProvider.cs:209`, `!item.HasImage(imageType)`).
- **Dead image URL.** The TMDB image provider fetches the item's *list* of images live, cached
  1 h in memory (`TmdbClientManager.cs:26`). The downloader walks that list
  (`ItemImageProvider.DownloadImage`, lines 551–620):
  - "Sometimes providers send back bad urls. Just move to the next image". On 404 or 403 it
    tries the next candidate.
  - Any other non-success status: "skipping all remaining requests" for that image type. This is
    a crude circuit breaker.
  - `HttpRequestException` → stop.
  - **No retry, no back-off.**
  - A failed image does not stamp `DateLastRefreshed` (`Failures > 0`). An item whose *first*
    refresh failed is therefore treated as never refreshed, and the next 12-hourly scan tries
    again. That is an implicit 12 h retry, for first fetches only (lines 205–231).
- **Rate limiting.** No 429 or `Retry-After` handling in Jellyfin's own code. TMDB goes through
  TMDbLib 3.0.0 (`Directory.Packages.props:84`), whose retry behaviour I did not check
  **[unverified]**.
- **Proxy.** None. Every server calls TMDB directly, with a shared project API key
  (`TmdbClientManager.cs:52`).

### Emby

- The library option to refresh internet metadata every **30, 60 or 90 days** exists (Luke,
  Emby team, 2018-04-10). In 2021 he agreed "we probably need a dedicated short term refresh
  option for new episodes". Missing images from bulk imports were handled as problem reports,
  not with automatic retry.
  [source](https://emby.media/community/topic/57907-refresh-episode-metadata-after-added-to-library/)
- The automatic refresh fills empty fields only. Placeholder values like "TBA" count as filled
  (ebr). [source](https://emby.media/community/topic/104908-automatically-refresh-metadata-doesn%E2%80%99t-work/)
- Its default, and whether it retries images, are **[unverified]**: closed source. Jellyfin
  forked from it, and the option, "Never" default included, is probably inherited **[est]**.

### Plex

- **Metadata comes through Plex's cloud.** The 2020 agents use "a cloud-based metadata service
  that does all the heavy lifting to bring together multiple metadata sources such as IMDb,
  TheMovieDb, Rotten Tomatoes". [source](https://forums.plex.tv/t/new-plex-media-server-movie-scanner-and-agent-preview/593269)
  So a Plex server does not call TMDB itself, and Plex, not the user, carries the provider
  relationship. Whether poster URLs are also served from Plex's CDN I could not confirm
  **[unverified]**.
- **Scheduled refresh.** "Refresh metadata periodically: Throughout the month, the server will
  update the information about music artists and TV shows in your library". "Refresh local
  metadata every three days" touches local files only. "Update all libraries during
  maintenance" finds new content, "Any metadata for existing content will not be refreshed", and
  is off by default. [Plexopedia](https://www.plexopedia.com/plex-media-server/general/scheduled-tasks/).
  A Plex moderator once said the periodic refresh "is only for Music libraries"
  ([forum](https://forums.plex.tv/t/refresh-metadata-periodically-how-often/222084)). Plex's own
  support pages returned 403 to me, so **film refresh is unverified**. Users asking for
  automatic movie refresh got no staff answer
  ([forum](https://forums.plex.tv/t/automatically-refresh-metadata/616159)).
- **Dead images.** Unknown **[unverified]**. Behind a proxy, the question moves to Plex's servers.

### Kodi **[code, xbmc HEAD 2026-09-24; kodi.wiki raw page]**

- **No automatic refresh.** "Update Library" scans for **new** items, optionally on startup or
  through an add-on on a timer. "Refresh" modifies **existing** entries, by hand, per item or per
  source, "when … the information at the site of the Information Provider has changed"
  (kodi.wiki *Updating or removing videos*, raw source in the cache).
- **Images are fetched lazily, when first displayed**, into the texture cache.
  `ShouldCheckForChanges` returns false for `http(s)` URLs: remote images are never re-validated
  (`xbmc/TextureCacheJob.cpp:88-97`).
- **Failures are not recorded.** `OnCachingComplete` adds a texture only on success
  (`xbmc/TextureCache.cpp:349-365`), so a failing URL is fetched again whenever something
  displays it. That is unbounded retries, paced by the user's scrolling.
- Scrapers are Python add-ons. TMDB is called directly with Kodi's project key.

### Sonarr and Radarr **[code, HEADs Sept 2026; Servarr wiki]**

- **They refresh on a schedule, and cannot be told not to.**
  - Sonarr runs `RefreshSeriesCommand` every **12 h**
    (`sonarr/src/NzbDrone.Core/Jobs/TaskManager.cs:102`). "No you cannot disable the task"
    ([Servarr wiki, Sonarr FAQ](https://wiki.servarr.com/sonarr/faq)).
  - Radarr runs `RefreshMovieCommand` every **24 h**
    (`radarr/src/NzbDrone.Core/Jobs/TaskManager.cs:88`).
- **But each run refreshes few titles.**
  - Sonarr (`Tv/ShouldRefreshSeries.cs`) refreshes a series if:
    - it was last synced **over 30 days** ago; or
    - an aired episode is still titled "TBA"; or
    - it is not ended and was not synced in the last 6 h; or
    - its last episode aired within 30 days.
  - Radarr (`Movies/ShouldRefreshMovie.cs`) refreshes a movie if:
    - it was last synced **over 180 days** ago; or
    - it is announced or in cinemas; or
    - it had a physical release within 30 days;

    and never if it was synced in the last 12 h.
- **Radarr uses a changes feed first.** If the previous run was **less than 14 days ago**, it
  asks its proxy for `movie/changed?since=…`, rounded down to the hour "to ensure we cover gap
  and don't kill cache every call". It then refreshes only those IDs. Otherwise it falls back to
  the rules above. (`Movies/RefreshMovieService.cs:262-270`,
  `MetadataSource/SkyHook/SkyHookProxy.cs:55-71`.) The 14 days is TMDB's own changes-window
  limit.
- **Through their own proxies.**
  - Sonarr calls `https://skyhook.sonarr.tv/v1/tvdb/…`
    (`NzbDrone.Common/Cloud/SonarrCloudRequestBuilder.cs:18`).
  - Radarr calls `https://api.radarr.video/v1/…` (`RadarrCloudRequestBuilder.cs:23`).
  - Servarr's FAQ: "TVDb has a 1-3 hour cache on their API", then "up to an hour" of CDN,
    "Sonarr's Skyhook has a much smaller few hour cache on top of that", then the 12 h task. So a
    change takes "between 3 and 19 hours" to arrive
    ([Servarr wiki source](https://github.com/Servarr/Wiki/blob/master/sonarr/faq.md)).
  - Why a proxy: TheTVDB licenses per project and recommends a caching proxy (§1). One proxy
    serves every install from a shared cache, so a popular show is fetched from TVDB once, not
    once per user. The project also holds one licence and can fix mappings (network time zones,
    scene numbering) centrally. That rationale is an inference; I found no first-hand statement
    **[unverified]**.
  - Radarr's older proxy repo (RadarrAPI.TMDB, Laravel, archived 2021) says only "Backend Stuff
    for Radarr regarding TMDB (Discover, lists, Mappings, etc.)".
    [source](https://github.com/Radarr/RadarrAPI.TMDB)
- **Images.** Image URLs come from the proxy's response (`RemoteUrl = arg.Url`). On each refresh
  of a title, `MediaCoverService` sends a **HEAD** to each cover and re-downloads when the
  `Content-Length` differs from the local file (`MediaCover/CoverAlreadyExistsSpecification.cs`).
  A failure is a logged warning: "Couldn't download media cover" (`MediaCoverService.cs:168-178`).
  The retry is the next scheduled refresh of that title. There is no dedicated back-off.
- **429.** Parsed into `TooManyRequestsException` with `RetryAfter` from seconds or a date
  (`NzbDrone.Common/Http/TooManyRequestsException.cs`).

### tinyMediaManager **[code, HEAD 2026-09-24]**

- A desktop tool. Scraping happens on the user's action, and "scrape new items" covers items the
  last data-source update found ([docs](https://www.tinymediamanager.org/docs/scrapers)). I found
  no scheduled refresh.
- Calls `api.themoviedb.org` directly (`scraper/tmdb/TmdbController.java:61`), with an on-disk
  25 MiB OkHttp cache that honours the server's cache headers (`TmmHttpClient.java:53`).
- **429:** `TmmHttp429RetryInterceptor`. At most 3 retries. It honours `Retry-After`, defaults to
  10 s, and **refuses to wait longer than 30 s**: past that it fails rather than stall.
- **Images:** `Url.getInputStreamWithRetry(5)`. On 403 or 404: "do not try more often". Any
  other status below 500 is a permanent failure. 5xx and I/O errors retry 5 times, **200 ms**
  apart, with no back-off (`scraper/http/Url.java:399-445`, `core/ImageUtils.java:331`).

### MediaElch **[code, HEAD 2026-08-28]**

- Manual scraper for Kodi libraries. No scheduled refresh and no retry logic found (only
  `src/network/HttpStatusCodes.h`).
- **Its built-in TMDB key "returns HTTP 400 … appears to have been revoked"**, with "no UI option
  to enter a personal TMDb API key" (2026-05-15).
  [source](https://github.com/Komet/MediaElch/issues/1992). The reason for the revocation is
  **[unverified]**.

### Infuse

- Firecore staff (james), 2025-06-28: **"Auto-refreshing of metadata no longer happens. This was
  changed awhile back as the number of daily updates on TMDB had grown quite large, so there
  often could be thousands of updates to fetch."** Manual refresh is in Settings → Library.
  [source](https://community.firecore.com/t/meta-data-refreshing/56175/2)
- Whether Infuse proxies TMDB through its own servers **[unverified]**.

### Comparison

| Product | Default refresh of known titles | Automatic? | What triggers a refetch | Dead or missing image | Retry of a failed image | Shared proxy |
| --- | --- | --- | --- | --- | --- | --- |
| **Jellyfin** | Never (30/60/90 d optional) | No, by default | New item; manual refresh; the optional interval | 404/403 → next candidate from a live image list; other errors → stop | None; an item whose first fetch failed is retried at the next 12 h scan | No |
| **Emby** | Never? (30/60/90 d optional) **[est]** | No, by default **[est]** | Same shape; fills empty fields only | **[unverified]** | Manual | No |
| **Plex** | Periodic for music (TV per Plexopedia); films **[unverified]** | Partly | Agent-driven | **[unverified]** | **[unverified]** | **Yes** (Plex metadata cloud) |
| **Kodi** | Never | No | Manual "Refresh" | Not recorded | Re-fetched every time it is displayed | No |
| **Sonarr** | Task every 12 h; a series at most every 30 d unless airing | **Yes** | Age > 30 d, TBA titles, continuing series, recent airing | Warning, file kept | Next refresh of that series | **Yes** (Skyhook) |
| **Radarr** | Task every 24 h; a movie at most every 180 d unless new | **Yes** | Proxy's changed-IDs feed (< 14 d); else age > 180 d or recent release | Warning; HEAD + size compare | Next refresh of that movie | **Yes** (api.radarr.video) |
| **tinyMediaManager** | Never | No | User action | 404/403 → give up | 5 × 200 ms on 5xx/IO | No |
| **MediaElch** | Never | No | User action | — | None found | No |
| **Infuse** | Never (removed) | No (was yes) | Manual | **[unverified]** | **[unverified]** | **[unverified]** |

---

## 3. Evidence of problems

- **fanart.tv, about 2020:** asked by Cloudflare to pay $2,000/month for its image bandwidth.
  Built its own Varnish cache. Deactivated keys that piracy add-ons shared after a DMCA notice,
  which broke Kodi's Artwork Beef (§1). The cause was aggregate image traffic from many clients.
- **fanart.tv, November 2025:** a redirect loop on the provider's side turned Emby clients into
  a flood of "endless API requests" (§1). A client with no cap on retries amplifies the
  provider's bug.
- **TheTVDB, 2020–2022:** moved from free legacy keys to per-project licences, or a paid user
  subscription with a PIN, and shut the legacy APIs (§1). Its stated best practices are "keep
  your own copy and follow /updates" or "caching proxy". The reason given is commercial;
  TheTVDB published no load figures that I found.
- **TMDB:**
  - Medusa's users got `429 Client Error` while "updating or scanning shows", 2018. The fix
    proposed was to read TMDB's rate-limit headers and cool down.
    [source](https://github.com/pymedusa/Medusa/issues/5529)
  - TMDB keeps a CDN-level cap for "DDOS" protection (§1).
  - MediaElch's embedded key was revoked in 2026 (§2).
- **Infuse:** chose not to follow TMDB's change volume at all (§2).
- **The pattern.** What hurts providers is many installs doing the same thing: shared keys, image
  bandwidth, loops without caps, whole-library bursts. One install refetching one title after a
  404 is not in that class. What gets a *project* into trouble is being the shared key or the
  anonymous agent behind that traffic.

---

## 4. Patterns for being polite

- **Identify yourself.** `Orpheum/<version> (+<project URL>)`, per MusicBrainz and Wikimedia
  (§1). TMDB's terms forbid concealing the application's identity.
- **Honour `429` and `Retry-After`.** `Retry-After` may be seconds or an HTTP date (RFC 9110
  §10.2.3; 429 is RFC 6585 §4).
  - Orpheum's TMDB client parses the seconds form only **[code]**
    (`internal/core/identify/tmdb/client.go:109-114`). A date falls through to a generic error.
  - Cap the wait you will honour and surface anything longer, as tinyMediaManager does (30 s).
  - fanart.tv's client retries 3 times.
- **Capped exponential back-off with jitter** for 5xx, timeouts and connection errors. Full
  jitter (a random wait between 0 and the current cap) spreads many clients out after an outage.
  AWS's architecture note ["Exponential Backoff And Jitter"](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
  is the usual reference. Orpheum's job queue already retries "with doubling back-off up to the
  retry limit, then fails" **[code]** (`internal/core/jobs/doc.go:22`). It does not say whether
  there is jitter or a cap.
- **A circuit breaker per host.** When several different requests to one host fail in a row,
  stop sending to that host for a while and probe with one request. Without this, an outage
  costs *images × attempts* requests. With it, it costs a few probes. Jellyfin's "skipping all
  remaining requests" is a per-item version of the same idea (§2).
- **Concurrency cap.** TMDB's image CDN allows 20 simultaneous connections per IP (§1). A few,
  not twenty, leaves room for the user's other devices behind the same IP.
- **Spread scheduled work.** Radarr's 24 h task refreshes only titles past their age limit, so
  the work spreads over time. A rolling refresh ("the N oldest records each day") makes the load
  flat. A fixed-time refresh of everything is the burst D170 feared.
- **Conditional requests.**
  - On the *image CDN* they work (304 with no body) **[measured]**. They are pointless for TMDB
    images, though: a path's bytes do not change (§1), so a path we have is a path we keep.
  - On the *API*, `If-None-Match` saves bytes but still counts against the limit (staff, §1).
- **Changes feeds.** TMDB `/movie/changes` and `/tv/changes` (24 h default, 14 days at most, 100
  IDs a page). TheTVDB `/updates`. fanart.tv "latest" endpoints.
  - Radarr shows the use: poll since the last run, refetch the intersection with your library,
    fall back to age rules if you missed more than 14 days (§2).
  - The feed is global, so its cost scales with **TMDB's** daily edits, not with your library.
    That is why Infuse gave up on it (§2).
- **A shared cache or proxy.** The strongest pattern (Plex, Skyhook, api.radarr.video,
  fanart.tv's Varnish): one fetch serves every install. It needs hosted infrastructure and a
  provider relationship. It is out of scope for a self-hosted Cut A **[analysis]**.
- **Don't treat 404 as transient.** Retry it at most once, after the CDN's 12 h negative cache
  (§1), in case it was a CDN fault.

---

## 5. What this suggests for Orpheum (analysis)

**Reference scale:** 3,500 films; 200 series with about 20,000 episodes. At about 11 episodes a
season, that is **about 1,800 season listings [est]**. Orpheum's series fetching (a series record plus one
listing per season) makes the source records:

| Records | TMDB API requests |
| --- | --- |
| Films (details, one request each) | 3,500 |
| Series details | 200 |
| Season listings | ~1,800 |
| **Total** | **≈ 5,500** |

A full refetch is ≈ 2.3 min at TMDB's ~40 req/s, or ≈ 23 min at a polite 4 req/s.

Images, at about 3 per film, 3 per series, 1 per season and 1 still per episode:
**≈ 10,500 + 600 + 1,800 + 20,000 ≈ 33,000 image requests [est]**, and about 10 GiB of
originals (artwork-cache.md).

Today, a 404 is `ErrNoImage` and is "recorded with no hash, so it is not fetched again" **[code]**
(`internal/core/identify/tmdb/images.go`; an earlier commit). Every other non-200 is a generic
error and goes to the job queue's retry.

### (a) A 404 from the image server

| Option | What it does | Cost at reference scale |
| --- | --- | --- |
| **A0. Status quo** | Missing forever | 0 requests. A blank that no user action short of a manual refresh repairs |
| **A1. Refetch the record once** (suggested) | Refetch the title's source record: film details, or the season listing for an episode still, or series details for a series image. If it names a **different** path for that image kind, fetch that. If it names the same path, or none, record the image as missing, stamped with the record's fetch time, and don't try again until the record changes | 1 API + ≤ 1 image request per dead image. If 1% of the 33,000 images were dead (a high guess **[est]**): ≤ 330 API + 330 image requests, once. Nothing repeats, because the refetch clears the stale record |
| **A2. Ask for the image list** | `GET /movie/{id}/images` (1 request) and choose a replacement ourselves | Same request count as A1, but leaves the rest of the record stale, and makes us pick the "primary" image instead of TMDB. No better than A1 |
| **A3. Store the candidate list** | Add `images` to `append_to_response`, then fall back to the next candidate locally, as Jellyfin does | 0 extra requests at 404 time, but every record gets larger (popular films list dozens of images). And a fallback taken from a stale list may itself be purged. Worth doing only if picking alternatives is wanted anyway |
| **A4. One delayed recheck** (add-on to A1) | If the refetched record still names the same path, try the image once more after ≥ 12 h (past the CDN's negative cache), to catch a CDN fault like the 2022 case | +1 image request per dead image, once |

- **Why A1 is D170-compatible in spirit:**
  - D170's refetch ban targets *TTL-driven background refresh* and *many sources per title*.
    A1 is triggered by evidence that this one record is wrong. It is one request to the
    *selected* source. It never touches a manual field.
  - D170 §2 already makes a first fetch for a new episode automatic. The code refetches a season
    listing when an episode arrives that the listing lacks **[code]**
    (`internal/core/identify/series.go`, `seasonsToFetch`). That is the same shape:
    "stored input shown stale by an observed fact → refetch it once".
  - It still reverses D170's letter ("a refetch happens only when the user asks"), so it needs
    the owner's decision.
- **Guard:** at most one A1 refetch per record per record-version. The trigger is "this image
  404ed and the record is older than the 404". That makes a loop impossible even if TMDB keeps
  naming a dead path.

### (b) A transient image-server failure (5xx, timeout, connection error, 429)

Suggested:

- **Per-image retry:** capped exponential back-off with full jitter, for example 1 min, 5 min,
  30 min, 2 h, 12 h, then daily for 7 days, then park the image as "failed, will retry when the
  item is next refreshed or opened".
- **Per-host breaker:** after 5 consecutive failures across *different* images on
  `image.tmdb.org`, pause the whole host with the same back-off, send one probe per step, and
  resume on the first success.
- **Honour `Retry-After`** in both forms, capped at an hour. Past the cap, park and surface it.
- **Concurrency:** ≤ 4 connections to the image CDN.

Cost:

- A single failing image costs ≤ 12 requests over 7 days.
- A full CDN outage during a first scan costs **a handful of probe requests per back-off step**,
  about 12 over 7 days, instead of 33,000 × 12. After recovery, the queue drains at the
  concurrency cap.

This keeps D170's "rate limiting delays metadata; it never delays the library", and costs nothing
when there is no failure.

### (c) Is any periodic or triggered refresh defensible?

**Against the providers' guidance:**

- TMDB asks for nothing more often than necessary. It forbids caching longer than **6 months**,
  and it forbids excessive bandwidth.
- TheTVDB and TMDB staff both *recommend* staying in sync ("a good idea to stay in sync with
  our changes").
- None of the providers objects to a slow per-title refresh. Their problems (§3) came from
  aggregate bursts and shared keys.

**Against D170's flooding concern:** the concern is scan-time bursts and N sources per title.
Spread refreshes are a different load shape.

| Option | What it does | API requests at reference scale | Notes |
| --- | --- | --- | --- |
| **C0. None** (D170 today) | Refetch only on request | 0 | Records older than 6 months break TMDB §1.C's letter. Thin early metadata stays thin |
| **C1. Rolling 6-month refresh** | Each day, refetch the records whose fetch is oldest, so that none passes ~180 days. Refetch images only where a path changed | 5,500 / 180 ≈ **31 API requests/day**, about 1 every 47 min. Images: only changed paths, probably a few a day **[est]** | Meets the ToS. Same horizon as Radarr's 180 days. Even OMDb's 1,000/day could carry it (3,500 / 180 ≈ 20/day) if scores were ever refreshed. Flat load, no burst |
| **C2. C1 + a recent-titles tier** | Also refetch titles first released or aired in the last ~60 days, weekly (Sonarr and Radarr's "recent" rules) | + tens a week **[est]** | Answers D170 §5's "thin metadata stays thin" without a user click |
| **C3. TMDB changes feed, daily** | Page `/movie/changes` + `/tv/changes` for the last 24 h, refetch the IDs we hold | Feed pages ≈ (TMDB's daily changed IDs) / 100. Probably **tens to hundreds of pages a day [est; volume unpublished; Infuse says "thousands" of updates]**, plus the matching refetches | Costs more than C1 at our scale, and grows with TMDB, not with us. Worth it only if freshness within a day matters. Must fall back to C1 after a > 14-day gap, as Radarr does |
| **C4. Triggered only** | A1 above, plus "the user opened a title whose record is > 6 months old" | Bounded by what users look at | Meets the ToS only for titles someone looks at. Adds a fetch on the request path unless queued |

- **Cheapest defensible design:** C1 for records, A1 for 404s, (b) for transient failures. The
  steady-state traffic is about 31 API requests a day plus images whose path changed. The
  first scan is D170's queue as today.
- **What C1 changes in D170:**
  - It reverses "no TTL-driven background refresh" (D170 §2, §4 "The TTL goes").
  - It keeps the rest: one selected source, manual fields never touched, the queue honouring 429.
  - It brings back D170's worry that a provider edit silently changes the library, for fields
    the user has not made manual. That trade is the owner's to make.
- **Whatever the refresh decision, two small things follow from §1 alone:**
  - send a descriptive **User-Agent**;
  - confirm the **TMDB attribution** (logo plus the "not endorsed" notice). A grep of `web/`
    and `docs/` found no such notice **[unverified; grep only]**.

### Could not verify

- TMDB's daily changed-ID volume. Whether the API still returns ETags in 2026 (needs a key).
  Whether a replaced TMDB image is ever overwritten in place (inferred "no" from the cache
  headers).
- Plex's film refresh policy and image hosting. Emby's defaults. Infuse's backend. Why
  MediaElch's key was revoked.
- TheTVDB v4 rate limits. The primary texts of fanart.tv's 2020 post and the Kodi TheTVDB
  article (both 403 to me; from search summaries).
- TMDbLib's retry behaviour inside Jellyfin.
