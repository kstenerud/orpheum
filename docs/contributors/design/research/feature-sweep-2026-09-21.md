> **ABOUTME:** A source-code sweep of every competitor and companion product we could clone,
> read for **features**, cross-referenced against this tree — with a demand number and an
> abandonment record for each, so the question "should we care?" has evidence behind it.

# The feature sweep, read from source

Gathered **2026-09-21**. Backs [D149](../../decisions/working-notes.md) and
[D150](../../decisions/working-notes.md). Prompted by the project owner: *"look at the source
code of every competitor and associated product we can find to get a list of functions and
features we don't yet have in our design … a measure of how popular those features are, and if
anyone abandoned them."*

[D140](../../decisions/working-notes.md) did a gap sweep in September 2026 from the **complaint
record**. This one is different in method and it found different things: it reads the
**shipped surface** — controllers, configuration schemas, RPC methods, enums, rule vocabularies
— because a product's settings page is a complete statement of what it does, and a complaint
record is not. It then adds two axes the earlier sweep did not have: **what people install a
second program to get**, and **what somebody built and later withdrew**.

---

## 1. Method, and what it cannot tell you

**Source.** Depth-1 clones read locally. Nothing in this section is from a marketing page.

| Product | What was read |
| --- | --- |
| **Jellyfin** | 60 API controllers, `ServerConfiguration`, `LibraryOptions`, `UserConfiguration`, `UserPolicy`, `NotificationType` |
| **Emby** | `MediaBrowser.Api/*Service.cs`, the top-level assemblies |
| **Kodi** | the JSON-RPC schema (`methods.json`) and `system/settings/settings.xml` |
| **Plex** (closed source) | `python-plexapi` — the reverse-engineered object model, which is the best available statement of Plex's surface |
| **Tautulli** | notification triggers and delivery agents, home-page statistic cards |
| **Kometa** | `modules/builder.py` and the `defaults/` tree |
| **Maintainerr** | the rule-property constants |
| **ErsatzTV, Threadfin, Jellyseerr, Ombi, Jellystat, Wizarr, Shoko, Komga, UMS, Stremio, Sonarr/Radarr** | READMEs and feature surfaces |
| **Jellyfin plugin catalogue** | `repo.jellyfin.org/files/plugin/manifest.json` — 36 official plugins |

**Three limits worth stating up front.**

- **Emby's public repository is frozen.** Its last commit is **2018-09-19 (v3.5.3)**; Emby
  closed its source that month and the product has moved on for eight years without it. Every
  claim here about Emby is a claim about the 2018 fork point — which is still useful, because
  that is precisely the moment Jellyfin forked from, so the diff is *what Jellyfin chose not to
  carry*.
- **The demand corpus is `is:open` issues**, so a feature that works well is invisible. A zero
  here means "no unmet demand recorded", never "nobody wants it". Two-factor auth scores zero
  and that is not evidence nobody wants it.
- **Stars measure installs of a separate program**, which is a much stronger signal than a
  vote on an issue — it is somebody running a second container forever — but it is biased
  toward things a companion app *can* do from outside.

---

## 2. What the products actually contain

### 2.1 Jellyfin's 60 controllers

```
ActivityLog ApiKey Artists Audio Backup Branding Channels ClientLog Collection Configuration
Dashboard Devices DisplayPreferences DynamicHls Environment Filter Genres HlsSegment Image
InstantMix ItemLookup ItemRefresh ItemUpdate Items Library LibraryStructure LiveTv Localization
Lyrics MediaInfo MediaSegments Movies MusicGenres Package Persons Playlists Playstate Plugins
QuickConnect RemoteImage ScheduledTasks Search Session Startup Studios Subtitle Suggestions
SyncPlay System TimeSync Trailers Trickplay TvShows UniversalAudio User UserLibrary UserViews
VideoAttachments Videos Years
```

`LiveTv` alone carries **38 routes** — tuners, tuner discovery, listing providers (including a
Schedules Direct integration), channel mappings, timers, series timers, recordings, recording
folders and groups, guide info. It is not a feature; it is a second product inside the first.

`Playlists` is more interesting than it looks: `{playlistId}/Users/{userId}` with GET, POST and
DELETE is a **per-user ACL on a playlist** — collaborative playlists with owners.

### 2.2 The Jellyfin settings surface, which is the honest feature list

Fields in `ServerConfiguration`, `LibraryOptions` and `UserConfiguration` that name a capability
we have no equivalent for:

| Field | What it is |
| --- | --- |
| `PathSubstitutions` | rewrite a server path to a client-reachable one, so the client opens the file directly over SMB/NFS instead of streaming |
| `RemoteClientBitrateLimit` | a server-wide bandwidth cap for remote clients |
| `EnableFolderView` | browse the raw directory tree |
| `DummyChapterDuration`, `ChapterImageResolution` | synthesised chapters and chapter thumbnails |
| `SaveLyricsWithMedia`, `LyricFetcherOrder`, `DisabledLyricFetchers` | lyrics as a fetched, ordered-provider subsystem |
| `DisplayMissingEpisodes` | show episodes the library does not have |
| `EnableLocalPassword` | a short local-network password (**removed in 10.9 — see §5**) |
| `OrderedViews`, `LatestItemsExcludes`, `MyMediaExcludes`, `HidePlayedInLatest` | per-user home-screen shaping |
| `ActivityLogRetentionDays`, `InactiveSessionThreshold` | an activity log and session reaping |
| `AutomaticallyAddToCollection`, `EnableGroupingMoviesIntoCollections` | automatic collection membership |
| `CustomTagDelimiters`, `PreferNonstandardArtistsTag` | music tag parsing |

### 2.3 Emby at the 2018 fork point, minus what Jellyfin kept

Emby shipped `GamesService.cs`, `GameGenresService.cs` and `NewsService.cs`, plus `Emby.Dlna`,
`Emby.Notifications` and `Emby.Photos` as top-level assemblies. **Jellyfin carries no Games and
no News** — grep finds nothing. Photos survived, DLNA survived until 10.9, and notifications
survived only as a *model* (`MediaBrowser.Model/Notifications/NotificationType.cs`, 14 values)
with delivery pushed out to a plugin.

### 2.4 Kodi

The JSON-RPC schema is a complete capability list. **`PVR` has 19 methods**; `Player` has 26,
including several we have no analogue for:

```
Player: SetSpeed SetTempo Zoom Rotate SetViewMode SetPartymode SetRepeat SetShuffle
        GetChapters SetAudioDelay SetSubtitle AddSubtitle GoTo
```

The settings tree names whole areas we do not touch: `services/upnp`, `services/airplay`,
`games`, `interface/screensaver`, `services/weather`, and a `pvr` section with **ten
categories** of its own (`pvrmanager`, `pvrmenu`, `epg`, `pvrplayback`, `pvrrecord`,
`pvrreminders`, `pvrpowermanagement`, `pvrparental`, `pvrclient`, `pvrtimers`).

### 2.5 Plex, via `python-plexapi`

The object model exposes things no open competitor has:

- **`CommonSenseMedia`** with `ageRatings`, **`parentalAdvisoryTopics`**, `parentsNeedToKnow`,
  **`talkingPoints`**, `anyGood`, `oneLiner` — an editorial parental-guidance layer, Plex Pass
  only. `AgeRating` carries `age`, `ageGroup`, a 5-star `rating` and a `ratingCount`;
  `ParentalAdvisoryTopic` carries `id` (violence, language, …), `label`, a per-topic `rating`
  and a **`positive`** flag, so a topic can be a *reason to watch*.
- **`ManagedHub`** — an admin-curated home row, with `homeVisibility` ∈
  `{none, all, admin, shared}`, `promotedToOwnHome`, `promotedToSharedHome`,
  `promotedToRecommended`. **The home screen is a curated, per-audience object.**
- `PlayQueue` as a separate thing from `Playlist`; `Optimized`/`Conversion` (pre-transcode for
  a device); `UltraBlurColors` (palette extracted from artwork, for UI tinting); `Autotag`,
  `Mood`, `Style`; `Review`; `Hub`; `ButlerTask` (scheduled maintenance); `GDM` (LAN discovery);
  `PlexSonosClient`; `TwoFactorRequired`; `LyricStream`; `Folder`.

### 2.6 Tautulli — the notification surface nobody's core has

**22 triggers:**

```
on_play on_stop on_pause on_resume on_buffer on_change on_watched on_created on_newdevice
on_concurrent on_intro on_commercial on_credits on_error on_intdown on_intup on_extdown
on_extup on_pmsupdate on_plexpyupdate on_plexpydbcorrupt on_tokenexpired
```

**28 delivery agents**: `boxcar browser discord email facebook gotify groupme growl ifttt join
lunasea microsoftteams mqtt ntfy osx plex plexmobileapp prowl pushbullet pushover remoteapp
scripts slack telegram twitter webhook xbmc zapier`.

Four of those triggers are *operational* rather than social — `on_buffer`, `on_concurrent`,
`on_newdevice`, `on_extdown` — and they are the ones an owner actually wants. Home-page
statistic cards: `top_movies top_tv top_music top_users top_platforms top_libraries
popular_movies popular_tv popular_music`.

### 2.7 Kometa — what people want automated

`all_builders` is assembled from **22 external services** (`anidb anilist floppy icheckmovies
imdb letterboxd mal mojo plex stevenlu tautulli tracearr textfile tmdb tvdb yamtrack serializd
mdblist simkl radarr sonarr flicklist`, plus Trakt); **129 distinct builder keys** appear in the
file and `custom_sort_builders` alone lists **102**. The shipped defaults are the demand
distilled:

- **16 award sets** — Oscars, BAFTA, Cannes, Emmy, Golden Globe, SAG, Sundance, TIFF, Venice,
  Berlinale, César, Spirit, Critics' Choice, PCA, NFR, Razzies.
- **10 chart sources** — IMDb, TMDb, Letterboxd, MyAnimeList, AniList, Simkl, Tautulli, …
- **24 overlay families** baked onto posters — `resolution`, `video_format`, `audio_codec`,
  `aspect`, `ratings`, `languages`, `language_count`, `runtimes`, `status`, `streaming`,
  `network`, `studio`, `versions`, `direct_play`, `episode_info`, `ribbon`, **`commonsense`**,
  **`mediastinger`** (is there a post-credits scene), and content-rating badges for AU, DE, NZ,
  UK and US.

### 2.8 Maintainerr — the retention vocabulary

Over **200 rule properties** across Plex, Jellyfin, Radarr, Sonarr and Overseerr. The shape of
them matters more than the count: they are overwhelmingly **cross-user aggregate watch state**
("Users that watched every episode", "Users that watched every episode since added", "Watch
time by user", "Newest view date across collection"), **external scores** (IMDb, Rotten Tomatoes
critic *and* audience, TMDb, Trakt, each with a vote count), **request provenance** ("Requested
in Seerr", "Request date", "Approval date"), and **disk space** ("Remaining disk space (GB)").

Its README states the gap in one line: *"Maintainerr is the janitor your media server doesn't
have."*

---

## 3. Demand, axis one: 932 top-reacted open issues

Harvested from the GitHub search API, `sort=reactions`, up to 100 per repository, across ten
**video** server and client repositories (`jellyfin` ×7, `Swiftfin`, `streamyfin`, `xbmc`).
`#t` is issues whose **title** matches; `Rt` is the total reactions on those.

| Feature | `#t` | `Rt` | Loudest single issue |
| --- | ---: | ---: | --- |
| Subtitles | 87 | **479** | +104 *Subtitles load after a while and can be out of sync* |
| Watch Together / SyncPlay | 10 | **127** | +65 *Add support for Jellyfin SyncPlay* |
| Direct play / path substitution | 12 | 91 | +49 *Force "direct play" in client* |
| Casting (Chromecast) | 16 | 87 | +43 *iOS – Chromecast* |
| Audio passthrough / Atmos / DTS | 13 | 73 | +22 *Add a fallback to AC3/EAC3 option* |
| HDR / Dolby Vision / tone mapping | 14 | 69 | +17 *DV regression triggers transcoding* |
| Theme songs / video backdrops | 11 | 67 | +37 *Video Backdrop and Theme song playback* |
| External player handoff | 9 | 63 | +28 *Play via external player from the item screen* |
| Live TV / DVR / tuner / EPG | 29 | 61 | +10 *Default to channel logos in Live TV* |
| Widgets / deep links | 10 | 61 | +26 *tvOS – Support Deep Links* |
| **User PIN / profile lock** | 1 | **56** | +56 *PIN protection for users* |
| Offline download | 2 | 55 | +53 *Offline mode* |
| Artwork / posters / logos | 11 | 53 | +28 *show titles in poster view* |
| Home-screen customisation | 15 | 52 | +13 *'Continue Watching' not displaying on home* |
| Playlists (manual) | 14 | 44 | +15 *Implement playlist ACL Interface* |
| Favourites | 6 | 42 | +25 *Song favorites get lost during playback* |
| Intro / credits skip | 3 | 35 | +17 *Add "Skip" button during media segments* |
| Sleep timer | 3 | 32 | +23 *Sleep Timer* |
| Playback speed | 7 | 25 | +10 *Playback Speed Controls* |
| 3D / stereoscopic | 1 | 23 | +23 *3D movie playback* |
| Shuffle / random play | 6 | 20 | +7 *android auto: shuffle all* |
| Picture-in-picture | 5 | 20 | +11 *PiP not working on Android 14* |
| DLNA / UPnP | 2 | 11 | +11 *Cast via DLNA/Miracast* |
| AirPlay | 1 | 11 | +11 *[tvOS] Audio Delays for AirPlay Speakers* |
| Watchlist | 1 | **3** | +3 *Add support for KefinTweaks watchlist* |
| External ratings (IMDb/RT) | 0 | **0** | — |
| Two-factor auth | 0 | **0** | — |

Full output: `/yoloai/cache/http/gh/score2.txt`.

**Subtitles at 479 is nearly four times the next entry**, which is the third independent
confirmation of [D11](../../decisions/working-notes.md) and a good sanity check on the method.

---

## 4. Demand, axis two: what people install a second program to get

A star is somebody running another container forever. Grouped by the **core gap** each tool
fills:

| Gap in the core | Tools | Stars |
| --- | --- | ---: |
| **Requests / discovery for household members** | Jellyseerr 12,649 · Overseerr 4,975 *(archived)* · Ombi 4,104 | **21,728** |
| **Play statistics and watch history** | Tautulli 6,589 · Jellystat 2,472 · Playback Reporting 135 | **9,196** |
| **Your library as live TV channels** | ErsatzTV 2,956 · xTeVe 2,263 · dizqueTV 1,766 · Threadfin 1,716 · pseudotv 266 | **8,967** |
| **Subtitle acquisition** | Bazarr 4,284 | 4,284 |
| **Transcode automation / offload** | Tdarr 4,328 · rffmpeg 1,076 | 5,404 |
| **Collection and metadata automation** | Kometa 3,414 · auto-collections 292 | 3,706 |
| **Retention / cleanup** | Maintainerr 2,287 · janitorr 753 · media-cleaner 261 | **3,301** |
| **User invitation and onboarding** | Wizarr 3,196 | 3,196 |
| **Intro detection** | intro-skipper 2,788 (+968 archived predecessor) | 3,756 |
| **DLNA** | Universal Media Server 2,652 · Jellyfin DLNA plugin **87** | — |
| **SSO** | jellyfin-plugin-sso **1,458** *(archived)* | 1,458 |
| **Home-screen sections** | jellyfin-plugin-home-sections 513 | 513 |
| **Merging duplicate versions** | mergeversions 442 | 442 |
| **Scrobbling** | jellyfin-plugin-trakt 296 | 296 |
| **Outbound notifications** | jellyfin-plugin-webhook 251 | 251 |

For reference, Jellyfin itself is 57,368★ and Kodi 21,241★.

**Requests is the single largest hole in this field by a factor of two**, and statistics and
live-TV-from-library are next. All three are things the server already has the data for and
declines to present.

---

## 5. Abandonment: what somebody built and then withdrew

This is the axis [D140](../../decisions/working-notes.md) did not have, and it is the most
decision-relevant part of the sweep.

| What | Who | When | Stated reason |
| --- | --- | --- | --- |
| **Plugins** (content channels) | Plex | Sept 2018 | *"Less than 2 percent of people were using Plugins"*; the technology needed a complete rebuild |
| **Cloud Sync** | Plex | Nov 2018 | adoption so low that Plex invited users to *"DM us a photo of your Cloud Sync library and today's newspaper"* |
| **Watch Later** (bookmarking) | Plex | Sept 2018 | *"costly to maintain and hard to justify when only a handful of folks used it"* |
| **Plex Media Player / HTPC mode** | Plex | announced Aug 2019, support ended Jan 2020 | replaced by a windowed desktop app; repo archived, last push **2022-05-31**, 1,211★ |
| **Plex Arcade** | Plex | 31 Mar 2022 | subscription product withdrawn |
| **Podcasts and Web Shows** | Plex | 15 Apr 2022 | withdrawn rather than fixed |
| **Watch Together** | Plex | **25 Feb 2025** | apps *"rebuilt from the ground up to boost our development speed"*; survives in the **web app only** |
| **DLNA in the core** | Jellyfin | 10.9 | moved to a plugin — *"independent updates, security, optional installation"*. The plugin now has **87★ and 84 open issues** |
| **EasyPassword (PIN)** | Jellyfin | 10.9 | *"a big security risk especially for administrator accounts"* |
| **Network paths in libraries** | Jellyfin | 10.10 | *"deprecated for a long time"* |
| **Games, News** | Jellyfin | at the 2018 fork | never carried over from Emby |
| **32-bit ARM, Pi V4L2** | Jellyfin | 10.10 → 10.11 | *"extremely poor in performance"* |
| **Open source itself** | Emby | Sept 2018 | public repo frozen at v3.5.3, 4,971★ |

And four cases where the **idea** survived but the **implementation** did not:

| Dead | Successor | Ratio |
| --- | --- | --- |
| `sct/overseerr` 4,975★ **archived** | `Fallenbagel/jellyseerr` 12,649★ | successor is **2.5×** the original |
| `9p4/jellyfin-plugin-sso` 1,458★ **archived** | none | the field's most-starred auth plugin is unmaintained |
| `ConfusedPolarBear/Intro-Skipper` 968★ **archived** | `intro-skipper/intro-skipper` 2,788★ | 2.9× |
| `DEFENDORe/pseudotv` 266★ → `vexorian/dizquetv` 1,766★ | `ErsatzTV` 2,956★ | three generations |
| `xteve-project/xTeVe` 2,263★ | `Threadfin` 1,716★ *(explicitly "based on xTeVe")* | — |

---

## 6. The cross-reference: what is missing from this tree

Every candidate was grepped against the whole tree as it stood (82 files). **Zero hits anywhere in the tree:**

3D/stereoscopic · Android Auto/CarPlay · awards metadata · Common Sense Media · external list
import · **external critic/audience ratings** · folder browsing view · games · Kodi sync queue ·
multi-server federation · music videos/concerts · poster overlays and badges · **shuffle/random
play** · Sonos/multi-room · sports · **two-factor auth**

**One to seven hits, i.e. named in passing and never designed:**

reverse proxy/base URL (1) · sleep timer (2) · **home-screen row curation** (2) · cloud/rclone
(2) · **bandwidth and concurrent-stream limits** (3) · bulk edit (3) · instant mix (3) ·
external player handoff (4, and designed in `tv-client.md`) · **user PIN** (4) · manual
identify/metadata editing (5) · Live TV (6, all of them declines) · playback speed (6) ·
artwork *types* — clearlogo, banner, disc (6) · **play statistics surface** (7) · transcode
throttling (7)

**Already covered, confirmed by the sweep** — chapters as distinct from segments
(`media-segments.md` says so explicitly), playlists and collections
([D79](../../decisions/working-notes.md)), favourites and user ratings
(`library-model.md`), watchlist (`play-state.md`), offline download
([D87](../../decisions/working-notes.md)), deep links
([D88](../../decisions/working-notes.md)), theme songs
([D97](../../decisions/working-notes.md)), intro/credit segments
([D82](../../decisions/working-notes.md)), Quick-Connect-shaped pairing
([D93](../../decisions/working-notes.md)), subtitle acquisition
([D147](../../decisions/working-notes.md)), duplicates and versions
([D79](../../decisions/working-notes.md)), and the declines already recorded in
[overview.md](../overview.md) for Live TV, requests, photos, audiobooks, comics and voice
assistants.

---

## 7. What the sweep says to do

Ranked by *demand ÷ cost*, with cost read against what this tree already has:

1. **Present the play statistics we already store.** 9,196★ of companion apps exist to show a
   user their own history. [D81](../../decisions/working-notes.md) already makes play state an
   append-only event log — the data is there and nothing reads it back. This is the highest
   ratio in the sweep and it is a query plus a page.
2. **Outbound notifications.** Tautulli's 22 triggers, 28 agents. We have two event sources
   already ([D8](../../decisions/working-notes.md)'s change journal,
   [D81](../../decisions/working-notes.md)'s play log) and no sink. The four that matter are
   operational: buffering, concurrent-stream limit hit, new device, server unreachable.
3. **Per-user and per-role bandwidth and concurrent-stream limits.** Absent, and not a nicety:
   [D25](../../decisions/working-notes.md)/[D64](../../decisions/working-notes.md)/[D86](../../decisions/working-notes.md)
   put a **paid relay** on the critical path, and a relay with no per-user ceiling is an
   unbounded bill.
4. **External critic and audience scores as registry fields.** Zero mentions here; twelve
   properties in Maintainerr; the basis of most of Kometa's 129 builders. Schema-shaped, so it
   is due before [library-model.md](../plans/library-model.md) freezes.
5. **External list import** (IMDb / Trakt / TMDb / Letterboxd → a collection).
   [D79](../../decisions/working-notes.md) already allows a collection to be *"an explicit
   member list or a predicate"*, so the model exists; the fetch-refresh-and-reconcile loop
   does not.
6. **Admin-curated home rows, per role.** Plex's `ManagedHub` with `homeVisibility`; 513★ for
   the Jellyfin plugin. We have per-user row *order*
   ([D138](../../decisions/working-notes.md)) and no way for an owner to say *"this collection
   is a row on the kids' home screen"* — which is exactly what
   [D130](../../decisions/working-notes.md)'s roles and
   [D134](../../decisions/working-notes.md)'s maintained values were built for.
7. **Retention as a presented decision, not a deletion.** 3,301★. We already *mark*
   ([D75](../../decisions/working-notes.md)) and deliberately do not delete
   ([D144](../../decisions/working-notes.md)). Maintainerr's actual product is the **grace
   period**: gather candidates into a visible collection, give everyone time, then act. The
   visible-collection half needs no deletion at all.
8. **Name the small client behaviours.** Shuffle, playback speed, sleep timer, folder view,
   picture-in-picture, bulk edit. Each is small, each is absent, and together they are the
   difference between "complete" and "obviously unfinished".

And three the sweep says to **decline on the record**, because silence reads as neglect:
DLNA (the incumbent moved it out of core and the plugin has almost as many open issues as
stars), 3D/stereoscopic, and Sonos/multi-room.

## Sources

- Local clones under `/yoloai/cache/repos/` — `jellyfin`, `Emby`, `kodi`, `python-plexapi`,
  `Tautulli`, `Kometa`, `Maintainerr`, `ErsatzTV`, `jellyseerr`, `Ombi`, `Jellystat`, `wizarr`,
  `ShokoServer`, `komga`, `UniversalMediaServer`, `Threadfin`, `stremio-web`, `Sonarr`,
  `Radarr`, `jellyfin-androidtv`, `jellyfin-roku`, and the Jellyfin plugin repos
- `https://repo.jellyfin.org/files/plugin/manifest.json` — the official plugin catalogue
- GitHub search and repository APIs — harvest and star counts cached in `/yoloai/cache/http/gh/`
- Jellyfin release announcements 10.9.0 and 10.10.0 (jellyfin.org)
- TechCrunch, 26 Sept 2018, on Plex's plugin/Cloud Sync/Watch Later sunset; PCWorld, Feb 2025,
  on the Watch Together withdrawal
