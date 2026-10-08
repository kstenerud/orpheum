> **ABOUTME:** Product-altitude overview — the goal, who it is for, the competitive opening,
> the architecture that follows from the research, and what is deliberately not being built.
> The orientation a reader wants before the per-topic design docs.

# Overview

**Status: pre-implementation.** Nothing has been built. This document states the intent that
the research supports.

## Goal

A personal media server for **video** that plays your files, on your hardware, on your TV,
without a company between you and them — and that is pleasant enough that the rest of the
household stops asking for Plex.

Music follows once video is good, cheaply, by speaking an API that already has clients. Photos
is out of scope (see [D1](../decisions/working-notes.md#d1--scope-video-first-audio-second-photos-probably-never)).

## Who it is for

The person leaving Plex. They are numerous, motivated, and currently landing on Jellyfin and
finding it *nearly* good enough. Concretely they have: a few thousand files they ripped or
acquired over fifteen years, in every container and codec ever made; a TV; a household of
people who are not going to read documentation; and relatives they used to share with.

## The opening

Both incumbents have a specific, durable weakness, and they are different weaknesses:

- **Plex's problem is commercial *and* — read first-hand — self-inflicted.** The software is
  good. In 2025–26 it raised prices sharply (lifetime to **$749.99** from 2026-07-01), moved
  previously-free remote streaming behind the paywall platform by platform, and kept pushing
  its own ad-supported catalogue into a UI people installed to watch their own files. Its users
  describe waiting for the other shoe to drop.
  **But on Plex's own forum the loudest topic of the year is not the price** — it is
  *Vote to roll back to Plex Classic!* at 1,124 likes, against 938 for the lifetime-pass
  thread, with a second cluster about a client update that broke Fire TV and could not be
  rolled back. Neither of those needs a business model to fix, which is why
  [D108](../decisions/working-notes.md) simply promises the opposite: a layout change is
  opt-in, and every client build stays directly installable.
  **Measured across the whole top fifty, interface complaints are 51% of the likes**
  ([D138](../decisions/working-notes.md)) — and they are not about looks. They are about
  *"one click instead of 30"* and a selection that *"lags by an entire second"*. The loudest
  thing in the field is **a community replacement client**: PM4K/PlexMod for Kodi, **5,312
  likes and 6,494 posts**, whose feature list opens *"Maintained. Very stable."* and continues
  into direct play, codec breadth and correct stream identification — which is this project's
  thesis, written by someone else, as a mod.
- **Jellyfin's problem is technical and structural.** Subtitles have been broken in
  well-documented ways since 2020. The playback-negotiation model produces wrong decisions in
  both directions. The client experience is described by its own users as "hit or miss", and
  the clients people actually recommend are third-party. A long-deferred database rewrite cost
  it a year of upgrade breakage. It is not a project that can turn quickly.

**The opening is the gap between them:** Plex's polish without Plex's business model, built on
an architecture that doesn't inherit Jellyfin's 2008-era assumptions.

**And a warning about what that opening is not** ([D139](../decisions/working-notes.md)).
Measured across 2,205 App Store reviews of the commercial services, their leading complaint is
**advertising** — 36% of Hulu's reviews, 34% of Disney+'s. That is a failure we cannot commit,
which means **we are not competing against their weaknesses**: the thing that makes them hated
is unavailable to us, so we get compared on navigation, resume and reliability, where they are
strong and well funded. "No ads" is a floor, not a lead.

## What we do differently

Six choices, each traceable to a specific failure in the research:

1. **The client decides what to play, from a manifest.** No server-side device-capability
   database. The server advertises every rendition it can produce, including the untouched
   original; the client — the only party that can ask its own decoder what it will actually
   accept, and whether it will be *smooth* and *power-efficient* — chooses. This deletes the
   largest bug class in the field and makes every new client cheaper to write.
   ([D2](../decisions/working-notes.md), [tech-playback.md](research/tech-playback.md))
2. **Direct play is the default and always on offer.** Transcoding is the exception, it is
   explicable in the UI, and the user can override the decision in both directions.
   ([D4](../decisions/working-notes.md))
3. **Subtitles are a subsystem.** Extracted at scan time with their fonts, served as HLS
   renditions, rendered with libass on every client. This is the most-complained-about part of
   every competitor and the cheapest place to be visibly better.
   ([D11](../decisions/working-notes.md))
4. **Your files stay yours.** Media read-only by default. The database is a rebuildable index,
   not an authority. Your corrections are written back to sidecars where you grant a writable
   path — a later feature, not Cut A's (F29, [D178](../decisions/working-notes.md)) — and
   per-user state deliberately is not ([D98](../decisions/working-notes.md)). No telemetry, no account,
   no storefront — as a stated promise. ([D7](../decisions/working-notes.md),
   [D13](../decisions/working-notes.md))
5. **One excellent TV client beats five mediocre ones.** The project is judged on its TV app.
   ([D10](../decisions/working-notes.md))
6. **Watch Together, because everyone else deleted theirs** — and Plex's own forum has a
   **350-post thread** asking for it back. Disney+, Amazon, Sling and even
   Plex withdrew their watch-party features — Plex's note says it *"won't be available for most
   platforms as they get the new experience"*, which names the cause: it lived in every client.
   Teleparty has ten million users because the platforms will not do this. We keep the
   client-specific surface to six methods so a client rewrite cannot quietly kill it
   ([D62](../decisions/working-notes.md)) — and **a session link is the invitation**, with no
   account required at the other end, because that is the whole point
   ([D117](../decisions/working-notes.md)); a participant who happens to be signed in also has
   their viewing recorded as their own ([D191](../decisions/working-notes.md)). The host owns
   what plays and may change it to anything in their library mid-session; whether anyone else
   may touch the transport is one setting ([D121](../decisions/working-notes.md),
   [D122](../decisions/working-notes.md)). A leaked link is survivable rather than prevented
   ([D119](../decisions/working-notes.md)): a **cap on participants** bounds the flood before
   anyone notices, and because **a link admits while a *membership* persists**, the host can
   rotate the link without the people already watching noticing anything. A **voice companion**
   — a phone doing microphone duty while the television plays — is a second device on the same
   membership rather than a second person ([D120](../decisions/working-notes.md)), and unequal
   links need no group compromise because the group shares a *position*, not a stream
   ([D123](../decisions/working-notes.md)).

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│ Clients — TypeScript SDK generated from OpenAPI, React-free      │
│   ├ web + cast : React + TypeScript + Vite; hls.js + JASSUB      │
│   ├ Android/TV : Media3 ‖ libmpv; the UI framework is chosen by  │
│   │              a prototype when TV work starts (D193)          │
│   └ Apple      : AVPlayer ‖ libmpv  (later)                      │
│ Each probes its own decoder and picks a rendition. (D2)          │
└──────────────────────────────┬───────────────────────────────────┘
                               │  HTTPS under one public name (DNS-01):
                               │  REST + OpenAPI, HLS playlists,
                               │  fMP4 segments, ranged GETs
┌──────────────────────────────┴───────────────────────────────────┐
│ Server — one Go binary (D9); never opens a media file (D193)     │
│                                                                  │
│ api/        REST handlers; OpenAPI generated from code           │
│ library/    ingests node reports → identify → enrich             │
│ stream/     rendition list (video ‖ audio ‖ subs axes); picks    │
│             direct play or a prepared rendition, authenticates,  │
│             and relays the node's bytes — no FFmpeg here (D26)   │
│ jobs/       typed, persisted, inspectable queue: prepare,        │
│             fetch, scan                                          │
│ voice/      TURN embedded (pion/turn) for Watch Together (D68)   │
│ store/      sqlc-generated typed SQL (D6)                        │
└───┬──────────────┬───────────────────────────────────────────────┘
    │              │
    │       ┌──────┴───────┐
    │       │ SQLite (WAL) │
    │       │ (D31)        │
    │       └──────────────┘
    │
    │  MEDIA NODES (D33) — same binary, `node` mode, one per machine
    │  holding files; media (ro) + cache/ (purge). Every media read and
    │  every FFmpeg run: scan, probe, extract subtitles, thumbnail, and
    │  PREPARE — remux, audio, and video encode where the node has an
    │  encoder (a capability it advertises). Each FFmpeg run is
    │  confined on its own (D43, D193). Never touch the database.
    ├─ node @ core box (supervised child over a Unix socket; the default)
    ├─ node @ NAS / GPU box / elsewhere  (later: dial-out TCP)
    │
    │  plugins
    ├─ jellyfin-adapter (tier 2, inbound — borrows other people's clients)
    └─ metadata providers (tier 1, WASM — deferred; until a third party
       asks, TMDB and OpenSubtitles are Go packages behind the interface)

    RELAY (D71) — a `relay` mode of the same binary, deferred past Cut A:
    reachability is a port-forward, then IPv6, then a standard tunnel.
```

Notes on the shape:

- **One binary, one file, no second service, no message broker.** Household scale does not
  justify any of it, and "boring, established tech" is a stated constraint. The database is
  SQLite in WAL mode ([D31](../decisions/working-notes.md)), so installation is a binary and
  backup is one SQL statement, `VACUUM INTO` — ~~a file copy~~, which on a live WAL database
  can produce an empty one ([D77](../decisions/working-notes.md)).
- **FFmpeg is a subprocess, not a linked library.** Every project in the survey that touches
  FFmpeg drives it as a process; cgo bindings to libav\* are a build-complexity liability. We
  ship a known-good build rather than accepting the distro's — a pinned jellyfin-ffmpeg
  portable build, bundled and checked by hash ([D193](../decisions/working-notes.md)). Jellyfin
  maintains an entire FFmpeg fork, which is evidence of what owning our own would cost, so our
  own hardened build is deferred.
- **The core never touches a media filesystem** ([D33](../decisions/working-notes.md)), **and
  never parses a media or image file** ([D26](../decisions/working-notes.md),
  [D99](../decisions/working-notes.md)). Scanning, probing, extraction and image derivation all
  happen in media nodes, and so does every FFmpeg run, playback included: for direct play the
  core authenticates the request and relays the node's byte range, and for anything else it
  relays what the node's prepare job wrote ([D193](../decisions/working-notes.md)). The core's
  whole input surface is HTTP, its own SQLite file, and node reports it validates at the
  boundary. A poster fetched from TMDB is bytes in flight, handed
  to the jail — TMDB is community-editable, so it is attacker-supplied like anything else.
- **The node's work is ten passes now, and that is a first-class fact**
  ([D102](../decisions/working-notes.md)). Enumerate · probe · content hash · extract
  subtitles · subset fonts · thumbnails, trickplay and poster derivation · measure loudness ·
  detect segments · render subtitle overlays · build the speech map
  ([D148](../decisions/working-notes.md)) — each added separately, each individually cheap,
  and nobody had added them up. (The same ten, in the same decomposition, in
  [scanner.md](plans/scanner.md).) They are named, individually deferrable per library,
  separately reported and restartable, because the alternative on a Celeron NAS is a first
  scan that takes a week.
- **The core encodes nothing; "prepare" is the one path besides direct play**
  ([D193](../decisions/working-notes.md)). ~~The core needs no video encoder — but it does
  encode audio.~~ One node job per File and profile runs one FFmpeg command that writes an HLS
  rendition into the node's cache — video copied (`remux`) or encoded (`h264`), audio
  converted with a dialogue-boost variant, text subtitles extracted — as a growing `EVENT`
  playlist, so playback starts once the first segments exist. The encoder is a **node
  capability** the node advertises (`aac`, `h264_qsv`, `h264_nvenc`, `x264`), so a GPU machine
  becomes a second node rather than a plugin. Every GPL trigger in FFmpeg is still a video
  encoder, and the whole audio menu is native LGPL
  ([D19](../decisions/working-notes.md), [D22](../decisions/working-notes.md)). Live
  segment-addressed transcoding waits for a measurement that says first-play latency hurts.
- **"Plugin" means two things for now**, chosen by shape: an inbound protocol adapter that
  consumes our API (tier 2), and — deferred until a third-party provider is asked for — a
  sandboxed WASM extension the core calls into (tier 1). The third tier, an outbound capability
  service, is retired into the node ([D193](../decisions/working-notes.md)).
- **The public HTTP API matters more than the plugin API**, by roughly an order of magnitude.
  The community's real extension mechanism in this field is a separate program over HTTP — some
  80,000 stars of companion apps (Sonarr, Radarr, Seerr, Tautulli, Bazarr, Kometa …) against
  ~20,000 for the whole Jellyfin plugin catalogue. OpenAPI is generated from the code and the
  build is gated on it being current. ([D20](../decisions/working-notes.md))
- **A bug report is a privacy problem wearing a diagnostics costume**
  ([D74](../decisions/working-notes.md)). Our logs are full of film titles, paths with real
  names and household usernames, and unlike an API key a library cannot be rotated after
  disclosure. So the shareable report **pseudonymises rather than redacts** — stable per-report
  tags keep correlation while dropping identity — and keeps the shape that actually diagnoses
  things: codecs, profiles, track counts, dispositions.
- **`cache/` is always safe to delete.** Nothing irreplaceable lives outside the database and
  the media files. It is **two caches, one per side** ([D128](../decisions/working-notes.md)):
  the node makes the derived artefacts and the core serves them — relays them, for anything the
  node keeps ([D193](../decisions/working-notes.md)) — and over a reference library
  that is **42 GB of subtitle overlays and 5.7 GB of trickplay** against 51 MB for a scan
  report. Small and hot goes to the core, large and cold stays on the node and is relayed —
  or, from a remote node later, redirected to, which is what Netflix's control plane does,
  steering a client to an appliance by URL.
- **The node dials out; the core never dials in** ([D127](../decisions/working-notes.md)). A
  Unix socket in the default install and an outbound HTTP connection from a remote node later:
  one transport, HTTP+JSONL. ~~The file exchange directory as the fallback~~ is deleted
  ([D193](../decisions/working-notes.md)). The reason is not the
  firewall: [D107](../decisions/working-notes.md) found the dangerous direction is core→node,
  because that channel carries commands and deletion, and a node that only ever initiates has
  no inbound surface to spoof.
- **Play state is a log, not a cell** ([D81](../decisions/working-notes.md)). A guarded position
  per user and edition, with a monotonic sequence so a stale device cannot rewind a newer one,
  plus an append-only event log. Jellyfin keeps one mutable row and its tracker carries *"Most
  movies now appear to be 75% watched"* and *"Media playback randomly starts over again or jumps
  back — STILL"*; Tautulli (6,590★) exists because the history is nowhere.
- **We are the ones doing the downmix, so the quiet dialogue is ours**
  ([D83](../decisions/working-notes.md)). Netflix normalises its catalogue at ingest — -27 LKFS
  dialog-gated, -2 dBTP. We cannot rewrite files, so the node measures loudness at scan and gain
  is applied at playback, which is ReplayGain's answer to the same constraint. Downmix
  gets one default and a dialogue control rather than Jellyfin's five algorithms, each of which
  its own documentation apologises for.
- **Captions have a statutory control set and we already serve it by accident**
  ([D90](../decisions/working-notes.md)). 47 CFR §79.103 requires the eight CEA-708 fonts,
  50–200% sizing, eight-colour text and background, three opacities and five edge attributes;
  [D55](../decisions/working-notes.md) chose exactly that font axis for typographic reasons.
  The consequence nobody had drawn: the server-rendered overlay forecloses every one of those
  controls, so it is never the only offer for a caption track.
- **An episode has a position per ordering, and a trailer is not a cut**
  ([D96](../decisions/working-notes.md), [D97](../decisions/working-notes.md)). Both are the
  four-level correction repeating in other organs. Jellyfin's episode carries one
  `IndexNumber` plus three nullable escape hatches for the specials that do not fit and a bare
  string `Series.DisplayOrder` compared against magic values two different ways; its extras are
  full items with an owner pointer, kept out of listings by convention. Aired, DVD and absolute
  are orderings of one set of episodes; an extra is a typed relation to a shorter work.
- **Four levels, because three is where everyone else's version bugs come from**
  ([D79](../decisions/working-notes.md)). Title → Edition → Version → File, matching the
  industry's EIDR hierarchy (Abstraction → Edit → Manifestation), with Collections orthogonal
  and covering series, seasons, box sets and smart collections alike. Jellyfin's `Video` is Edit,
  Manifestation and Item at once, which is why it needs four overlapping mechanisms —
  `PrimaryVersionId`, `LinkedAlternateVersions`, `LocalAlternateVersions`, `AdditionalParts` —
  two of them bare arrays of path strings.
- **Metadata is multi-language by schema, not by configuration**
  ([D45](../decisions/working-notes.md)). Every title has a language-invariant row and a set of
  translation rows — including per-language artwork, because posters have title text in the
  pixels — and the request's `Accept-Language` chooses among them. Jellyfin resolves one
  language per library at *ingest*, so two people in one household cannot each have their own;
  the request for a fallback chain has been open there since 2020. Sort order is a materialised
  CLDR sort key per locale ([D47](../decisions/working-notes.md)) rather than a transliteration
  to ASCII, so Swedish Å Ä Ö sort after Z instead of being folded to the front.
- **A scan is a stream you can steer, not a batch that finishes**
  ([D118](../decisions/working-notes.md); in Cut A a FIFO with "play this now" at the front,
  [D193](../decisions/working-notes.md)). Passes are ordered so an item becomes *playable*
  first — measured at **17 minutes to a browsable 3,500-item library**, against 9.2 single-core
  days for trickplay alone, because the node's passes turn out to be two populations a
  thousand-fold apart split by whether a pass decodes video
  ([D114](../decisions/working-notes.md)). The user can push one title to the front mid-scan
  and have it playable in seconds, and the core reads the node's report **while it is still
  being written**, so items appear as they are found. A progress bar is a promise about the
  future; items appearing is evidence about the present.
- **A node publishes a snapshot plus a journal, and interprets nothing**
  ([D59](../decisions/working-notes.md)). It enumerates its roots, looks inside the files
  because people misname them, and reports what it saw — filenames verbatim, probe output,
  what it could not read. Parsing `Show.S01E02.mkv` into a series and episode happens in the
  **core**, over stored observations, so that improving the parser is a re-interpretation pass
  that opens no files and needs no node to be switched on. The periodic snapshot is not only
  compaction: a watcher that fails silently is a watcher whose journal cannot tell you so.
  Legibility — "I can't tell what the scanner did" is a real, recurring complaint — comes from
  the node's status endpoint and report stream and its error and skip records.

## What we are deliberately not building

| Not building | Why |
| --- | --- |
| Household verification, device counts, IP entitlement checks | [D76](../decisions/working-notes.md). Netflix needs it because it licenses content per household; we license nothing. "Your phone must occasionally connect to the living-room Wi-Fi" is a requirement imposed on someone's life, and a large part of what our users are leaving. **Not to be confused with parental controls**, which we *are* building ([D84](../decisions/working-notes.md)): a kids profile is a grantee plus a predicate, a fifth binding of the rules engine. Netflix's apparatus enforces a licence; a maturity ceiling protects a child, and only the first is someone else's business. |
| DRM | The user owns the files. Skipping it removes key servers, licence proxies and platform certification — the main reason a small team can compete here. |
| A device-profile database | [D2](../decisions/working-notes.md), [D21](../decisions/working-notes.md). It is the incumbents' largest bug source. Our menu is a closed enumeration and must never become a grammar of conditions. |
| Dolby Vision / Atmos *output* | Passthrough only. We direct-play them where the client copes and fall back to a menu profile otherwise; we never encode into them. |
| Photos | [D1](../decisions/working-notes.md). Immich exists, is AGPL, and is moving faster than we could. |
| A Netflix-style encoding ladder | Bandwidth is free on a LAN — but **not over the relay**, which is the paid feature. [D86](../decisions/working-notes.md): no ladder, and the client re-picks on its own buffer level, which is what Netflix's own SIGCOMM'14 study found beats bandwidth estimation by 10–20% on rebuffer rate. The consequence is a schedule change, not an architecture one: bandwidth transcoding moves onto remote access's critical path. |
| A detector for intro and credit boundaries, at launch | [D82](../decisions/working-notes.md). The **model** is due with the schema — typed segments at the Version, because a timecode indexes an encoding — and the chapter-derived provider is nearly free. Netflix's fingerprint-and-human-review detector is not something we can build, and `intro-skipper` (2,780★ plus 968★ archived) shows it is plugin-shaped anyway. |
| Microservices, Kafka, Elasticsearch, Redis | Household scale. SQLite FTS5 covers search — two tokenisers, because no single one handles Latin, CJK and Thai ([D48](../decisions/working-notes.md)) — Go covers fuzzy matching, and job wakeups are a channel in one process ([D31](../decisions/working-notes.md)). |
| A Roku client | BrightScript shares no code with anything and Roku cannot render bitmap subtitles at all. |
| FFmpeg in the core process | [D26](../decisions/working-notes.md). Media is attacker-supplied data and FFmpeg will eventually be compromised; all of it runs in the media node, **each invocation confined on its own** by a re-exec shim — argument hardening, read-only mounts, a seccomp denylist, and Landlock where the kernel has it ([D43](../decisions/working-notes.md), [D193](../decisions/working-notes.md)) — so the core never parses hostile bytes. |
| **An image decoder in the core process either** | [D99](../decisions/working-notes.md). The same rule, generalised: TMDB is community-editable, so a poster is attacker-supplied — and unlike a film it arrives unprompted during a background refresh. 39 CISA KEV entries touch media, image or font parsing. Derivation happens in the node's jail; the core moves bytes it never decodes. |
| In-process plugins | [D12](../decisions/working-notes.md). Third-party scrapers are the last code you want unsandboxed. Protocol surfaces get a second tier — out-of-process adapters ([D15](../decisions/working-notes.md)). |
| Any encoder in the core | [D17](../decisions/working-notes.md), [D19](../decisions/working-notes.md), [D22](../decisions/working-notes.md). ~~It is a plugin on the public API~~ It is a **node capability** ([D193](../decisions/working-notes.md)): the node that holds the file, or a GPU node later, runs the encode, which still gives remote transcoding without a plugin API. Audio encoding runs in the node's prepare job too. |
| **Deleting media**, for now | [D144](../decisions/working-notes.md). Measured: **0 of 120** top-reacted Jellyfin issues and **0 of 50** Plex forum topics mention it, and the one relevant remark in 2,205 App Store reviews is that a server *failed to notice* a deletion made elsewhere. [D89](../decisions/working-notes.md) came from an audit, not from demand. The policy is an **enforced absence** — no code path removes a file under a media root — and the acquisition layer (Radarr, Sonarr) already owns upgrade-and-replace. [D143](../decisions/working-notes.md)'s trash design stays recorded as the precondition if it is ever asked for. |
| **Request flows** (Overseerr/Jellyseerr-shaped) | [D140](../decisions/working-notes.md). Companion-app territory, and [D20](../decisions/working-notes.md) already makes the public HTTP API the seam that makes them possible. Not ours to build; ours to be good enough to build against. |
| **Voice assistants** | [D140](../decisions/working-notes.md). Plex dropping its Alexa skill drew 66 likes, so the want is real and small. Declined — and the cast path already gives away the useful half, since Cast carries voice-originated load requests. |
| A separate **desktop application** | [D140](../decisions/working-notes.md). The web client is the desktop client. Worth stating because *Are the Desktop Apps Dead?* is 186 likes on Plex's forum, and an unstated answer reads as neglect. |
| **DLNA / UPnP** | [D149](../decisions/working-notes.md). Demand is 11 reactions across a 932-issue sweep, and **Jellyfin moved DLNA out of its core in 10.9** — the resulting plugin has **87★ against 84 open issues**, nearly as many open bugs as stars. Cast ([D135](../decisions/working-notes.md)) covers the useful half. |
| **3D / stereoscopic** | [D149](../decisions/working-notes.md). One issue, 23 reactions, and a whole extra rendition axis. |
| **Sonos / multi-room audio** | [D149](../decisions/working-notes.md). Plex has it, nobody asks us, and audio is already second ([D1](../decisions/working-notes.md)). |
| **Games, sports, music videos as library types** | [D149](../decisions/working-notes.md). Emby shipped a Games library and Jellyfin did not carry it; Plex built an arcade and closed it on 31 March 2022. |
| **Poster overlays baked into artwork** | [D149](../decisions/working-notes.md). Kometa's **24 overlay families** are its most visible feature and it implements them by rewriting the poster — destructive to a file we do not own ([principles §2](../principles/README.md)). Badges, if ever, are a client render over an untouched image. |
| **A content-source plugin tier** (channels: news, web video, streaming services) | [D150](../decisions/working-notes.md). **Plex killed exactly this in 2018 at "less than 2 percent of people using Plugins."** Our plugin tiers are metadata providers and protocol adapters, which is a different product; this one is declined with the number attached. |
| Audiobooks, comics, live TV/DVR | Each is a different product wearing the same clothes. Audiobookshelf's 1,191 open issues are the evidence for the cost. The demand is real too and we should say so rather than imply otherwise: *Support for audiobooks* on Plex's forum is **1,156 posts**, the largest thread found anywhere in this research. Declined with the number visible. |

## The open problems

Honest statement of what the research did **not** settle:

1. **Whether to operate a hosted service at all.** Narrowed three times, and now shaped.
   [D72](../decisions/working-notes.md) settles *how* it would work if it exists: **no free
   tier** — a free tier whose marginal cost scales with usage is precisely the trap that forced
   Plex to claw remote access back from its own users, and here the free path is not crippled
   but better — and **no data, no accounts**, so a relay box is cattle and leaving takes an
   afternoon. What remains genuinely open is only whether to run it at all. Narrowed three
   times now.
   [D71](../decisions/working-notes.md) made the relay **a mode of our own binary that anyone
   can run on a €5 VPS** (deferred past Cut A, which uses a port-forward, IPv6 or a standard
   tunnel — [D193](../decisions/working-notes.md)) — which carries ~2,200 film-hours a month at 10 Mbps uncapped, against
   Plex's reported 2 Mbps paid relay cap. So the hosted tier's remaining justification is
   convenience for people who will not rent a VPS: a real market, and a far smaller claim than
   the one this project started with. Narrowed, and then narrowed again.
   Watch Together's voice chat was once thought to need a hosted **TURN relay**; [D68](../decisions/working-notes.md) found it does not, because both
   participants are already connected to the media server by construction and it can relay the
   voice itself — at one endpoint rather than in a datacentre, so with almost no latency cost.
   It does so as a standard TURN server embedded in our binary (`pion/turn`), with browser
   WebRTC picking the path and no upgrade ladder ([D193](../decisions/working-notes.md)).
   So the justification for a paid tier is **remote access itself** and nothing else, which is a
   cleaner claim than the one it replaced. What remains is below, unchanged. Researched
   ([remote-access.md](research/remote-access.md), [D25](../decisions/working-notes.md)): the
   topology is settled — there is no discoverability without a signpost, and the TLS-certificate
   problem needs one even on the LAN — and a three-tier shape is proposed. What remains is not
   technical. A hosted tier implies a company, uptime obligations and an entity that can be
   pressured; declining it is coherent, at the cost of leaving the headline use case unsolved.
2. **Granular sharing** — the *identity* half is the same question as (1), because sharing needs
   a stable identifier for the other person and that has to live somewhere. It is *why* Plex has
   accounts. But the **access-control half is now settled**
   ([D76](../decisions/working-notes.md)): Plex's mechanism is a *filter*, so **a share is a
   grantee plus a predicate** and it becomes a fourth binding of the rules engine rather than a
   subsystem. Nobody in the open field has solved this — Immich's sharing issue has 1,020
   reactions under a feature freeze, PhotoPrism's 844 over seven years, Jellyfin users symlink
   subsets into fake libraries — and it turns out to be nearly free once the predicate language
   exists.
3. **The client cold-start.** A new server has no clients. Settled in shape by
   [D18](../decisions/working-notes.md): a Jellyfin-compatible **out-of-process adapter** makes
   Findroid, Swiftfin, Infuse and Streamyfin work without constraining our model — and it
   quarantines device-profile logic permanently outside the core. What remains open is whether
   it is good enough to delay the first-party TV client.
4. **Metadata provider licensing.** TMDB's terms condition commercial use on a written
   agreement; TheTVDB tiers by the licensee's revenue. Must be answered before a public release.
   **A second licensing question was closed on 2026-09-19** and is recorded here because it was
   an unnoticed one-way door: a whole-project GPLv3 would have meant **no Apple client at all**,
   for a project that says it is judged on its TV app. VLC was pulled from the App Store in
   January 2011 and returned in July 2013 under MPL-2.0; Swiftfin and Streamyfin are both
   MPL-2.0 against a GPL Jellyfin server. So: **GPLv3 for the server, node and relay; MPL-2.0
   for the clients and the shared SDK** ([D80](../decisions/working-notes.md)) — and it has to
   be set before the first client commit, because relicensing needs every contributor.
5. **What a client's own probe is worth.** [D2](../decisions/working-notes.md) is right that
   the client is the only party that can ask its own decoder — and the answer is often wrong,
   which the large operators absorb with device certification programmes we cannot have.
   [D92](../decisions/working-notes.md) settles the shape: a fallback ladder the client walks
   on failure, plus a memory it keeps **about itself**, reported to the core so a household can
   see the pattern. The server still never builds a device-capability database
   ([D21](../decisions/working-notes.md)). What is open is how often it happens, which is a
   Phase 0 number.
6. **The real direct-play rate.** [D17](../decisions/working-notes.md) bets that direct play
   covers the common case. That rests on a number **nobody has published** — Tautulli exposes it
   per-server, but no aggregate survey exists. Measuring it is days of work and it re-orders the
   roadmap in either direction, which is why it is now Phase 0.
7. **Codec patents.** Separate from copyright licence and not resolved by being open source. The
   field's uniform practice is to ship H.264/HEVC anyway; direct-play-first reduces a default
   install's exposure to nearly nothing because it never encodes.
   ([D16](../decisions/working-notes.md))
