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

- **Plex's problem is commercial.** The software is good. In 2025–26 it raised prices sharply,
  moved previously-free remote streaming behind the paywall platform by platform, and kept
  pushing its own ad-supported catalogue into a UI people installed to watch their own files.
  Its users describe waiting for the other shoe to drop. This is not a problem Plex can fix
  without giving up revenue.
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
   not an authority. Your corrections are written back to sidecars. No telemetry, no account,
   no storefront — as a stated promise. ([D7](../decisions/working-notes.md),
   [D13](../decisions/working-notes.md))
6. **One excellent TV client beats five mediocre ones.** The project is judged on its TV app.
   ([D10](../decisions/working-notes.md))

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
│  library/    scan → identify → enrich, as named phases           │
│  stream/     rendition list; direct-play; subtitle + trickplay   │
│              (no encoder — the transcoder is a plugin, below)     │
│  subtitle/   extraction, conversion, font attachments (D11)      │
│  jobs/       typed, persisted, inspectable queue                 │
│  plugin/     three tiers: WASM sandbox, protocol adapters,       │
│              capability services (D12, D15, D19)                 │
│  store/      sqlc-generated typed SQL (D6)                       │
└───┬──────────────┬─────────────────────────────┬────────────────┘
    │              │                             │
    │       ┌──────┴───────┐             ┌───────┴────────┐
    │       │ PostgreSQL   │             │ Media (ro) +   │
    │       │ (D5)         │             │ cache/ (purge) │
    │       └──────────────┘             └────────────────┘
    │
    │  registered plugins — same public API a third party would use
    ├─ transcoder      (tier 3, first-party, optional, MAY BE REMOTE)
    ├─ jellyfin-adapter (tier 2, inbound — borrows other people's clients)
    └─ metadata providers, scrapers, scrobblers  (tier 1, WASM sandbox)
```

Notes on the shape:

- **One binary, one database, no message broker, no service mesh.** Household scale does not
  justify any of it, and "boring, established tech" is a stated constraint. Postgres ships in
  the default compose file so installation stays one command.
- **FFmpeg is a subprocess, not a linked library.** Every project in the survey that touches
  FFmpeg drives it as a process; cgo bindings to libav\* are a build-complexity liability. We
  ship a known-good build rather than accepting the distro's — Jellyfin maintains an entire
  FFmpeg fork, which is evidence of what the alternative costs.
- **The core needs no encoder.** It demuxes, probes, extracts subtitles and makes thumbnails.
  Encoding happens in the transcoder plugin, on its own FFmpeg build, possibly on another
  machine ([D19](../decisions/working-notes.md)).
- **"Plugin" means three different things**, chosen by shape: a sandboxed WASM extension the
  core calls into, an inbound protocol adapter that consumes our API, or an outbound capability
  service the core calls and clients may be redirected to. Each tier exists because a real
  member needs it.
- **The public HTTP API matters more than the plugin API**, by roughly an order of magnitude.
  The community's real extension mechanism in this field is a separate program over HTTP — some
  80,000 stars of companion apps (Sonarr, Radarr, Seerr, Tautulli, Bazarr, Kometa …) against
  ~20,000 for the whole Jellyfin plugin catalogue. OpenAPI is generated from the code and the
  build is gated on it being current. ([D20](../decisions/working-notes.md))
- **`cache/` is always safe to delete.** Nothing irreplaceable lives outside the database and
  the media files.
- **The scanner runs as named phases** (Navidrome's model) with its own structured log channel,
  because "I can't tell what the scanner did" is a real, recurring complaint.

## What we are deliberately not building

| Not building | Why |
| --- | --- |
| DRM | The user owns the files. Skipping it removes key servers, licence proxies and platform certification — the main reason a small team can compete here. |
| A device-profile database | [D2](../decisions/working-notes.md), [D21](../decisions/working-notes.md). It is the incumbents' largest bug source. Our menu is a closed enumeration and must never become a grammar of conditions. |
| Dolby Vision / Atmos *output* | Passthrough only. We direct-play them where the client copes and fall back to a menu profile otherwise; we never encode into them. |
| Photos | [D1](../decisions/working-notes.md). Immich exists, is AGPL, and is moving faster than we could. |
| A Netflix-style encoding ladder | Bandwidth is free on a LAN. |
| Microservices, Kafka, Elasticsearch, Redis | Household scale. Postgres covers search, fuzzy matching and job wakeups. |
| A Roku client | BrightScript shares no code with anything and Roku cannot render bitmap subtitles at all. |
| In-process plugins | [D12](../decisions/working-notes.md). Third-party scrapers are the last code you want unsandboxed. Protocol surfaces get a second tier — out-of-process adapters ([D15](../decisions/working-notes.md)). |
| Transcoding in the core binary | [D17](../decisions/working-notes.md), [D19](../decisions/working-notes.md). It is a plugin on the public API: smaller default install, a real licence/patent boundary, and remote transcoding for free. |
| Audiobooks, comics, live TV/DVR | Each is a different product wearing the same clothes. Audiobookshelf's 1,191 open issues are the evidence. |

## The open problems

Honest statement of what the research did **not** settle:

1. **Remote access for non-technical family.** Simultaneously Plex's most-resented feature and
   its moat. The self-hosted field's answer is "set up a VPN", which is not an answer for the
   audience that matters. No mechanism chosen.
   ([D14](../decisions/working-notes.md))
2. **Granular sharing.** Nobody has solved it. Immich's sharing issue has 1,020 reactions under
   a feature freeze; PhotoPrism's has 844 and is seven years old; Jellyfin users symlink subsets
   into fake libraries. Whoever solves it wins something real.
3. **The client cold-start.** A new server has no clients. Settled in shape by
   [D18](../decisions/working-notes.md): a Jellyfin-compatible **out-of-process adapter** makes
   Findroid, Swiftfin, Infuse and Streamyfin work without constraining our model — and it
   quarantines device-profile logic permanently outside the core. What remains open is whether
   it is good enough to delay the first-party TV client.
4. **Metadata provider licensing.** TMDB's terms condition commercial use on a written
   agreement; TheTVDB tiers by the licensee's revenue. Must be answered before a public release.
5. **The real direct-play rate.** [D17](../decisions/working-notes.md) bets that direct play
   covers the common case. That rests on a number **nobody has published** — Tautulli exposes it
   per-server, but no aggregate survey exists. Measuring it is days of work and it re-orders the
   roadmap in either direction, which is why it is now Phase 0.
6. **Codec patents.** Separate from copyright licence and not resolved by being open source. The
   field's uniform practice is to ship H.264/HEVC anyway; direct-play-first reduces a default
   install's exposure to nearly nothing because it never encodes.
   ([D16](../decisions/working-notes.md))
