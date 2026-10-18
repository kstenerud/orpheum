> **ABOUTME:** Where Orpheum stands, feature by feature, on 2026-10-18 against the self-hosted
> servers (Plex, Jellyfin, Emby; Infuse, Kodi, Channels DVR briefly) and the commercial streaming
> services (Netflix, Apple TV, Prime Video, Disney+, HBO Max, YouTube) that set what viewers expect.
> Orpheum's side is read from the code at an earlier commit, not from plans; the others' from current
> documentation, release notes and press, with sources. Ends with where Orpheum leads, the gaps
> ranked by how viewers will judge them, and the owner's questions.

# Feature standing against the field, 2026-10-18

The owner asked (2026-10-18) for an audit of where Orpheum stands, feature-wise, against its
competitors, "including the big boys", and then against Netflix, Apple, Amazon and the rest. This
complements [competitors-video.md](competitors-video.md) (architecture and where each breaks,
2026-09-17) and [commercial-streaming.md](commercial-streaming.md) (what the big services do that
transfers): those are about how; this is a dated inventory of what each has now.

## Method

- **Orpheum:** every feature marked built was checked against the code (its handler under
  `internal/core/api`, its package, or its screen in `web/apps/web/src/router.ts`) at an earlier commit.
  *Partial* says what is missing; *planned* cites the plan or decision; *absent* means nothing.
- **Self-hosted servers:** official docs and release notes first (Jellyfin 12.2 of 2026-10-05, Emby
  4.10.1.0 of 2026-09-29, Plex Media Server 1.43.5 of 2026-10-06, Kodi 22 RC1), then press and
  forums. Several plex.tv support pages refuse automated fetches (HTTP 403), so some Plex rows rest
  on press coverage and Plex's own forum posts.
- **Streaming services:** official help centres, then reviews. US tiers and prices. Second-hand
  sources are marked (2°), unverified claims (u).
- Everything was gathered 2026-10-18. Raw pages are cached under
  `/yoloai/cache/competitor-research/`.

## The short answer

- **Orpheum leads where the others have given up or never went:** reaching a home server from
  anywhere with no port forwarding, no VPN for the viewer and no paywall; Watch Together with voice;
  adaptive quality built for a home's upload; dialogue boost and loudness levelling for every film;
  and a media node that cannot open what it was not given.
- **Orpheum is level on the core of a film library:** scanning, matching and correcting it,
  versions and extras, resume and Continue Watching, track choices remembered, text subtitles.
- **Orpheum is behind on reach and polish:** no TV app, no Chromecast, no native phone app; no
  skip intro, trickplay, chapters or Next Up; no HDR tone mapping or bitmap subtitles; no profiles
  or parental controls; a bare title page (no cast, genres or ratings) and a browse with two sorts.
- **Out of scope by decision, not gaps to close now:** music (D1, D186), photos (D1), live TV and
  DVR, plugins (D12, D15, D19).

## 1. Where Orpheum leads

| What | Orpheum | Self-hosted field | Commercial services |
| --- | --- | --- | --- |
| Reaching home from anywhere | WebRTC through a discovery that carries no content; works behind CGNAT/DS-Lite with no port forwarding; direct paths (IPv6 first, PCP/NAT-PMP/UPnP) as upgrades; client checked against the core's signed manifest (D273, D291, D308) | **Plex:** relay capped at 2 Mbps, and remote play paywalled since 2025-04-29 (enforced on TVs through 2026, on third-party clients from 2026-09). Remote IPv6 is a Plex Pass feature (1.43.3) needing an inbound pinhole. **Jellyfin/Emby:** nothing without port forwarding; their own docs point to Tailscale, which every viewer must install | n/a, but all police the household: Netflix's monthly home-Wi-Fi check-in, paid extra members, HBO Max (2025), YouTube Family (2025) |
| Watch Together | Own sync model (D264, D268, D282–D286), the host may keep pause and seek (D271), a roster of each person's position, guests by link, **voice built in**, voice on a phone while the film plays on the computer (D281) | **Plex:** Watch Together dropped from new apps (2025-02), web only. **Jellyfin:** SyncPlay, no voice. **Emby:** none native. No one has voice | Netflix never had one; Disney+ GroupWatch removed 2023-09; Prime Watch Party ended 2024-04; SharePlay is Apple-only over FaceTime; YouTube co-watch is Android-only with a Premium host |
| Quality on a poor or shared link | Own ABR controller in hls.js (D265), a ladder from the start (D260), start quality from the speed remembered per device and network (D257), a lower version made at the first stall (D247, D253), the household's upload shared fairly between viewers and groups (D286) | No true ABR in any of the three: a fixed per-session quality, a manual choice or a bitrate test | Their ABR is the benchmark viewers know |
| Dialogue and loudness | Dialogue-boost rendition and loudness levelling to −23 LUFS for every film (D222, D226) | Some Plex clients; not Jellyfin or Emby (u); Infuse and Kodi have night modes | Prime's Dialogue Boost, Apple's Enhance Dialogue; Netflix has none of its own |
| No tiers on the file | The file's full quality, in every browser the client supports (D271) | Plex: hardware transcoding, HDR tone mapping and skip intro need Plex Pass ($749.99 lifetime from 2026-07-01); Emby: hardware transcoding and skip intro need Premiere | 4K and Atmos only on the top tier (Netflix, HBO Max, Prime Ultra, Disney+ Atmos); browsers capped by DRM (Netflix 1080p in Firefox, 720p in Edge on a Mac) |
| Security and privacy | The core never opens a media file; the node is confined by Landlock and seccomp; secrets sealed (D193, D194); no cloud account needed beyond the optional discovery | Plex needs a plex.tv account, had breaches in 2022 and 2025 (forced resets), and turned public profiles on by default (2025-01); Jellyfin fixed path-traversal and FFmpeg argument-injection advisories in 10.11/12.0 | Viewing data feeds ad targeting and household detection |
| Explaining the library | What the scanner saw, a filing report per title, guesses marked for review (D301), missing disks shown rather than emptied (D241, D277) | Fix-match exists everywhere; the reasons are not shown (u) | n/a |
| Content that stays | The household's own files | Same | Titles leave (Netflix removing 100+ originals in 2026 (2°), Max pulled 87); PlayStation deleted 551 purchased films 2026-09-01 (2°) |

## 2. Category by category

Cells are short; the notes after each table hold the nuance. "Pass" is Plex Pass, "Prem" Emby
Premiere. The last column is what the commercial services have taught viewers to expect.

### 2.1 Library and metadata

| Feature | Orpheum | Plex | Jellyfin | Emby | Viewers expect |
| --- | --- | --- | --- | --- | --- |
| Films, series | Yes | Yes | Yes | Yes | — |
| Music / photos / books | No (D1, D186) | Yes / Yes / workaround | Yes / Yes / Yes | Yes / Yes / Yes | — |
| Real-time scanning | Yes: inotify, staged filing (D304) | Yes | Yes | Yes | — |
| Metadata | TMDB (key the owner's own, D16), local NFO | Plex's service; NFO agent in preview (2026-02) | TMDb, TVDb, OMDb, MusicBrainz | TMDb, TVDb, OMDb | — |
| Fix a wrong match | Yes, with candidates, overrides and reasons (D301) | Yes | Yes | Yes | — |
| Versions, editions, extras | Yes | Yes (editions Pass) | Yes (episode versions 12.0) | Yes | — |
| Collections, playlists | Planned (D75) | Yes | Yes | Yes | Yes |
| Genres, cast, ratings on a title | **No** | Yes | Yes | Yes | Yes: cast, trailers, X-Ray-style detail |
| Browse filters | Two sorts, no filters | Many | Many (language filters 12.0) | Many | Yes |
| Search | Yes, own index | Yes | Yes | Yes | Voice and natural-language search |
| Continue Watching | Yes, hide with undo | Yes | Yes | Yes | Yes, synced to the second |
| Next Up | Planned (past M3) | Yes | Yes | Yes | Yes |
| Watched badges, mark watched | **No** (positions kept) | Yes | Yes | Yes | Yes |
| Recommendations | No | Yes, "Match Score" (2026) | Providers (12.0) | Basic | Heavily personalised |
| Trakt sync | No | Third-party | Plugin | Plugin | — |

### 2.2 Playback

| Feature | Orpheum | Plex | Jellyfin | Emby | Viewers expect |
| --- | --- | --- | --- | --- | --- |
| Direct play, remux | Yes | Yes | Yes | Yes | — |
| Transcoding | H.264 8-bit "prepares", served as made (D284) | Yes, HEVC encode on by default (1.43.1) | Yes, FFmpeg 8.1 | Yes | Instant start |
| Hardware encoding | Quick Sync, NVENC; **no VAAPI/AMD** | Pass | Free: QSV, NVENC, VAAPI, AMF, VideoToolbox, RKMPP | Prem | — |
| HDR | Direct play when the device declares it; **no tone mapping** | Pass | Free, 3D LUT | Prem for hardware | Dolby Vision, HDR10+ (top tiers) |
| Adaptive quality | **Yes, own controller** | No | No | No | Yes |
| Dialogue boost, loudness | **Yes** | Some clients | No (u) | No (u) | Prime, Apple |
| Text subtitles (SRT, ASS, VTT) | Yes, ASS as styled (JASSUB) | Yes | Yes | Yes | Styling options, CC and dialogue-only |
| Bitmap subtitles (PGS, VobSub) | **No**, nor burn-in | Yes, burn-in | Yes, client-side (12.0) | Yes | — |
| Fetched subtitles | OpenSubtitles on request (D223) | Yes | Plugin | Plugin | — |
| Track choices remembered | Per file, carried through a series (D219, D230) | Yes | Yes | Yes | Per profile |
| Skip intro / credits | Planned (D82) | Pass | Media segments + plugin | Intro: Prem | **Everywhere**; authored by hand |
| Next-episode countdown | No (Next Up planned) | Yes | Yes | Yes | **Everywhere**, about 15 s |
| Trickplay thumbnails | Planned | Yes | Yes (10.9) | Yes | Yes |
| Chapters | Probed, not shown | Yes | Yes | Yes | — |
| Playback speed | The browser's own only | Yes (u) | Yes (u) | Yes (u) | Common (u) |
| Offline downloads | Planned (D87) | Pass | Free | Prem | Mobile, with limits |
| Resume across devices | Yes; a lost playback reopens where it was | Yes | Yes | Yes | Yes |

### 2.3 Clients and devices

| Platform | Orpheum | Plex | Jellyfin | Emby | Viewers expect |
| --- | --- | --- | --- | --- | --- |
| Web | Yes, installable; four browsers (D271) | Yes | Yes | Yes | Yes |
| Phones | The web client; no native app (D10) | Official | Official + third-party | Official (unlock) | Native |
| Chromecast / Google Cast | **Stub only** (M6, D245) | Yes | Yes | Yes | Mostly; Netflix removed it 2025-11 on Google TV |
| AirPlay | No | Partial | Partial | Partial | Yes |
| Android TV / Fire TV / Apple TV / Roku / Samsung / LG | **None** (M6, D245) | Official everywhere | Official (Tizen 2026-05), Swiftfin | Official | Every TV OS and console |
| DLNA, Kodi | No; a Jellyfin API adapter is planned (D18) | DLNA; Kodi third-party | Plugin; official Kodi add-on | DLNA; official add-on | — |
| Pairing by code | Yes (D218, D288) | plex.tv/link | Quick Connect | Yes | Yes |

### 2.4 Remote access

| Aspect | Orpheum | Plex | Jellyfin | Emby |
| --- | --- | --- | --- | --- |
| Paid? | No | Pass or Remote Watch Pass ($2.99/mo) | No | No |
| No port forwarding (CGNAT, DS-Lite) | **Full quality over WebRTC**; TURN embedded | Relay at 2 Mbps | Tailscale, tunnel or VPS | Same |
| Direct path when possible | Yes, IPv6 first; PCP, NAT-PMP, UPnP, IGD2 pinholes | UPnP; IPv6 behind Pass | Manual | UPnP |
| TLS | ACME DNS-01 via Cloudflare or the discovery, wildcard | `*.plex.direct` | Bring your own | Bring your own |
| Fair sharing of the upload | Yes (D286) | Per-session caps | Per-user caps | Per-user caps |

### 2.5 Social

| Feature | Orpheum | Plex | Jellyfin | Emby | Commercial |
| --- | --- | --- | --- | --- | --- |
| Watch together | **Yes** | Web only | SyncPlay | None | Mostly withdrawn |
| Voice | **Yes** | No | No | No | FaceTime (Apple), Meet (YouTube, Android) |
| Guests without an account | Yes, by link (D246) | No (u) | No | No | — |
| Share with friends | Invitations; roles choose libraries | Yes, friends need a pass to stream remotely | Local accounts | Local accounts | Paid extra members |

### 2.6 Users and security

| Feature | Orpheum | Plex | Jellyfin | Emby | Viewers expect |
| --- | --- | --- | --- | --- | --- |
| Accounts, roles | Yes, custom roles | Yes | Yes | Yes | — |
| Profiles, kids, parental controls | **Planned** (D84) | Yes | Yes | Yes | **Everywhere**: kids profile, maturity levels, PIN |
| Sign-in | Password, device code, realm sign-in at the discovery (D297), single-use hand-off (D308) | plex.tv account, Google/Apple | Local, Quick Connect, LDAP/OIDC plugins | Local, Emby Connect | — |
| 2FA, OIDC, passkeys | No | 2FA on plex.tv | Plugins only | No (u) | — |
| Admin only from home unless opted in | Yes (D292, D302) | No (u) | No (u) | No (u) | — |
| Media isolated from the server | **Yes**, Landlock and seccomp | No | No | No | — |

### 2.7 Administration

| Aspect | Orpheum | Plex | Jellyfin | Emby |
| --- | --- | --- | --- | --- |
| Setup, settings, jobs, logs | Yes | Yes (easiest) | Yes | Yes |
| Backups | Hourly, thinned, with restore (D305) | Manual | Built in (10.11) | Prem |
| Health, invariants checked | `/health`, `/doctor`, database invariants daily | Partial (u) | Partial | Partial (u) |
| Several media nodes | Built, a second is out of Cut A | Several servers, one account | Separate servers | Separate servers |
| Plugins, API | Planned; OpenAPI published | Agents only; unofficial API | Plugins; open API | Plugins; open API |
| Languages | English only (catalogue in place) | Many | Many | Many |
| Accessibility | Partial (D90 planned) | (u) | (u) | (u) |

## 3. The gaps, ranked by how viewers will judge them

Ranked by what the commercial services have made a baseline, then by what the self-hosted field
all have. Each is already planned or out of scope by a decision; this ranks, it decides nothing
(Cut A's design is frozen, D193 §8).

1. **A TV and the phone as remote:** no Cast receiver and no TV app (M6, D245). Every competitor,
   and every service but Netflix, plays on the TV. The largest gap in reach.
2. **Between episodes:** no Next Up, no next-episode countdown, no skip intro (D82). Every
   streaming service has all three; so do the self-hosted servers (some behind a pass).
3. **Profiles and parental controls** (D84): universal elsewhere.
4. **The title page and browsing:** no cast, genres or ratings, two sorts, no filters, no watched
   badges. Every competitor has these; the streaming services set a high bar for title pages.
5. **Picture formats:** no HDR tone mapping (an HDR file sent to an SDR screen looks washed out) and
   no bitmap subtitles (Blu-ray rips carry PGS); no VAAPI or AMD encoding.
6. **Player furniture:** trickplay, chapters, playback speed.
7. **Downloads** (D87): Jellyfin free, the others paid; the services allow mobile downloads.
8. **Languages and accessibility:** English only; D90 planned.

## 4. Where the commercial services have left room

What the services do that a household server need not, each a reason someone would choose
Orpheum, and each something Orpheum already has or can keep:

- **Ads keep growing:** ad load up 18% across services in 2026; Prime's default tier has ads and
  pause ads are spreading. Orpheum has none.
- **Quality behind tiers and DRM:** 4K and Atmos only on the top tiers; browsers capped at 1080p or
  720p by DRM. Orpheum plays the file as it is.
- **Household policing and stream caps:** two streams on Netflix's and HBO Max's lower tiers; monthly
  home-Wi-Fi checks. Orpheum's owner decides who watches and from where, and its upload budget
  shares fairly rather than refusing.
- **Features withdrawn:** casting (Netflix, 2025-11), watch parties (Disney+ 2023, Prime 2024).
  Orpheum's Watch Together with voice fills the gap they left.
- **Content that leaves:** removals and even deleted purchases. A household library stays.
- **Browsing built for engagement:** vertical-video feeds (Netflix Clips, Disney+ Verts), autoplay
  previews. A household library can be calm and complete.

## 5. Questions for the owner

1. Section 3's order is by what viewers judge first. Does it match the order you want M5 and M6 to
   take, especially the TV (1) against the episode furniture (2)?
2. Skip intro needs markers: hand-authored for the services, detected by audio fingerprint in
   Jellyfin's plugin and in Kyoo. D82 plans media segments; is detection in scope for it?
3. The title page's missing cast, genres and ratings are all in TMDB already. Is that a Cut A
   gap to raise against D193 §8, or M5's?

## Sources

Self-hosted servers (accessed 2026-10-18):

- [Jellyfin 12.0 release notes](https://github.com/jellyfin/jellyfin/releases/tag/v12.0); [Jellyfin 10.11.0](https://jellyfin.org/posts/jellyfin-release-10.11.0/); [Jellyfin clients](https://jellyfin.org/downloads/clients/all/); [Jellyfin on Tailscale](https://jellyfin.org/docs/general/post-install/networking/tailscale/)
- [Plex relay](https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/) (403 to automated fetch; via search summary); [Plex IPv6 beyond CGNAT](https://forums.plex.tv/t/bypass-cg-nat-via-ipv6-with-newest-beta-release/940261); [Plex Watch Together change](https://forums.plex.tv/t/an-important-watch-together-change/906796); [Plex NFO agent preview](https://forums.plex.tv/t/plex-nfo-agent-forum-preview/936104)
- [MacRumors: lifetime Plex Pass to $750](https://www.macrumors.com/2026/05/19/lifetime-plex-pass-price-increase/); [How-To Geek: Plex enforcing remote play on TVs](https://www.howtogeek.com/plex-is-now-enforcing-remote-play-restrictions-on-tvs/); [NetGuide: third-party apps need a licence](https://netguide.io/news/en/2026/09/22/plex-remote-streaming-via-third-party-apps-to-require-a-license/); [BleepingComputer: Plex 2025 breach](https://www.bleepingcomputer.com/news/security/plex-tells-users-to-reset-passwords-after-new-data-breach/); [The Desk: Plex social discovery](https://thedesk.net/2026/06/plex-launches-social-discovery-features/)
- [Emby Premiere](https://emby.media/premiere.html); [Emby Intro Skip](https://emby.media/support/articles/Intro-Skip.html); [Emby 4.10.1.0](https://github.com/MediaBrowser/Emby.Releases/releases/tag/4.10.1.0); [XDA: Emby has no watch parties](https://www.xda-developers.com/emby-doesnt-have-watch-parties-community-solution-actually-works/)
- [Infuse](https://firecore.com/infuse); [Kodi 22 RC1](https://betanews.com/article/kodi-22-piers-rc-1-features/); [Channels subscription](https://getchannels.com/docs/getting-started/quick-start-guide/subscription/); [Kyoo](https://www.linuxlinks.com/kyoo-self-hosted-media-server/)

Commercial services (accessed 2026-10-18):

- Netflix Help: [plans](https://help.netflix.com/en/node/24926), [downloads](https://help.netflix.com/en/node/54816), [extra member](https://help.netflix.com/en/node/123277), [browser requirements](https://help.netflix.com/en/node/30081), [screen readers](https://help.netflix.com/en/node/116023); [MacRumors: Netflix kills casting](https://www.macrumors.com/2025/12/01/netflix-kills-casting-from-mobile-app-to-tvs/); [TechCrunch: dialogue-only subtitles](https://techcrunch.com/2025/04/24/netflix-now-offers-dialogue-only-subtitles/)
- [9to5Mac: Apple Enhance Dialogue](https://9to5mac.com/2024/11/15/tvos-18-added-a-new-apple-tv-4k-setting-thats-been-a-no-brainer-upgrade/); [Apple Support: downloads](https://support.apple.com/en-us/118239)
- [About Amazon: Prime Video Ultra](https://www.aboutamazon.com/news/entertainment/prime-video-ultra-ad-free-streaming-subscription); [About Amazon: Prime Video AI features](https://www.aboutamazon.com/news/entertainment/artificial-intelligence-prime-video-streaming); [Android Authority: Prime Watch Party ended](https://www.androidauthority.com/amazon-prime-viewing-party-axed-3458167/)
- [How-To Geek: Disney GroupWatch removed](https://www.howtogeek.com/disney-quietly-removed-its-groupwatch-feature/); [9to5Mac: Disney+ Verts](https://9to5mac.com/2026/03/12/disney-is-launching-vertical-video-today-heres-how-it-works/)
- [MacRumors: HBO Max sharing crackdown](https://www.macrumors.com/2025/08/07/hbo-max-password-sharing-crackdown/); [Android Police: YouTube co-watching](https://www.androidpolice.com/youtube-premium-watch-videos-together/); [Android Authority: more ads in 2026](https://www.androidauthority.com/streaming-services-more-ads-2026-3708866/); [TechRadar: PlayStation deletes purchases](https://www.techradar.com/streaming/entertainment/this-should-be-illegal-sony-is-deleting-over-500-movies-that-people-bought-from-their-digital-libraries-just-proving-further-why-4k-blu-rays-popularity-keeps-growing) (2°)
