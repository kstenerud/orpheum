> **ABOUTME:** The append-only, D-numbered decision log — what was decided, what was rejected,
> why, and what follows. The authority other docs cite by number.

# Working Notes

All entries dated **2026-09-17** unless stated. Every decision below is **provisional** — they
are made from research, not from a running system. Each carries a **Status** line saying what
would validate or overturn it.

---

## D1 — Scope: video first, audio second, photos probably never

**Decided.** Build a video media server. Design the data model so music can be added without a
schema rewrite, and add it once video is good. Treat photos as out of scope for the
foreseeable future.

**Why.**

- Video is where the pain is. Plex's paywall and product drift have produced a large,
  motivated, actively-churning audience ([sentiment.md](../design/research/sentiment.md)), and
  Jellyfin is not catching all of it — its own users describe the client experience as "hit or
  miss".
- **Music is cheap because of OpenSubsonic.** A server that speaks the OpenSubsonic API
  inherits a mature third-party client ecosystem at zero client cost
  ([competitors-audio.md](../design/research/competitors-audio.md)). Nothing equivalent exists
  for video. This makes audio a *server-side-only* project, which is a fraction of the work.
- **Photos is a different product.** Its core job is reliable background acquisition from
  phones, which means a first-class mobile app with background upload, delta sync and
  deletion semantics — roughly the cost of the entire video project again, almost all of it in
  the client ([competitors-photos.md](../design/research/competitors-photos.md)). Immich
  already exists, is AGPL, has 114k stars, and is moving faster than we could.

**Rejected:** "one-stop server for all three from the start". It is the stated ambition, and it
is how Audiobookshelf ended up with 1,191 open issues and how PhotoPrism ended up with an
eight-year-old Postgres request. Scope discipline is visible in a tracker, and we have the
trackers to look at.

**Consequences.** The data model must not *exclude* audio or photos — generic `Library`,
`Item`, `MediaFile` shapes rather than `Movie`/`Episode` everywhere. Photos, if ever, is a
separate service sharing auth and storage, not a third library type.

**Status:** Provisional. Revisit the audio decision once video direct-play and the TV client
are solid. Revisit photos only if Immich becomes unusable or hostile.

---

## D2 — Playback negotiation: the client decides, from a manifest

**Decided.** No server-side device-capability database. The server publishes an HLS manifest
listing every rendition it can produce — including passthrough of the original — and the client
picks, using its own runtime capability probe.

**Why.** This is the highest-leverage architectural decision available and it is thoroughly
evidenced ([tech-playback.md](../design/research/tech-playback.md) §1):

- Jellyfin/Emby/Plex use static `DeviceProfile` declarations evaluated server-side. Jellyfin's
  `StreamBuilder` is 2,493 lines in a namespace literally called `Dlna`, and the resulting
  wrong decisions are the top user complaint in both directions — unnecessary transcodes and
  failed direct plays. Users are asking for manual override of an automatic decision
  (`Force "direct play" in client` +49, `Feature to force transcode` +21).
- A profile is a static claim about a dynamic fact. Only the client can ask
  `navigator.mediaCapabilities.decodingInfo()` or `MediaCodecInfo.CodecCapabilities` and learn
  not just *supported* but **smooth** and **power-efficient**.
- Kyoo has already validated the inverse design in exactly our problem space and reports a
  simpler API for it.

**Rejected:** device profiles (the incumbent design; an open-ended maintenance tax and the
largest bug class in the field).

**Consequences.** Transmuxing gets harder — the passthrough rendition must be segmented on the
same keyframe grid as the transcoded ones (D4). A bad client picks badly, so the probe-and-pick
logic lives in a shared client SDK, and the server may include a *hint*, never a mandate.

**Status:** Provisional; the strongest of these decisions. Overturned only if a major target
platform turns out to have no usable runtime capability probe.

---

## D3 — Streaming format: CMAF/fMP4 segments, HLS manifest, no DRM

**Decided.** fMP4 (CMAF) segments with an HLS manifest. No DRM, ever.

**Why.** Apple's HLS authoring specification requires fMP4 for HEVC, HDR10 and Dolby Vision —
MPEG-TS cannot carry them, and TS paths are implicated in "plays for four minutes then errors"
reports. fMP4 is also DASH's container, so a DASH manifest over the same segments is nearly
free later. HLS is the most universally consumed manifest format across browsers, Apple,
Android and TV platforms.

No DRM because **the user owns the files**. Skipping FairPlay/Widevine/PlayReady removes key
servers, licence proxies and platform certification — and is the main reason a small team can
compete here at all.

**Status:** Provisional; low risk.

---

## D4 — Three playback paths: direct, on-demand segmented, background-prepared

**Decided.** (a) Direct play — a ranged GET of the original — is the default and is always
offered as a rendition. (b) On-demand, segment-addressed transcoding is the fallback, built on
the Kyoo model. (c) Background pre-preparation of a single compatibility rendition is a later
*job type*, designed for but not built first.

**Why.** Every commercial service encodes ahead of time and serves static segments; self-hosted
servers transcode live, which is why home media servers need a GPU and why "will it direct
play?" is a question users learned to care about
([commercial-streaming.md](../design/research/commercial-streaming.md)). We cannot pre-encode
everything at home, but we can stop treating live transcoding as the only answer.

**Mechanics for (b)**, verified in Kyoo's source: extract all source keyframe timestamps up
front from packet-level `ffprobe` data (not frame decoding), cache them versioned, use them as
the segment grid for *every* rendition including passthrough, force IDR frames there, start
transcodes at the requested segment rather than from zero, and share one transcode between
viewers. Known trap: **NVENC ignores `-force_key_frames` and needs `-force_idr 1`.**

**Consequences.** Audio-only renditions are first-class: many "video transcodes" are really
just a TrueHD/DTS problem, and transcoding audio while passing video through is a large cheap
win. The job system must exist early enough that (c) is an added job type, not a rewrite.

**Status:** **Refined by [D17](#d17--direct-play-first-transcoding-additive-and-optional)** —
the three paths stand, but v1 ships only (a), and (b)/(c) are re-ordered and re-scoped. Path (b)
still needs a spike against a real library before it is trusted.

---

## D5 — Database: PostgreSQL only, shipped in the default compose

**Decided.** PostgreSQL. Not SQLite. Ship it in the default Docker Compose so installation is
still one command.

**Why.** ([tech-storage.md](../design/research/tech-storage.md) §1) SQLite's single writer lock
collides directly with this workload — a library scan writing thousands of rows while
transcodes and progress updates also write. This is not theoretical: Audiobookshelf #5349 (an
unhandled `SQLITE_BUSY` during a scan took the whole server down), Jellyfin #11589 (`database
is locked` after 10.9), and Jellyfin's `"Database Locked" on LXC containers using ZFS` group
(228 comments — note that SQLite's locking is sensitive to the filesystem, and self-hosters run
ZFS, btrfs and NFS).

The asymmetry decides it: adding Postgres later is a project nobody finishes — PhotoPrism's
Postgres request has **189 reactions and is eight years old**; Audiobookshelf's has 66. Nobody
is asking for the reverse. Postgres also supplies FTS, `pg_trgm` fuzzy matching for the
scanner, JSONB, `LISTEN`/`NOTIFY` for job wakeups without a broker, and `pgvector` if ever
needed — each otherwise a separate dependency.

**Rejected:** SQLite (chosen by Navidrome, Stash and Kavita and working fine for them — the
tiebreaker is the scan-while-streaming write pattern and the ZFS/NFS deployment reality, not
throughput).

**Consequences.** All SQL behind a repository layer, so an embedded engine remains addable.
The install story must stay one command or this decision costs us users.

**Status:** Provisional, and the one most worth re-examining. If a spike shows the write
pattern is lighter than expected, SQLite's single-binary deployment is worth real money to
self-hosters.

---

## D6 — Data access: `sqlc`, no ORM

**Decided.** Write SQL; generate typed Go with `sqlc`. A small hand-written dynamic-filter
builder (or `squirrel`) for search and smart playlists. No GORM, no Ent, no EF Core.

**Why.** The two ORM users in the survey are both cautionary. Jellyfin adopted EF Core late in
10.11 over an un-normalised schema and produced hour-long migrations plus severe slowdowns from
client-side enumeration; 12.0 is still finishing the cleanup. PhotoPrism runs the *deprecated
v1* of GORM. The two cleanest data layers in the survey (Immich's `kysely`, Kyoo's `sqlc`) both
use typed SQL rather than an ORM. `sqlc` also best matches this project's stated criterion —
strong typing with fast mechanical feedback — because a wrong column name is a build failure.

**Status:** Provisional; low risk.

---

## D7 — The files are the source of truth; the database is a rebuildable index

**Decided.** Tags and sidecars on disk are authoritative for anything derived from the media.
The database is a derived index that can be dropped and rebuilt. The exceptions — user-authored
metadata corrections and playback state — are ours, and are mirrored to sidecar files so a user
can rebuild from scratch and lose nothing.

**Why.** This is Navidrome's model and it is why Navidrome has no migration horror stories,
while Jellyfin's users now open upgrade threads with "Upgrade experiences, anyone?". It also
removes the "a migration ate my library" fear class entirely, which is a reputational event
that lasts years.

**Consequences.** Media directories are **mounted read-only by default**; writing to the media
tree is an explicit per-library opt-in. (A Jellyfin complaint on HN is that the UI says a
deletion "will be removed from Jellyfin" when it in fact deletes the file.)

**Status:** Provisional; low risk.

---

## D8 — A change journal and delta-sync cursor exist in the first schema

**Decided.** Every syncable table gets a paired audit table recording deletions (UUIDv7 primary
key, indexed timestamp), and the API serves a cursor-based "what changed since X" endpoint
including deletes, from day one.

**Why.** Immich's `*_audit` tables plus `sync.service` are exactly this, and they exist because
offline clients need deletes — an `updatedAt` column cannot express one. We will have offline
clients (download-for-travel is a non-negotiable client feature). Retrofitting a change journal
is a migration over the whole schema.

**Status:** Provisional; low risk, and cheap now versus expensive later.

---

## D9 — Server language: Go, one language on the server

**Decided.** Go, with `golangci-lint` in strict mode. **One language on the server**, with a
single sanctioned future exception: an out-of-process, optional ML service in Python if photos
or content-based search ever happen.

**Why.** ([tech-backend.md](../design/research/tech-backend.md)) It satisfies both stated
constraints. Boring: three of the surveyed competitors plus Kyoo's transcoder are Go.
Strongly typed with excellent linting: `golangci-lint` is the de-facto standard and
`gofmt` ends formatting debate permanently — both directly serve "easy for agents to write
quality software", as does the 1–3s incremental build that keeps the feedback loop tight.
Fit: FFmpeg is a subprocess and Go is very good at subprocesses; goroutines plus `context`
match many simultaneous long-lived streams; a single static binary cross-compiles to the
arm64 NAS and Pi hardware self-hosters actually use.

**Rejected:** Rust (better type system, but 10–30s incremental builds tax an agent loop
thousands of times, async adds complexity to exactly our hottest code, and no prior art in this
domain — the closest call); C#/.NET (best prior art, but building in the incumbent's stack
invites building the incumbent's architecture, and we are specifically trying not to);
TypeScript/Node (best structural prior art in Immich, but weaker feedback than a real compiler,
a churning dependency graph, and a poor host for supervising many long-lived FFmpeg processes).

**Consequences.** The one-language rule is the load-bearing half. Kyoo's five-language,
two-package-manager monorepo is the thing not to copy: every extra language is another
toolchain, lint config, CI lane and set of conventions to get right.

**Status:** Provisional. Revisit only if the transcode core turns out to *be* the project, in
which case Rust's correctness starts to outweigh its build times.

---

## D10 — Clients: React Native shell, native player modules, two engines

**Decided.** A shared React Native/TypeScript UI shell with a **native player module per
platform** (Swift for Apple, Kotlin for Android, `hls.js` + `jassub` for web). Each client
supports **two engines**: the platform-native one by default, libmpv as the "play anything"
fallback, user-switchable.

**Why.** ([tech-clients.md](../design/research/tech-clients.md)) Both Kyoo and Streamyfin
independently arrived at this shape; Streamyfin ships `modules/exoplayer-player` and
`modules/mpv-player` side by side, with `MPVLayerRenderer`, `PiPController` and
`MPVNowPlayingManager` on Apple. The engines with the best format coverage (mpv, VLC) are not
the ones with the best platform integration (AVPlayer, Media3), so a client that wants both
must ship both. Playback cannot be shared JavaScript — it needs the hardware decoder, surface
compositing, PiP, media session, HDR passthrough and audio routing.

**Order:** v1 web + Android/Android TV. v2 Apple (iOS/tvOS), desktop, Chromecast receiver.
v3 Tizen/webOS/Kodi. Roku probably never — BrightScript shares no code and its decoder set
cannot render bitmap subtitles at all.

**Risk recorded:** `react-native-tvos` is a community fork of React Native, not first-party.
Streamyfin running it in production is the mitigating evidence.

**Status:** Provisional. The TV focus-navigation experience is the thing to prototype first —
it is what a TV app lives or dies on and it is not something research can settle.

---

## D11 — Subtitles are a subsystem, not a feature

**Decided.** Extract embedded subtitle tracks and font attachments **at scan time**, not at
playback. Serve subtitles as HLS renditions and fonts as addressable attachments. Render ASS/SSA
with libass on every client (`jassub` on web, native libass elsewhere). Never burn in unless
there is no alternative. Expose subtitle offset adjustment in every player UI.

**Why.** This is the single largest defect area in the field. The most-reacted Jellyfin server
issue is subtitle desync on resume (+103, **open since 2020-03**); the top `jellyfin-web` bug is
subtitle desync; subtitles are the most frequently named pain point across the HN threads. The
workaround an HN user arrived at independently — pre-extract everything to SRT and disable
serving embedded subs — should simply be the default behaviour, because on-demand extraction
from a large MKV is exactly the "progress indicator loops forever" complaint. Burning in is
worse than it looks: it forces a video transcode of content that would otherwise direct-play.

**Status:** Provisional; well-evidenced.

---

## D12 — Providers are sandboxed WASM plugins; no in-process plugin code

**Decided.** Metadata providers, scrapers and scrobblers run as **Extism/WebAssembly plugins**
with an explicit host-capability list, out of the server's address space. Field-level provider
ranking with recorded provenance. Provider API keys belong to the user.

**Why.** ([tech-metadata.md](../design/research/tech-metadata.md)) Navidrome demonstrates the
model — plugins in Go, Rust, Python or TypeScript, sandboxed, with host services for HTTP,
cache, KV, scheduler and artwork. The alternatives in the field are Jellyfin's in-process .NET
assemblies (a plugin can do anything the server can) and Stash's in-process JS interpreter for
user-supplied scrapers — and third-party scrapers are precisely the code you least want running
unsandboxed. Provenance matters because Jellyfin has live bugs where a refresh silently
overwrites user corrections or pulls from providers the user disabled.

Keys belong to the user because TMDB's terms make commercial use conditional on a written
agreement, TheTVDB tiers by the licensee's revenue, and OpenSubtitles budgets downloads per
account (20/day free).

**Status:** **Extended by [D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters)**
(WASM is one of two plugin tiers). The licensing question is answered for an open-source release
by [D16](#d16--gplv3-no-nonfree-components-and-provider-keys-belong-to-the-user).

---

## D13 — No telemetry, no account, no upsell — as a stated promise

**Decided.** Zero telemetry by default and opt-in only when added. No third-party account ever
required. No storefront, no recommendations for content the user doesn't own, no upsell. Stated
in the README as a promise, not buried in a privacy policy.

**Why.** This is the emotional core of the Plex churn, and it is nearly free to deliver.
Verbatim from the threads: "Plex lost their best users by missing the point of self hosting
entirely"; "I don't want my adblocker to have to block services running on my local network";
"they were recently hacked exposing customer accounts details. their push for online accounts
is what ruled it out for me"; "i am prepared to pay them to NEVER EVER change the ui, never
shove another streaming shit in my face".

**Status:** Decided, and not provisional. This one is a product commitment.

---

## D14 — Remote access for non-technical family is a headline feature

**Decided.** Treat "your parents can watch your library from their house without you
configuring their router" as a first-class product requirement with its own design, not a
deployment note.

**Why.** It is simultaneously the thing Plex users most resent (the cloud account) and the
single thing keeping them on Plex. From the threads: "the sore point right now is outside
access, which Jellyfin really isn't made for... I can't give friends and family access, either,
which was dead easy with Plex" and "Tailscale is great for me, but not feasible for family".
The self-hosted field's answer to this is uniformly "set up a VPN", which is not an answer for
the audience that matters.

Related and unsolved everywhere: **granular per-user sharing of a subset of a library.**
Jellyfin users symlink subsets into separate libraries to fake it; Immich's sharing issue has
**1,020 reactions** under a feature freeze; PhotoPrism's has 844 and is seven years old.
Whoever solves sharing wins something real.

**Status:** Provisional and **unsolved**. No mechanism is chosen yet. This is the hardest
open product question in the project and it needs its own research round before any design.

---

## D15 — Two plugin tiers: in-process WASM, and out-of-process adapters

**Decided.** Extend [D12](#d12--providers-are-sandboxed-wasm-plugins-no-in-process-plugin-code)
from one plugin mechanism to two, chosen by what the extension needs to do:

| Tier | Mechanism | For |
| --- | --- | --- |
| **In-process, sandboxed** | Extism / WebAssembly with an explicit host-capability list | Small, request-shaped work: metadata providers, scrapers, scrobblers, lyrics, filename-parser rules |
| **Out-of-process adapter** | A separate process speaking our REST API, owning its own listener | Whole protocol surfaces: the Jellyfin-compatible API, OpenSubsonic, DLNA |

**Why two.** WASM is right for "given this title, return metadata" and wrong for "serve an HTTP
API and stream gigabytes of video". Streaming large bodies across the WASM boundary is
pathological, and a protocol surface needs its own auth, its own websockets and its own route
namespace. Forcing both into one mechanism would either cripple the sandbox or bloat the host
interface until the sandbox is meaningless.

**Why the adapter tier is out-of-process rather than a compiled-in module.** Zero coupling to
the core; it can be developed, versioned, shipped and **abandoned** independently; a bug in it
cannot take the server down; and it is written against exactly the same public API a third party
would use, which keeps that API honest. Byte-streaming does not need proxying — the adapter
answers with a redirect to the core's own media URL.

**Consequences.** The core server's public REST API must be complete enough to build a full
competing protocol surface on top of it. That is a constraint, and a healthy one.

**Status:** Provisional.

---

## D16 — GPLv3, no nonfree components, and provider keys belong to the user

**Decided.** Licence the project **GPLv3**. Build and pin our own FFmpeg with `--enable-gpl`;
**never** `--enable-nonfree`, and never a patch that reclassifies a nonfree component. Use the
native AAC encoder plus Opus and `libmp3lame` rather than `libfdk-aac`. Prefer **AV1/Opus**
wherever we choose an output codec. Provider API keys are always supplied by the user.

**Why.** ([tech-licensing.md](../design/research/tech-licensing.md)) FFmpeg is a licence ladder
and the build configuration picks the rung: x264 and x265 both require `--enable-gpl`, which
makes the whole binary GPL. Jellyfin — a GPL-2.0 project — ships a **GPLv3** FFmpeg build with
`--enable-gpl --enable-version3`, so GPLv3 is where this dependency chain lands regardless;
choosing GPLv2 would create a one-way incompatibility with our own encoder.

The `libfdk-aac` exclusion is deliberate and specific. FFmpeg's own configure requires
`--enable-nonfree` for it, which produces a binary FFmpeg says may not be redistributed at all.
Jellyfin carries a patch literally named `0026-remove-fdk-aac-from-nonfree.patch` to get around
that. Whether that is legally sound is not ours to rule on — but the existence of the patch is
evidence that the boundary is real, and the audio-quality difference does not justify standing
on a contested point in a project meant for other people to redistribute.

**Codec patents are a separate question from copyright licence** and are not resolved here.
Every distribution and every project in the survey ships H.264/HEVC; the pools have
historically pursued commercial vendors. Two things follow that are ours to control: **decoding
carries less exposure than encoding** — which the direct-play-first direction (D17) reduces
almost to nothing for a default install — and **AV1/Opus are royalty-free by design**, so they
are the right default wherever we pick an output codec.

Provider keys belong to the user because TMDB conditions *commercial* use on a written
agreement and shipping our own key would make us the API consumer for every install; because
TheTVDB tiers by the licensee's revenue; and because OpenSubtitles budgets downloads per
account. It is also what [D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise)
implies anyway.

**Rejected:** AGPL (this is software people run at home, not a network service; AGPL's benefit
does not apply and its friction does), and a permissive licence (nothing stops the exact
re-close that created Jellyfin, which is this project's whole cautionary tale).

**Status:** Provisional on the licence choice; firm on nonfree components. Get real legal
advice before anything commercial ever happens, and do not answer the patent question in a
README.

---

## D17 — Direct play first; transcoding additive and optional

**Decided.** v1 ships **direct play only**. The playback API returns a list of available
renditions from day one, initially containing just the original. Transcoding is added later as
extra entries in that list, and ships as a **separable optional component**, not as part of the
core server.

**Why.**

1. **The manifest shape makes it additive.** Because the client picks from a list
   ([D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)), growing the list
   requires no client change, no protocol version and no migration. Under a device-profile
   design the transcode decision is welded into the protocol from the first release; under ours
   it is a schedule decision. This is the payoff from D2 that was not obvious when D2 was made.
2. **The common case is a ranged GET.** A modern library watched on a LAN needs no encoder at
   all. Both drivers of transcoding are exceptions:
   **compatibility** (old or odd files — a *library* problem, solvable offline, once per file)
   and **bandwidth** (remote or weak network — a *network* problem, unrelated to the file, and
   the only one that genuinely needs live adaptive segmenting).
3. **The client's engine choice is a bigger lever than the server.** A client shipping libmpv
   alongside the platform decoder direct-plays close to everything, at zero server CPU. Swiftfin
   states this as its rationale. This is already [D10](#d10--clients-react-native-shell-native-player-modules-two-engines);
   D17 makes it load-bearing.
4. **It shrinks the licence surface of the default install.** A core that only demuxes, probes,
   extracts subtitles and makes thumbnails needs no encoder — which also means no encoding, and
   encoding is where the codec-patent exposure concentrates (D16).
5. **It enforces the boundary for free.** Jellyfin's `EncodingHelper.cs` is 8,042 lines because
   nothing stopped transcoding logic from spreading through the core. A packaging boundary
   stops it mechanically.

**The unknown this rests on.** The real direct-play rate across a real library and real clients
is **not published anywhere** — Tautulli exposes it per-server but no aggregate survey exists.
This is now the first thing to measure (`plans/playback-survey.md`), because it decides how
urgent transcoding is and nothing else should be guessed on it.

**Order once measured:** compatibility transcoding first (pre-prepared background job, the
simpler problem), then bandwidth transcoding (live segmented, coupled to remote access
[D14](#d14--remote-access-for-non-technical-family-is-a-headline-feature), not to the library).

**What must exist in v1 so this stays additive:** the rendition-list API shape; a job system
that can host an encode job; a transcode *reason* field the client can display even when the
answer is "we can't"; and a capability handshake so a server without the transcoding component
can say so — which has to exist anyway for hardware too weak to transcode.

**Rejected:** building the segmented transcoder first (the original Phase 0). It was ordered
first because it was the riskiest assumption; it is better handled by *measuring* the
assumption cheaply and deferring the build.

**Status:** Provisional, pending the playback survey. If the measured direct-play rate is low,
this decision is wrong and the transcoder returns to Phase 1.

---

## D18 — The Jellyfin-compatible surface is an adapter, and it quarantines device profiles

**Decided.** Build the Jellyfin-compatible API as an **out-of-process adapter**
([D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters)), in this repository
but shipped separately and optional. It is explicitly a **bridge, not a foundation**: our own
clients speak our own API, the adapter is allowed to be lossy, and if it ever starts constraining
the native API it gets frozen rather than extended.

**Why an adapter and not core.** The concern that it would constrain our model to what
Jellyfin's API can express is real and is the reason it must not be core. As a separate process
it constrains nothing: it translates what it can, returns sensible failures for what it cannot,
and its limitations are visibly *its* limitations.

**The part that makes it better than a bridge.** Jellyfin clients authenticate with Jellyfin's
auth and negotiate playback with a `DeviceProfile` — the exact design D2 rejects. The adapter is
therefore where a device-profile evaluator has to live: it accepts a profile and answers it from
our rendition list, rather than from a capability database. **That puts the 8,000-line problem
permanently outside the core, in a component we are free to abandon**, while still getting
Findroid, Swiftfin, Infuse, Streamyfin and Wolphin working against our server. Navidrome does
the mirror image of this deliberately, serving a Jellyfin-compatible API to borrow Jellyfin's
music clients.

**Consequences.** Byte-streaming is handled by redirecting to the core's media URLs, not by
proxying. The adapter needs the core's public API to be complete — a healthy constraint. Under
D17 the adapter can only offer what the core can produce, so a device profile demanding a
transcode the core cannot perform gets an honest failure rather than a broken stream; this is a
correctness improvement over guessing, and it is another reason the transcode-reason field must
exist in v1.

**Status:** Provisional. Schedule it after the native API stabilises; it may reduce the urgency
of the first-party TV client, and that trade should be re-examined once it exists rather than
assumed now.
