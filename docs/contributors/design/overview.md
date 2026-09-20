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
  *Vote to roll back to Plex Classic!* at 1,124 likes, against 937 for the lifetime-pass
  thread, with a second cluster about a client update that broke Fire TV and could not be
  rolled back. Neither of those needs a business model to fix, which is why
  [D108](../decisions/working-notes.md) simply promises the opposite: a layout change is
  opt-in, and every client build stays directly installable.
- **Jellyfin's problem is technical and structural.** Subtitles have been broken in
  well-documented ways since 2020. The playback-negotiation model produces wrong decisions in
  both directions. The client experience is described by its own users as "hit or miss", and
  the clients people actually recommend are third-party. A long-deferred database rewrite cost
  it a year of upgrade breakage. It is not a project that can turn quickly.

**The opening is the gap between them:** Plex's polish without Plex's business model, built on
an architecture that doesn't inherit Jellyfin's 2008-era assumptions.

## What we do differently

Five choices, each traceable to a specific failure in the research:

1. **The client decides what to play, from a manifest.** No server-side device-capability
   database. The server advertises every rendition it can produce, including the untouched
   original; the client — the only party that can ask its own decoder what it will actually
   accept, and whether it will be *smooth* and *power-efficient* — chooses. This deletes the
   largest bug class in the field and makes every new client cheaper to write.
   ([D2](../decisions/working-notes.md), [tech-playback.md](research/tech-playback.md))
2. **Direct play is the default and always on offer.** Transcoding is the exception, it is
   explicable in the UI, and the user can override the decision in both directions.
   ([D4](../decisions/working-notes.md))
4. **Subtitles are a subsystem.** Extracted at scan time with their fonts, served as HLS
   renditions, rendered with libass on every client. This is the most-complained-about part of
   every competitor and the cheapest place to be visibly better.
   ([D11](../decisions/working-notes.md))
5. **Your files stay yours.** Media read-only by default. The database is a rebuildable index,
   not an authority. Your corrections are written back to sidecars where you grant a writable
   path, and per-user state deliberately is not ([D98](../decisions/working-notes.md)). No telemetry, no account,
   no storefront — as a stated promise. ([D7](../decisions/working-notes.md),
   [D13](../decisions/working-notes.md))
6. **One excellent TV client beats five mediocre ones.** The project is judged on its TV app.
   ([D10](../decisions/working-notes.md))
7. **Watch Together, because everyone else deleted theirs** — and Plex's own forum has a
   **350-post thread** asking for it back. Disney+, Amazon, Sling and even
   Plex withdrew their watch-party features — Plex's note says it *"won't be available for most
   platforms as they get the new experience"*, which names the cause: it lived in every client.
   Teleparty has ten million users because the platforms will not do this. We keep the
   client-specific surface to six methods so a client rewrite cannot quietly kill it
   ([D62](../decisions/working-notes.md)) — and **a session link is the invitation**, with no
   account required at the other end, because that is the whole point
   ([D117](../decisions/working-notes.md)). The creator owns what plays; whether anyone else
   may touch the transport is his setting. A leaked link is survivable rather than prevented
   ([D119](../decisions/working-notes.md)): a **cap on simultaneous clients** bounds the flood
   before anyone notices, and because **a link admits while a *membership* persists**, the
   owner can rotate the link without the people already watching noticing anything at all.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ Clients                                                          │
│  React Native / TypeScript UI shell  ── generated from OpenAPI   │
│    ├ web        : hls.js + jassub (libass/WASM)                  │
│    ├ Android/TV : Media3  ‖ libmpv   (native module, Kotlin)     │
│    └ Apple      : AVPlayer ‖ libmpv  (native module, Swift)      │
│  Each probes its own decoder and picks a rendition. (D2)         │
└──────────────────────────────┬──────────────────────────────────┘
                               │  HTTPS: REST + OpenAPI, HLS manifests,
                               │  CMAF/fMP4 segments, ranged GETs
┌──────────────────────────────┴──────────────────────────────────┐
│ Server — one Go binary (D9)                                      │
│                                                                  │
│  api/        REST handlers; OpenAPI generated from code          │
│  library/    ingests node reports → identify → enrich            │
│  stream/     rendition list (video ‖ audio ‖ subs axes);          │
│              direct-play, remux, AUDIO encode (LGPL only)         │
│              (no video encoder — that is a plugin, below)         │
│  subtitle/   extraction, conversion, font attachments (D11)      │
│  jobs/       typed, persisted, inspectable queue                 │
│  plugin/     three tiers: WASM sandbox, protocol adapters,       │
│              capability services (D12, D15, D19)                 │
│  store/      sqlc-generated typed SQL (D6)                       │
└───┬──────────────┬─────────────────────────────┬────────────────┘
    │              │                             │
    │       ┌──────┴───────┐             ┌───────┴────────┐
    │       │ SQLite (WAL) │             │ Media (ro) +   │
    │       │ (D31)        │             │ cache/ (purge) │
    │       └──────────────┘             └────────────────┘
    │
    │  MEDIA NODES (D33) — same binary, `node` mode, one per machine
    │  holding files. Scan, probe, extract subtitles, thumbnail, and
    │  optionally transcode. Never touch the database.
    ├─ node @ core box (supervised child, the default install)
    ├─ node @ NAS / seedbox / elsewhere  (opt-in topology)
    │
    │  RELAY (D71) — same binary, `relay` mode, on any public host.
    │  The core dials OUT to it: no port forward, no DDNS, works
    │  behind CGNAT. Byte pipe by default; terminates nothing.
    ├─ relay @ your own €5 VPS   (self-hosted assist)
    ├─ relay @ ours              (hosted assist, optional, paid)
    │
    │  registered plugins — same public API a third party would use
    ├─ video transcoder (tier 3, first-party, optional, MAY BE REMOTE)
    ├─ jellyfin-adapter (tier 2, inbound — borrows other people's clients)
    └─ metadata providers, scrapers, scrobblers  (tier 1, WASM sandbox)
```

Notes on the shape:

- **One binary, one file, no second service, no message broker.** Household scale does not
  justify any of it, and "boring, established tech" is a stated constraint. The database is
  SQLite in WAL mode ([D31](../decisions/working-notes.md)), so installation is a binary and
  backup is a file copy.
- **FFmpeg is a subprocess, not a linked library.** Every project in the survey that touches
  FFmpeg drives it as a process; cgo bindings to libav\* are a build-complexity liability. We
  ship a known-good build rather than accepting the distro's — Jellyfin maintains an entire
  FFmpeg fork, which is evidence of what the alternative costs.
- **The core never touches a media filesystem** ([D33](../decisions/working-notes.md)), **and
  never parses a media or image file** ([D26](../decisions/working-notes.md),
  [D99](../decisions/working-notes.md)). Scanning, probing, extraction and image derivation all
  happen in media nodes; the core's whole input surface is HTTP, its own SQLite file, and node
  reports it validates at the boundary. A poster fetched from TMDB is bytes in flight, handed
  to the jail — TMDB is community-editable, so it is attacker-supplied like anything else.
- **The node's work is nine passes now, and that is a first-class fact**
  ([D102](../decisions/working-notes.md)). Enumerate, probe, hash, extract subtitles, subset
  fonts, thumbnail and trickplay, derive posters, measure loudness, detect segments — each
  added separately, each individually cheap, and nobody had added them up. They are named,
  individually deferrable per library, separately reported and restartable, because the
  alternative on a Celeron NAS is a first scan that takes a week.
- **The core needs no *video* encoder** — but it does encode audio. Every GPL trigger in FFmpeg
  is a video encoder, and the whole audio menu (AAC, AC-3, E-AC-3, FLAC, ALAC, Opus) is native
  LGPL, so audio costs nothing in licence or patent surface. Since the most common real transcode
  is audio-only, **the core handles the common case and the plugin handles the rare one**
  ([D19](../decisions/working-notes.md), [D22](../decisions/working-notes.md)).
- **"Plugin" means three different things**, chosen by shape: a sandboxed WASM extension the
  core calls into, an inbound protocol adapter that consumes our API, or an outbound capability
  service the core calls and clients may be redirected to. Each tier exists because a real
  member needs it.
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
  the media files.
- **Play state is a log, not a cell** ([D81](../decisions/working-notes.md)). A guarded position
  per user and edition, with a monotonic sequence so a stale device cannot rewind a newer one,
  plus an append-only event log. Jellyfin keeps one mutable row and its tracker carries *"Most
  movies now appear to be 75% watched"* and *"Media playback randomly starts over again or jumps
  back — STILL"*; Tautulli (6,590★) exists because the history is nowhere.
- **We are the ones doing the downmix, so the quiet dialogue is ours**
  ([D83](../decisions/working-notes.md)). Netflix normalises its catalogue at ingest — -27 LKFS
  dialog-gated, -2 dBTP. We cannot rewrite files, so the node measures loudness at scan and the
  core applies gain at playback, which is ReplayGain's answer to the same constraint. Downmix
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
  ([D118](../decisions/working-notes.md)). Passes are ordered so an item becomes *playable*
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
  the node's status record and its error and skip records.

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
| FFmpeg in the core process | [D26](../decisions/working-notes.md). Media is attacker-supplied data and FFmpeg will eventually be compromised; all of it runs in a Landlock+seccomp-confined worker, so the core never parses hostile bytes. |
| **An image decoder in the core process either** | [D99](../decisions/working-notes.md). The same rule, generalised: TMDB is community-editable, so a poster is attacker-supplied — and unlike a film it arrives unprompted during a background refresh. 39 CISA KEV entries touch media, image or font parsing. Derivation happens in the node's jail; the core moves bytes it never decodes. |
| In-process plugins | [D12](../decisions/working-notes.md). Third-party scrapers are the last code you want unsandboxed. Protocol surfaces get a second tier — out-of-process adapters ([D15](../decisions/working-notes.md)). |
| A *video* encoder in the core binary | [D17](../decisions/working-notes.md), [D19](../decisions/working-notes.md), [D22](../decisions/working-notes.md). It is a plugin on the public API: smaller default install, a real licence/patent boundary, and remote transcoding for free. Audio encoding does stay in the core. |
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
   can run on a €5 VPS** — which carries ~2,200 film-hours a month at 10 Mbps uncapped, against
   Plex's reported 2 Mbps paid relay cap. So the hosted tier's remaining justification is
   convenience for people who will not rent a VPS: a real market, and a far smaller claim than
   the one this project started with. Narrowed, and then narrowed again.
   [D64](../decisions/working-notes.md) thought Watch Together's voice chat needed a hosted
   **TURN relay**; [D68](../decisions/working-notes.md) found it does not, because both
   participants are already connected to the media server by construction and it can relay the
   voice itself — at one endpoint rather than in a datacentre, so with almost no latency cost.
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
