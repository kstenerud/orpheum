> **ABOUTME:** The external services a media server must talk to for identification, artwork,
> subtitles and scrobbling — what each provides, what each costs, the licensing traps, and how
> to keep any one of them from becoming a single point of failure.

# Technology: Metadata and External Services

Metadata is where a media server earns or loses its user's trust in the first ten minutes. It
is also the layer most exposed to other people's business decisions.

## 1. The identification problem

Before any provider is called, a file has to be turned into a guess. This is fuzzy string
parsing over filenames and directory structures, and it is harder than it looks:

```
The.Movie.2019.2160p.UHD.BluRay.x265.10bit.HDR.TrueHD.7.1.Atmos-GROUP.mkv
Show Name - S01E02E03 - Double Episode [1080p][EAC3 5.1].mkv
[SubGroup] Anime Title - 13v2 (1080p) [ABCD1234].mkv
VIDEO_TS/  |  BDMV/  |  Disc 1/  |  CD2/
```

**Prior art.** `guessit` (Python) is the best filename parser in the field and is what Kyoo
uses — via a fork, which tells you it needed patching. Jellyfin has `Emby.Naming`, inherited
from Emby. Anime needs separate handling: absolute episode numbering, version suffixes,
fansub-group brackets, and providers that season-split the same show differently.

**Positions to take.**

- **Filename parsing is a named, separately-tested component**, not code sprinkled through the
  scanner. It should be a pure function from path to a candidate structure, with a large
  table-driven corpus of real-world names as its test suite.
- **Kyoo's stance is the right one:** "Media not being scanned correctly (even with weird
  names) is considered a bug." No mandated folder layout. Users have decades-old libraries.
- **Always offer manual override, and make it stick.** The user's correction must survive
  rescans, provider changes and file moves, and must be stored as *ours* (see
  [tech-storage.md](tech-storage.md) — user-authored data mirrors to sidecars).
- **Content hashing for identity.** An HN commenter's complaint is worth quoting: "I'll leave
  as soon as someone has a good solution for identifying raw Bluray rips automatically. Why do
  I have to manually name the episodes? We had CDDB last century." For audio this is solved —
  AcoustID/Chromaprint fingerprints — and video has no equivalent public service. A local
  perceptual-hash index (Stash uses `goimagehash`) at least solves *duplicate and re-encode
  detection within one library*, which is the tractable half.

## 2. Providers

### Video

| Service | Covers | Terms (checked 2026-09-17) | Notes |
| --- | --- | --- | --- |
| **TMDB** | Movies, TV, images, people | **Free for non-commercial use** with attribution and logo display. Commercial use requires a written agreement; using it commercially without one is "a material breach". | The default. Best images, good API, community-editable. |
| **TheTVDB** | TV, especially older/non-English series and episode ordering | Tiered by the *licensee's* revenue: **<$50k/yr free with attribution**, $50k–250k **$1,000/yr**, $250k–1M **$10,000/yr**, above that custom. Personal projects can instead use **per-user subscriptions at $11.99/yr**, which supply a PIN. | Jellyfin uses the v4 API and requires a user-supplied key. The per-user-key model is a real UX cost — but it is also a precedent we can follow. |
| **AniDB / AniList / Kitsu** | Anime | Varies; AniDB has strict rate limits and client registration | Required if anime is in scope; absolute numbering and season splits make generic providers wrong. |
| **OMDb, Trakt, Simkl** | Ratings, watch-state sync | Trakt/Simkl are scrobble targets | Kyoo syncs watch state to Simkl. |

**The licensing trap, stated plainly:** TMDB's terms make *commercial* use conditional on a
written agreement. A GPL/AGPL self-hosted server that ships an API key is in a grey area, and
the safe design is **the user supplies their own key** (Jellyfin's approach for TheTVDB) or the
server ships with a clearly non-commercial default and documents the terms. This needs to be
settled before release, not after.

### Audio

| Service | Covers | Terms | Notes |
| --- | --- | --- | --- |
| **MusicBrainz** | The canonical open music database | Open data (CC0 core). Rate limits: **1 request/sec per IP**, 50/sec per user-agent, 300/sec globally. **Self-hosting a mirror lifts the limit.** | Must send a real, identifying user-agent. |
| **AcoustID + Chromaprint** | Audio fingerprint → recording ID | Open source, closely tied to MusicBrainz | This is the CDDB-for-audio that video lacks. Identifies untagged files by their sound. |
| **ListenBrainz / Last.fm** | Scrobbling, similarity | ListenBrainz is the open one | Navidrome supports both; ListenBrainz timeouts are a live issue there. |
| **TheAudioDB / Deezer / Spotify** | Artist art, biographies | Varies, mostly restrictive | Artwork only; treat as optional enrichment. |

Audio is in much better shape than video: an open canonical database, an open fingerprinting
service, and a self-hostable mirror as the escape hatch from rate limits.

### Subtitles

**OpenSubtitles** is the de-facto source, and its terms are restrictive enough to matter:
5 requests/sec per IP; **5 downloads/day with no account, 20/day with a free account, up to
1000/day for VIP**. The legacy `opensubtitles.org` XML-RPC API is being shut down for non-VIP
users.

The practical consequences:

- **Per-user credentials, not a shipped key.** 20 downloads/day is a per-account budget and it
  must belong to the user.
- **Cache aggressively and permanently.** Never re-download a subtitle we already have.
- **Design for multiple providers from the start.** Bazarr — the tool everyone in this
  community actually uses — supports dozens (Addic7ed, Podnapisi, SubDL, Subsource, Titlovi,
  and more), and its community plugin catalogue ships 60+. A single-provider subtitle
  implementation is obsolete on arrival.
- **Bazarr integration is a legitimate v1 answer.** It exists, it works, people already run it.
  A well-documented "point Bazarr at this" path may be better than building our own fetcher.

## 3. Architecture: providers are plugins, and none of them is load-bearing

Every service above can change its terms, its prices, or its availability. The design must
absorb that.

1. **One `MetadataProvider` interface**; every provider implements it; the core knows none of
   them by name.
2. **Providers are ranked and merged per field, with provenance recorded.** Title from TMDB,
   episode ordering from TheTVDB, artwork from fanart — and the database remembers which field
   came from where, so a provider change doesn't silently rewrite a user's library. Jellyfin's
   `Metadata is file based, not library based` (+13) and `Jellyfin fetching metadata from
   TheAudioDb and MusicBrainz when providers are unchecked` (+18) are both provenance failures.
3. **Cache everything, with a TTL and an explicit refresh.** Rate limits make this mandatory,
   and it makes the server work offline.
4. **Providers run out-of-process, sandboxed.** Navidrome's **Extism/WebAssembly** plugin
   system is the demonstrated answer: plugins in Go, Rust, Python or TypeScript, running in a
   WASM sandbox with an explicit host-capability list (HTTP, cache, KV, scheduler, artwork,
   config). Compare Jellyfin's in-process .NET assemblies — a plugin can do anything the server
   can — and Stash's in-process JS interpreter for user-supplied scrapers. Since third-party
   scrapers are exactly the code you least want running unsandboxed, this is not a luxury.
5. **The whole thing must be switchable off.** A library with no internet access must still
   scan, play and remember progress. Local NFO/sidecar reading (`MediaBrowser.XbmcMetadata` in
   Jellyfin) covers users migrating from Kodi and users who curate by hand.

## 4. What this settles

1. **Filename parsing is its own tested component**, with a real-world corpus as its test
   suite, and no mandated folder layout.
2. **Provider keys belong to the user**, with the licensing terms documented. TMDB's
   non-commercial clause and TheTVDB's revenue tiers are settled before release.
3. **Field-level provider ranking with recorded provenance**, so a refresh never silently
   overwrites a user's correction.
4. **Providers are sandboxed WASM plugins (Extism), not in-process code.**
5. **Aggressive permanent caching**; the server works fully offline.
6. **Audio identification is a solved problem** (MusicBrainz + AcoustID, self-hostable mirror);
   video identification is not, and manual correction must therefore be excellent.
7. **Subtitles: multi-provider from day one, user-supplied credentials, and consider
   integrating with Bazarr rather than competing with it.**
