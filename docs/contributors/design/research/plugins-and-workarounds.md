> **ABOUTME:** What people actually extend and hack around in existing media servers — the
> plugin catalogues, the companion applications, and the workarounds that exist only because a
> seam was missing. The requirements list for a plugin architecture, read off the field.

# Plugins and Workarounds

Gathered **2026-09-17**. Star counts and dates are from the GitHub API; code observations are
from local clones.

A plugin catalogue is a **bug report against the core**, sorted by severity. A popular
workaround is a **bug report against the architecture**. This document reads both, because they
are the cheapest available requirements list for our own extension design.

The intent here is to know **what seams to leave room for**, not to build them now. The plugin
API stays private and unstable until a first-party consumer has proven it
([D19](../../decisions/working-notes.md)).

---

## 1. The Jellyfin plugin catalogue, by stars

Jellyfin is the only system in this field with a live, open plugin ecosystem, so it is the
primary evidence. The `jellyfin/` org ships 36 official plugin repos; the third-party ecosystem
is larger and more popular.

**Top plugins across all of GitHub** (`jellyfin-plugin` in name, by stars):

| Stars | Plugin | Category |
| ---: | --- | --- |
| 4,492 | metatube-community/metatube | Metadata (adult) |
| **2,773** | **intro-skipper/intro-skipper** | **Media analysis** |
| 2,141 | cxfksword/metashark | Metadata (Chinese film) |
| 1,458 | 9p4/sso | **Auth** |
| 1,134 | kookxiang/bangumi | Metadata (anime, bgm.tv) |
| 784 | ankenyr/youtube-metadata | Metadata |
| 664 | Libitum/douban | Metadata (Chinese) |
| 659 | danieladov/skin-manager | **Client theming** |
| 659 | cxfksword/danmu | Client rendering (bullet comments) |
| 580 | ThePornDatabase/ThePornDB | Metadata |
| **538** | **IAmParadox27/file-transformation** | **Missing seam — see §2** |
| 514 | IAmParadox27/home-sections | **Client UI** |
| 445 | DirtyRacer1337/PhoenixAdult | Metadata |
| 439 | danieladov/mergeversions | Library curation |
| 363 | jyourstone/smartlists | **Rules engine** |
| 331 | jesseward/lastfm | Scrobbling |
| 329 | arnesacnussem/meilisearch | **Search** |
| 302 | streamyfin/streamyfin | Client companion |
| 292 | KeksBombe/auto-collections | Rules engine |
| 273 | Felitendo/lyrics | Metadata (lyrics) |
| 260 | shemanaev/media-cleaner | Library curation |
| 257 | 0belous/Universal-Plugin-Repo | **Discovery is broken** |

**Official plugins**, for comparison — the most popular is `trakt` at 295, `opensubtitles` at
267, `webhook` at 248, `ldapauth` at 224. **The top third-party plugin outscores the top
official one by 15×.** Two-thirds of the official catalogue is metadata providers
(`tvdb`, `anidb`, `anilist`, `kitsu`, `tvmaze`, `fanart`, `discogs`, `comicvine`,
`openlibrary`, `googlebooks`, `vgmdb`, `imvdb`, `coverartarchive`, `lrclib`).

### What the categories say, ranked by demand

1. **Metadata providers are the bulk of the catalogue by count and by stars.** Writing one must
   be the easiest thing a plugin author can do. Note especially how much of the top of the table
   is **regional**: MetaShark, Bangumi, Douban and Danmu are all Chinese-market, and together
   they outweigh every official provider combined. A provider interface designed only around
   TMDB/TVDB shapes will fail exactly the people most motivated to extend it.
2. **Media analysis is the single most-wanted individual capability** — Intro Skipper, 2,773
   stars. See §3; it is architecturally the most interesting item in this whole document.
3. **Authentication.** A third-party SSO plugin at 1,458 stars, plus official LDAP at 224. This
   is not a plugin category, it is **a bug report about the core**. Navidrome's two
   most-requested issues are LDAP (+148) and SSO (+64). Kyoo shipped OIDC from the start.
4. **Client UI extension** — skin-manager (659), home-sections (514), file-transformation (538),
   danmu (659), the Streamyfin companion (302). Jellyfin's plugin API is **server-side C# only**,
   so all of this is achieved by fighting the architecture. See §2.
5. **Rules and curation** — smartlists (363), auto-collections (292), mergeversions (439),
   media-cleaner (260), official tmdbboxsets (176), plus the external Kometa (3,416) and
   Maintainerr (2,271). Nearly 7,000 stars' worth of "I want to express a rule about my library
   and the server cannot." Another core gap, not a plugin category.
6. **Search** — a Meilisearch plugin at 329 stars is a bug report against a core whose
   `Search results are limited to 24.` issue has been open since 2020.
7. **Scrobbling and sync** — trakt (295), lastfm (331), listenbrainz (218), simkl (50).
8. **Discovery of plugins is itself broken** — the "Universal Plugin Repo" aggregator at 257
   stars exists because every plugin author hosts their own manifest URL.

---

## 2. Four pathologies of Jellyfin's in-process plugin model

Jellyfin plugins are .NET assemblies loaded into the server process and compiled against the
server's own types. Every one of the following is a direct consequence, and all four are
avoidable by construction.

### (a) The ABI breaks every release

Plugins declare a `targetAbi` and are, in one plugin author's words, "**strictly 1 version
compatible**". The user-facing FAQ answer to "I updated Jellyfin and my plugin disappeared" is
*wait for the author*.

The ecosystem has built infrastructure to cope with this. Intro Skipper's install URL is a
**manifest that varies by requesting server version**: "This URL returns a manifest based on the
Jellyfin version used to access it. It will NOT return a manifest when viewed in a browser, as
no Jellyfin version is provided." Its git branches are named after Jellyfin versions (`12.0`).

**This is the single largest tax on the ecosystem**, it is paid every release by every plugin
and every user, and it exists solely because the plugin boundary is a linked assembly reference
rather than a declared contract.

### (b) There is no client seam, so someone built one out of middleware

`jellyfin-plugin-file-transformation` (538 stars) is an HTTP middleware that **rewrites
`jellyfin-web`'s served JavaScript and HTML on the fly** so that other plugins can inject UI.
Its own README states the purpose plainly: modify the web content "without having to modify the
files directly", "non destructive", and allowing "multiple plugins to manipulate the served
data."

Its origin is documented in the README too: it came from **Jellyfin PR #9095, which was not
merged**. The core declined the seam, so the community built a plugin whose job is to patch the
client, and two other popular plugins (`home-sections`, `pages`) are built on top of it. Intro
Skipper needed it as well until Jellyfin 10.10 absorbed media segments natively — and still
needs it for skip-button timeout configuration.

A string-rewriting middleware over another project's minified bundle is a fragile,
security-relevant thing to have load-bearing in a catalogue. It exists because the alternative
was nothing.

### (c) Plugins cannot call each other except by reflection

From the same README: "Due to issues with Jellyfin's plugins being loaded into different load
contexts this cannot be referenced directly. Instead you can use **reflection to invoke the
plugin** directly to register your transformation."

Plugin-to-plugin composition — which the catalogue clearly wants, since three popular plugins
depend on this one — is achieved through runtime reflection across .NET assembly load contexts.

### (d) No sandbox

A Jellyfin plugin can do anything the server process can. Kodi, which has the most mature
add-on ecosystem in this space (C++ core, Python add-ons), reached the same conclusion from
experience — a Kodi developer's summary of the sandboxing attempt was that a working prototype
had to execute Python **in a separate process**, because "there is no sensible way to sandbox
things living in the same memory space." Kodi's security reputation is dominated by third-party
add-ons, not by its core.

Third-party scrapers are the last code anyone should run unsandboxed, and metadata scrapers are
the largest plugin category in the catalogue.

---

## 3. Intro Skipper: the most-wanted plugin is a capability service

Worth its own section, because it is the best available specification for
[D19](../../decisions/working-notes.md)'s tier 3 and it arrived independently.

**What it does:** analyses episode audio with **Chromaprint fingerprinting** to find the
repeated segment across a season, and emits intro/credit timestamps.

**What it needs, read from the source:**

- **Raw media file access** — it reads the actual files, not an API projection.
- **A specific FFmpeg build** — its README pins "Jellyfin's fork of `ffmpeg` ... version
  `7.1.3-1` or newer". Exactly the same dependency our transcoder plugin will have.
- **Long-running batch compute**, run as a scheduled task over the whole library.
- **Its own persistent state with its own schema migrations** — the repo contains an EF
  migrations directory of its own.
- **A way to publish results back into the core's model** so clients can act on them, plus a
  way to put a button in the client UI (which is what forced the File Transformation dependency
  before 10.10).

Compare that list against a metadata provider ("given a title, return JSON") and it is obvious
they are not the same kind of thing. **A plugin system with one tier cannot serve both.**

The practical conclusion for us: **intro/credit detection should be the second tier-3 plugin we
write**, after the transcoder. The two share almost their whole capability list — media bytes,
a known FFmpeg, batch scheduling, private storage, results published to the core — but have
completely different workloads, which is exactly what a tier needs in order to be validated
rather than fitted to a single consumer.

One more lesson from its history: before 10.10 it patched the web UI, and then **the core
absorbed the feature** (media segments became native). That is the healthy end state for a
popular plugin, and it only works if the core can absorb without breaking the plugin's other
users.

---

## 4. The positive models: Immich and Navidrome

Both arrived at **Extism/WebAssembly**, independently, in the last couple of years. That is the
strongest available signal that it is the right tier-1 mechanism.

### Immich — plugins as steps in event-triggered workflows

Read from `packages/plugin-sdk/` and `server/src/services/plugin.service.ts`:

- Plugins are WASM modules that expose **methods**, registered against **workflow triggers**
  (`WorkflowTrigger`, `WorkflowEventMap`), with declared config schemas and **templates** the
  user can instantiate.
- The host-function list is **deliberately tiny**: `searchAlbums`, `createAlbum`,
  `addAssetsToAlbum`, `addAssetsToAlbums`, `httpRequest`, `bulkTagAssets`. Every call carries an
  auth token and returns a discriminated success/error result.
- Plugin methods are matched to triggers by compatibility check (`isMethodCompatible`), so the
  UI can offer only the plugins that fit a given event.

The model is **"a plugin is a step in a user-configured pipeline fired by an event"**, not "a
plugin is arbitrary code with access to the server". That is a much smaller, much safer
contract, and it is the one to copy for tier 1.

### Navidrome — capabilities and host services, declared

Read from `plugins/`: named **capabilities** (`MetadataAgent`, `Scrobbler`, `Lyrics`,
`SonicSimilarity`, `TaskWorker`, `Lifecycle`, `SchedulerCallback`, `WebSocketCallback`) and an
explicit list of **host services** a plugin may use (HTTP, Scheduler, Cache, KVStore, Storage,
Task, WebSocket, Library, Matcher, Artwork, SubsonicAPI, Config). Plugins may be written in Go,
Rust, Python or TypeScript via Extism's PDKs.

Two ideas worth taking: **capability names are part of the contract** (a plugin declares what
kind of thing it is, so the host knows when to call it), and **host services are enumerated**
(so the security surface is a list someone can read).

---

## 5. Plex killed its plugin system — and said why

Plex sunset plugins/channels in 2018. The stated reasons, from its own announcement as reported
at the time:

1. **Fewer than 2% of users used them.**
2. **The protocol was outdated and hard to support**; keeping it meant rebuilding it.
3. It no longer fitted the business as Plex moved toward its own streaming catalogue.

Three lessons, and they cut in different directions:

- **A plugin system nobody important uses is pure maintenance cost, and will eventually be
  killed.** This is the strongest argument *for*
  [D19](../../decisions/working-notes.md): if our own transcoder is a plugin, the API can never
  be in the "<2% usage" position, because the product depends on it.
- **The plugin boundary is a compatibility commitment.** Plex's was "an outdated protocol that's
  difficult to support" seven years in. Ours will be too, unless it is small and versioned
  from the start.
- **When the vendor's interests diverge from the users', the extension point is what gets cut
  first.** Plex's users lost their plugins to a strategy change. That is a governance argument
  for keeping our extension points in the open and building on them ourselves.

Emby is the other data point: its plugin ecosystem shares Jellyfin's lineage (several plugins
in the table above advertise "Jellyfin/Emby" compatibility), but with a closed core the
community cannot route around a missing seam the way `file-transformation` did.

---

## 6. The real extension mechanism: companion applications over HTTP

This is the largest finding in the document, and it is not about plugins at all.

| Stars | Project | What it works around |
| ---: | --- | --- |
| 16,293 | Sonarr | The server does not acquire media |
| 14,380 | Radarr | ″ |
| 12,615 | Seerr (Jellyseerr) | No request/discovery workflow for household members |
| 7,227 | Prowlarr | Indexer management for the above |
| 6,585 | Tautulli | Plex has no statistics or monitoring |
| 5,660 | Lidarr | ″ (music) |
| 4,976 | Overseerr | ″ (Plex-side predecessor) |
| 4,279 | Bazarr | Subtitle acquisition is inadequate in both Plex and Jellyfin |
| 3,416 | Kometa | Cannot express rules about collections/metadata |
| 2,271 | Maintainerr | Cannot express rules about what to remove |
| 1,073 | rffmpeg | **No seam for remote transcoding** |
| 752 | Janitorr | Disk-space policy across the whole stack |

**Roughly 80,000 stars of external applications**, versus roughly 20,000 across the entire
Jellyfin plugin catalogue. The community's real extension mechanism is *a separate program
talking to the server's HTTP API*, and it always has been.

Two conclusions follow, and they are the most actionable things here:

1. **The public HTTP API is the most important extension point in the product**, by an order of
   magnitude, and it matters far more than the plugin API. This validates
   [D15](../../decisions/working-notes.md)'s tier 2 and [D18](../../decisions/working-notes.md)'s
   adapter, and it raises the stakes on generating OpenAPI from the code rather than
   maintaining it by hand.
2. **`rffmpeg` is the outlier that proves the rule.** Every other entry in that table could be
   built because Plex/Jellyfin exposed an adequate HTTP API. Remote transcoding could not, so it
   was implemented by **impersonating the `ffmpeg` binary and SSHing to another host** — by a
   Jellyfin founder, 1,073 stars, still maintained in 2025. When the API has no seam for
   something people want, they do not give up; they do something worse.

---

## 7. What nobody could work around

Telling by its absence. There is no plugin and no companion app for:

- **Remote access for non-technical users.** The answer is always a VPN
  ([D14](../../decisions/working-notes.md)).
- **Granular per-user sharing of part of a library.** Users symlink subsets into fake libraries
  instead.
- **Playback negotiation.** Nobody can fix a wrong transcode decision from outside, which is
  why the trackers are full of requests for a manual override rather than plugins that do it.

These are exactly the three open problems already recorded in
[overview.md](../overview.md). That the community could not route around them is
confirmation that they are architectural, not missing features.

---

## 8. Requirements this produces for our plugin architecture

Read directly off the above. These are seams to **leave room for**, not to build now.

**The tier-1 contract (sandboxed extensions):**

1. **A declared, versioned interface — never a link against server internals.** The per-release
   ABI break is the ecosystem's largest tax and it is entirely self-inflicted. WASM plus a
   semver'd host interface makes it impossible by construction.
2. **Capability names are part of the contract** (Navidrome), and **plugins are steps triggered
   by events** (Immich). Both, not either.
3. **Enumerate host services in a list a human can read**, and keep it short. Immich's is six
   functions.
4. **Metadata providers must be the easiest plugin to write**, and the interface must not assume
   TMDB/TVDB shapes — the most popular providers in the field are regional ones with different
   identity models, different title semantics and different episode ordering.
5. **Plugin-to-plugin composition needs a designed answer**, or reflection becomes it.

**The tier-3 contract (capability services):** specified by the transcoder and intro-detection
together — media bytes, a pinned FFmpeg, batch job scheduling, private storage with its own
migrations, results published into the core's model, and the option to run on another machine.

**A client extension seam, designed in.** Home-screen sections, themes, and custom pages are
between them ~1,700 stars of demand, and the absence of a seam produced a middleware that
rewrites the web client's served JavaScript. If we do not design this, someone will build that.

**Things the catalogue says belong in the core, not in plugins:**

- **Auth: OIDC/OAuth and LDAP** (1,682 stars of plugin saying so). Already
  [D9's](../../decisions/working-notes.md) framework stance; this raises it to a v1 requirement.
- **A rules engine** for smart collections, smart playlists and retention policy (~7,000 stars
  across plugins and companion apps). This is a core data-model feature and should inform
  `library-model.md`.
- **Good search** (a Meilisearch plugin is a bug report). Already decided — **SQLite FTS5**
  ([D31](../../decisions/working-notes.md)), with fuzzy matching in Go.
- **Version merging** for multiple copies of one title (mergeversions, 439). Already planned as
  `Title → Edition → MediaFile`; this confirms it matters.
- **Webhooks/events** as a first-class core feature, since it is both a popular official plugin
  and the foundation of Immich's whole plugin model.

**One catalogue, first-party, with signing.** Fragmented per-author manifest URLs produced an
aggregator plugin. Plugin distribution is part of the design, not an afterthought.
