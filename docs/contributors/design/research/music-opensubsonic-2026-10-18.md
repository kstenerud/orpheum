> **ABOUTME:** Whether Orpheum's music can lean on the OpenSubsonic API and its existing apps: the
> spec and its extensions in 2026, every music feature against what the API and its clients carry,
> the clients by platform, the effort of the shared music backend and of the two client routes, and
> the finding that decides the route: a Subsonic app cannot reach a core over WebRTC. Answers the
> owner's question of 2026-10-18, with the owner's questions it raises.

# Music: OpenSubsonic, effort and reach, 2026-10-18

The owner, bringing music into scope at the milestone reassessment
([milestone-reassessment-2026-10-18.md](milestone-reassessment-2026-10-18.md)): *"Look into how much
work this will be. If we can piggyback off an existing API, that's great, but I want to know that the
features I want are possible under that API."* It builds on [music.md](music.md) and
[competitors-audio.md](competitors-audio.md) (2026-09-17), and D1 and D186.

**The answer:** OpenSubsonic covers nearly every library and playback feature, and maintained apps
exist on every platform, Android Auto, CarPlay and Sonos included. But a third-party Subsonic app
speaks plain HTTPS to one URL and cannot reach a core over WebRTC: for a household behind carrier
NAT it works at home and over a direct path only. That, not the API, decides the route.

All URLs read 2026-10-18; shallow clones of the spec, Navidrome, gonic and five clients are cached
under `/yoloai/cache/music-research/`.

## 1. The API

- **Versions:** OpenSubsonic builds on Subsonic REST 1.16.1 (servers support at least 1.14.0); its
  own version is 1, with extensions added within it. Responses carry `openSubsonic: true`, `type` and
  `serverVersion`. 88 endpoint pages: system, browsing by folder and by tag, lists, search,
  playlists, media (`stream`, `download`, `hls`, `getCoverArt`, `getLyrics`), annotation (`star`,
  `setRating`, `scrobble`), shares, podcasts, jukebox, internet radio, chat, users, bookmarks and the
  play queue, scanning.
- **Its additions to the response, all optional:** a song's `artists`, `albumArtists`,
  `contributors` with roles, `displayComposer`, `musicBrainzId`, `isrc`, `genres`, `moods`, `bpm`,
  `replayGain` (track and album gain and peak), `explicitStatus`, `works`, `movements`,
  `groupings`, `bitDepth`, `samplingRate`; an album's `isCompilation`, `releaseTypes`,
  `recordLabels`, `discTitles`, `originalReleaseDate`.
- **Extensions**, listed by `getOpenSubsonicExtensions`, the one call answered unauthenticated:

| Extension | What it adds |
| --- | --- |
| `apiKeyAuthentication` | `apiKey=` in place of username and token: server-made, revocable; the spec recommends dropping token auth (2025) |
| `formPost` | A POST form, so credentials leave the URL (2025) |
| `songLyrics` v1, v2 | Synced lyrics in several languages; v2 adds word timing, translations and pronunciation |
| `transcodeOffset` | Start a transcoded stream at an offset |
| `getPodcastEpisode` | One episode (2025-03) |
| `indexBasedQueue` | Play queue by index (2025-06) |
| `transcoding` | The client sends what it can play and the server decides (2025-12) |
| `playbackReport` | Position, state and rate as it plays (2026-03) |
| `sonicSimilarity` | Similar tracks and a path between two (2026-04) |
| `topSongsByArtistId` | Top songs by an artist's id (2026-07) |

- **Subsonic's own authentication is weak, as its spec says:** the token is `md5(password + salt)`,
  so the server must hold the password reversibly; a stored salt and token replay as the password;
  `p=` sends the password in the query string; every form puts credentials in URLs. OpenSubsonic's
  answer is `apiKey` with `formPost`. It is new: Navidrome took it on 2026-09-28 (PR #6219) and
  Symfonium sends it, but Amperfy, Feishin, Supersonic, Tempus and Ultrasonic do not yet.

## 2. Music features, and what the API carries

| Feature | OpenSubsonic | Clients | Notes |
| --- | --- | --- | --- |
| Browse by artist, album, track, genre, year | core | all | |
| Composers and other roles | core fields | Symfonium, Feishin | |
| Several artists, album artists | core fields | Symfonium, Feishin, Supersonic | Fits D186's artists as entities |
| Compilations, several discs | core fields | all | |
| MusicBrainz ids | core field | a few | |
| Cover art, artist images | core | all | Artist images need a provider |
| Classical works and movements | fields (2026-03) | few | |
| Lyrics, plain and synced | core; `songLyrics` | Feishin, Tempus, Amperfy, Symfonium | |
| Gapless | nothing on the server | Supersonic, Feishin, Tempus, Amperfy, Symfonium, Narjo | A client's job; a transcode must not pad (Opus) |
| ReplayGain, track and album | field | Amperfy, Feishin, Supersonic, Tempus, Symfonium, Ultrasonic | D226's measurement gives track gain |
| Transcoding, quality caps | core; extensions | Tempus, Symfonium, Feishin | |
| Lossless and hi-res | `stream`, `download` | all | |
| Scrobbling (Last.fm, ListenBrainz) | core | all | |
| Playlists | core | all | |
| Smart playlists | **no** (shown as plain playlists) | Feishin edits through Navidrome's own API | |
| Artist radio, similar artists | core | Supersonic, Tempus, Symfonium | Needs similarity data |
| Sonic analysis (Plexamp-style) | extension | **none checked call it** | The analysis is the cost |
| Starred, ratings, play counts | core | all | `favourite`, `user_rating`, `play_event` exist |
| Shares (public links) | core | Amperfy, Tempus, Ultrasonic | Against D187 (§6) |
| Podcasts | core | Amperfy, Tempus, Ultrasonic | |
| Internet radio | core | Amperfy, Feishin, Tempus, Supersonic | |
| Audiobooks: chapters, resume | partial (bookmarks, no chapters) | Ultrasonic, DSub | Another product (competitors-audio.md) |
| Offline downloads | `download` | Symfonium, Amperfy, Tempus, Substreamer, Narjo | |
| Several users | core | all | |
| Jukebox | core | Ultrasonic, DSub, Feishin | The node has no speakers: out |
| Chromecast, UPnP, Sonos | nothing (the device fetches the URL) | Symfonium, Tempus, Supersonic, Feishin; Sonos through bonob | The device needs the app's reachability |
| Android Auto, CarPlay | nothing | Symfonium, Tempus, Ultrasonic; Amperfy, play:Sub, Narjo | |
| Listen together, with voice | **no** | none | Only Orpheum's own client (Watch Together) |
| Queue sync across devices | core | Feishin, Tempus, Supersonic, Symfonium | |

## 3. Clients

| Client | Platforms | Notes |
| --- | --- | --- |
| Symfonium | Android, Wear OS, Android Auto | Paid after a trial; offline, Sonos, Chromecast, UPnP, Kodi; `apiKey` |
| Tempus | Android, Android Auto | Active; Chromecast, gapless, ReplayGain, podcasts |
| Ultrasonic, DSub | Android | Older; offline, jukebox |
| Amperfy | iOS, iPadOS, macOS, CarPlay | Offline, gapless, ReplayGain, podcasts |
| play:Sub, Narjo, Arpeggi, Substreamer | iOS (Substreamer also Android) | Paid or TestFlight; offline |
| Feishin | Windows, macOS, Linux, web | mpv, synced lyrics, UPnP |
| Supersonic | Windows, macOS, Linux | Gapless; takes a second hostname; no offline |
| Castafiore, Airsonic Refix | web | |
| bonob | Sonos bridge | On Sonos S2, must be reachable from the internet |
| Finamp, Jellify | Android, iOS | Jellyfin's API only |
| Plexamp | all | Plex only, mostly Plex Pass |

## 4. Effort

Sizes: S days, M one to two weeks, L three to six weeks, XL longer. For scale, Navidrome's Subsonic
layer is about 5,700 lines of Go behind 66 endpoints, in a 104,000-line server; gonic is 15,000 lines
in all.

**The shared music backend, needed by either route: about XL.**

| Piece | Size | Why |
| --- | --- | --- |
| Reading tags in the node | M–L | `probe` keeps little of a file's tags and ffprobe flattens multi-valued ones; Navidrome and gonic use `go.senan.xyz/taglib` (TagLib in WASM): **a new dependency** |
| Schema | L | D186's additive list: people and credits, albums, genres and moods, playlists that allow repeats, lyrics, ReplayGain, a `music` library kind; **what defines an album** is D186 §3's open question |
| Artwork | M | Embedded covers and `folder.jpg` through the node; artist images from a provider |
| Loudness | S–M | D226 gives track gain; album gain is new |
| Audio transcoding | M | Direct bytes are relayed with ranges already; a progressive Opus or AAC stream is not (chunks are video HLS) |
| Search, stars, ratings, history | S | The tables exist, keyed on `item` |
| Scrobbling, similarity providers | M | ListenBrainz and Last.fm with the person's own key |
| Sonic analysis | XL | A model and a dependency: deferred |

**Route (a), an OpenSubsonic layer and other people's apps:** the API M–L (about 45 endpoints, XML
and JSON, each behind the role's libraries); authentication S–M; client work none; reach away from
home L–XL where a household is behind carrier NAT (§5). Orpheum's passwords are Argon2 hashes, so
token auth is impossible without a reversible secret, and accepting `p=` would put the account
password in URLs and step around D312's second factor. The fit is a per-device **app password**:
random, music-only, revocable, listed with the person's devices (the schema's `grant.kind` has
`api_key` already), taken as `apiKey`, and, for apps without it, as the token's password, which
means holding that one random secret reversibly (as Nextcloud Music and Ampache do).

**Route (b), Orpheum's own music view:** L for a good web player, XL for a Plexamp-class one; gapless
needs Web Audio or MSE with trimming, not a plain `<audio>`; no CarPlay, Android Auto or Sonos, and
iOS background audio is fragile for a web app. Its API is Orpheum's own, through the loader and the
transport, so it works wherever the web client does, WebRTC included, and it is the only route to
listening together with voice.

## 5. Whether a Subsonic app reaches a core behind carrier NAT

**No, except over a direct path or a relay.** Orpheum's client finds its core through the
discovery's paths call, races LAN, IPv6, IPv4 and WebRTC (D273 §7, D274) and runs the core's own
client over the winner (D308). A Subsonic app takes one base URL and speaks HTTPS to it.

1. **At home:** the LAN name (`https://192-168-x-y.<coreId>.orpheum-core.com`) works with a valid
   certificate, unless the router blocks rebinding.
2. **Away, over a direct path:** with IPv6 at both ends and the port let in, or a mapped IPv4 port.
   But the names carry the address (discovery-protocol §2.7), so a rotated prefix kills the URL in
   the app. For the owner's DS-Lite household: from IPv6 networks (mostly mobile data), not from
   IPv4-only Wi-Fi.
3. **Behind carrier NAT, from an IPv4-only network: impossible.** WebRTC is the only path, and no
   Subsonic app speaks it.
4. **Cast devices** fetch the URL themselves and inherit the same limits.

What would close it, each a design change for the owner: (i) a stable name per core for its current
addresses, which the zone deliberately does not publish today; (ii) a relay for the music API alone,
cheap at 0.1–1 Mb/s, but against D273 §2 and waiting on D274 §3's count; (iii) the household's own
VPS, reverse proxy or VPN, which works today.

## 6. Recommendation, and the owner's questions

**Recommendation:** build the shared backend first, needed either way; then the OpenSubsonic layer
as the first client surface, for Android Auto, CarPlay, Sonos, offline and gapless at no client
cost, with per-device app passwords only; and a lighter music view in Orpheum's own client for the
remote case behind carrier NAT and for listening together. A Jellyfin music surface (Finamp) could
share D18's shim later. The API cannot carry listening together, smart-playlist editing, jukebox or
chapters.

**Questions:**

1. Which music features are wanted (§2)? Smart playlists, sonic radio and listening together are
   the costly or outside-the-API ones.
2. Must other apps work away from home behind carrier NAT? If so: a stable name, a music-only relay
   (revisiting D273 §2), or "your VPS or VPN"?
3. The dependency `go.senan.xyz/taglib` (with wazero) for tags (rule 3).
4. May an app password be held reversibly, so apps without `apiKey` can use token auth, or is the
   API `apiKey`-only (today Symfonium and few others)?
5. Shares make public links without an account, which reads as rebroadcast under D187: leave them
   out, or limit them to named accounts?
6. Podcasts and audiobooks: in, or out as competitors-audio.md advises?
7. What defines an album (D186 §3): the MusicBrainz id, else stated tags, else the folder?

## Sources

[OpenSubsonic spec](https://opensubsonic.netlify.app/) and [repository](https://github.com/opensubsonic/open-subsonic-api);
[Navidrome](https://github.com/navidrome/navidrome) and [PR #6219](https://github.com/navidrome/navidrome/pull/6219);
[gonic](https://github.com/sentriz/gonic);
[Symfonium](https://symfonium.app/) and [its feature table](https://docs.symfonium.app/wiki/providers/supported-features-by-media-providers/);
[kima-hub #233](https://github.com/Chevron7Locked/kima-hub/issues/233);
[Plexamp](https://www.plex.tv/plexamp/); [Finamp](https://github.com/jmshrv/finamp); [bonob](https://github.com/simojenki/bonob).
All accessed 2026-10-18.
