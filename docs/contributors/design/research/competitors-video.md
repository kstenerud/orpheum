> **ABOUTME:** Survey of the video half of the personal-media-server field — the stacks each
> project chose, the architectural decisions underneath them, and where each one breaks for its
> users. Source was read locally for every open competitor.

# Competitors: Video

Repository facts (stars, open issues, language, licence, last push) were read from the GitHub
API on **2026-09-17**. Code observations come from a depth-1 clone read locally on the same day.

| Project | Stars | Open issues | Language | Licence | Created |
| --- | --- | --- | --- | --- | --- |
| jellyfin/jellyfin | 57,238 | 690 | C# | GPL-2.0 | 2018-12-09 |
| stashapp/stash | 12,979 | 734 | Go | AGPL-3.0 | 2018-06-04 |
| Kareadita/Kavita | 11,696 | 174 | C# | GPL-3.0 | 2020-12-12 |
| zoriya/Kyoo | 2,524 | 61 | TypeScript | GPL-3.0 | 2019-08-04 |

Plex and Emby are closed; their numbers are not comparable and are not listed.

---

## Plex

**Stack.** The server is C++ with Python for the scanner and the (now-retired) plug-in/channel
system. Plex's own engineering writing describes a large C++ codebase shipping to many device
targets, built through a long tail of toolchains before a consolidation onto Clang; the first
commit is dated 2008-09-20. Plex began life as a fork of XBMC/Kodi for macOS, which is the
origin of both its maturity and its legacy.

**Architectural decisions.**

- **Closed server, open-ish client ecosystem, account-mediated.** Even a fully-local Plex
  server authenticates users through `plex.tv`. This is the decision everything else follows
  from: it gives Plex effortless remote access and relay, and it gives Plex a kill switch and a
  billing surface over a server you own and run on your own hardware.
- **Server-side playback decision.** Like Emby/Jellyfin, the server decides whether a client
  can direct-play, direct-stream or must transcode.
- **Ship your own player.** Plex writes and ships first-party apps for essentially every
  platform that matters, which is the single biggest reason people stay. Plexamp (its music
  client) is repeatedly named on HN as a thing with no free-software equivalent.

**How well it works.** Better than anything else in the field on client breadth, first-run
experience, and music. The server is reliable at large library sizes. Its failure is not
technical.

**Where it fails users.** Commercially, not architecturally. In 2025 Plex raised Plex Pass
from $4.99 to $6.99/month and $39.99 to $69.99/year, moved the lifetime pass from $119.99 to
$249.99 (and, per community reports on HN in 2026, later to $750), and **moved remote
streaming — previously free — behind the paywall**, rolling the enforcement out platform by
platform (mobile first, Roku in late 2025, TVs and consoles in 2026). Non-subscribers can buy
a "Remote Watch Pass" at $1.99/month. The second complaint is product direction: the client
increasingly surfaces Plex's own ad-supported streaming catalogue and store in a UI the user
installed to watch their own files. Both complaints are near-universal in the HN threads (see
[sentiment.md](sentiment.md)).

---

## Emby

**Stack.** C#/.NET. The public GitHub repository `MediaBrowser/Emby` is frozen at **version
3.5.3, last commit 2018-09-19** — the codebase read locally confirms this: `MediaBrowser.Api`,
`Emby.Server.Implementations`, `Emby.Dlna`, `Emby.Drawing.{ImageMagick,Skia,Net}`,
`MediaBrowser.ServerApplication`, plus a vendored `SocketHttpListener`, `RSSDP` and `Mono.Nat`.

**Architectural decisions.** Emby 4.x went closed-source in December 2018. Everything after
3.5.x is unavailable for inspection. The 3.5 tree is the direct ancestor of Jellyfin, so
reading it is reading Jellyfin's inheritance: the `DeviceProfile` capability-matching engine,
the DLNA-derived stream builder, and the file-per-item metadata model all predate the fork.

**How well it works.** By reputation, a somewhat more polished and better-QA'd version of what
Jellyfin does, with paid first-party apps. It sits in an awkward middle: the freedom cost of
Plex without the client breadth.

**Where it fails users.** The close itself is the complaint, and it drove the fork. Jellyfin's
own account lists the causes: the 4.x close, prior GPL-violation reports, a paywall added for
gratis users during 3.x, the progressive removal of client and server source, and a lack of
openness to community contributions. Community sentiment since is that Emby offers too little
over free Jellyfin to justify Premiere, with the countervailing view — seen on HN — that
Emby's *first-party apps* are better than Jellyfin's, which is the one thing keeping some users
on it.

---

## Jellyfin

**Stack (read locally, `main` @ 2026-09-17).** C# on **.NET 10**, ASP.NET Core, Entity
Framework Core 10 over SQLite (`Microsoft.Data.Sqlite`), central package management, StyleCop +
`Microsoft.CodeAnalysis.BannedApiAnalyzers` + `IDisposableAnalyzers` in the build, xUnit v3 +
Moq + AutoFixture + FsCheck for tests, Prometheus metrics, SkiaSharp for images. Projects still
carry the inherited `Emby.*` and `MediaBrowser.*` names. `jellyfin-web` is a webpack build of
React 18 + TypeScript 5.9 with ESLint 9 and Stylelint. `jellyfin-ffmpeg` is a maintained
FFmpeg fork shipped as the server's encoder. The desktop client is Qt WebEngine wrapping the
web UI with **libmpv** for playback.

**Architectural decisions, and the cost of each.**

1. **Server-side capability matching via `DeviceProfile`.** The decisive one. Each client
   sends (or the server assumes) a profile listing `DirectPlayProfiles`, `TranscodingProfiles`,
   `ContainerProfiles`, `CodecProfiles` and `SubtitleProfiles`; `StreamBuilder` (2,493 lines,
   in the `MediaBrowser.Model.Dlna` namespace — the DLNA ancestry is literal) matches the media
   against the profile and emits a play method plus a `TranscodeReason`. It is a declarative
   capability language that the server evaluates on the client's behalf.

   This is the root of the single most-reported class of bug in the project: the server decides
   to transcode content the client could have played, or decides the client can play something
   it can't. A profile is a *static claim*; real decoder support is dynamic (codec profile,
   level, bit depth, HDR format, hardware state, whether another app holds the decoder). HN
   users describe exactly this — "it constantly tries to re-encode video that could be played
   perfectly fine". The Android TV tracker carries `Force "direct play" in client` (+49) and
   `Feature to force transcode` (+21) — users asking for manual override of a decision the
   server got wrong in both directions.

2. **FFmpeg argument construction as a single decision surface.**
   `MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs` is **8,042 lines** — by a wide
   margin the largest file in the tree — and builds the encoder command line for every
   combination of container, codec, hardware accelerator, tone-mapping path and subtitle burn.
   `LibraryManager.cs` (4,176) and `BaseItem.cs` (3,090) are the next two. Correctness here is
   combinatorial and largely untestable without the hardware.

3. **A long-deferred database rewrite, cashed in at once.** Jellyfin ran on hand-written SQLite
   access inherited from Emby until **10.11 (October 2025)** replaced it with EF Core. The
   migration went badly enough to have its own issue groups in the tracker: `[Group] Jellyfin
   10.11.x migration and database issues` (+23), `[Group] "Database Locked" on LXC containers
   using ZFS as storage` (+17, **228 comments**), `SQLite core dump on systems without SSE4.1`
   (+24, 54 comments). Reported symptoms included migrations hanging for hours, `no such table:
   ActivityLog`, and severe post-migration slowdowns traced to EF Core enumerating result sets
   client-side instead of pushing filters into SQL. **12.0 (2026-09-08)** is explicitly a
   continuation — "finishing what the database conversion started" — and converts playlist and
   collection membership into real rows with real foreign keys, i.e. the schema normalisation
   that should have preceded the ORM.

4. **Security posture: LAN-shaped, internet-deployed.** An April-2025 sweep of the tracker
   filed a cluster of endpoints that authenticate nobody: `GetUser()`, `GetUsers()`,
   `GetAttachment()`, all of `AudioController`, the video/HLS/LiveTV stream controllers, the raw
   image endpoints, most of `DashboardController` (+12 to +25 each). Separately: `Refactor
   "Copy Stream URL" to not leak the user's session API key` (+23) and `Granular apikey
   permissions` (+23). Published CVEs in the same period cover FFmpeg argument injection
   through unauthenticated `/Videos/{id}/stream` reaching arbitrary file write, and an
   unauthenticated `/System/Restart` reachable by spoofing a LAN source IP. Dashboard-issued
   API keys do not expire. The maintainers' position, as relayed on HN, is that Jellyfin should
   sit behind a VPN — which is a coherent threat model, but it is not the one users deploy.

5. **Clients are separate projects with separate maturity.** `jellyfin-web`, `-androidtv`,
   `-roku`, `-tizen`, `-expo`, `-media-player`, `-kodi` are distinct repos with distinct
   teams and very different levels of activity. The Android TV tracker's top item by a factor
   of three is `The Playback Rewrite` (+214, open since **2021-07**).

**Where it fails users.** Subtitles first and by a distance — desync on resume (+103, the
single most-reacted server issue, open since 2020), disappearing in fullscreen, inconsistent
sizing, styling, offset controls, per-client format support. Then transcode decisions. Then
the client experience, which HN summarises as "the server is pretty solid, the client
experience is a bit hit or miss". Then, recently, upgrade risk.

**Governance note.** Founder Andrew Rabert left the project in July 2026; the transition was
reported as peaceful. A contributor posting under a throwaway on HN described hostility toward
third-party clients, plugins and themes in the project's official spaces, with bans for
dissent. Treat as one unverified account, but it is consistent with the visible pattern that
the best third-party clients (Findroid, Swiftfin, Streamyfin, Wolphin) live entirely outside
the project.

---

## Kyoo — the closest thing to what we want to build

2,524 stars, one principal developer. Worth more attention than its size suggests, because it
has already made the architectural bet this project is considering.

**Stack (read locally).** A polyglot monorepo:

| Component | Stack |
| --- | --- |
| `transcoder/` ("gocoder") | **Go 1.26**, Echo v5, pgx, golang-migrate, direct FFmpeg process control |
| `api/` | **Bun + Elysia + Drizzle ORM**, Postgres, OpenTelemetry throughout |
| `auth/` ("keibi") | **Go**, sqlc, argon2id, OIDC, JWT |
| `scanner/` | **Python 3.13**, FastAPI, `guessit` (fork), watchfiles |
| `front/` | **React Native + Expo** (web, Android, Chromecast), `jassub` for ASS, `libpgs` for PGS |

**The decision that matters.** From `transcoder/README.md`, verbatim:

> Instead of having complex logic to know which client supports which codecs, we use HLS to
> it's full potential. Each quality & codec supported by gocoder is specified in the manifest
> and the client is free to pick the one it can play (and switch on the fly to change quality
> if network changes).

That is the **exact inverse of Jellyfin's `DeviceProfile`**, and it is the strongest single
finding in this survey. The server advertises; the client — which is the only party that
actually knows what its decoder will accept — chooses. The author's stated consequence is
"this makes the API easier (to adopt for new clients & to write) but it means it's harder to
handle transmuxing."

**How the transcoder works.** Lazy, on-demand HLS. No pre-generated ladder on disk. The hard
part is that HLS variants must have **segment boundaries aligned across renditions**, and
segments must start on keyframes, while source keyframe spacing is arbitrary. Kyoo's answer:
extract every keyframe timestamp from the source up front (via packet-level `ffprobe`, not
frame decoding — the author reports ~20× on that alone), use those timestamps as the segment
grid for *all* renditions including the passthrough one, and force FFmpeg to emit IDR frames
exactly there with `-force_key_frames`. Multiple clients watching the same title share one
transcode. Quality switches start a new FFmpeg at the requested segment rather than from zero.
Documented sharp edges: NVENC ignores `-force_key_frames` and needs `-force_idr 1`; a
mid-stream quality switch can outrun the new rendition's progress.

`RunLock` is built for an eventual Redis-backed multi-replica mode, and the transcoder ships
as a standalone image that two other projects (Meelo, Blee) already reuse.

**Other decisions worth stealing.** No plugin system, on purpose — every feature is built in.
"Media not being scanned correctly (even with weird names) is considered a bug", i.e. no
imposed folder structure. OIDC from the start. Postgres from the start.

**Where it fails.** One developer. Android TV is "planned". Download/offline and watch-list
sync were dropped in the v5 rewrite and not yet restored. Five languages and two package
managers is a large surface for a small team — this is the thing to *not* copy.

---

## Stash and Kavita — narrow-scope references

**Stash** (Go, AGPL-3.0, 12,979 stars): gqlgen **GraphQL-first** API with a generated
TypeScript client, SQLite via `goqu`, React UI, Chromedp for scraping, perceptual hashing
(`goimagehash`) for duplicate detection, `dop251/goja` to run user-supplied JS scrapers
in-process. Relevant for two reasons: it is the best evidence that a **Go + SQLite + generated-
client** media server holds up at scale, and its scraper-plugin sandboxing question is one we
will face.

**Kavita** (C#, GPL-3.0, 11,696 stars): comics/manga/ebooks. Notable mainly for having a
clean multi-project .NET layout (`Kavita.{API,Models,Services,Database,Common,Email}` each with
a paired test project) — the layering Jellyfin never got — and for the lowest open-issue count
and reaction counts in this table, which suggests a tractable scope well served.

---

## What this survey settles

1. **The playback-negotiation model is the architecture.** Everything downstream — API shape,
   client complexity, bug volume, how hard a new client is to write — follows from whether the
   server or the client decides what to play. Jellyfin/Emby/Plex put it on the server; Kyoo put
   it on the client via the HLS manifest and reports a simpler API for it. See
   [tech-playback.md](tech-playback.md).
2. **Subtitles are not a detail.** They are the top-reacted issue on the Jellyfin server, the
   top cluster on `jellyfin-web`, and a recurring theme in every HN thread. A design that
   treats subtitle rendering as a first-class client concern (libass everywhere, fonts shipped
   with the stream) starts ahead.
3. **Deferring the data layer is expensive.** Jellyfin carried hand-rolled SQLite for seven
   years and paid for it across two major releases and a year of user-visible breakage. Pick
   the storage model and the migration discipline before writing features.
4. **Clients are the project.** Server breadth is not what keeps people on Plex; app breadth
   is. A server with no good TV client loses to a worse server that has one.
5. **Scope discipline is visible in the issue tracker.** Kavita (narrow, well-defined) has 174
   open issues and single-digit reaction counts. Audiobookshelf (open-ended feature surface)
   has 1,191. Immich's most-reacted issue has **1,020 reactions** and sits under a declared
   feature freeze.
