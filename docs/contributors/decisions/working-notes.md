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
platform turns out to have no usable runtime capability probe — a risk since
**substantially reduced** by [D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list),
which lets a client with a poor capability API ship a static list instead of probing.

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

**Status:** **Superseded by [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5)**
(2026-09-18). Re-examined as this status line invited: the concurrency argument does not survive
the actual write rate, most of the capability arguments evaporate, and the three cited failures
were respectively an application bug, unattributed, and a driver issue.

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

**Status:** Provisional; low risk, and cheap now versus expensive later. Note that this design
has **no consumer until a mobile client exists** — so it is unvalidated until then, and the
first offline-capable handset build is where it gets tested for real.

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

**Why.** ([tech-clients.md](../design/research/tech-clients.md);
**re-examined and corrected in [client-framework.md](../design/research/client-framework.md)**)
Streamyfin ships `modules/exoplayer-player` and `modules/mpv-player` side by side, with
`MPVLayerRenderer`, `PiPController` and `MPVNowPlayingManager` on Apple.

**Two corrections to this entry as originally written**, recorded rather than quietly fixed:
Kyoo was cited as independent validation, but **Kyoo has not shipped a TV client** (its README
says Android TV is "planned") — so it validates the pattern for phone and web only, and the
TV evidence is one project, not two. And the Streamyfin star count quoted elsewhere in this
tree (302) was its companion *plugin* repo; the application has **5,197 stars**, more than
either native competitor. The engines with the best format coverage (mpv, VLC) are not
the ones with the best platform integration (AVPlayer, Media3), so a client that wants both
must ship both. Playback cannot be shared JavaScript — it needs the hardware decoder, surface
compositing, PiP, media session, HDR passthrough and audio routing.

**Order:** v1 web + Android/Android TV. v2 Apple (iOS/tvOS), desktop, Chromecast receiver.
v3 Tizen/webOS/Kodi. Roku probably never — BrightScript shares no code and its decoder set
cannot render bitmap subtitles at all.

**Risk recorded:** `react-native-tvos` is a fork of React Native, not first-party. Re-checked
2026-09-18: releases track upstream within weeks (0.86/0.87), 15 open issues, and the commit log
is largely upstream React Native core contributors because it is a thin tracking fork. Better
than "community fork" implied — though a low issue count is also consistent with few users.

**The strongest counter-signal, recorded in full because it is recent and serious:** Shopify
announced on 2026-09-10 that it is **moving from React Native back to Swift and Kotlin** (HN:
1,272 points, 958 comments), explicitly because *"LLMs changed one of the core assumptions
behind our 2020 decision"*. That reasoning is uncomfortably close to this project's own premise.

It does not overturn D10, for three reasons argued in
[client-framework.md](../design/research/client-framework.md): it is a **resourcing** argument
and Shopify has hundreds of engineers where we have one person plus agents; the thread's
best objection — that parity is a *divergence* cost accruing across edge cases "which current
agents aren't great at tracking" — **cuts against a small team maintaining parallel native
codebases**, not for it; and it is an unproven plan eight days old.

Against it stands domain evidence pointing the other way: in the Jellyfin client ecosystem the
React Native client (Streamyfin, 2024, 5,197 stars) has **more adoption than either native
client** (Findroid 4,255, Swiftfin 4,171, both 2021), while Findroid is "not optimized for TV
use yet" and Jellyfin's own native Android TV app's top issue is a playback rewrite open since
2021. Measured share ratio in Streamyfin's tree: **31% native, 69% TypeScript** across 79
screens — high for the native fraction, but the "you drop into native anyway" objection is
still wrong as a blanket claim here.

**Kotlin/Compose Multiplatform considered and rejected** (Compose went stable on iOS in 2026, so
this is newly a real option): no tvOS target and no coherent cross-platform TV story, and it
does not help the web client we need regardless — it would solve the platforms we worry about
least and skip the one we worry about most, while adding a third language.

**Status:** Provisional, **re-examined 2026-09-18 and upheld**. The risk was never performance,
the fork, or code sharing — it is **TV focus navigation**, and the mitigation is unchanged:
prototype it before writing any screens, now testable on real hardware including the
first-generation touchpad Siri Remote. What is added is a falsifiable bail-out: reverse this
decision if the focus prototype feels wrong on the oldest Fire Stick, if `react-native-tvos`
falls two minor versions behind upstream for a quarter, if our own native fraction passes
roughly half, or if Shopify's migration is reported a success with numbers in a year.

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

**Status:** **Extended by [D23](#d23--axis-switching-is-mid-session-by-default-burn-in-is-the-one-thing-that-breaks-it)**
— scan-time extraction turns out to be the *enabling requirement* for mid-playback track
switching, not merely a latency optimisation, and closed captions need separate handling
entirely. Provisional; well-evidenced.

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

**Status:** **Amended by [D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)**
— the no-telemetry, no-ads, no-storefront, no-upsell half stands unchanged and is not
provisional. The absolute "no third-party account is ever required" does not survive contact
with NAT topology and TLS certificates, and is replaced by a falsifiable promise about *what* we
would ever charge for.

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

**Status:** **Researched; see [D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)**
and [remote-access.md](../design/research/remote-access.md). The topology is settled (there is no
discoverability without a signpost) and a tiered shape is proposed. What remains open is a
product-strategy question rather than a technical one: whether this project wants to operate a
hosted service at all.

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

**Status:** **Refined by [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)**
— two tiers become three, with the transcoder as the first member of the third.

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

**Refined by [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three):**
"separable optional component" is weaker than what we actually want. The transcoder is a
**plugin on the public API**, which makes the separation structural rather than a packaging
convention, and unlocks remote transcoding as a side effect.

**Rejected:** building the segmented transcoder first (the original Phase 0). It was ordered
first because it was the riskiest assumption; it is better handled by *measuring* the
assumption cheaply and deferring the build.

**Status:** Provisional, pending the playback survey. If the measured direct-play rate is low,
this decision is wrong and the transcoder returns to Phase 1. **Note how to read that
measurement:** the rate is per-client and the plan must survive the *worst* client in a
household, not the mean — an H.264-only Apple TV against a mostly-HEVC library is the
counter-example, and it sets the video transcoder's priority regardless of what the average
says.

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

---

## D19 — The transcoder is a plugin we happen to maintain; plugin tiers become three

**Decided.** The transcoder is not "an optional component" in a packaging sense
([D17](#d17--direct-play-first-transcoding-additive-and-optional)'s weaker formulation). It is a
**first-party plugin built on the public plugin API**, with no privileged access to the core that
a third party could not also have.

This refines [D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters) from two
tiers to three, because a transcoder fits neither existing one:

| Tier | Mechanism | Direction | Members |
| --- | --- | --- | --- |
| **1. Sandboxed extensions** | Extism / WebAssembly, explicit host-capability list | Core calls in, request-shaped | Metadata providers, scrapers, scrobblers, lyrics, filename-parser rules |
| **2. Protocol adapters** | Separate process, own listener, consumes our public REST API | **Inbound** — clients talk to it, it talks to us | Jellyfin-compatible API ([D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles)), OpenSubsonic, DLNA |
| **3. Capability services** | Separate process, registers with the core, may serve bytes to clients directly | **Outbound** — we call it; clients may be redirected to it | **The transcoder.** Later: an ML service, if photos ever happen |

A transcoder is long-lived, stateful and streaming, so tier 1 is wrong — pushing video through
a WASM boundary is pathological. It is not a protocol surface consuming our API, so tier 2 is
wrong by direction. Tier 3 is the shape it actually has, and it has confirming prior art:
Immich's machine-learning service is exactly this, and Kyoo's `gocoder` is a standalone
transcoding service already reused by two unrelated projects (Meelo, Blee).

### Why this is better than an "optional component"

1. **It keeps the plugin API honest.** The transcoder is the most demanding extension this
   project will ever have. If it can be built on the public API with no special privileges,
   the API is proven adequate; if it cannot, we find out while we still own both sides. This is
   the library-first / first-party-consumer-as-honesty-keeper argument. The counter-example is
   in the survey: Jellyfin's plugin API is weak partly because nothing load-bearing is built on
   it, so nothing ever forced it to be good.
2. **It unlocks remote and distributed transcoding, which nobody in this field has.** If the
   transcoder registers with the core rather than being linked into it, it does not have to be
   on the same machine. The NAS serves files; the desktop with a GPU transcodes. Several
   transcoders can register and the core picks by advertised capability (this one has NVENC,
   that one has QSV).

   **The demand is proven and the absence of a seam is proven with it:** `rffmpeg` — 1,073
   stars, created 2019, still maintained in 2025, **written by a Jellyfin founder** — is an
   out-of-tree Python wrapper that impersonates the `ffmpeg` binary and SSHes to a remote host,
   because Jellyfin's architecture offers nowhere to put this. A founder resorting to an
   impersonation hack outside his own project is about as clear as architectural evidence gets.
3. **It makes the degraded path the default path.** D17 already requires a "this server cannot
   transcode" answer. With the transcoder as a plugin, that answer is what an install returns
   until one is registered — so it is exercised constantly rather than being a rarely-taken
   branch. Untested fallbacks are where bugs live.
4. **It gives the licence and patent boundary a mechanism rather than a convention.** The GPL
   encoder build and the x264/x265 linkage live in a component the user installs deliberately
   (D16). A third party can ship an alternative — a commercially-licensed encoder, a
   vendor-SDK build, a hardware appliance — without touching us, and a user in a jurisdiction
   where this matters can simply not install one.
5. **It does not conflict with [D9](#d9--server-language-go-one-language-on-the-server).** The
   one-language rule governs *our server code*, and our transcoder will be Go. A defined network
   protocol is a boundary, not a language choice; a third-party transcoder can be anything.

### The hard part, stated honestly

**Who owns the rendition list?** The core knows the file, its probe results and its keyframe
grid. The transcoder knows what it can encode on this hardware. The list the client picks from
is a *join* of those two, computed per item. That join is the whole interface, and getting it
wrong yields either a leaky abstraction or a transcoder that cannot do its job.

**Largely defused by [D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list).**
With a closed, named output menu the transcoder's capability declaration is just *the set of menu
names it can produce on this hardware*, and the join becomes a three-way set intersection —
`client ∩ transcoder ∩ {original}` — rather than an open negotiation. The remaining interface
questions below still stand.

Secondary questions the interface must answer: how the transcoder reads media (shared mount, or
the core streaming bytes to it — the former is what everyone already does, the latter is what
makes a truly remote transcoder work); how capabilities are advertised and re-probed when
hardware changes; how a transcoder failing mid-stream is reported to the client; and how
segment URLs are signed so a redirect to a transcoder does not become an open proxy.

### Sequencing — the part that keeps this from becoming speculative generality

**Do not design the plugin API up front.** Build the transcoder as a separate process against a
*private* interface, get it working, and then **promote that interface to public once it has
survived contact with real hardware.** An API designed against one imaginary consumer is how
plugin systems become bad; an API extracted from one working consumer is how they become good.

Until promotion, the transcoder is a first-party service with an unstable contract, and that is
fine — it is not shipped in the first releases anyway (D17).

**Consequences.** The default Docker Compose ships the transcoder alongside Postgres, so the
install is still one command and the boundary is still real. The core must expose probe results
and keyframe data through its public API, because the transcoder needs them and must not get
them by reaching inside. Segment URLs need signing.

**Status:** Provisional. The interface is deliberately not specified here; it is an output of
building the thing, not an input.

---

## D20 — Plugin-architecture constraints, and four things the catalogue says belong in the core

**Decided.** Constraints on the plugin design, derived from surveying what people actually
extend and work around
([plugins-and-workarounds.md](../design/research/plugins-and-workarounds.md)). These are
**seams to leave room for**, not work to start: the plugin API stays private and unstable until
a first-party consumer has proven it ([D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)).

### The constraints

1. **The plugin boundary is a declared, versioned interface — never a link against server
   internals.** Jellyfin plugins are .NET assemblies compiled against the server's types and are
   "strictly 1 version compatible"; the standing user-facing answer to "I upgraded and my plugin
   vanished" is *wait for the author*. Intro Skipper serves a **manifest that varies by
   requesting server version** and branches its git repo per Jellyfin release. That is
   infrastructure built solely to survive a self-inflicted problem, and it is the ecosystem's
   largest recurring tax. WASM plus a semver'd host interface makes it impossible by
   construction; out-of-process tiers make it impossible twice.
2. **Capability names are part of the contract, and plugins are steps triggered by events.**
   Both, from the two positive models: Navidrome declares named capabilities (`MetadataAgent`,
   `Scrobbler`, `Lyrics`, `SonicSimilarity`, `TaskWorker`, …) plus an enumerated host-service
   list; Immich registers plugin **methods** against **workflow triggers** with a compatibility
   check, so the UI offers only the plugins that fit an event. "A plugin is a step in a pipeline
   fired by an event" is a far smaller contract than "a plugin is code with access to the
   server".
3. **Host services are a short list a human can read.** Immich's is six functions
   (`searchAlbums`, `createAlbum`, `addAssetsToAlbum`, `addAssetsToAlbums`, `httpRequest`,
   `bulkTagAssets`). The security surface must be reviewable at a glance.
4. **Metadata providers must be the easiest plugin to write, and the interface must not assume
   TMDB/TVDB shapes.** Providers are the largest category by count and by stars, and the most
   popular ones are **regional** — MetaShark (2,141), Bangumi (1,134), Douban (664), Danmu (659)
   are all Chinese-market and together outweigh every official provider combined. Different
   identity models, different title semantics, different episode ordering. An interface fitted to
   TMDB will fail the people most motivated to extend it.
5. **Plugin-to-plugin composition needs a designed answer**, or reflection becomes it — which is
   literally what happened: three popular Jellyfin plugins depend on `file-transformation`, and
   its README instructs authors to "use reflection to invoke the plugin" because assemblies land
   in different .NET load contexts.
6. **A client extension seam must be designed in.** Jellyfin's plugin API is server-side only,
   and the result is `jellyfin-plugin-file-transformation` (538 stars): an HTTP middleware that
   **rewrites the web client's served JavaScript and HTML on the fly** so other plugins can
   inject UI. It originated as Jellyfin PR #9095, which was not merged. Home sections (514),
   skins (659) and danmu (659) sit on top of it. A string-rewriter over a minified bundle is a
   fragile, security-relevant thing to be load-bearing — and it exists because the alternative
   was nothing. **If we do not design this seam, someone will build that.**
7. **One first-party catalogue, with signing.** Fragmented per-author manifest URLs produced an
   aggregator plugin (257 stars). Distribution is part of the design.

### Tier 3's specification arrived on its own

The most-starred plugin in the entire field is **Intro Skipper (2,773)** — Chromaprint audio
fingerprinting to detect intros and credits. Its requirements, read from its source: raw media
file access; **a pinned FFmpeg** (its README requires jellyfin-ffmpeg ≥ 7.1.3-1 — the same
dependency our transcoder has); long-running batch compute over the whole library; **its own
persistent state with its own schema migrations**; results published into the core's model; and
a way to put a button in the client UI.

That list is nothing like a metadata provider's, which confirms D19's three tiers were not
over-taxonomy. It is also almost exactly the transcoder's list, with a completely different
workload — so **intro/credit detection should be the second tier-3 plugin we write**. Two
unlike consumers is what validates a tier rather than fitting it to one.

### What the catalogue says belongs in the core, not in a plugin

A popular plugin for a thing every user wants is a bug report against the core. Four of these
are loud enough to change the roadmap:

- **Auth: OIDC/OAuth and LDAP.** A third-party SSO plugin at **1,458 stars** plus official LDAP
  at 224, and Navidrome's two most-requested issues are the same two things. **v1 requirement**,
  not a plugin.
- **A rules engine** — smart playlists, smart collections, retention policy. Roughly **7,000
  stars** across plugins (smartlists 363, auto-collections 292, mergeversions 439, media-cleaner
  260, tmdbboxsets 176) and companion apps (Kometa 3,416, Maintainerr 2,271, Janitorr 752). This
  is a core data-model feature and it must inform `library-model.md` before the schema is
  written, not after.
- **Good search.** A Meilisearch plugin at 329 stars against a core whose `Search results are
  limited to 24.` issue has been open since 2020. Already [D5](#d5--database-postgresql-only-shipped-in-the-default-compose);
  this says it is not optional.
- **Webhooks and an event bus.** A popular official plugin (248) *and* the foundation of
  Immich's entire plugin model. It is core infrastructure that the plugin system then consumes.

### The finding that outweighs all of the above

**The community's real extension mechanism is a separate program talking to the server's HTTP
API, not a plugin.** Roughly **80,000 stars** of companion applications — Sonarr 16,293,
Radarr 14,380, Seerr 12,615, Prowlarr 7,227, Tautulli 6,585, Lidarr 5,660, Bazarr 4,279,
Kometa 3,416, Maintainerr 2,271, Janitorr 752 — against roughly 20,000 across the whole Jellyfin
plugin catalogue.

So: **the public HTTP API is the most important extension point in the product by an order of
magnitude**, and it matters more than the plugin API. This raises the stakes on generating
OpenAPI from the code and gating the build on it being current
([D9](#d9--server-language-go-one-language-on-the-server)), and it is independent confirmation of
tier 2 ([D15](#d15--two-plugin-tiers-in-process-wasm-and-out-of-process-adapters)) and of the
adapter approach ([D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles)).

`rffmpeg` is the exception that proves it: every other companion app could be built because the
HTTP API had a seam, and remote transcoding could not, so it was built by impersonating the
`ffmpeg` binary and SSHing elsewhere. **When the API has no seam for something people want, they
do not give up — they do something worse.**

### The cautionary tale, and why it argues *for* D19

Plex sunset its plugin system in 2018, stating: fewer than **2% of users** used it; the protocol
was outdated and hard to support; and it no longer fitted the business.

- A plugin system nobody important uses is pure maintenance cost and will eventually be killed.
  **This is the strongest argument for D19**: with our own transcoder built on the public plugin
  API, that API can never be in the "<2% usage" position, because the product depends on it.
- The plugin boundary is a long-term compatibility commitment. Plex's was unsupportable seven
  years in. Keep ours small and versioned from the start.
- When a vendor's interests diverge from its users', the extension point is cut first. That is a
  governance argument for building on our own extension points in the open.

**Status:** Provisional, and deliberately not a specification. The four core-not-plugin items
are the actionable part now — auth and the rules engine both need to land in
`library-model.md` and the v1 scope before the schema is written.

---

## D21 — A fixed, named output menu, negotiated by a prioritised client list

**Decided.** The server's **output** formats are a short, fixed, project-owned **enumeration of
named profiles** — not a capability space. A client requests playback by sending a **prioritised
subset of that menu**; the server returns a manifest containing the intersection of what the
client asked for and what it can produce, always including the untouched original when it
matches. Every client must include the baseline and every transcoder must produce it, so the
intersection can never be empty.

### Why the asymmetry is real

**Inputs are unbounded** — a home library contains everything anyone has ever encoded, and
handling that is the hard, irreducible part of this project. **Outputs are not**, and it is
worth being precise about why: the output set is **already chosen for us by silicon**. Every
hardware encoder that matters supports the same three video codecs and nothing else worth
having — NVENC, QSV, AMF and VAAPI all do H.264/HEVC/AV1; VideoToolbox does H.264/HEVC.

So a broad output matrix was never available in hardware. Fixing a small menu **costs nothing we
could otherwise have had**; it only stops us writing software-encoder paths for formats no client
wants and no GPU accelerates.

### The menu

Named profiles bundling codec + profile + level + bit depth + transfer characteristics, because
those are not independently selectable in practice and naming the combination is what stops this
becoming a grammar:

- **Video:** `h264-sdr` *(mandatory baseline)*, `hevc-sdr`, `hevc-hdr10`, `av1-sdr`, `av1-hdr10`
- **Audio:** `aac-stereo` *(mandatory baseline)*, `aac-multi`, `ac3`, `eac3`, `opus`, `flac`
- **Container:** always fMP4/CMAF ([D3](#d3--streaming-format-cmaffmp4-segments-hls-manifest-no-drm)), so not a dimension.

**Resolution and bitrate are not menu entries.** They are what ABR varies *within* a chosen
profile. Conflating the two is part of what makes device profiles unmanageable.

**Dolby Vision and Atmos are deliberately absent** — passthrough-only. Direct-play them where the
client handles them; fall back to a menu profile otherwise. We never re-encode into them.

**AV1 is an entry, not a baseline.** Hardware decode reaches roughly 80% of consumer devices and
~88% of large-screen devices certified 2021–2025, but the remaining fifth is pre-2020 TVs and
older streaming sticks — precisely the long tail a home media server exists to serve.

### Why this is not a device profile in disguise

The most important thing to get right, since
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) exists to reject exactly that
design. The difference is categorical:

| | `DeviceProfile` | Output menu |
| --- | --- | --- |
| Client sends | An open-ended **description** of capabilities: containers, codecs, profiles, levels, bitrates, conditional rules | A **subset of the server's published enumeration**, in preference order |
| Server does | Evaluates **the media** against **the client's rules** to pick a play method | Intersects **two sets of names** |
| Grows by | Devices and users inventing new conditions | The project adding an entry, deliberately and under review |

The server never reasons about whether this client can play this file. It computes a set
intersection over a closed vocabulary. The precedent is boring and established: **HTTP `Accept`
with q-values**, applied to a media menu.

### The mandatory baseline makes negotiation total

Every client must list `h264-sdr` + `aac-stereo`; every transcoder must produce them. The
intersection therefore cannot be empty and negotiation cannot fail. This upgrades "a reasonable
assumption that we'll handle at least one of them" into a guarantee written into the contract,
which is worth considerably more than the assumption.

### What it fixes elsewhere

1. **Dumb clients stop being a problem.** D2's honest caveat was that the client picks, and a bad
   client picks badly. A Tizen, webOS or Roku client with a poor capability API no longer needs
   runtime probe-and-pick — it ships a static compiled-in list and is correct by construction.
2. **It largely defuses D19's hardest open question.** "Who owns the rendition list" becomes a
   three-way **set intersection** — `client ∩ transcoder ∩ {original}` — over a closed
   vocabulary, because the transcoder plugin's capability declaration is now just a set of menu
   names it can produce on this hardware. That is a far smaller interface than the open
   negotiation D19 anticipated.
3. **It makes the Jellyfin adapter easier** ([D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles)):
   mapping an inbound `DeviceProfile` onto our menu is *classification into ~11 buckets*, not an
   evaluation engine. The quarantined complexity shrinks.
4. **The manifest never advertises something that will fail**, because it is filtered against the
   client's own list before it is built.

### What it does not fix — stated so nobody is surprised later

**Most of `EncodingHelper.cs`.** That file is 8,042 lines mainly because of the
**hardware-acceleration matrix** — per-backend filter chains, pixel-format upload/download,
tone-mapping paths — not because of output-format breadth. A fixed menu removes a real slice of
the complexity and removes it permanently, but hwaccel remains the dominant cost and is handled
separately (one interface per accelerator family, principles §5).

### The failure mode to watch

**The menu must stay an enumeration and must never become a grammar.** The pressure will be
constant and each instance will sound reasonable: "this TV needs H.264 but capped at Level 4.0";
"this one does 8-bit HEVC but only at 30fps". Each concession is a *condition*, and conditions
are what `DeviceProfile` is made of.

The discipline: the menu is owned by the project and versioned. A device needing something not on
it takes the baseline, or someone adds an entry as a deliberate reviewed change. **If we ever find
ourselves adding a condition rather than an entry, we have rebuilt the thing D2 rejected** — and
that is the review question for any change to this list.

**Status:** Provisional. The initial menu contents are a starting point to be checked against the
playback survey (`plans/playback-survey.md`), which will say what real clients actually accept.
The *shape* — closed enumeration, prioritised client subset, mandatory baseline — is the part
that matters.

---

## D22 — Video, audio and subtitles negotiate as independent axes

**Decided.** The output menu from
[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) is **three
orthogonal axes**, not one joint vocabulary. A client sends a prioritised list per axis; the
server intersects each axis separately. Subtitles were already separate
([D11](#d11--subtitles-are-a-subsystem-not-a-feature)); this makes the split explicit and total.

### HLS already works this way

We are not inventing a mechanism. HLS declares alternate audio and subtitle renditions with
`EXT-X-MEDIA:TYPE=AUDIO|SUBTITLES` and a `GROUP-ID`, and video variants reference an audio group
by name. Every player we target — `hls.js`, ExoPlayer/Media3, AVPlayer, libmpv — handles this
natively. Separating the axes is the *conventional* shape; muxing them is the special case.

### Why it matters

1. **Additive, not multiplicative — and a joint menu would violate D21 on its own terms.**
   Five video profiles and six audio profiles are eleven names as separate axes and **thirty** as
   pairs. More importantly, a menu of pairs *is* a grammar: `hevc-hdr10+eac3` is a rule about a
   combination, and D21's stated failure mode is exactly the menu acquiring combinatorial
   structure. Orthogonal axes are what keep the enumeration an enumeration.
2. **The most common real transcode is audio-only.** Video fine, audio TrueHD or DTS-HD and the
   client cannot decode it. Joint negotiation makes that one indivisible choice and risks
   re-encoding video that was already perfect — the single loudest complaint about the incumbents
   ([sentiment.md](../design/research/sentiment.md)). Separate axes let us copy the video and
   transcode only the audio, which costs roughly one percent as much CPU.
3. **Multi-track is intrinsic to this domain.** A film commonly carries three to eight audio
   tracks — languages, commentary, descriptive audio — and users switch between them
   mid-playback. Under a joint vocabulary, changing language is a different variant and can
   restart or re-transcode the video. Jellyfin's trackers carry this as `Configurable default
   language for audio/subtitle` (+18) and `Selectable Audio and Subtitle Tracks Prior to Playback
   Start` (+18).
4. **Audio capability is dynamic in a way video capability is not.** The same device answers
   differently depending on the *audio path* — HDMI/ARC into a receiver that does E-AC-3, the
   TV's own speakers, or Bluetooth headphones — and that path changes mid-session when someone
   plugs something in. Video capability is a property of the decoder and does not move. Separate
   axes make a mid-session audio switch a cheap rendition change rather than a full
   re-negotiation.
5. **Music needs only one axis.** When the audio library arrives (Phase 7) it uses the audio axis
   and nothing else, with no null-video special case to carve out.

### The consequence that changes where code lives

**The licence boundary falls exactly between the axes.** Verified against FFmpeg's own
`configure` on 2026-09-17:

- `EXTERNAL_LIBRARY_GPL_LIST` contains **no audio encoder whatsoever** — it is `x264`, `x265`,
  `xvid`, `xavs`, `xavs2`, `davs2` plus non-codec tools (`avisynth`, `frei0r`, `libcdio`,
  `libdvdnav`, `libdvdread`, `librubberband`, `libvidstab`). Every GPL trigger is a **video**
  encoder.
- AAC, AC-3, E-AC-3, FLAC, ALAC and DCA all have **native LGPL encoders inside libavcodec**
  (`aacenc.c`, `ac3enc.c`, `eac3enc.c`, `flacenc.c`, `alacenc.c`, `dcaenc.c`) — no external
  library, no GPL rung.
- `libopus` appears on none of the GPL, version3 or nonfree lists. (`EXTERNAL_LIBRARY_NONFREE_LIST`
  is `decklink`, `libfdk_aac`, `libmpeghdec` — confirming
  [D16](#d16--gplv3-no-nonfree-components-and-provider-keys-belong-to-the-user)'s exclusion of
  `libfdk_aac`.)

**Therefore the entire audio menu is producible from a plain LGPL FFmpeg build**, and:

- **Audio transcoding lives in the core**, not in the transcoder plugin. No `--enable-gpl`, no
  x264/x265, and none of the video-codec patent exposure that §2 of
  [tech-licensing.md](../design/research/tech-licensing.md) is careful about.
- **The transcoder plugin is a *video* transcoder.** That is a smaller, sharper component than
  "the transcoder", with a clearer boundary.
- **The core therefore handles the common case and the plugin handles the rare one** — which is a
  much better split than the one D17 assumed. A default install with no plugin can already fix
  the TrueHD-on-a-cheap-TV problem, which is most of what real users hit.

This refines [D16](#d16--gplv3-no-nonfree-components-and-provider-keys-belong-to-the-user),
[D17](#d17--direct-play-first-transcoding-additive-and-optional) and
[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three) without
overturning any of them.

**Qualified by [D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp):**
audio encoding stays in the core *distribution* — packaging follows the licence — but it does
not run in the core *process*. The trust boundary does not follow the licence boundary, and all
FFmpeg work runs in a confined worker.

### Mechanics

- **Audio segmentation is easier than video, not harder.** Audio frames are independently
  decodable, so audio segments can always be cut on the video keyframe grid that
  [D4](#d4--three-playback-paths-direct-on-demand-segmented-background-prepared) already
  computes. Known bounded wrinkle: audio frame granularity does not divide arbitrary timestamps
  evenly (AAC is 1024 samples, ~21.3 ms at 48 kHz), so boundaries drift slightly; this is a
  standard, solved problem, not a design risk.
- **Whole-file direct play is unaffected and remains the v1 happy path** (D17). A ranged `GET` of
  the untouched container satisfies all three axes at once and bypasses HLS entirely.
- **Per-axis passthrough is a remux, not an encode** — copying a video track into fMP4 segments
  needs a muxer and no encoder, so it also belongs in the core.
- **Subtitles are the third axis** and were already modelled that way (D11): separate renditions,
  fonts as addressable attachments, never burned in by default.

### The risk to measure, not assert

Some older TV platforms are reported to handle **demuxed HLS** (separate audio group) less
reliably than muxed audio+video. That would be an argument for offering a muxed variant as a
compatibility fallback — a fourth thing the menu would have to express, which is unwelcome.

**This is not asserted here because it is not verified.** It is added to
`plans/playback-survey.md` as a question for real devices. If it proves true on a platform we
care about, the fallback is a muxed variant flagged on the video axis, and that concession must
be reviewed against D21's no-grammar rule.

**Status:** Provisional. The axis split and the LGPL/GPL boundary are solid; the demuxed-HLS
compatibility question is open and **now directly measurable** — several generations of Fire TV
Stick and an old Apple TV are exactly the older-firmware population the concern is about. The
dynamic-audio-path claim is likewise testable, and a **handset is the right device for it**,
where route changes mid-playback are routine rather than occasional
([test-hardware.md](../design/research/test-hardware.md)).

---

## D23 — Axis switching is mid-session by default; burn-in is the one thing that breaks it

**Decided.** Changing the subtitle track — turning captions on, switching language — and changing
the audio track are **normal mid-playback operations**, not session restarts. This is promoted to
an invariant over [D22](#d22--video-audio-and-subtitles-negotiate-as-independent-axes):

> **No operation on one axis may require restarting or re-negotiating another.**

There is exactly one thing that breaks it, and it is named and labelled rather than forbidden:
subtitle burn-in.

### What makes mid-session switching work

HLS declares every alternate rendition in the master playlist up front, so a client switches by
fetching different segments — no new session, no server round-trip beyond the fetch. That works
**only if every track is enumerated and already materialised when playback starts.**

Which means **[D11](#d11--subtitles-are-a-subsystem-not-a-feature)'s scan-time extraction is not
an optimisation — it is the enabling requirement for this.** If a user turns subtitles on forty
minutes into a film and the server only then starts demuxing a 60 GB remux to produce them, that
is precisely the complaint recorded in
[sentiment.md](../design/research/sentiment.md): "I will just add manually - can't, progress
indicator loops forever." The workaround an HN user arrived at independently — pre-extract
everything, disable serving embedded subs — is the correct default, and this decision is why.

Corollary: the subtitle axis is negotiated **once, at session start**. The client declares what it
can *render*; the manifest carries every matching track; the client then switches freely.

### Burn-in collapses the subtitle axis into the video axis

This is the sharpest available statement of why burn-in is a last resort, and it is stronger than
D11's original "it forces a video transcode":

If subtitles are burned into the picture, then turning them off, or changing language, requires a
**different video stream**. The axes are no longer independent, and the invariant above is
violated.

So burn-in is not a subtitle rendition at all — **it is a video rendition with the subtitle
selection baked in**, and it must be labelled as such in the manifest so a client can tell the
user that choosing it locks their subtitle choice for the session. It is offered only when a
client can render neither text nor bitmap subtitles and the content has no text alternative.

### The axis is logical; the transport differs by format

Worth stating explicitly, because "subtitles are an HLS rendition" is only true for one of the
four cases. Verified against FFmpeg's `hlsenc.c` and `libavcodec` on 2026-09-17:

| Format | Transport | Notes |
| --- | --- | --- |
| **WebVTT** | In-manifest HLS subtitle rendition | FFmpeg's HLS muxer uses the WebVTT muxer for subtitle variant streams. This is the mandatory baseline. |
| **ASS/SSA** | **Out-of-band sidecar + font attachments**, rendered by the client's libass | HLS has no ASS rendition type. Never silently convert ASS to WebVTT — it discards styling, positioning, karaoke and typesetting, which for anime *is* the subtitle. Offer a converted WebVTT as a separate, clearly degraded entry for clients without libass. |
| **PGS / VobSub** | Out-of-band bitmap, composited by the client | Burn-in only if the client can do neither. Note Roku cannot render bitmap subtitles at all — the one platform where burn-in is unavoidable, and a reason it ranks poorly in [tech-clients.md](../design/research/tech-clients.md). |
| **CEA-608/708** | **Not a track at all** — see below | HLS signals these with `EXT-X-MEDIA:TYPE=CLOSED-CAPTIONS`, distinct from `TYPE=SUBTITLES`. |

### Closed captions are a trap, and "turn on CC" is exactly where it springs

CEA-608/708 captions are **carried inside the video elementary stream** as SEI side data, not as a
separate track. Two consequences that no amount of subtitle-track handling will cover:

1. **Transcoding the video destroys them** unless side data is explicitly carried through
   (FFmpeg provides `vf_ccrepack` and A53 side-data handling for exactly this). Remuxing preserves
   them. So a broadcast recording or OTA rip that direct-plays with working captions can lose
   them the moment a bandwidth transcode kicks in — a bug that appears only under load, on only
   some content, which is the worst kind.
2. **They cannot be pre-extracted by demuxing**, because there is nothing to demux. They must be
   *decoded* out of the video stream (FFmpeg's `ccaption_dec`).

**Decision:** at scan time, detect embedded captions and **extract them into a real subtitle
track**, so they become an ordinary switchable rendition like everything else. Separately,
**preserve the in-stream captions on remux, and require the video transcoder plugin to carry
them through as a conformance condition** — a transcoder that silently drops captions fails its
tests, not the user's evening.

This decouples "turn on CC" from whether the video happens to be transcoding, which is the whole
point.

### Track semantics that have to be modelled, not bolted on

Each of these is a live complaint on an incumbent's tracker, and each is a property of the data
model rather than of the player:

- **Forced subtitles.** The subset shown only for foreign dialogue, flagged in the container
  (`AV_DISPOSITION_FORCED`). A different user intention from "subtitles on": forced tracks should
  auto-enable when the audio language matches the user's, and must be a flag on the track rather
  than a separate concept.
- **Default and preferred track selection, persisted.** Per user, and ideally per series: setting
  Japanese audio with English subtitles on episode 1 should carry to episode 2. Jellyfin carries
  this as `Configurable default language for audio/subtitle` (+18) and `Subtitle default option
  per user` (+12).
- **Per-track offset/sync adjustment**, which must survive a track switch. Requested on every
  client tracker in the survey.
- **Sidecars added after the scan.** A `.srt` dropped next to the film later must be picked up by
  the watcher and appear on next play at the latest.
- **Overlapping and multi-part tracks**, which Jellyfin carries as `Stack subtitles with
  overlapping timestamps` (+15).

**Status:** Provisional. The closed-caption extraction-plus-passthrough requirement is the part
most likely to be got wrong quietly, so it belongs in the playback survey's corpus and in the
transcoder's conformance tests rather than in a checklist somewhere.

---

## D24 — Language is a preference axis, and track selections stick to the series

**Decided.** Clients send a prioritised **language preference** alongside their capability lists.
An **explicit** track choice by the user is remembered as a re-resolvable descriptor and applied
to the rest of the series until they choose again.

### Language is a preference, not a capability — keep them apart

[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) and
[D22](#d22--video-audio-and-subtitles-negotiate-as-independent-axes) negotiate **capability**.
Language is categorically different and must not be merged into the same list:

| | Capability (codec profiles) | Language |
| --- | --- | --- |
| Nature | **Hard constraint** — the client physically cannot decode it | **Soft preference** — the client just wants it |
| Effect | *Filters* what the server may produce | *Orders* what already exists |
| On no match | Must not happen; a mandatory baseline guarantees it | Falls back down a chain; never fails |
| Scoped to | The **device** (and its current audio path) | The **user** |

The last row decides where state lives: capability is per-device and travels in the request;
language preference is per-user, device-independent, and therefore **server-side state that
syncs** ([D8](#d8--a-change-journal-and-delta-sync-cursor-exist-in-the-first-schema)). Setting
Japanese audio on the TV must be honoured by the tablet.

Pleasingly, the whole negotiation remains ordinary HTTP content negotiation: `Accept` with
q-values for the menu, **`Accept-Language` with q-values for language**. The boring, established
mechanism already has a header for this axis.

### Language matching is fiddlier than it looks — verified, not assumed

- **ISO 639-2 has two competing 3-letter codes for the same language**, bibliographic and
  terminological: `ger`/`deu`, `fre`/`fra`, `chi`/`zho`, `dut`/`nld`, `gre`/`ell`, `ice`/`isl`.
  Media containers use 639-2 and are inconsistent about which; BCP 47 uses 639-1. This is real
  enough that FFmpeg ships a **695-entry conversion table** (`libavformat/avlanguage.c`) and an
  explicit `AVLangCodespace` enum covering `ISO639_2_BIBL`, `ISO639_2_TERM` and `ISO639_1`.
  **Normalise every track's tag to BCP 47 at scan time, and keep the original** for provenance.
- **Script subtags are load-bearing**, above all for Chinese: `zh-Hans` and `zh-Hant` are not
  interchangeable for a reader.
- **Region subtags matter less but still matter**: `pt-BR` vs `pt-PT`, `es-419` vs `es-ES`. Use
  RFC 4647 basic filtering — prefix match on subtag boundaries — so a preference for `pt` matches
  `pt-BR`, but `pt-PT` does not match `pt-BR`.
- **Untagged and `und` tracks are common.** Resolution order: exact tag → language-only match →
  `und` → the file's own `DEFAULT` disposition → first track of that type.

### Language alone is not enough: remember (language, kind)

A track tagged `eng` may be the full dialogue, SDH, forced signs-and-songs, a director's
commentary, or audio description. Remembering "English" and re-applying it next episode is
therefore wrong often enough to be a bug.

FFmpeg's dispositions give the vocabulary, verified present in `avformat.h`:
`FORCED`, `HEARING_IMPAIRED`, `VISUAL_IMPAIRED`, `COMMENT`, `DESCRIPTIONS`, `CAPTIONS`, `DUB`,
`ORIGINAL`, `KARAOKE`, `DEFAULT`.

Real files set these inconsistently or not at all, so the **scanner** must also derive kind from
the track title — "English (SDH)", "Signs & Songs", "Commentary by the director" — and record
which source it used. That is scanner work
([scanner.md](../design/plans/scanner.md)), not player work.

### The rule that makes stickiness correct: remember the criteria, not the selection

**Never remember a track index.** Track 3 in episode 1 is a different language in episode 2. This
is a real and common bug class in existing systems.

Store a **descriptor** — `{language, kind}`, or the explicit value `off` — and **re-resolve it
against each new episode's actual tracks**. Three rules follow, each of which is easy to get
wrong:

1. **Fallback never overwrites the preference.** If episode 5 has no Japanese track and we fall
   back to English, episode 6 must still try Japanese first. A fallback is not a choice.
2. **Only an explicit user action updates the sticky preference.** Automatic selection never
   does, or the preference drifts silently toward whatever the files happen to contain.
3. **`off` is a choice, distinct from unset.** Turning subtitles off on episode 1 must turn them
   off on episode 2. The stored state is tri-valued: `off` / unset / `{language, kind}`.

### The resolution chain, most specific first

```
explicit choice in this session
  → remembered choice for this series          ← the user's ask
    → per-library default                      ← "anime: ja audio + en subs"
      → user's global language preference
        → the forced-subtitle rule (below)
          → the file's own DEFAULT disposition
            → first track of that type
```

**Per-library defaults earn their place**: an anime library wants Japanese audio with English
subtitles while a Western TV library wants English audio and no subtitles, and expressing that
once per library avoids doing it once per series.

**Sticky scope** is the nearest grouping ancestor: the series for an episode; the item itself for
a film. Collections are the obvious generalisation and are deliberately not built now.

### The forced-subtitle default rule, stated concretely

This is the sensible starting point of the chain and connects to
[D23](#d23--axis-switching-is-mid-session-by-default-burn-in-is-the-one-thing-that-breaks-it):

- Chosen audio language **≠** the user's primary language → enable **full** subtitles in the
  primary language.
- Chosen audio language **=** the user's primary language → enable **forced-only** subtitles
  (signs and songs) if such a track exists, otherwise none.

It is what people expect, and everything above it in the chain can override it.

### Two consequences for the data model

- **The subtitle axis selection is a set, not a single value.** Usually empty or one, sometimes
  two — dual subtitles for language learners is a real want (Jellyfin's `Dual Subtitles` carries
  +33). Allowing a set now is free; retrofitting it is a migration.
- **These preferences are user data and go in the change journal** (D8), so they reach every
  device and survive a rebuild ([D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index)
  — they are ours, not derived from the files, so they mirror to sidecars too).

### Say why, not just what

Per principles §3, the playback response reports what was pre-selected **and the reason**:
"English subtitles — you chose them for this series", versus "— your account default", versus
"— the file marks this track default". It costs almost nothing and it removes the entire "why
did it do that?" class of confusion. No incumbent does this.

### Evidence

Jellyfin's trackers: `Configurable default language for audio/subtitle` (+18), `Subtitle default
option per user` (+12), `Selectable Audio and Subtitles Tracks Prior to Playback Start, **and
Context Awareness**` (+18) — that last title is asking for precisely this decision — and `Dual
Subtitles` (+33).

**Status:** Provisional. The language-normalisation table and kind-inference heuristics need a
real-world corpus in the scanner's test suite; both are the kind of thing that is only ever
right against real files.

---

## D25 — Remote access is tiered, and the promise changes from "no accounts" to "we only charge for another machine"

**Decided.** Supersedes the absolute form of
[D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise) and gives
[D14](#d14--remote-access-for-non-technical-family-is-a-headline-feature) a shape.

**The topology fact is not negotiable.** Two machines behind different NATs cannot find each
other without a third party — neither knows its own public address as seen from outside and
neither can be contacted first. Tailscale's own data path makes this explicit: **every**
connection begins via a DERP relay to exchange endpoints, and only then attempts a direct path
in parallel. Direct connection is an optimisation over a rendezvous, never a replacement for
one. ([remote-access.md](../design/research/remote-access.md))

**And the collision starts earlier than remote access — it starts with HTTPS.** A home server
cannot get a publicly-trusted certificate for itself: no domain, and ACME will not issue for a
bare IP in any way a normal user can drive. Plex solved this by operating `plex.direct`,
resolving `1-2-3-4.<hash>.plex.direct` to `1.2.3.4`, and issuing each server a wildcard cert for
`*.<hash>.plex.direct`. **So a user who never leaves their LAN and never shares with anyone
still needs a third party to get warning-free HTTPS.** D13 was written thinking about remote
access; the certificate problem reaches it first.

### Three tiers

| Tier | What it is | Who runs it | Cost |
| --- | --- | --- | --- |
| **Local** | LAN access, self-signed or user-supplied certificates, server-local accounts, capability links | The user | Free, forever, no account |
| **Self-hosted assist** | The user runs their own rendezvous, relay, certificate story and IdP — **our software, their machine, documented** | The user | Free |
| **Hosted assist** | We run rendezvous, relay, `*.<hash>.<ourdomain>` certificates and a cross-server identity directory | Us | Subscription |

**The middle tier is load-bearing and is not optional.** If a user can self-host the signpost,
the paid tier is a convenience rather than a gate and the promise below is true. If it is
theatre, the promise is a lie. It also costs little, because the hosted tier runs the same
software.

### The restated promise

> **We charge only for things that genuinely require another machine.** Everything that can run
> on your own hardware does, for free, forever. No feature that works locally will ever be moved
> behind a payment. No telemetry, no ads, no storefront, no upsell in the UI.

This is better than "no accounts" because it is *falsifiable* and it names the exact category —
rendezvous, relay, certificates, cross-server identity — where a third party is a law of physics
rather than a business decision. It is also the precise inversion of the grievance against Plex,
which took a feature that already worked and paywalled it.

### The economics work, and the reason is the 90%

Signalling is nearly free. Relay carries video and is the expensive part — a two-hour film at
20 Mbps is about 18 GB — but at commodity European prices (Hetzner: roughly €1/TB beyond a large
included allowance) that is around **two euro-cents per relayed film**. Hyperscaler egress is
30–75× that and indie-PaaS egress is disqualifying, so **the provider choice matters more than
the architecture**. Above all, Tailscale's >90% direct-connection rate means relay is an
exception path rather than the service. A modest subscription covers this comfortably; the
binding constraint is abuse, not cost.

### Identity: sharing is the same question

Sharing with someone requires a stable identifier for that person that both ends agree on, and
that identifier has to live somewhere. **That is why Plex has an account system** — not because
Plex wanted a login. Every open competitor that skipped the account also skipped sharing:
Immich's sharing issue has 1,020 reactions under a feature freeze, PhotoPrism's 844 over seven
years, and Jellyfin users symlink subsets into fake libraries.

Four identity options, not exclusive, and all four are the design: **server-local accounts**
(always available, no identity across servers), **capability URLs** (unguessable scoped links;
excellent for one item, poor for ongoing access because they cannot be attributed or revoked per
person), **bring-your-own OIDC** (identity exists, we operate nothing, needs configuration the
target user cannot do), and **our optional directory** (maximum convenience, the premium tier,
and exactly what D13 promised not to require).

**Rejected:** keeping "no third-party account is ever required" as an absolute. It is keepable
only by abandoning the non-technical-family case, which is the single largest reason people stay
on Plex. Quietly weakening it later would be worse than restating it now.

**Consequences.** The software must **degrade to tier 1 rather than break** if a hosted service
disappears, with a documented and *tested* exit path — the audience for this project was burned
by exactly this kind of dependency. Signed, scoped, expiring media URLs become load-bearing
(also required by D19 and D26). Abuse handling must be designed before anything is operated.

**Status:** Provisional and the most consequential open question left. The *technical* half is
unusually cheap to settle: a handset on mobile data is a real symmetric-CGNAT client, and the
Proxmox hosts cover the server side, so every row of the mechanism table can be measured rather
than argued. **Whether this project wants to operate a hosted service at all is not a technical
decision** — it implies a company,
uptime obligations, liability and an entity that can be pressured. Declining it is coherent;
the cost is that the headline use case stays unsolved. Tiers 1 and 2 are worth building either
way, and building them first keeps the option open.

---

## D26 — No FFmpeg in the core process; confine it with Landlock + seccomp

**Decided.** **No FFmpeg invocation ever happens in the core server process.** Probe, demux,
subtitle extraction, thumbnailing and audio encoding all run in a confined **media worker**
child — the same shape as the video transcoder from
[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three), using
the same interface. The default confinement is **Landlock + seccomp-bpf**.

**Why: assume compromise, not prevention.** Every file in the library is attacker-supplied data
parsed by a large C codebase. FFmpeg has roughly **494 published CVE records since 2005**, with
recent years in the mid-20s to high-30s annually, dominated by memory-safety failures across
demuxers, decoders, protocol handlers and parsers. Two things make 2026 different: **discovery
is accelerating because LLM-driven analysis is now pointed at it** — published 2026 work
describes an AI agent finding 21 FFmpeg zero-days, and separate AI-assisted analysis finding
seven memory-safety flaws traditional tools missed — and **our typical user's library contains
files from places that are not a shop.** This is the same assumption every browser vendor makes
about their own media stack, which is why Chrome and Firefox sandbox codecs.

### The finding that changes the architecture

**The trust boundary does not follow the licence boundary.**
[D22](#d22--video-audio-and-subtitles-negotiate-as-independent-axes) put audio encoding in the
core because the GPL rung falls between video and audio. But probe, demux, subtitle extraction,
thumbnailing *and* audio encoding all parse hostile input, and all of them were going to run in
the core process.

Separate the two concerns:

- **Packaging follows the licence** — audio encoding ships in the core distribution, video
  encoding as the separately-installed plugin. D16 and D22 unchanged.
- **Process follows trust** — all FFmpeg work runs in the confined worker.

The core then **never parses attacker-controlled bytes at all**: it handles HTTP, SQL and JSON
and delegates every encounter with a media file. That is a far stronger claim than "we sandboxed
the transcoder", it unifies D19's worker interface with the trust boundary, and it costs a
process boundary we were already building.

### Why Landlock + seccomp rather than containers

The constraint was to pick a stance that does not consume the development budget. Modern Linux
provides **Landlock** (filesystem and network access control, kernel 5.13+) and **seccomp-bpf**
(syscall filtering) as *unprivileged, per-process* mechanisms requiring **no root, no namespaces
and no cgroups** — a process can be confined about as tightly as a container with no container
runtime installed. Maintained Go bindings exist for both (`landlock-lsm/go-landlock`,
`elastic/go-seccomp-bpf`).

The policy is small enough to audit: read one input file, write one cache directory, and — for
probe, subtitle and thumbnail work — **no network at all**. A decoder compromised under that
confinement can corrupt one cache directory.

Stronger rungs stay **opt-in**: a dedicated user plus systemd hardening (older kernels), a
container per invocation, a microVM. The strongest rung is already free — D19 lets the
transcoder run on another machine entirely, which is the "locked-down box that gets wiped"
option.

**Consequences.** Confinement capability must be **reported, not assumed**: the user can see
whether their install is confined, because Landlock needs kernel 5.13+ and macOS and Windows
need different mechanisms. The confinement needs an actual escape test in
[`testing-strategy.md`](../design/plans/testing-strategy.md) — a sandbox nobody verified is a
comment. And a **hardened FFmpeg build** that disables decoders nobody needs is free at build
time; the playback survey's format census tells us which those are.

**Status:** Provisional. The per-platform confinement matrix is the open part.

---

## D27 — Change detection is a strategy interface that reports which strategy it is using

**Decided.** Library change detection is a per-root strategy with an explicit capability report:
native filesystem watching where it works, a NAS-side index agent where offered, mtime-delta or
full walking otherwise, and an explicit rescan trigger always.

**Why.** `inotify` does not work over SMB or NFS — how a large share of self-hosters mount media
— has watch limits a big library exhausts, and **fails silently**, which is the worst property a
detection mechanism can have. "The scanner did not notice my new file" and "the scanner
rescanned everything at 3 a.m. and pegged the CPU" are the same bug seen from two sides.

**The reporting half is the important half.** The server says which strategy is active per
library root and warns when the chosen one is degraded or stale. The field's current failure is
that watching silently is not happening and nobody knows.

**The NAS index agent** — an agent on the NAS watching locally, maintaining an index file on the
share, with our server watching one file — is the only approach that genuinely solves the
network-share case, and it is architecturally free: it is a **tier-3 capability service** under
D19, the second independent case for that tier after the transcoder, which is good evidence the
tier is real rather than fitted to one consumer.

Its transport is the elegant part: **no network protocol, no port, no authentication, no
firewall rule** — both sides already have the share mounted. Its costs are real and recorded:
consumer NAS platforms are closed and architecturally fragmented so adoption will be a minority;
a stale or dead agent reproduces the exact silent-failure we are removing, so it must heartbeat
and the server must surface staleness; and the index needs write-temp-then-atomic-rename plus a
checksum so a reader never trusts a torn file.

**The load-bearing corollary: periodic walking must be good enough on its own**, because most
installs will use it. That makes its efficiency a requirement with a number in
[`performance-targets.md`](../design/plans/performance-targets.md), not a fallback afterthought.
If the product is unusable without the agent, the agent is a trap.

**Status:** Provisional. The agent is worth building later and with real demand — note that on
the author's own hardware both NAS units run Debian/Proxmox, so the server runs *on* the NAS and
native watching works; this solves other people's problem.

---

## D28 — Keep the exit from React Native cheap, and designate the fallbacks

**Decided.** [D10](#d10--clients-react-native-shell-native-player-modules-two-engines) stands,
and three rules preserve the ability to leave it. The fallback is **not** "go fully native" —
it depends on how React Native would fail, and the options are named in advance.

### The fallbacks, by failure mode

| How RN fails | Fallback | Why |
| --- | --- | --- |
| **TV only** — the likely case: focus navigation or performance on weak sticks | **Hybrid** — keep RN for web and phone, go native for TV (Compose for TV on Android, SwiftUI on tvOS) | RN's weak spot is exactly and only TV. Isolating the fallback to the platform that failed is far cheaper than a wholesale migration, and a TV UI is a small surface: rows, a grid, a detail page, a player. |
| **Generally** — the fork stalls or the ecosystem decays | **KMP for shared logic + native UI on every platform** | A proven path with real adoption. Web still needs its own client, unless Kotlin/JS or Kotlin/Wasm supplies its logic layer — possible, less-trodden. |
| We wanted maximum sharing | Compose Multiplatform | **Rejected: no tvOS target.** It solves the platforms we worry about least. |

**KMP is the designated general-purpose Plan B**, and the earlier dismissal of it in D10
conflated two different things. **KMP** (share logic, native UI everywhere) is conservative and
widely adopted — professional-developer usage roughly doubled in a year, 7% → 18%; production
users include Cash App, Forbes, McDonald's, Quizlet and Block's Bitkey; Google and JetBrains are
both invested, and Google being the vendor of one of the two mobile platforms is the part that
matters. **CMP** (share the UI too, via Skia) is a different and newer bet, and is the one that
fails our requirements.

**One claim qualified, because it is the one that looks most relevant and is not:** Netflix's
KMP usage is in its **Studio** applications — internal production tooling — not the consumer
streaming client. It is not evidence for KMP in consumer media playback.

**Flutter is rejected as a fallback**, which is worth stating because it is the more popular
framework overall: **no official tvOS support** (the request has been open since 2019), and
Android TV support that Flutter's own umbrella issue describes as having gaps in "tooling, I/O,
navigation, focus, core design language, documentation, samples, and tests". It is worse than
React Native for our target set. Note in passing that **webOS is now officially supported** by
an LG-shipped Flutter SDK, which is the one place Flutter leads — relevant only if the v3
webOS target is ever built, where **Lightning.js** is the stronger option anyway.

### Why the exit is cheap, and the three rules that keep it that way

**The expensive asset is already framework-independent.** The native player modules — player
engine, renderer, PiP, media session, libmpv and ExoPlayer integration — are plain Swift and
Kotlin behind a thin bridge. Streamyfin's 33,097 lines of them are not React Native code.
Migrating means dropping the wrapper and keeping the implementation. Combined with a generated
OpenAPI contract ([D9](#d9--server-language-go-one-language-on-the-server)), a closed rendition
menu ([D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list)) and a
small probe-and-pick interface
([D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)), **a framework switch is a
UI rewrite, not a system rewrite.**

That property is worth preserving deliberately rather than by luck:

1. **Native player modules carry no React Native types at their core.** Engine, renderer, PiP
   and media-session code are plain Swift/Kotlin; only a thin adapter knows about RN.
2. **The shared SDK stays framework-agnostic TypeScript** — no React or React Native imports in
   the API client, negotiation or offline logic. It must be usable from a plain web page, and
   the device-lab web harness ([device-lab.md](../design/plans/device-lab.md)) is the forcing
   function that proves it continuously.
3. **Track the native fraction.** Streamyfin sits at 31%; passing roughly half is one of D10's
   bail-out triggers and is only observable if measured.

None of these costs anything today. All three stop being available if ignored for a year.

**Status:** Provisional. **The open question is now closed by
[D29](#d29--a-native-client-reimplements-the-cheapest-cross-platform-logic-is-server-side-logic):**
a native TV client reimplements, and it is cheap because most of "the shared SDK" is generated,
server-side, or inherently platform-specific — and a TV needs no offline download, which is the
only genuinely hard shareable piece.

---

## D29 — A native client reimplements; the cheapest cross-platform logic is server-side logic

**Decided.** Closes the question
[D28](#d28--keep-the-exit-from-react-native-cheap-and-designate-the-fallbacks) left open. If the
hybrid fallback is taken and the TV client goes native, it **reimplements** — no bridge, no
shared core, no fourth language. And the reason it is cheap becomes a standing design rule.

### Why: "the shared SDK" is much less than it sounds

Decomposing it ([shared-core.md](../design/research/shared-core.md)) deflates the problem:

| Part | Status |
| --- | --- |
| API client | **Generated**, not shared — `openapi-generator` emits Kotlin, KMP and Swift from the same document ([D9](#d9--server-language-go-one-language-on-the-server)). Regenerating is not porting. |
| Capability probe | **Cannot be shared by nature** — `MediaCodecList`, `decodingInfo()`, VideoToolbox. Nothing is lost. |
| Rendition negotiation | **Mostly already server-side** — [D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) made it a set intersection over ~11 names, performed by the server. The client's share is tens of lines. |
| Language preference | **Server-side** ([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)). |
| Queue / resume | Shareable, moderate. |
| **Offline download** | **The one genuinely hard shareable piece.** |

**And the clincher: a TV does not go anywhere, so a TV client needs no offline download.** The
single piece of logic that would justify an exotic sharing mechanism is a *phone* concern, and
the hybrid fallback keeps phones on React Native.

This is not luck. D9's generate-the-client rule and D21's closed-enumeration negotiation made
the reimplementation small, and both were adopted for unrelated reasons.

### The mechanisms, including one verified dead

- **Embedding a JS engine** (Hermes standalone, QuickJS, JavaScriptCore) — **rejected.** It
  reintroduces the bridge the migration was escaping, ships another runtime, and on iOS
  JavaScriptCore outside a `WKWebView` cannot JIT. Only justified for a large shared core, which
  we do not have.
- **A Go core via `gomobile`** — **verified dead for us**, and it was the conceptually most
  attractive option because D9 already makes the server Go. From `cmd/gomobile/env.go`:
  `applePlatforms = []string{"ios", "iossimulator", "macos", "maccatalyst"}`. **No tvOS.** It
  fails on precisely the platform the fallback exists for. (Otherwise healthier than its
  reputation: maintained through September 2026, 37 open issues.)
- **A Rust core via UniFFI** — the strongest option on capability, and the only one that is not
  a bet on how the client question resolves: Swift (so iOS *and* tvOS), Kotlin and Kotlin
  Multiplatform, **Wasm for the web**, and **React Native Turbo Modules via Hermes bindings** —
  so it would serve the current architecture *and* every fallback. Used extensively by Mozilla
  in Firefox. **Not adopted**, because it is a fourth language against D9's one-language
  discipline, bought to hedge a failure that may not happen, for an offline state machine not
  yet written. Speculative generality.
- **A KMP core** — already D28's general-purpose Plan B. Larger adoption and a first-party
  vendor; weaker web story. Versus Rust/UniFFI, that trade should be re-run **with the offline
  module actually written**, not now.

### The durable rule this produces

> **The cheapest cross-platform logic is logic that runs on the server.**

Every piece of logic moved server-side needs porting zero times instead of three. This is
available to us in a way it is not available to most mobile applications, **because we ship the
server** — and it is the general form of what D21 and D24 already did by instinct.

Four things keep the §1 decomposition true, and it is a property of decisions rather than a law:

1. **The OpenAPI document stays the source of the API client** — generated, CI-gated, never
   hand-maintained. A hand-written client accumulating logic is how porting cost appears.
2. **Negotiation stays a closed enumeration intersected server-side.** D21's no-grammar rule now
   has a second reason behind it: conditions creeping into the menu grow the client's share and
   therefore the reimplementation cost.
3. **Offline logic is quarantined in its own module.** It is the one genuinely portable asset and
   is only portable if separable.
4. **Anything that can live on the server does.**

**Status:** Provisional. Revisit only if React Native fails generally rather than on TV — at
which point the KMP-versus-Rust/UniFFI comparison is a real decision with real inputs.

---

## D30 — tvOS is cheap from inside the Apple stack, and never has a web fallback

**Decided.** Two standing rules and a re-pricing, from noticing that tvOS was the consistent
failure point across every client-technology evaluation in this tree
([tvos.md](../design/research/tvos.md)).

### Why every cross-platform option failed on the same platform

Three structural reasons, and none is "the vendors haven't got around to it":

1. **There is no web runtime.** tvOS has **no WebKit, no `WKWebView`, no browser** — the classes
   to fetch a page exist, nothing to render one does. This kills Capacitor, Ionic, Lightning.js,
   Tizen/webOS-style HTML apps and the Jellyfin-style wrap-the-web-UI shell, in one stroke.
2. **The Focus Engine needs real UIKit views.** tvOS navigation is a platform-owned traversal
   over the view hierarchy, not an input-event stream the app interprets. **A canvas renderer
   presents no focusable views** — Flutter and Compose Multiplatform both paint through Skia
   into one surface, so the platform's navigation model has nothing to attach to. React Native
   works precisely because it maps to real UIKit views; `react-native-tvos` carries UIFocus
   handling in its Fabric component views plus `TVFocusEventHandler`/`TVEventHandler`.
3. **It is a whole engine port for ~8% of the market.** Flutter's maintainers list "an entirely
   new embedder in the engine, as well as test harnesses" plus buildroot/Dart/Skia changes, with
   wireless debugging as a prerequisite **because tvOS has no USB tether** — and label it
   **P3**. tvOS is about 8% of US primary streaming viewing, tied with Tizen, against Roku ~37%,
   Fire TV ~17%, Google TV ~14%.

Apple also **deprecated TVMLKit at WWDC 2024** — its own JavaScript/XML framework for TV apps —
directing developers to SwiftUI. Apple tried the scripted-TV-app path and retired it; no
blessed non-native route is coming.

### The measurement that re-prices the fallback

From the Swiftfin tree — the Jellyfin Apple client, 4,171 stars:

| Target | Lines | Files | Views |
| --- | ---: | ---: | ---: |
| `Shared/` | **59,018** | 604 | 89 |
| `Swiftfin` (iOS) | 12,361 | 116 | 93 |
| `Swiftfin tvOS` | **2,382** | **22** | **5** |

**~80% shared between iOS and tvOS; the tvOS-specific portion is ~3.2%.** So:

> **tvOS is expensive to reach from outside the Apple stack, and nearly free from inside it.**

The difficulty is not a property of the platform; it is a property of reaching it from a
non-Apple toolchain.

### The consequences

1. **[D28](#d28--keep-the-exit-from-react-native-cheap-and-designate-the-fallbacks)'s hybrid
   fallback is cheaper than it was priced.** Going native on Apple buys **iOS *and* tvOS for
   roughly the cost of iOS plus 3%**. That makes
   [D10](#d10--clients-react-native-shell-native-player-modules-two-engines) a safer bet, not a
   riskier one.
2. **The question to ask is "do we ever write Swift?", not "which framework reaches tvOS".** If
   yes, tvOS is nearly free. If no, it is a wall.
3. **Standing rule: never plan a tvOS story whose contingency is the web client.** The
   ship-the-web-UI panic button exists on every other platform we target — Tizen and webOS apps
   *are* web apps, Android TV has WebView — and does not exist here. Any plan that quietly
   assumes it is already broken.
4. **Standing test for any future client framework: canvas or platform views?** Canvas
   renderers will be absent or bad on tvOS; view-mapping frameworks will work. This predicted
   all six evaluations in this tree and costs nothing to apply.
5. **Skipping tvOS stays legitimate.** Spotify said in 2016 not to expect an Apple TV app "any
   time soon" and shipped one in **October 2025** — nine years — with no visible consequence;
   Immich and Kyoo have no TV client at all. But for *our* audience there is a counter-signal:
   the most-praised client in [sentiment.md](../design/research/sentiment.md) is **Infuse, which
   is Apple-only**, and HN threads repeatedly pair it with an Apple TV. The 8% figure probably
   understates the enthusiast segment this project is aimed at.

**Status:** Provisional. Rules 3 and 4 are the durable part and cost nothing. The re-pricing in
rule 1 should be re-checked against our own code if the hybrid is ever taken — Swiftfin's 3.2%
is one project's ratio, not a law.

---

## D31 — SQLite with a pure-Go driver; supersedes D5

**Decided.** **Supersedes [D5](#d5--database-postgresql-only-shipped-in-the-default-compose).**
The database is **SQLite**, in WAL mode, via a **pure-Go driver**. Postgres is not shipped, not
required, and not supported for now.

D5's own status line named it "the one most worth re-examining", so this follows that note
rather than reversing on a whim. Full working in
[sqlite-reconsidered.md](../design/research/sqlite-reconsidered.md).

### D5's evidence was partly misread

- **Audiobookshelf #5349** — a scan hit `SQLITE_BUSY` "and that rejection was **unhandled** —
  which took down the entire server." An **application bug**, not a SQLite limit. `SQLITE_BUSY`
  is the documented signal to wait and retry.
- **Jellyfin's lock complaints** — real pain, but **unattributed**. They arrived with the EF
  Core migration whose other symptom was transactions held open far too long. I presented
  contention as SQLite's semantics when it may have been Jellyfin's usage.
- **"core dump without SSE4.1"** — a **native-library** failure, which is a driver argument and
  disappears with pure Go (below).

### The numbers do not support the concurrency worry

The metric is writers, not users. Twenty concurrent streams at one progress update per ten
seconds is about **two writes per second**; SQLite in WAL mode does thousands of batched writes
per second. Three orders of magnitude of headroom. The worry was contention, and contention is
a design question — one **Navidrome has already solved in production** with Go, SQLite, a media
library and a scanner:

- `_busy_timeout=15000&_journal_mode=WAL&_foreign_keys=on&synchronous=normal`, pool sized
  `max(4, runtime.NumCPU())` because more connections buy no write parallelism;
- and the sharp edge worth stealing: **`SQLITE_BUSY` and `SQLITE_BUSY_SNAPSHOT` share the
  message "database is locked", and `busy_timeout` can retry the first and never the second** —
  a stale snapshot can only be recovered by rolling back and retrying the whole transaction.
  Inspect the extended result code to tell them apart.

### Most of D5's capability arguments evaporate

SQLite has excellent **FTS5** and has **JSONB**. `LISTEN`/`NOTIFY` is only needed *because*
Postgres is a separate process — in one binary it is a channel, which argues for SQLite.
`pgvector` is moot because photos are out of scope ([D1](#d1--scope-video-first-audio-second-photos-probably-never)).
Only **`pg_trgm`** is a genuine loss, and filename→title fuzzy matching belongs in the
identification component in Go anyway.

### The driver is not a detail — cgo would break a D9 promise

`mattn/go-sqlite3` is the most used, but **cgo breaks trivial cross-compilation**, which is one
of [D9](#d9--server-language-go-one-language-on-the-server)'s stated reasons for choosing Go,
and it reintroduces the native-crash class. So: **pure Go**, choosing between
`modernc.org/sqlite` (v1.59.0, actively released) and `ncruces/go-sqlite3` **by benchmark on the
low-end target**, not by reputation.

### What this buys, which D5 under-weighted

One binary and no second service — a large part of the product for self-hosted software.
**Backup becomes copying a file**, which closes a gap our own
[backup-restore.md](../design/plans/backup-restore.md) had flagged as an unpaid cost of D5. No
Postgres major-version upgrade dance, against "upgrades must be boring" being one of the
strongest findings in [sentiment.md](../design/research/sentiment.md). And RAM back on the 8 GB
Celeron that [test-hardware.md](../design/research/test-hardware.md) calls the honest low end.

### The multi-process objection dissolves, and pleasingly

[D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three) and
[D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp) put workers in
separate processes, possibly on separate machines — two processes writing one SQLite file over a
share would be a serious mistake. But those workers **must not touch the database**: they take a
job and return results over the core's API, and the core stays the sole writer. That is exactly
what D26's threat model wants anyway, since a confined process parsing hostile media should not
hold database credentials. **The architecture that makes the security story good is what makes
SQLite viable.**

### Conditions and remaining risks

1. **Refuse to run with the database on a network filesystem.** SQLite locking over NFS/SMB is
   unsafe; detect at startup and fail with a clear message rather than corrupt quietly.
2. **Retry-on-`BUSY_SNAPSHOT` lives in the repository layer**, once.
3. **Single-writer discipline**: serialise writes through one path; batch scanner writes into
   transactions.
4. **Adding Postgres later is not free** — `sqlc` emits dialect-specific code — but it is
   bounded because every query lives in the repository layer regardless.

**Rejected:** supporting both engines. That doubles the generated query surface and the test
matrix to serve a deployment shape that does not exist yet.

**Status:** Provisional. Postgres remains right for a genuinely multi-tenant or multi-node
deployment; if that ever materialises it arrives with a repository layer in place and a real
requirement to design against, which beats paying the operational cost now for a workload that
does not exist.

---

## D32 — Being told beats watching; the no-agent path is a tiered, measured, self-reporting walk

**Decided.** Refines [D27](#d27--change-detection-is-a-strategy-interface-that-reports-which-strategy-it-is-using)
by correcting its ordering and specifying the path for users who cannot or will not install
anything on their NAS — which is most of them.

### The correction: build the inbound endpoint first

D27 treated an explicit rescan trigger as a last-resort safety valve. That is backwards.
**Nothing that polls or watches beats the component that wrote the file telling us it did**, and
for this audience that component usually exists: the companion-app survey found roughly 80,000
stars of `*arr` ecosystem, and **Sonarr and Radarr already emit webhooks on "On File Import",
"On File Upgrade" and "On Import Complete", with paths in the payload.**

So the first thing built is **an inbound endpoint** — `POST` "this path changed", plus a CLI
command and a UI button on the same path. A few dozen lines, instant, zero running cost, and for
a large share of real libraries it is the entire feature. Polling becomes the fallback for
manually-managed content rather than the primary mechanism.

### The ladder

**A** be told (webhook/API/CLI) → **B** native watching (`inotify`/`FSEvents`/`kqueue`, local
only) → **C** SMB `CHANGE_NOTIFY` (deferred, see below) → **D** directory-mtime pruned walk →
**E** full `stat` walk as the safety net → **F** content hashing, on demand only.

The NAS index agent (D27) sits between B and D for those who will install it.

### Tier D is the workhorse, and it suits this domain specifically

A directory's mtime moves when an entry is added or removed, not when a file is edited in place.
For most workloads that makes mtime pruning unreliable; for a **media library it is close to
ideal, because libraries are append-mostly** — files are added, removed or replaced wholesale
and essentially never edited. The walk collapses from *O(all files)* to *O(changed
directories)*.

Caveats, each requiring tier E behind it: `rsync -t` and `mv` can **preserve** mtimes; in-place
modification is invisible at directory level; and some SMB/NFS configurations do not update
directory mtimes dependably.

### Optimise round trips, not bytes

Network-walk cost is **one round trip per operation**. At 1 ms RTT, stat-ing every file in a
100,000-file library is minutes; over Wi-Fi far worse. Therefore:

- **Never `stat` per file.** SMB2 and NFS carry attributes in the directory listing, so a
  correct walk costs one round trip *per directory*. In Go that means `filepath.WalkDir` and not
  calling `Info()` where the entry already has what is needed — an easy and expensive mistake.
- **Slice the walk** so it never blocks for minutes or presents as a stall.
- **Adaptive per-directory cadence.** Libraries are extremely skewed — a downloads folder
  changes daily, a 2004 films folder never will. Hot directories get checked often, cold ones
  fall to a slow cycle. A global interval is either too slow for the hot corner or too expensive
  for the cold mass.
- **Let the user declare a library archival or hot.** An explicit statement beats any heuristic
  and costs one config field.

### Probe at setup, and report the question the user actually has

Make D27's capability report measured rather than descriptive, when a library is added:

1. Write a temp file and check whether the parent directory's mtime moved — **a direct test of
   whether tier D can be trusted on this share**.
2. Time a listing of a few hundred entries to derive a per-directory cost and an **estimated
   deep-scan duration**.
3. Report protocol, chosen tier, estimated cost and planned cadence.

And extend the reporting requirement: under polling the user's real question is not "which
strategy" but **"when did you last actually look?"** Surface **last successful deep scan and
current staleness**, per library.

### SMB `CHANGE_NOTIFY` — deferred, with a trigger

SMB2 has a real change-notification primitive that would give instant, agent-free detection on
SMB shares. Two blockers, checked 2026-09-18:

- **No Go client implements it.** `CloudSoda/go-smb2` has the command constant and its
  `STATUS_NOTIFY_ENUM_DIR` error handling but **no request struct, no completion-filter
  constants and no public API** — a placeholder.
- **An OS `cifs` mount cannot expose it**, since the kernel does not surface SMB change
  notifications to userspace.

NFS offers no equivalent worth relying on (none in v3; v4.1 directory delegations are rarely
enabled).

**The contained design if it is ever warranted:** keep the OS mount for reading media — faster,
page cache, `sendfile` — and open a **separate minimal SMB session used only for
`CHANGE_NOTIFY`** on the library roots. That needs connect, authenticate, tree-connect,
open-directory and change-notify, not a full filesystem client. A real differentiator, since no
incumbent does it. **Trigger:** tiered polling proving inadequate for SMB users in practice,
*and* agent adoption being as low as expected.

**Status:** Provisional. The inbound endpoint and tier D are v1; everything else is ordering.
The `*arr` webhook integration should be documented as a first-class setup step, not buried —
it is the difference between instant and a poll interval for most of this audience.

---

## D33 — Hub-and-spoke: a core that never touches media, and N media nodes that do

**Decided.** The core does not scan, probe, or read media files. That work happens in a **media
node** — the same binary in a different mode — which runs wherever the files are. One core, N
nodes, whatever transport the topology allows. Design in
[media-nodes.md](../design/media-nodes.md).

### The consolidation is the reason to do it

The proposal was a standalone scanner binary. Followed through, **the scanner, the media worker
and the transcoder all want the same three things** — to be next to the files, to have FFmpeg,
and to be confined without database credentials. They are one component:

> A **media node** does everything that requires access to a media file: discovery, probing,
> subtitle and font extraction, thumbnails and trickplay, and optionally transcoding.

That merges [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)'s
tier-3 capability service, [D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)'s
media worker and [D27](#d27--change-detection-is-a-strategy-interface-that-reports-which-strategy-it-is-using)'s
scanner into one deployable thing.

**It also completes D26 instead of half-satisfying it.** That decision stopped FFmpeg running in
the core but left the core walking user filesystems, where directory entries are also
attacker-influenced. The core's entire input surface is now: HTTP requests, its own SQLite file,
and node reports validated at the boundary.

### One artifact, two roles — non-negotiable

`mediaserver serve` and `mediaserver node --core=<url>`: **the same binary**, and in the default
single-machine install **the core supervises a co-located node as a child process**. The user
installs one thing and runs one thing. Distribution is an opt-in topology change, not the
baseline. Anything else would undo [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5)'s
single-file simplicity, which was a large part of why SQLite won.

### It simplifies D27 rather than adding to it

D27 had the *core* selecting between four change-detection strategies per root, **plus** a
separate NAS agent — the same job implemented twice, with the distributed path second-class.

Now there is **one scanner implementation**, normally *local to its files*, so it uses
`inotify`/`FSEvents`/`kqueue` and the network-filesystem watching problem largely evaporates. It
survives only for users who will not install a node next to the files, and for them the node
runs on the core box and walks the mount using [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)'s
mtime-pruned strategy. **Same binary, same code, degraded mode.** D27's "NAS agent" stops being
a distinct concept — it is a node that happens to be on a NAS.

### Transport is pluggable, deliberately

HTTP POST node→core; core polling a node endpoint (for nodes that cannot dial out); **a report
file on a shared mount** (no port, no protocol, no auth, no firewall rule — the elegant option
when a share already exists); or a poke via signal file or API, composing with D32's inbound
endpoint that the `*arr` stack can already drive. The report *content* is one contract; how it
travels is deployment detail.

### Do not copy Kyoo's coupling

Kyoo already runs its scanner as a separate service — validating the "core doesn't scan" half —
but that scanner holds a **direct Postgres connection and its own migrations** alongside calling
the API. That violates D31's single-writer discipline and hands database credentials to the
component handling hostile files, which is precisely backwards from D26. **Nodes talk to the
core's API and nothing else; the core is the only writer.**

### The traps, and the one that matters most

1. **A dead node is not deleted media.** The failure mode that destroys someone's library. An
   offline node's items become **unavailable, never gone**: last-seen timestamps, explicit node
   state, and tombstoning **only** on a positive deletion report from a live node — never on
   silence.
2. **A node's report is attacker-influenced data** (paths and probe output derive from hostile
   files). Validate at the boundary; treat a node as a semi-trusted peer, not part of the core.
3. **Overlapping roots** — two nodes seeing the same share must dedupe, not double-count.
4. **Clock skew** across nodes if mtimes are compared.
5. **Idempotency and ordering** — a replayed or late report must not corrupt state.
6. **Version skew.** One artifact, but two running versions in distributed deployments. The
   protocol version and compatibility policy must be real, not aspirational.

### What it unlocks later at no cost

In a true multi-machine topology the core may have no media access at all, so it cannot serve
direct play. D19 already built **signed, scoped, expiring URLs with client redirection** for the
remote transcoder; the same mechanism lets a **node serve its own files** with the core issuing
the redirect. Not needed for v1 — pointless indirection on one machine — but "the core never
touches media, full stop" is reachable without new machinery.

**Status:** Provisional, and the most structurally significant decision since
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest). The report contract is the
real design work and is not specified here. Trap 1 deserves a test before the feature is
believed.

---

## D34 — A node status record that distinguishes dead from wedged from idle

**Decided.** Every media node maintains a small JSON **status record**, separate from its scan
report, that the core reads to reason about the node's health. Detail in
[media-nodes.md](../design/media-nodes.md) §The status record. This specifies the liveness half
of [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)'s
contract, and is what makes its trap 1 enforceable rather than aspirational.

### A heartbeat alone cannot detect the failure that matters

Three states share a fresh heartbeat and only one of them is fine:

| | Pulse | Progress | Verdict |
| --- | --- | --- | --- |
| Working | fresh | advancing | fine |
| **Idle** | fresh | static | **fine — nothing to do** |
| Dead | stale | static | process gone |
| **Wedged** | **fresh** | **static** | **alive, achieving nothing** |

So the record carries **three independent things**, not one:

- **`state`** — what the node believes it is doing (`idle`, `scanning`, `probing`,
  `transcoding`, `error`). This is what separates *idle* from *wedged*: static progress is
  correct when idle and a fault when scanning.
- **A liveness pulse**, updated on a timer regardless of work.
- **A progress signal**, updated *only* on completed work, at a granularity fine enough that a
  sensible timeout exists — per directory or per item, never per scan, since a large library
  legitimately takes an hour.

### Compare counters, not timestamps

Timestamps in the record are **for humans**. The core must not compare a node's clock to its
own — clock skew, NTP corrections and VM suspends all break that. The node emits a **monotonic
counter** with each signal, and the core stamps *its own* observation time when it first sees a
new counter value. Staleness is then measured entirely on the core's clock against a
monotonically increasing value.

### Why a file beats an endpoint here

1. **It survives the process.** An API dies with the node; a file still says what the node was
   doing when it stopped — which is precisely what post-mortem needs.
2. **It distinguishes three failure modes where an API gives one.** Stale counter → node died.
   File absent → never ran, or wrong path. Share unreachable → transport problem. An unanswered
   request cannot tell these apart.
3. **It needs no port, protocol or credentials** over the shared-mount transport.
4. **A human can `cat` it** — a cheap answer to "logging around library scanning is difficult to
   interpret", which is a direct complaint about Jellyfin in
   [sentiment.md](../design/research/sentiment.md).

### Requirements that make it work rather than half-work

- **Stable `node_id`, generated once and persisted.** A fresh id per restart makes the core see
  a new node every reboot while the old one goes silent — straight into the dead-node trap. One
  file per node id so nodes sharing a directory never clobber.
- **Atomic writes**: temp, `fsync`, rename in-directory, plus a length or checksum so a reader
  never trusts a torn file — and readers handle torn reads anyway, because SMB caching surprises.
- **Write on state transition *and* on a timer**, always populating `last_error`, so a node that
  dies mid-scan leaves behind what it was doing.
- **Status is not the report.** Two files, two cadences — small and frequent versus large and
  occasional. Conflating them makes the cheap thing expensive.
- `roots[].last_full_scan` lives here, which is where
  [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)'s
  "when did you last actually look?" reporting requirement naturally belongs — the node knows,
  so the node reports it.

### What the core does with it

Surface **dead and wedged differently**, because the remedies differ: dead means start it,
wedged means look at `state` and progress detail to see what it is stuck on. Mark an absent
node's items **unavailable, never deleted**. **Never tombstone on silence** — absence of a pulse
is absence of information, not evidence of deletion — and never act on a wedged node's reports,
which are by definition not current.

**Status:** Provisional. The thresholds (how stale is wedged, per state) need real measurements
from a large library before they are fixed, and D33's trap 1 deserves a test that kills a node
mid-scan and asserts nothing is deleted.

---

## D35 — Node exchange directory: progress pair, config generation, and permissions as the trust boundary

**Decided.** Extends [D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)
into the full node↔core contract: a task-defined progress pair, a bidirectional control channel
through the same directory, and a directory layout whose permissions *are* the security model.
Detail in [media-nodes.md](../design/media-nodes.md).

### Progress is a moving pair; either half moving means alive

Progress is **task-defined** — files, directories, bytes, items enriched — expressed as
`current` and `end`, where **`end` is an estimate allowed to move**. A scan's denominator grows
as directories are discovered, and that growth is itself evidence of life. The rule: **if either
number moves, the node is working.** Wedged is both static, for long enough, while `state` is
not `idle`.

Four things it needs to be usable rather than merely present:

- **A `unit` label**, or the UI cannot render it.
- **`end: null` is legal** — some tasks have no knowable end, and then only `current` signals.
- **An `epoch` that increments on task restart.** Without it, a reset to zero is
  indistinguishable from being stuck at zero, and a fresh scan gets called wedged.
- `current` monotonic within an epoch.

### The correction: idle must still tick

The progress pair alone is **not sufficient**. An idle node changes nothing; a dead node also
changes nothing. Watching only progress collapses the two into one state — which is exactly the
distinction D34 exists to preserve.

So the record keeps a **pulse that advances unconditionally, including when idle**. Progress
answers *"is it achieving anything?"*; the pulse answers *"is it there at all?"*. Dropping
either loses a state.

### Clock skew dissolves, and the formulation is simpler than D34's

D34 specified a monotonic counter compared against the core's observation time. The cleaner
statement: **the core times its own probes.** It reads the state file at `T1` and `T2` on its
own clock and compares contents; if any monotonic field changed, the node lived in that window.
Node timestamps become purely **decorative** — a `node_clock` field is worth keeping for
diagnostics ("your NAS clock is four hours out") but must **never** be an input to liveness
logic. This supersedes D34's framing without changing its conclusion.

### Bidirectional control through the same directory

The core writes `config.json`; the node watches and re-reads on change. Scan now, change the
interval, pause, rescan one root.

- **Acknowledgement is the part easily left out.** The config carries a **`generation`** the
  core increments on every change; the node echoes the last applied value as
  **`observed_config`** in its state file. Desired-state / observed-state, which is what makes
  the channel reliable over a shared directory that offers no acknowledgement of its own.
- **The node must poll the config file as well as watch it**, because the exchange directory may
  be on the share — exactly where watching does not work. One small file; the cost is nothing
  and it removes a silent failure.
- **Where HTTP exists, prefer it.** The file channel is the zero-configuration fallback; "scan
  now" in particular already has a better route through
  [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)'s
  inbound endpoint.

### The exchange directory, and why its permissions are the security model

```
<exchange-dir>/<node-id>/
  config.json   ← core writes, node reads + watches
  state.json    ← node writes, core reads
  reports/      ← node writes, core reads   (large)
  log/          ← node writes, core reads   (rotated)
```

**Namespaced by node id**, since several nodes may share one location; a duplicate `node_id`
from a copied config must be detected and reported, not silently interleaved. **Reports and logs
need a retention policy from day one** — an exchange directory that fills a share is a memorable
way to break someone's NAS.

The permissions split is the strongest part of this, because it matches the trust model exactly.
The node handles hostile media ([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)),
so it is the less-trusted half, and a careful user can enforce that with filesystem permissions
alone: core writes only `config.json`, node writes only its own outputs, media roots read-only.

**And that same table is the node's Landlock ruleset.** The directory layout and the sandbox
policy are one statement, which is good evidence both are right.

### Degraded modes, stated rather than discovered

- **Core cannot write the exchange directory** → no bidirectional control; the node runs on its
  own defaults and the core reads state only. The core must *say so* rather than silently
  failing to apply settings.
- **Node cannot write anywhere shared** → use HTTP, or run the node on the core's machine
  pointed at a mount ([D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)'s
  degraded mode).
- **Read-only for both** → the user hand-writes `config.json` and the core gets no liveness.
  Everything works; nothing is observable. Supported, and warned about.

When installed as a daemon, the only thing in `/etc` is the path to this directory.

**Status:** Provisional. Wedged thresholds still need measurement against a real large library,
and the report retention policy is unspecified.

---

## D36 — Idle ticks the progress pair; the node declares its own tick contract

**Decided.** Drops the separate pulse field from
[D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle) /
[D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary).
**An idle node ticks the progress pair itself** — enter idle at `current=0, end=1`, then
`1/2`, `2/3`, and so on. One mechanism, and the core's liveness rule becomes uniform: *did
either number move?*, with no branch on state.

### Why the simplification is right

Idle was the only state where the pair could not move, and it is also the only state where
ticking it **tells no lie** — there is no task whose completion the numbers could
misrepresent. So idle can satisfy the liveness contract using the same field as everything
else, and the separate pulse was carrying exactly one case.

The UI does not suffer: it already branches on `state` to choose its wording, so it simply does
not render a bar when `state == idle`.

### But a uniform *rule* is not a uniform *threshold* — and that is the real trap

Removing the pulse removes the state-dependent *mechanism*. It does not remove the
state-dependent *timing*, and assuming it does would make the core call healthy nodes wedged.

Progress rates differ by orders of magnitude between tasks. An idle node can tick every few
seconds. A node probing a 60 GB remux over a slow link may legitimately produce no movement for
minutes — and mid-task it **cannot** bump the numbers to prove liveness without lying about
completion, which is precisely the honesty that makes idle-ticking acceptable.

So the node **declares its own contract**: a field such as `max_tick_interval_s`, meaning *"I
guarantee at least one of these numbers moves this often, in my current state."* Short when
idle, longer during a slow probe, set by the component that actually knows.

The core's rule is then genuinely uniform **and** correct:

> Wedged when neither `current`, `end` nor `epoch` has changed for more than N × the node's
> declared interval — whatever state it is in.

One rule, one comparison, no per-state tuning in the core, and no lying in the node. That is
better than either the separate pulse or a fixed global threshold.

### Details that still hold

- **`epoch` covers restarts.** A node that restarts goes from `847/848` back to `0/1` —
  numbers moving *downward*. The core must read that as movement, and the incremented epoch
  from D35 is what makes it unambiguous rather than a suspected stall.
- Monotonic `current` within an epoch, unaffected.
- Unbounded growth is a non-issue: ticking every few seconds for a year is a rounding error.
- `state` remains the field that tells the UI and the human *what* the movement means.

### What is lost, honestly

A fixed-interval pulse would have let the core estimate node clock drift or spot process
pauses. Minor, and `node_clock` is diagnostic-only anyway
([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)),
so nothing depended on it.

**Status:** Provisional. `max_tick_interval_s` per state, and the multiplier N, both need
measurement against a real large library over a real share before they are fixed — the failure
mode of getting them wrong is calling a healthy slow node wedged, which is worse than a late
detection.

---

## D37 — The core judges liveness from observation, not from a node-declared interval

**Decided.** Drops `max_tick_interval_s` from
[D36](#d36--idle-ticks-the-progress-pair-the-node-declares-its-own-tick-contract). The node
reports **facts**; the core does the **judging**, from its own observations, on a graded scale.

### Three reasons, and the third is the one that settles it

1. **The core is better placed to know the cadence.** A declared interval is the node author
   guessing once, at build time, about an environment they cannot see — this NAS, this link,
   this library, this hardware. The core has something better: **history**. It has been watching
   this node, in this state, on this deployment, and can hold an empirical baseline of actual
   inter-movement intervals. An observed distribution adapts; a constant cannot.

2. **A single missed check is not an event.** D36 stated one threshold, which reads as a cliff.
   Most lateness is a big directory or a slow file. See the graded model below.

3. **A wedged node is not a reliable source of its own health thresholds.** This is the one that
   settles it. `max_tick_interval_s` asks the component *being judged* to supply the standard it
   is judged against — and the whole point of this machinery is detecting a node that has
   stopped behaving correctly. Self-reported health criteria are a smell, and the node that most
   needs catching is the one least able to report accurately.

### What the node does instead: be more specific about state

The node's real contribution is not a number, it is precision about **what it is doing**.
Prefer finer-grained states — `enumerating`, `probing`, `hashing`, `extracting`, `transcoding`
— over a single coarse `scanning`.

This is strictly better than a declared interval because it solves the same problem *and* two
others:

- **It gives the core a clean signal to learn against.** A coarse `scanning` that silently
  switches from fast enumeration to slow per-file probing produces a bimodal, useless baseline.
  Split states produce clean ones.
- **It covers the "about to go quiet" case** without a numeric field: `state: "hashing"`,
  `detail: "bigfile.mkv (61 GB)"` tells the core to widen its expectation *and* tells a human
  exactly what is happening.
- **It is legible.** Someone reading the file learns more from `probing` than from a number.

### Graded escalation, with hysteresis, and nothing destructive

| Level | Trigger | Action |
| --- | --- | --- |
| **Healthy** | Moving within the observed baseline | none |
| **Late** | Past expectation, briefly | **Nothing.** Perhaps a subtle UI hint. Most lateness is legitimate. |
| **Stalled** | Persistently late across consecutive observations | Surface in the UI with `state` and `detail`; stop treating its data as fresh |
| **Presumed dead** | No movement for a long multiple | Mark that node's items **unavailable** |

- **Hysteresis in both directions.** Require consecutive observations to escalate, and a clean
  run to recover, so the indicator does not flap.
- **The core chooses its own polling interval**, so "a missed check" is a core-side concept and
  can be made small in absolute time without making escalation twitchy.
- **No level is destructive.** Even *presumed dead* only marks items unavailable — tombstoning
  still requires a positive deletion report from a live, progressing node
  ([D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) trap 1,
  [D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)).

That last point is why the precision D36 was chasing was never needed: **the cost of guessing
wrong is a UI warning, not data loss.** Engineering exactness into a mechanism whose worst
outcome is a misleading status badge was over-design.

### Cold start

No history on first contact, so the core uses a conservative built-in default and refines it by
observation. Being wrong early is cheap, per the above.

**Status:** Provisional; simpler than what it replaces. The baseline model needs to be dull —
something like a rolling percentile of observed intervals per node per state — and explicitly
not a learned model anyone has to reason about.

---

## D38 — Three-tier state reporting, and an ordered command list in the config

**Decided.** Two refinements to the node contract: split what the node reports about itself into
**`state` / `task` / `detail`**, and give the config an **ordered list of commands** so that
"cancel and rescan" is a single atomic intention.

### `state` / `task` / `detail` — and which one the core baselines against

| Field | Nature | Used by |
| --- | --- | --- |
| **`state`** | Small, stable, coarse enum: `idle`, `scanning`, `transcoding`, `error` | API contract; UI headline |
| **`task`** | **Bounded enum** within a state: `enumerating`, `probing`, `hashing`, `extracting` | **The core's cadence baseline**; UI subtitle |
| **`detail`** | Free text: `films/the-unknown.mkv` | Humans only. **Never** keyed on. |

The important constraint, or [D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)
breaks silently: **`task` must be a bounded enum, and the core baselines on the `(state, task)`
pair, not on `state` alone.** D37's whole argument was that a coarse state which switches
between fast enumeration and slow probing yields a useless bimodal baseline. Moving the
fine-grained information into a free-text field would reintroduce exactly that problem while
looking like it had been solved — free text has unbounded cardinality and cannot be a baseline
key.

So: three tiers, with the middle one carrying the machine-readable precision.

### An ordered command list, because "cancel and rescan" is one intention

Real and frequent frustration: you add files, a scan is already running, and you must wait for
it to finish before you can rescan. So the config carries an ordered list —
`["cancel", "scan"]` — executed in sequence.

**A list rather than a single command is the right call**, because two separate config writes
would race: the core writes `cancel`, the node cancels and goes idle, and the second write may
or may not arrive before something else changes state. One list is one atomic intention.

Mechanically it rides on what already exists: the commands are part of the desired state, the
`generation` counter marks them, and `observed_config` acknowledges that they ran
([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)).
A closed vocabulary — `cancel`, `scan`, `rescan <root>`, `pause`, `resume` — not arbitrary
strings.

### The trap: a cancelled scan must never drive deletions

**This is the dead-node trap wearing a different costume, and it is the dangerous part of this
feature.**

A cancelled scan has enumerated part of the library. If its partial inventory is treated as
authoritative, every file it had not yet reached looks absent — and the core deletes most of
someone's library on the strength of a scan the user themselves interrupted.

So: **a scan report carries a completeness flag, and an incomplete inventory can only ever add
or update, never remove.** Same rule as
[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) trap 1 —
removal requires positive evidence from a complete, successful pass, never inference from
absence. This deserves the same test: cancel a scan mid-run and assert nothing is deleted.

### Other things cancellation needs to actually work

- **Poll for commands *during* work, not between phases.** If the node only checks at phase
  boundaries and a phase takes twenty minutes, "cancel" does not deliver "on a dime". Check at
  the same granularity as the progress tick.
- **Stop at a safe point and say so.** Abandon at a directory boundary, leave the report marked
  incomplete, and return to `idle` with a fresh `epoch` — which is exactly what the core needs
  to see that the cancel took effect.
- **`scan` while already scanning is a no-op**, or at most sets a rescan-when-done flag. It
  does **not** implicitly restart — which is precisely why the explicit `["cancel", "scan"]`
  form is needed and unambiguous.
- **Commands must be safe to re-run.** On crash recovery the node sees `generation` ahead of
  `observed_config` and re-executes. Every command in the vocabulary must tolerate that, which
  they do — and any future command that cannot must not be added to this channel.
- **Scope is the node's background work.** Playback-driven transcoding has a viewer attached and
  its own lifecycle; it is not cancelled by this channel.
- **Where HTTP exists, prefer it** — same as [D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk).
  This is the file-transport equivalent, not a competing mechanism. Note also that with the
  `*arr` webhook path in place, the "I just added files" case largely stops arising; cancel is
  for manual additions and for impatience, both of which are legitimate.

**Status:** Provisional. The completeness flag is the load-bearing part and should exist in the
report contract from the first version, because retrofitting it means a period where cancelled
scans can delete libraries.

---

## D39 — Reports are JSONL; completeness is in the filename and is scoped, not global

**Decided.** The scan report is **JSONL** — one JSON object per line, header first, entries,
trailer last. But the authoritative completeness signal is the **filename**, not the trailer,
and completeness is **per-subtree rather than per-report**.

### JSONL is right, and the best reason is not the obvious one

The obvious reasons hold: streaming write with no need to hold an inventory in memory,
streaming read at the other end, append-only so a crash leaves a valid prefix, and a human can
`grep` it — which matters given how much of this design is about legibility.

The strongest reason is different: **a single JSON document is all-or-nothing to parse, while
JSONL degrades per line.** This file is produced by the component that handles hostile media
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)), from filenames
and probe output that are attacker-influenced. One malformed record should cost one entry and
an error count, not the entire report.

### Completeness belongs in the filename

A trailer says "complete" by its presence — but its **absence** means crashed, cancelled, still
being written, or read raced the writer, and those are indistinguishable.

That ambiguity is at least *safe*: unknown → treat as incomplete → add-or-update only, never
remove ([D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)). The
fail-safe direction is the correct one. But it is still worse than knowing, and checking
requires reading to the end of a file that may have a hundred thousand lines.

So: the node writes `report-<epoch>.jsonl.partial` and **atomically renames** to
`report-<epoch>.jsonl` when the pass finishes. Completeness becomes an **O(1) check on the
directory listing**, and the core never has to parse a file that is still being written. It may
still read the `.partial` opportunistically for early adds and progress; it acts on removals
only for the final name.

**Keep the trailer anyway** — not as the completeness signal, but as the summary: entry count,
duration, per-root outcome, error count, and a checksum the core can verify. Cheap, and it
catches a truncated-but-renamed file.

And a **header line first**, carrying protocol version, `node_id`, `epoch`, report kind, and the
roots covered — so the metadata is available without reading to the end.

### Completeness is scoped, and that makes cancellation genuinely useful

The refinement that matters most here. A report is not simply complete or incomplete: **each
root, and ideally each subtree, was either fully enumerated or was not.**

- A scan cancelled after `/mnt/films` finished but before `/mnt/tv` started has *complete*
  coverage of films. **Removals within films can be applied safely**; tv is untouched.
- A scan that completed but could not read `/mnt/tv/Series X` because of a permissions error is
  complete everywhere *except* that subtree — and the files beneath it **must not be treated as
  deleted**. That is the dead-node trap at finer granularity, and it is the likeliest way to
  lose data in normal operation, because unreadable directories are common and undramatic.

So the report records **per-root (and per-errored-subtree) enumeration status**, and the core
applies removals only within regions positively marked complete. Without this, a cancelled scan
contributes nothing and cancellation is much less useful; with it, partial work still counts.

### Two report kinds, marked per line

- **Inventory** — a full enumeration of a region. Absence implies removal, *only* where that
  region is marked complete.
- **Delta** — explicit `added` / `changed` / `removed` events, per
  [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do)'s rule
  that deletions are stated, never inferred.

Each line carries its record type, so a reader never has to guess from context.

### Smaller points

- **Compression is worth it and does not break append.** Paths and probe output are highly
  repetitive; gzip's concatenated-stream property allows append-with-flush, so `.jsonl.gz` stays
  streamable.
- **Retention is easy with epoch-named files** — keep the last N — which discharges the
  obligation [D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)
  flagged.

**Status:** Provisional. Per-subtree completeness is the load-bearing part and, like the
completeness flag itself, belongs in the format from the first version — retrofitting it means
a period in which an unreadable directory silently deletes its contents from the library.

**Extended by [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)** — the report is the *snapshot* half of a snapshot-plus-journal pair. Every property here survives unchanged.

---

## D40 — `error` and `skip` records, and why the error record carries the completeness scope

**Decided.** The JSONL report gains two record types beyond header / entry / trailer:
**`error`** and **`skip`**. The error record is not merely diagnostic — **it is the mechanism
that expresses [D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)'s
scoped incompleteness**, and therefore the thing standing between a permissions glitch and a
deleted library.

### Errors belong in the stream, in order

Position carries meaning: an error record sits exactly where the failure happened in the
enumeration, so a reader — human or core — knows what was and was not covered around it. That
is strictly better than a side log, and it is why this is a record type rather than a separate
file.

### `scope` is the field the sketch was missing, and it is the important one

Not every error invalidates the same amount, and treating them alike would either lose data or
make cancellation useless:

| `scope` | Cause | Effect on removal safety |
| --- | --- | --- |
| `entry` | One file could not be `stat`ed or probed | That entry is unknown. **Siblings are unaffected** — the directory listing succeeded, so the region stays complete. |
| `subtree` | A directory could not be opened or listed | **Everything beneath it is unknown.** No removals anywhere under that path. |
| `root` | A whole library root is unreachable — share down, mount gone | No removals for that root at all. |
| `run` | The pass itself failed or was cancelled | Falls back to D38's rule: add-or-update only, except within subtrees positively marked complete. |

Without `scope`, a single unreadable file would have to invalidate its whole directory — or,
far worse, an unreadable *directory* would be treated as "no files here", which is precisely
the data-loss path D39 warns about. **This field is what makes per-subtree completeness
expressible at all.**

Note the probe case specifically: a file that enumerated fine but failed to probe **exists**.
It is present-but-unprobed, never absent. That is an `entry`-scoped error and must never
contribute to a removal.

### `code` is a project enum, not an errno

The sketch used `EBADF`. Raw errnos are the wrong contract: they differ across platforms,
Windows has its own set, Go wrapping makes extraction unreliable, and several relevant failures
have no errno at all — a parse failure, a timeout, a cancellation.

So: a **stable, closed project vocabulary** — `permission_denied`, `not_found`, `io_error`,
`timeout`, `unsupported`, `probe_failed`, `cancelled` — which the core keys its logic on, plus
an **optional raw field** (`syscall`, `errno`, driver text) carrying the platform truth for a
human diagnosing it. Same closed-enumeration discipline as
[D21](#d21--a-fixed-named-output-menu-negotiated-by-a-prioritised-client-list) and the command
vocabulary in [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config).

And **`path` is a structured field**, not prose embedded in `reason`. A path inside a sentence
cannot be aggregated, matched against a root, or acted on.

### Errors must be aggregated and capped

A share that disappears mid-scan can generate an error per file — a hundred thousand records
that bloat the report and drown everything useful in it. The error channel becoming a
denial-of-service on its own report is a real operational failure, not a hypothetical.

So: **collapse repeated identical `(code, scope, path-prefix)` errors into one record with a
`count`**, and impose a cap after which the node stops enumerating that root, emits a
`root`-scoped error, and moves on. Failing fast on a dead share is better behaviour than
faithfully logging its every file.

### `skip` is a different thing and must not be conflated

A file excluded by an ignore rule or an extension filter is **not** a failure — but it is also
**not in the inventory**, and if that is indistinguishable from an error the core will get the
policy wrong in one direction or the other.

So a separate `skip` record carrying the **rule that caused it**. The distinction matters
because the correct behaviour differs: a file the user *deliberately excluded* probably should
leave the library, while a file we *failed to read* definitely should not. Recording them as
the same thing means choosing one behaviour for both and being wrong half the time.

### Mirror the last error into the status file

The report carries the full ordered set for the core; `last_error` in the status file
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)) carries the
most recent one plus a count for the human who is glancing at it. Two audiences, two views,
one source.

**Status:** Provisional. `scope` is the load-bearing field and belongs in the format from the
first version, for the same reason as the completeness flag itself — without it, an unreadable
directory is indistinguishable from an empty one.

---

## D41 — Diagnose the failure boundary by walking up; and guard the silent case, which is worse

**Decided.** On a read failure the scanner **walks up the tree to find the smallest broken
subtree** rather than reporting per-file errors, and separately guards against the mount failure
that produces **no error at all**.

### Escalate by probing, not by counting

When `/mnt/tv/Series X/S01E01.mkv` fails, the useful question is not "log this" but **"what is
actually broken?"** So the scanner stats the parent, then its parent, upward, until something
succeeds. The highest failing ancestor is the boundary, and that boundary is exactly
[D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)'s
`scope`:

- parent readable → `entry` scope; carry on with the siblings.
- `Series X/` unreadable → `subtree` scope; emit one record and skip the rest of it.
- `/mnt/tv` unreadable → `root` scope; abandon the root.

Path depth is small, so this converts ten thousand failures into one diagnosis plus a handful
of `stat` calls. **This supersedes D40's error cap as the primary mechanism** — the cap was a
blunt instrument that gave up after N failures; escalation finds the actual boundary and is
both cheaper and more precise. Keep the cap as a backstop for pathological cases only.

**Memoise the failed prefix for the run**, or the walk-up repeats for every subsequent file
beneath a directory already known to be gone.

### The silent case, which is the real danger

An unmounted mountpoint on Linux usually reverts to **the empty directory underneath it**. So:

- The path exists.
- It is readable.
- Listing it **succeeds** and returns **zero entries**.
- **No error is raised. `scope` never fires. The walk-up finds nothing wrong.**

A naive scanner then reports a *complete* enumeration of a root containing nothing — and every
removal-safety rule built so far permits the core to delete the entire library, because the
report is honestly and correctly marked complete.

**This is the most dangerous failure mode in the design**, precisely because every mechanism
added so far is triggered by errors and this one has none.

Two independent defences, both required:

1. **Verify the mount, not the path.** Record each root's device id (`st_dev`) at setup. A
   mountpoint has a different device id from its parent directory; if they have become equal,
   the mount is gone. This is automatic, needs no user action, and is checked before a root is
   enumerated and again before its removals are applied. A user-placed sentinel file is the
   portable fallback where device ids are not available.

2. **Never believe implausible shrinkage.** Independently of mounts: if a root has lost a large
   fraction of the entries it had at the last complete scan, **the report is marked incomplete
   and removals are withheld pending confirmation.** This catches the whole class of
   no-error failures — a wrong path after reconfiguration, a permissions change that empties a
   listing, a half-finished remount, a typo'd config.

   It must **ask**, not refuse: a user who genuinely deleted a library should be able to say so
   in the UI. Silently refusing is its own bug, just a quieter one.

### A hung mount presents as wedged, not failed

NFS hard mounts block indefinitely rather than returning an error, so filesystem operations need
timeouts — and a timeout is a `subtree`- or `root`-scoped error, not a hang.

Pleasingly, the existing machinery already covers the gap: a scanner blocked on a dead mount
stops moving its progress numbers while its state stays `scanning`, which is exactly the
**wedged** signature from [D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle).
The core will surface it even if the timeout logic is imperfect. `ESTALE` on NFS escalates the
same way as a lost mount.

### Recovery restores; it does not re-add

When a mount returns, items previously marked unavailable must be **restored**, not ingested as
new — which requires the stable identity
[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) and
[D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)
already mandate. Re-adding would lose watch state, user metadata corrections and playlist
membership, which is a data-loss event wearing a recovery costume.

**Status:** Provisional. The shrinkage threshold needs a real number and should probably scale
— a root of 20 items losing 10 is noise, a root of 40,000 losing 39,000 is not. Both defences
belong in the first version; the silent case cannot be retrofitted safely because by the time it
is noticed the deletions have happened.

---

## D42 — Nodes are sensors; the core is the only thing that decides

**Decided.** A general principle, and a correction to
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse):

> **A node reports what it observed. The core decides what it means.**

D41 put the mount check and the implausible-shrinkage guard *in the node* — "mark the report
incomplete and withhold removals pending confirmation". That is the node making policy, and it
is wrong in four separate ways.

### Why the node cannot and should not judge

1. **It lacks the information.** The shrinkage guard needs *"what did this root contain at the
   last complete scan?"* — which is **core state**. For the node to judge, it would have to keep
   its own history of every root, duplicating the core's inventory and creating a second source
   of truth about the library. The core already knows. This is the decisive argument.
2. **It is the component under suspicion.** A node whose mount just vanished is not who you want
   ruling on what that means — the same reasoning as
   [D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval),
   where a node declaring its own health thresholds was rejected because the node most in need
   of catching is the one least able to report accurately.
3. **Policy lives where the policy is configurable.** The confirmation prompt, the user's
   preferences, the override — all in the core. A node withholding removals puts the decision
   and its override in different processes.
4. **Only the core can reconcile.** With overlapping roots or several nodes seeing one share, a
   node judging in isolation could withhold removals another node has good data for.

### The reframing this produces, which is cleaner than D41's version

The silent-mount problem is **not** "the node must detect a missing mount". It is:

> **The node must report "I listed this directory and it was empty" as distinct from "I did not
> enumerate this directory".**

That distinction is an *observation* only the node can make — and once it is in the report, the
core, which knows the root held forty thousand files yesterday, draws the conclusion itself.
The node needs no threshold, no history and no confirmation mechanism.

That is a genuine simplification of the node, which is usually the sign the boundary is now in
the right place.

### What the node reports — facts only

- **Observed-empty vs not-enumerated**, per directory. The fact above.
- **Filesystem observations** at each root: current `st_dev`, whether the path is a mountpoint,
  filesystem type, read-only flag. The node reports the *current* value; **the core** compares
  it against what it recorded and decides whether a change is alarming.
- **Errors with `scope`** ([D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)).
- **Whether the pass finished**, was cancelled, or died.

### The line between fact and judgement, which needs care

"This subtree errored" and "this scan was cancelled" are **facts the node owns** — factual
incompleteness, and it stays in the report and in the filename
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)).

"This shrinkage is implausible" is a **judgement requiring history**, and moves to the core.

The test: *could the node determine this from what it saw during this one pass, with no memory
of previous passes and no knowledge of the library?* If yes it is a fact; if no it is a
judgement and belongs in the core.

### It generalises, and it buys testability

Every input to the core is a sensor — media nodes, `*arr` webhooks
([D32](#d32--being-told-beats-watching-the-no-agent-path-is-a-tiered-measured-self-reporting-walk)),
plugins, user actions. If judgement is scattered across them, nobody can reason about how the
library reaches its state.

Concentrating it also means **the core's decision logic can be tested against synthetic reports
with no filesystem, no nodes and no hardware at all** — a large win for
[testing-strategy.md](../design/plans/testing-strategy.md), where the standing problem is that
most of this product's correctness lives somewhere a unit test cannot reach. Removal safety is
exactly the logic that most deserves exhaustive testing and is now exactly the logic that can
be.

**Status:** Decided, and promoted to a principle
(`principles/README.md` §9) because it is cross-cutting rather than specific to scanning.

---

## D43 — One binary, one process per role, and a thin optional supervisor

**Decided.** The single binary runs **one role per process**. A machine running a core, a node
and a transcoder runs three processes, not one process with three goroutines. The binary can
supervise them itself for convenience, but that supervisor is thin and optional.

### Core and node are separate processes even when co-located

Already implied by [D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do),
and worth stating why:

- **The boundary should be enforced by the operating system, not by discipline.**
  [D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) says nodes are sensors
  that must not hold database credentials or make policy. In one address space that is a
  convention a tired afternoon can breach; in separate processes it is impossible.
- **The local and distributed cases must be one code path.** If co-located means goroutines and
  remote means processes, there are two implementations and the local one — the common one — is
  the one that never exercises the real protocol.

### Roles within a machine are also separate, and Landlock is the clincher

The tempting design is one process per machine with roles as goroutines. It is wrong, and the
reason is concrete rather than aesthetic:

**Landlock is per-process and monotonic** ([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)) —
a ruleset can only be tightened, never relaxed, and it applies to the whole process. Different
roles need genuinely different rulesets:

| Role | Needs |
| --- | --- |
| Scanner/probe node | read media roots; write its exchange directory |
| Transcoder | read media roots; write a cache directory; **access GPU device nodes** (`/dev/dri`, `/dev/nvidia*`) |

Combining them into one process means applying **the union** of those rulesets — a strictly
weaker sandbox, in which the component doing the riskiest work carries permissions it does not
need. Splitting the processes is what lets each be confined to exactly its job.

Three lesser reasons in the same direction: **resource isolation** (transcoding is long and
CPU/GPU-heavy, scanning is bursty and I/O-heavy, and they want different `nice`/cgroup
treatment); **failure isolation** (a GPU driver hang should not take down the scanner); and
**optionality** — [D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)
makes the transcoder a separately-installed plugin, and "not installed" is cleanly expressed as
"not running".

The memory objection does not bite: a Go process idles in the tens of megabytes, so three of
them is noise even on the 8 GB NAS that
[test-hardware.md](../design/research/test-hardware.md) calls the honest low end.

### Two invocation styles, because two audiences

```
mediaserver run                 # reads config, supervises every enabled role
mediaserver core                # exactly one role, foreground
mediaserver node
mediaserver transcode
```

`run` serves the default install: one thing to start, one thing to stop. The single-role forms
serve anyone who would rather use **systemd units, separate containers, or Kubernetes** — which
is the right way to do it if you already have a supervisor, and it must not be second-class.
One config file listing enabled roles; each role reads the sections it needs.

### The supervisor must stay thin — this is a boundary, not a feature

Fork the enabled roles, pipe their stdio into the log stream, restart on unexpected exit with
backoff, forward signals, exit when told. **Roughly two hundred lines.**

What it explicitly is **not**: a dependency graph, service discovery, socket activation, health
checking beyond process liveness, or anything resembling an init system. Every one of those
exists already in systemd, Docker and Kubernetes, and reimplementing them badly is a known way
to spend a year.

**PID 1 duties, which are a classic bug.** In a container the supervisor *is* PID 1 and must
**reap zombies** and **forward `SIGTERM`/`SIGINT` to children**. Getting this wrong produces
containers that accumulate defunct processes and take ten seconds to stop because nothing
propagated the signal.

### Local IPC uses the remote mechanisms

Co-located roles talk over the **same protocol as the distributed case**, defaulting to a
**Unix socket** locally rather than TCP. Two benefits: the common deployment exercises the same
code the rare one depends on, so the distributed path cannot rot unnoticed; and the socket
file's permissions are the authorisation, which is the same
"[permissions are the security model](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)"
pattern already adopted for the exchange directory.

### The resulting process tree

```
mediaserver run                    (thin supervisor; PID 1 in a container)
├── mediaserver core               SQLite, HTTP, decisions
├── mediaserver node               → forks confined ffprobe/ffmpeg per job
└── mediaserver transcode          → forks confined ffmpeg per job
```

FFmpeg remains a **confined child per invocation** (D26), unchanged: the role processes
supervise it, they do not host it.

**Status:** Provisional. The supervisor's scope is the thing to guard — every feature request
for it should be answered with "use systemd" until there is evidence otherwise.

---

## D44 — Prefer the platform's supervisor; ship the units; and never exit when you could report

**Decided.** Restart is a **deployment** concern, and the right answer is usually not ours.
Order of preference, and we **ship** the first two rather than documenting them:

1. **systemd units** (bare-metal Linux) — restart, watchdog, sandboxing and cgroups for free.
2. **Separate containers** (Compose / Kubernetes) — restart policies and per-container limits.
3. **Our `run` supervisor** ([D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor)) —
   the fallback for everything else: one container holding all roles, Windows, macOS, or plain
   simplicity.

The supervisor exists because a fallback must exist, not because it is the recommendation.

### What systemd gives us, beyond restarting

- **`Restart=on-failure`** with `RestartSec=`, plus `StartLimitIntervalSec=`/`StartLimitBurst=`
  (5 starts in 60 s by default) so a crash-looping unit **parks in `failed`** rather than
  spinning forever. Deliberate and correct — and it means the failure is only visible if
  something is watching, which the core is
  ([D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)).
- **`Type=notify` + `sd_notify(READY=1)`** for real readiness rather than "the process started".
- **`WatchdogSec=` + `WATCHDOG=1`** — active liveness, killing and restarting a stuck process
  that exit-based restart cannot catch. See the nuance below.
- **Per-unit sandboxing** — `ProtectSystem=strict`, `ReadWritePaths=`, `ProtectHome=`,
  `NoNewPrivileges=`, `SystemCallFilter=`, `DeviceAllow=`, `PrivateTmp=`,
  `MemoryDenyWriteExecute=`.
- **Per-unit cgroups** — `MemoryMax=`, `CPUWeight=`, `IOWeight=`, `Slice=`.
- **`StateDirectory=` / `CacheDirectory=` / `LogsDirectory=`**, created and owned correctly —
  which is where the exchange directory
  ([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary))
  should live.

**Two of these validate [D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor)
from a second direction:** systemd's sandboxing and resource control are **per unit**, exactly
as Landlock is per process. Three roles as three units get three different confinement profiles
and three resource budgets. One unit running all three would take the union — the same argument
that made D43 split the processes, arriving independently.

It also means **the unit file is the outer sandbox on a systemd host**, with D26's in-process
Landlock as defence-in-depth and as the only layer elsewhere. So hardened unit files are part
of the product, not an appendix.

### The watchdog is not our wedged detector — keep the layers separate

Tempting, and wrong to collapse: `WatchdogSec` detects a process that **stops pinging**, which
is liveness of the *process*, not progress of the *work*. A scanner whose work goroutine is
stuck while its runtime is healthy will keep pinging happily.

Gating the ping on progress would turn systemd into a wedged-detector with restart authority —
but that walks straight into D37: a node judging its own health is the least reliable source,
and a legitimately slow task (probing a 60 GB remux) would earn spurious restarts.

So, two layers that do different jobs:

- **systemd's watchdog**: fast, blunt, catches hard hangs. Ping conservatively — fire unless
  definitely stuck.
- **The core's observation** (D37): slower, smarter, catches subtle wedging, graded, and holds
  **no restart authority**.

They compose rather than overlap, and systemd restarting a unit in two seconds is invisible to
the core because D37's escalation has hysteresis and "late" does nothing.

### Restart is already safe, by construction

Nothing special is needed, because earlier decisions bought it:

- A scanner killed mid-scan leaves a `.partial` report that can only add or update, never remove
  ([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)).
- `epoch` increments, so the core reads the reset progress as movement rather than a stall
  ([D36](#d36--idle-ticks-the-progress-pair-the-node-declares-its-own-tick-contract)).
- `node_id` persists, so a restarted node is the *same* node and its items are never orphaned
  ([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)).

### The rule: report failure, do not exit

**A node that cannot do its job should stay alive and say so.** A scanner whose share is
unreachable emits a `root`-scoped error ([D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope),
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse))
and keeps running. It does **not** crash.

Four reasons: exiting **destroys the reporting channel**, which is the thing that would have
explained the problem; repeated exits trip `StartLimitBurst` and park the unit, converting a
recoverable outage into a manual intervention; the core cannot distinguish "exited because the
share is down" from "exited because of a bug"; and a live node reporting `root unreachable` is
strictly more informative than a dead one. This is the natural consequence of
[D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) — sensors report.

Exit is reserved for conditions that genuinely cannot be reported: a corrupt local config, a
failed sandbox setup, an unrecoverable internal state.

### Kill the process group, or you get orphaned FFmpeg

A common and memorable bug in this domain. When a role process dies, its confined FFmpeg
children must die with it, or they keep a CPU busy forever and nobody knows why.

- systemd's default `KillMode=control-group` kills the whole cgroup — handled.
- Our supervisor must kill the **process group**, not the process.
- Belt and braces on Linux: set `Pdeathsig` in the child's `SysProcAttr` so an FFmpeg child is
  signalled when its parent dies, even if the parent died in a way that skipped cleanup.

### For our own supervisor

Exponential backoff, and — the important part — **never give up silently.** A role that has
stopped restarting must be visible as a state the core can surface, not a line in a log nobody
reads. That is the same failure mode `StartLimitBurst` has, and the same remedy.

**Status:** Provisional. The shipped unit files are a deliverable and want hardening review;
`s6-overlay` is a legitimate alternative to our supervisor inside a single container and is
worth revisiting if the supervisor starts growing features — which is the signal D43 said to
watch for.

---

## D45 — Metadata is multi-language in the schema, chosen per request

**Decided** (2026-09-19). Split every metadata-bearing entity into a **language-invariant row**
and a set of **translation rows** keyed `(item, language)`. Choose among them **at query time**
from the request's `Accept-Language`. Never at ingest.

**Why.** Jellyfin resolves `PreferredMetadataLanguage` through item → parent → library → server
and uses it at *fetch* time, so the database holds exactly one title. Two people in one
household cannot have different languages, and there is no fallback chain — only en-US.
*Language Selection Fallback Heirarchy* (#2521) has been open since **2020-03-05**. This is not
a bug they have failed to fix; it is what the schema permits.

Kyoo has the shape that works, and three of its details are the actual content of this decision:

- **Artwork belongs in the translation row.** Posters have title text baked into the pixels, so
  poster language is a separate axis from title language — which is exactly what Jellyfin #10076
  asks for and cannot have.
- **Genres are an enumeration; tags are free text.** Genres are a closed set stored as symbols
  and rendered from our own UI catalogue, so a Chinese user sees 科幻 from our translation file
  rather than whatever string TMDB returned. Tags cannot be enumerated, so they stay
  per-language. Cheap, sharp, and it removes a whole class of half-translated UI.
- **The original-language record is distinguished**, not just another translation. It always
  exists, which gives a guaranteed fallback anchor and a meaningful "sort by original title".

**`Accept-Language` is the fallback chain.** It is an ordered, weighted preference list with a
wildcard; it is standard; every client already sends something sane. Six years of Jellyfin
feature requests are asking for a thing HTTP has always had.

**Two cautions, both load-bearing.**

Language selection participates in `ORDER BY`, so it must be a **total** order — Kyoo's own
comment says a non-deterministic tie-break makes keyset pagination return the same row on two
pages. And parse `Accept-Language` with `golang.org/x/text/language` rather than by hand:
Kyoo's hand-rolled parser sorts the `q=` weights ascending, which appears to select the
*least*-preferred language, and has no test. `ParseAcceptLanguage` + `NewMatcher` implement full
BCP 47 lookup including likely-subtags and mutual intelligibility (nb/no, zh-Hans/zh-Hant,
pt-BR/pt).

`DISTINCT ON` is Postgres-only; under SQLite this is `ROW_NUMBER() OVER (PARTITION BY item_id
ORDER BY lang_rank, language)`, available since 3.25.

**Status:** Provisional, but schema-shaping and therefore effectively due before
[library-model.md](../design/plans/library-model.md). Overturned by evidence that per-language
artwork is not worth its storage at household scale — which nobody has measured.

---

## D46 — No translated string is ever stored in the database

**Decided** (2026-09-19). Store the fact and its parameters; render at display time. One
catalogue, owned by the client. The server renders text only for delivery that leaves a client
session, and renders it in the **recipient's** language.

**Why.** Jellyfin maintains two catalogues — 1,947 keys in `jellyfin-web`, 123 in the server —
which **overlap by twelve keys**, including `HeaderContinueWatching`. Twelve concepts translated
twice, by two sets of translators, in two Weblate components, displayed in the same UI.

The instructive entry is `ChapterNameValue`: `"Chapter {0}"`, used to generate a name that is
then *written to the database*. It is frozen in whatever language the scanner was running in. A
chapter is `{index: 3}`, not `"Chapter 3"`.

Once the rule holds, the server catalogue collapses to the cases that genuinely leave the
session — notification and activity-log text delivered by email or push — and even those should
render in the recipient's language, which the server knows.

`golang.org/x/text/message` + `catalog` covers that with CLDR plural rules, for a measured
**+594 KB** of binary over hello-world.

**Status:** Provisional. Free if adopted now, expensive later. The failure mode to watch for is
a "convenient" generated display name creeping into a column; a test asserting that no
user-visible text column is written by the scanner would catch it.

---

## D47 — Sort order is a materialised CLDR sort key, per item per locale

**Decided** (2026-09-19). Generate a `collate.KeyFromString` byte key with
`golang.org/x/text/collate` for each (item, locale-in-use), store it in an indexed `BLOB`
column, and let SQLite order on it with the default `BINARY` collation. Not a transliteration,
and not a custom SQLite collation.

**Why not transliteration.** Jellyfin materialises a sort string by lowercasing, stripping
hard-coded English stop-words (`the, a, an` — from the start, the *middle* and the end), removing
diacritics, and then transliterating anything non-ASCII with
`Any-Latin; Latin-Ascii; Lower; NFD; …`, tunable by the environment variable
`JELLYFIN_TRANSLITERATOR_ID`. Four separate mistakes: the stop-words are English and
server-wide; `Any-Latin` renders Japanese kanji as pinyin, so 千と千尋の神隠し does not sort under
*Sen*; transliteration destroys exactly the distinctions the target locale cares about, folding
Swedish Å Ä Ö to the front when Swedish puts them after Z; and a global environment variable to
override the rule is the tell that one server-wide order was never enough.

**Measured, rather than assumed** (Go 1.26.2, x/text v0.42.0). `x/text/collate` gets Swedish,
German phonebook (ü ≡ ue), Chinese pinyin *and* stroke, and Japanese kana equivalence right;
`collate.Numeric` orders `Episode 2` before `Episode 10` natively, deleting Jellyfin's
zero-padding hack. It gets **Korean wrong** — output identical to codepoint order, because
`// TODO: implement script reordering` is still in the source and
[golang/go#19087](https://github.com/golang/go/issues/19087) has been open since 2017 with zero
comments. And `collate/tables.go` declares `UnicodeVersion = "6.2.0"`, `CLDRVersion = "23"` —
2012 and 2013 — while the rest of `x/text` tracks current Unicode.

That is the honest trade: correct for Latin, Cyrillic, Greek, Chinese, Japanese; wrong for
Korean; thirteen years behind on data. The alternative, cgo-linked ICU, costs the
cross-compilation that [D31](#d31--sqlite-with-a-pure-go-driver-supersedes-d5) went out of its way
to buy.

**Why materialise.** Sort keys are byte-comparable, so they need no collation support from the
database at all. Measured at 10,000 titles: sorting in Go at query time takes 16.3 ms but
forfeits `ORDER BY … LIMIT 50` and index paging; key generation is ~1 µs/title and keys average
**~140 B**, so 1.4 MB per 10k items *per locale*, and SQLite pages against an index in O(log n).
A household uses one to three locales.

**Why not a custom collation**, which both pure-Go drivers support
(`modernc.org/sqlite.RegisterCollationUtf8`, `ncruces/go-sqlite3` `CreateCollation`): an index is
bound to the collation it was created with, so you need one index per locale regardless — the
materialised design with extra steps — and it makes the database file unopenable by the `sqlite3`
CLI or any recovery tool, every affected query failing with *no such collation sequence*. A BLOB
column merely degrades to an opaque column, which is what
[D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise)'s "rebuildable index" stance wants.

**Status:** Provisional. Korean is a known, accepted defect — see
[internationalisation.md](../design/research/internationalisation.md) §9 for the three ways out.
Revisit if the locale count per household turns out to be large.

---

## D48 — Two FTS5 tokenisers, routed by script and length, and exactly one normaliser

**Decided** (2026-09-19). Index every searchable entity **twice**: `unicode61
remove_diacritics 2` for Latin word search and bm25 ranking, and `trigram` for substring search.
Route: 1–2 characters → `LIKE`, ≥3 characters → trigram, Latin word queries → unicode61. Feed the
unicode61 index a **normalised-alternates column** generated in Go. That normalisation function
has exactly one implementation.

**Why two.** Measured in SQLite 3.46.1, because the documentation does not say this plainly:

| | Latin + diacritics | ø æ œ ß | CJK | Thai | prefix / bm25 |
| --- | --- | --- | --- | --- | --- |
| `unicode61 remove_diacritics 2` | ✓ | ✗ | ✗ | ✗ | ✓ |
| `trigram` | ✗ | substring only | ✓ (≥3 chars) | ✓ | ✗ |

unicode61 tokenises `千と千尋の神隠し` as **one token**, so `千尋` finds nothing, and shreds Thai
into fragments (`ผ`, `หญ`, `งล`, `ลลา`) that match no word. Trigram handles both but needs three
characters and cannot do prefix search or rank.

Neither fixes atomic letters. `remove_diacritics` removes **combining marks**; Ø Æ Œ ß Ð Ł are
single code points with no decomposition, so `"Bjork"` misses `Bjørk` and `"GOGGS"` misses
`GØGGS` under both tokenisers. No option fixes this — hence the alternates column, which is what
Navidrome arrived at after shipping the bug.

**So run both**, because the cost is a rounding error. Measured on 20,000 mixed-script titles:
content 532 KB, unicode61 index 476 KB, trigram index 1,004 KB — **1.5 MB for both**, each
building in ~30 ms. Query latency: 0.012–0.10 ms for either index; `LIKE` worst case (no match,
full scan) 1.554 ms against trigram's 0.001 ms, which is the case that matters because it is what
happens on every keystroke of a type-ahead.

**One normaliser, and why.** Navidrome shipped FTS5 in February 2026 and a corrective migration
in July: the migration had back-filled `search_normalized` *"with a SQL approximation that cannot
transliterate atomic letters (Ø, æ, ß, …)"*, so `GØGGS` was unfindable. The Unicode lesson is
above; the engineering lesson is that the logic existed twice — once in Go, once as a SQL
`REPLACE` chain — and drifted. A migration that needs normalisation calls the Go function row by
row.

Four further details taken from Navidrome verbatim: contentless tables (`content=''`) with
trigger population and a `RebuildFTS`, because the index is derivable and FTS5 corruption is a
real operational event; **lowercase the user's `AND OR NOT NEAR`**, which are case-sensitive
operators in FTS5 and otherwise throw a syntax error; **no minimum query length for CJK**,
because single CJK characters are meaningful words; and `(token OR token*)` rather than bare
`token*`, which adds no matches but lets bm25 rank literal hits above prefix hits.

**Status:** Provisional but schema-shaping — the tokeniser is baked into the FTS table, so this
is due before the search schema. The routing thresholds are guesses pending a real corpus.

---

## D49 — Unicode discipline at the boundaries, enforced by tests

**Decided** (2026-09-19). Two boundary rules, each with a test that fails the build.

**1. Normalise paths to NFC at the core's ingest boundary, and keep the raw bytes for I/O.**

[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) puts nodes on arbitrary
machines reporting paths to the core. macOS/APFS returns decomposed filenames; Linux returns the
bytes on disk. Measured:

```
macOS-style NFD : "Amélie"  len=8  bytes= 41 6d 65 cc 81 6c 69 65
Linux-style NFC : "Amélie"  len=7  bytes= 41 6d c3 a9 6c 69 65
equal? false      equal after NFC(both)? true
```

Same file. A macOS node and a Linux node reporting the same SMB share therefore report two
different paths for one file: two rows under `UNIQUE(path)`, or a delete-and-add churn cycle
every time the reporting node changes. **Both halves of the rule matter** — identity, dedup,
display and search use the NFC form; *opening* the file uses the original bytes, because APFS
normalises on lookup and ext4 does not, so an NFC string will not open an NFD file on Linux.
This is a field pair in the report contract
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)), not an
assumption.

**2. No ASCII-only character classes in the naming-regex corpus.**

Jellyfin's episode expressions use `[\w\s]+?` for the series name. In .NET `\w` is Unicode-aware;
**in Go's RE2 it is not.** Ported verbatim, the expression fails on `進撃の巨人`, `기생충`,
`Матрица` — and on **`Amélie`**. It is not a CJK problem, it is an everything-but-ASCII problem.
`\d` likewise misses Arabic-Indic digits, and `\s` misses U+3000 IDEOGRAPHIC SPACE, which is the
usual separator in Japanese and Chinese filenames, and U+00A0, which Windows renamers emit.

It fails **silently**: the expression does not match, the filename falls through to a worse
expression, and the result is a misparse rather than an error. So ban `\w \d \s \b [[:alpha:]]`
in that corpus, require `\p{L} \p{N} \p{M} \p{Z}`, and run the whole corpus against a fixture of
non-Latin filenames in CI.

**Why a decision and not a note.** Both are free now and near-invisible later — the NFC one is a
wire-format field, and the regex one is a lint rule. Both are the *parse, don't validate* stance
of [media-nodes.md](../design/media-nodes.md) applied to text, and the NFC pair is the same shape
as [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s rule for
track tags: normalise for use, keep the original for provenance.

**Status:** Provisional only in the sense that the fixture corpus does not exist yet. The
findings are measured, not inferred.

---

## D50 — Build libass correctly and turn off VSFilter bug-compatibility for non-authored subtitles

**Decided** (2026-09-19). Build libass with **HarfBuzz**, a current **FriBidi** and
**libunibreak**. Enable `ASS_FEATURE_WHOLE_TEXT_LAYOUT` and `ASS_FEATURE_BIDI_BRACKETS` for
sources that are not authored ASS, and enable `ASS_FEATURE_WRAP_UNICODE` with the **track
language** set. Leave all three off for authored `.ass`/`.ssa`.

**Why.** [D11](#d11--subtitles-are-a-subsystem-not-a-feature) chose libass everywhere partly
because browsers render WebVTT without regard to direction — jellyfin-web #4179, *"Right-to-left
languages subtitles having punctuation in the wrong order"*. libass has the same failure
available and it is **on by default**. From `ass.h`, with `ASS_FEATURE_WHOLE_TEXT_LAYOUT`
disabled, *"text is split into VSFilter-compatible segments and text in each segment is processed
in isolation… The individual runs are then laid out **left-to-right, even if they contain
right-to-left text**."* That is #4179, reproduced inside the renderer we picked to avoid it.

`ASS_FEATURE_BIDI_BRACKETS` gives Unicode 6.3 bracket pairing and needs a new-enough FriBidi.
`ASS_FEATURE_WRAP_UNICODE` gives UAX #14 line breaking, needs **libunibreak** at compile time,
and *"if the track language is set, some additional language-specific tweaks may be applied"* —
without it there is no correct line breaking for Thai, Japanese or Chinese, which have no spaces
to break at. This is the payoff for [D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s BCP 47
normalisation: we already have the language to pass.

The defaults are right for libass and wrong for us. Fansubbed ASS is authored against VSFilter's
behaviour, so bug-compatibility is correct there — and only there. SRT, VTT, PGS-converted text
and CC were never authored against it.

**The uncosted item this exposes.** JASSUB is our web path. Its submodules are `libass, fribidi,
freetype, harfbuzz, brotli` — **no libunibreak** — and it never calls `ass_track_set_feature`. So
the browser client as it stands gets segmented bidi, pre-6.3 brackets, and no Unicode line
breaking. Getting this right means patching JASSUB's build and exposing the toggles: a real line
item for D11 that was not in its estimate.

`ass_track_set_feature` returns −1 when a feature is unavailable at runtime, which makes it a
capability probe. Check it once and report the answer rather than silently rendering wrong —
[principles §9](../principles/README.md), sensors report.

**Status:** Provisional. Needs a rendering comparison on real Arabic, Hebrew, Thai and Japanese
subtitle files before the flags are fixed; the split between "authored ASS" and "everything else"
is a guess at where the boundary belongs.

---

## D51 — react-i18next, catalogues served by us, Weblate deferred

**Decided** (2026-09-19). `react-i18next` in the client. Serve catalogues from our own server,
bundling only English. Choose the catalogue format for a translation platform now; connect one
when translators exist.

**Why react-i18next.** The only two projects solving our exact client problem converged on it
independently — Kyoo (`react-i18next` 17 + `i18next-browser-languagedetector` +
`i18next-http-backend`) and Streamyfin (`react-i18next` 17 + `i18next` 26 + `expo-localization`).
Two settings come with it, both from Kyoo: `returnEmptyString: false`, because translation
platforms export untranslated keys as `""` and the default renders them, so a
partially-translated language shows **blank UI** rather than English; and `load: "currentOnly"`.

Plural handling must **not** depend on `Intl.PluralRules`. Hermes does not bundle ICU — it
projects the platform's, whose *"results… may vary between devices running different Android
versions"* — and neither `Intl.PluralRules` nor `Intl.Segmenter` is in its supported list.
Streamyfin pins `compatibilityJSON: "v4"` explicitly; either pin it or ship
`@formatjs/intl-pluralrules`. The same platform variance is an independent argument for
[D47](#d47--sort-order-is-a-materialised-cldr-sort-key-per-item-per-locale): `localeCompare` can
order the same list differently on an Android 10 phone and an Android 14 TV, so **the server owns
sort order** and the client renders the list it is given.

**Why serve rather than bundle.** Streamyfin statically imports all 34 catalogues; a translation
fix then needs an app-store review cycle, which is the mechanism by which translations go stale
on TV. We are unusual in *being* the server the client is already talking to, so a merged
translation reaches every client on next connect, over the LAN, for free.

**Why Weblate, and why not yet.** It is the field's convention — 9 of 11 surveyed projects
(Jellyfin, jellyfin-web, Immich, Kyoo, Swiftfin, Audiobookshelf, PhotoPrism, Stash, Kavita)
against Crowdin for Streamyfin and POEditor for Navidrome. It is AGPL, self-hostable, and free
for libre projects. But Immich's integration is the real cost: two GitHub Actions workflows, a
GitHub App with scoped tokens, and a **required PR check that fails any PR touching
`i18n/!(en)**.json` without the bot's approval** — humans may edit only the English catalogue;
every other file is machine-owned. Their release flow locks the component, forces a commit and
push, finds the single `weblate` PR, approves, merges, polls, and unlocks, with cleanup on
timeout. None of that buys anything before there are translators.

**RTL.** `I18nManager.forceRTL()` sets a native flag read at startup; changing it does not
re-lay-out the running app (react-native #16215, #32509, #47583, and #49451 still reproducing on
0.77). On a TV a restart is worse than on a phone, so follow the OS locale's direction and do not
offer an in-app direction switch. Jellyfin has the matching layout bugs (jellyfin-web #7297,
Arabic series detail page), which is what happens when RTL is absent from the test matrix — so
one RTL locale belongs in the device-lab pass.

**Status:** Provisional. The bundle-vs-serve choice should be revisited if offline use of a
non-English client turns out to matter more than expected.

---

## D52 — Pseudo-localisation is the default development build

**Decided** (2026-09-19). Debug builds render every UI string through a pseudo-localisation
transform by default. Turning it *off* is the opt-in.

**Why.** Taken from Netflix, who published both the mechanism and the retrospective. Their
problem statement: translated text can be **up to 40% longer** than English — worst in German,
Hebrew, Polish, Finnish and Portuguese — and a UI designed in English assumes English string
lengths, line heights and glyphs. At 26 languages one layout mistake becomes "26 defects that
need to be logged, managed, and resolved," when it could have been fixed once before translation
started.

The transform, from their example:

```
Find Help Online   →   [ƒîกี้ð Ĥéļþ Öกี้ļîกี้é one two]
```

Three elements, each catching a different class of defect: the `[` `]` markers reveal
**clipping**, because a developer who cannot see both brackets is looking at a truncated string;
extended characters stress **line height and font coverage**; and the padding simulates the
**40% expansion**. The quiet fourth benefit is the best one — a string that was never
externalised **does not get transformed**, so it stays plain English and stands out on a screen
of accented gibberish. That is a free lint for the failure mode that is otherwise invisible
until a translator asks why a button has no key.

**Three details that make it work, all from their experience rather than the idea:**

- **Opt-out, not opt-in.** Netflix made it "the default development language for all UI
  developers" in debug builds. Opt-in tooling is tooling nobody runs.
- **Exclude fields already length-capped upstream** — synopses, titles — or padding produces
  false positives.
- **Metadata has to be transformed too.** Titles, cast names and ratings come from elsewhere, and
  without them "the experience felt incomplete, half pseudo localized, half English." This is
  exactly the catalogue/metadata split of [D45](#d45--metadata-is-multi-language-in-the-schema-chosen-per-request)
  and [D46](#d46--no-translated-string-is-ever-stored-in-the-database), and it confirms which
  half is harder.

**Why this is unusually cheap for us.** Netflix's own retrospective says the engineering "was not
half of the battle, I would argue it was even less — the real work starts while advocating,
influencing, educating, and convincing development teams." We have no teams to convince. We get
the expensive part of the idea at the cheap part's price, and we get it before there is a single
translated string, which is the only time it is free to adopt.

Their six-week survey found readability suffered, and they proposed replacing `one two three`
padding with vowel multiplication (`[ƒîîîกี้ð Ĥéééļþ …]`). Start there rather than repeating the
mistake.

**Status:** Provisional in form, not in principle. The exact transform should be tuned once
somebody has stared at it for a week; the default-on part is the decision.

---

## D53 — Font coverage is not assumable; the server serves fonts on demand

**Decided** (2026-09-19). Treat script coverage as a client capability that is **reported, not
assumed**. Ship no large fallback fonts in the client. When a client cannot render a script, the
server supplies the font over the same addressable-resource mechanism
[D11](#d11--subtitles-are-a-subsystem-not-a-feature) already uses for embedded subtitle fonts.

**Why.** Measured against the Android 15 Compatibility Definition Document — the document that
decides what a certified device must contain:

```
"CJK" 0 · "Chinese" 0 · "Arabic" 0 · "Hebrew" 0 · "Thai" 0   occurrences
```

It mandates Latin, Greek and Cyrillic (Latin Extended A–D, the Unicode 7.0 currency block), that
`NotoColorEmoji.ttf` not be removed, and that `sans-serif` be Roboto 2.x **"for the languages
that Roboto supports."** Nothing more. A device may ship with no CJK, Arabic, Hebrew, Thai or
Indic font and remain compliant — and on a single-market TV box, which is most of the hardware in
[test-hardware.md](../design/research/test-hardware.md), that is the likely case rather than the
edge case.

The symptom is tofu, and the user's reading of it is **"the subtitles are broken"**, not "my TV
lacks a font". It lands on us either way.

**Why not just bundle one.** Measured:

```
NotoSansCJKjp-Regular.otf   15.70 MB    one weight
NotoSansCJKjp-Bold.otf      16.24 MB
NotoSerifCJK full OTC      ~132 MB
```

~16 MB per weight per CJK variant, shipped to every user including the overwhelming majority who
will never see a CJK glyph, on platforms where app size is a real constraint and an update is a
store review.

**Why serving works.** We are the server the client is already connected to, usually over a LAN.
The client reports which scripts it can render — the same shape as
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)'s "the client is the only party that
can ask its own decoder" — and the server sends what is missing, once, cached. The mechanism
already exists: D11 serves embedded subtitle font attachments as addressable resources, and a
script-fallback font is that with a different key.

This also closes a gap in [D50](#d50--build-libass-correctly-and-turn-off-vsfilter-bug-compatibility-for-non-authored-subtitles).
Correct bidi and line breaking are worthless if the glyphs are boxes, and libass will happily
render tofu with perfect Unicode line breaking between the boxes.

Which font goes where is [D54](#d54--the-plan-b-font-is-unicode-last-resort-he-bundled-noto-is-served-unifont-is-the-middle-option).

**Status:** Provisional. Needs a real measurement of what the Fire TV sticks and the older Android
TV devices actually carry — a direct job for the device lab, and cheap: enumerate the system font
set and the covered scripts on every device in the rig.

---

## D54 — The plan-B font is Unicode Last Resort HE, bundled; Noto is served, Unifont is the middle option

**Decided** (2026-09-19). Three tiers, and the point of the decision is that they are three
rather than one:

1. **The requested font** — an embedded subtitle font, or the client's UI font.
2. **Noto, per script *and per language*, served on demand** by the server
   ([D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand)). Which faces,
   and how they are cut down, is
   [D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle).
3. **Unicode Last Resort High-Efficiency, bundled in every client** — 588 KB, never fetched,
   never absent.

**Why there is no single answer.** OpenType caps a font at **65,535 glyphs** and Unicode 18 has
over 150,000 assigned characters, so one file physically cannot do it. The evidence is the only
project that tried: GNU Unifont splits by plane, and its BMP file reports `numGlyphs = 58,911` —
not a design choice, the ceiling arriving. Every platform therefore uses a **cascade**, and the
real question is what sits at the end of it.

**What the platforms put there.** Apple has shipped `.LastResort` since Mac OS 8.5 and selects
the rest of the cascade with `CTFontCopyDefaultCascadeListForLanguages`, which takes a **locale**.
Android does not: its `fonts.xml` is 161 families ending at `und-Yezi`, and grepping AOSP for
`lastresort` or `notdef` returns nothing — fall off the end of Android's chain and you get tofu.
Windows has `IDWriteFontFallback`; Linux has fontconfig, which is the weak link, to the point
that browsers hardcode CJK font names because "at least Debian simply doesn't have a correct
config".

**Why Last Resort HE specifically.** Measured from the binaries:

```
LastResortHE-Regular.ttf   0.56 MB    387 glyphs   1,114,112 code points mapped
LastResort-Regular.ttf     9.15 MB  5,776 glyphs   1,114,112 code points mapped
unifont + unifont_upper   10.75 MB  116,640 glyphs  ~116,600 code points
NotoSansCJKjp-Regular     15.70 MB   (one weight, CJK only)
```

The HE variant covers **every code point in Unicode for 588 KB** by using a cmap **format 13**
(many-to-one range mappings) subtable — the correct table for a font where thousands of code
points share a glyph. It renders a script-identifying character in a framed box (ක for Sinhala,
Ω for Greek), with the block name and code-point range engraved in the frame, and it
distinguishes unassigned code points and the 66 noncharacters from merely-unavailable ones.

Its documented risk is that format 13 "may not be supported in some environments, such as most
Windows and Adobe apps." **That does not reach us, and this is checked rather than assumed:**
our entire text stack is FreeType — libass links it natively and JASSUB compiles it to WASM —
and FreeType implements `tt_cmap13` in `src/sfnt/ttcmap.c` with `TT_CONFIG_CMAP_FORMAT_13`
enabled by default in `ftoption.h`. JASSUB patches FreeType only to disable pthreads.

**Why not just bundle Noto.** 15.7 MB per weight per CJK variant, shipped to every user
including the great majority who will never see a CJK glyph. And the observation that settles
it: **Netflix does not do this either.** Netflix Sans, commissioned from Dalton Maag for a
company in 190 countries, covers per its designer's own portfolio *"Cyrillic, Greek, Latin,
Latin (Vietnamese Extension)"* — no CJK, Arabic, Hebrew, Thai or Indic. Amazon Ember is the same
shape. The two largest streaming services cover exactly the scripts the Android CDD already
mandates and fall through to the platform for the rest, which means on a device without a
Japanese font, **Netflix renders tofu too**. Their remedy is commercial — relationships with
device manufacturers and preinstallation — and is not available to us. Ours is topological:
being a server on the same LAN, we can just send the font, which for our situation is better
than what they do rather than a poor imitation of it.

**Unifont is the middle option and worth keeping.** 10.75 MB for ~116,600 code points — less
than one weight of Noto CJK — dual-licensed GPLv2-with-Font-Embedding-Exception and OFL 1.1,
with the explicitly stated purpose of rendering "something meaningful for each Unicode code
point, rather than a blank box". Legible and ugly. Since we serve fonts anyway it costs nothing
to offer as a server-side option for someone who would rather read a bad glyph than a script
label.

**One thing the tiering must get right: CJK fallback is chosen by *language*, not by script.**
Han unification gives Japanese, Simplified and Traditional Chinese the same code points with
different expected letterforms — "a rough analogy is to use the same code point to encode Latin
capital A and Greek capital alpha". Apple's cascade API takes a locale for precisely this. A
script-keyed fallback renders Japanese with Chinese letterforms: not tofu, just quietly wrong,
and much harder to notice than tofu. The language is available to us —
[D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series) normalises
track language to BCP 47 and [D45](#d45--metadata-is-multi-language-in-the-schema-chosen-per-request)
knows the user's — so this is a plumbing requirement, not a research one.

**The dissent, recorded because it is credible.** Firefox refuses to use Last Resort: Mozilla
bug 1416762, *"prefer to draw hexboxes for unsupported characters"*, fixed November 2017, on the
grounds that hex boxes give "better cross-platform consistency and… actually convey more
information (the exact codepoint involved)". Right for a browser, wrong for us, and the
difference is the reader. A developer wants `U+304B`; a person on a sofa wants to know whether
the app is broken or their TV lacks a Japanese font, and a framed か answers that. So: **Last
Resort in release builds, hex boxes in development**, which pairs with
[D52](#d52--pseudo-localisation-is-the-default-development-build).

**Status:** Provisional. Two things would change it: a device-lab measurement showing FreeType
is *not* in the path on some client we care about (in which case that client takes the 9.15 MB
format-12 variant), or evidence that users find the framed-script glyph more alarming than
informative — which is a question for whoever first sees it on a TV.

---

## D55 — The served font set is the CTA-708 style axis, in variable fonts, subset per subtitle

**Decided** (2026-09-19). Adopting the proposal to host a *set* of fallback faces rather than a
single one, with the set defined by an existing standard and the CJK problem solved by
subsetting rather than by shipping.

Three parts:

1. **Six upright variable faces** covering the seven real CTA-708 styles for Latin, Greek and
   Cyrillic. **5.63 MB raw, ≈2.42 MB as woff2**, served once and cached forever.
2. **Everything else subset per subtitle track at scan time** — ~441 KB for a film's Japanese
   dialogue instead of 15.7 MB.
3. **Last Resort HE underneath**, bundled, per
   [D54](#d54--the-plan-b-font-is-unicode-last-resort-he-bundled-noto-is-served-unifont-is-the-middle-option).

**Why a standard defines the set.** "Enough faces to cover the common subtitle styles" sounds
like a judgement call, and it is not: **CEA/CTA-708 enumerates exactly eight font tags** —
undefined, monospaced serif, proportional serif, monospaced sans-serif, proportional
sans-serif, casual, cursive, small capitals — and the W3C's 608-to-WebVTT mapping gives each a
concrete CSS stack (Courier, Times New Roman, Helvetica, Arial/Swiss, Dom/Impress,
Coronet/Marigold, Engravers Gothic). Every one of those faces is proprietary, which is precisely
why an open-source server has to bring substitutes. Using the standard's axis instead of
inventing one means the set is defensible, bounded, and already matches what captioning
decoders and player UIs expose.

Two collapses take it from eight to six, both from the captioning literature and both confirmed
by measurement. **Small capitals is not a font** — "small caps can be applied to any typeface
category" — it is the OpenType `smcp` feature, and all three Noto faces report `smcp` present.
And **all eight styles are optional in decoders**, so casual and cursive can fold into the sans
face wherever no equivalent exists, which is every non-Latin script.

**Measured** (fonttools, against the real binaries):

| CTA-708 style | OFL substitute | raw | woff2 |
| --- | --- | --- | --- |
| proportional sans | `NotoSans[wdth,wght]` | 2001 K | 848 K |
| proportional serif | `NotoSerif[wdth,wght]` | 1842 K | 765 K |
| monospaced sans | `NotoSansMono[wdth,wght]` | 1668 K | 704 K |
| monospaced serif | `CourierPrime-Regular` | 69 K | 24 K |
| casual | `ComicNeue-Regular` | 55 K | 22 K |
| cursive | `DancingScript[wght]` | 130 K | 55 K |
| small capitals | the `smcp` feature | 0 | 0 |
| **total** | **6 files** | **5.63 MB** | **≈2.42 MB** |

**Variable fonts are the reason this is small.** The Noto faces carry `wght` *and* `wdth` axes,
so one file replaces nine weights × three widths. Without them this is a combinatorial problem;
with them it is six files. Real italics would add two files and take it to 10.18 MB / 4.30 MB —
worth having eventually, not needed first, because **libass synthesises both**:
`ass_glyph_embolden()` and `ass_glyph_italicize()` in `ass_font.c` apply when the face lacks the
style. (There is no `NotoSansMono-Italic` at all, and it does not matter for the same reason.)

**Subsetting is the part only we can do.** Google Fonts serves CJK by cutting fonts into ~100
fixed `unicode-range` slices because a web font server cannot know what text a page will render.
**We have the subtitle file**, so we can subset to exactly the code points it uses:

```
20,000 chars of Japanese (about one film's dialogue)  1,202 code points  →  582 K raw / 441 K woff2
21,000 chars of Chinese                               1,816 code points  →  988 K / 732 K
168,000 chars of Japanese (generous upper bound)      2,405 code points  → 1220 K / 900 K
NotoSansCJKjp-Regular, whole                         65,535 glyphs       →  15.70 M / 11.15 M
```

**36× smaller for a film.** The reason is linguistic, not algorithmic: 168,000 characters of
running Japanese contain only 2,405 distinct code points, because the Jōyō set is 2,136 kanji
and real text saturates below it.

**Where it runs: the node, at scan time.** [D11](#d11--subtitles-are-a-subsystem-not-a-feature)
already extracts subtitles at scan time in the media node, which is where FFmpeg lives. The
subset font is one more scan-time artifact beside the extracted track, and no font parser ever
enters the core's request path. **The tool is already in the build** — `hb-subset` is part of
HarfBuzz (`util/hb-subset.cc`, plus a public C API), and we ship HarfBuzz for libass
([D50](#d50--build-libass-correctly-and-turn-off-vsfilter-bug-compatibility-for-non-authored-subtitles)).
No new dependency.

**A security consequence, which happens to cut our way.** Serving fonts means that if we serve
one **extracted from a media file**, we hand attacker-supplied binary to every client, whose
FreeType then parses it — and font parsers are a classic CVE surface
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)). Subsetting improves this: the served file is
re-serialised by our subsetter rather than passed through. That is **not** a sanitiser and must
not be described as one, but the client does then parse a file our tooling produced. The
subsetter itself parses the hostile input, so it belongs in the same Landlock+seccomp jail as
FFmpeg — which is where it already is, because it runs in the node.

**What still needs a human.** The casual and cursive substitutes above are placeholders chosen
for licence and availability, not for how they look at 40 px on a television from three metres.
Comic Neue and Dancing Script cover Latin only (303 and 559 code points). Someone should look at
them on a TV before they ship, and the monospaced-serif slot deserves a second look too —
Courier Prime is a faithful Courier, and Courier is itself criticised as unsuitable for
captioning.

**Status:** Provisional. The style→face table is a configuration file, not code, so it is cheap
to revise. The subsetting numbers are measured and are unlikely to move; the per-track subset
should be validated against a real anime release with a heavy `.ass` typeset, which is the
adversarial case.

---

## D56 — Embedded fonts are off by default; the client must ask for them

**Decided** (2026-09-19). A font extracted from a media file is **not** used and **not** served
unless the client explicitly requests it. The default rendition uses our own served faces
([D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle)).
The manifest *advertises* that a track carries embedded fonts; acting on that is the client's
call.

**Why the default matters more than the capability.** Media is attacker-supplied data
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)), and a font is a
program in the sense that matters: FreeType interprets it. **Two FreeType bugs sit in CISA's
Known Exploited Vulnerabilities catalogue** — confirmed exploited, not theoretical:

| CVE | CVSS | KEV added | Surface |
| --- | --- | --- | --- |
| CVE-2020-15999 | 9.6 | 2021-11-03 | Heap overflow in `Load_SBit_Png` — **PNG embedded inside a font**; CISA notes it as part of an exploit chain |
| CVE-2025-27363 | 8.1 | 2025-05-06 | Out-of-bounds write in **TrueType GX / variable-font subglyph** parsing; arbitrary code execution, FreeType ≤ 2.13.0 |

The second is from **March 2025**. And ≤ 2.13.0 describes exactly the old Fire TV sticks and
Android TV boxes that [D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand)
already identified as the weak devices. The server is an **amplifier**: one bad file reaches
every screen in the household, including the ones nobody updates and cannot update.

**The commercial players do not accept fonts at all.** Netflix's IMSC 1.1 Text Profile:
`tts:fontFamily` *"Should not be used. If specified, the tts:fontFamily should be equal to
`proportionalSansSerif`."* One generic family, resolved by the player — from a company with
**contracts with the studios supplying the content**. If Netflix will not take a named font from
a vetted partner, taking an arbitrary binary from a torrent by default needs an argument nobody
has made.

**The open-source field does the exact inverse**, and it is worth seeing how. `jellyfin-web`'s
`renderSsaAss()` hands every attachment to libass whose **`MimeType` looks like a font** — and
`MimeType` is metadata from inside the container, written by whoever made the file. Their own
comment says the filter exists to skip cover art, not to screen anything. Meanwhile the
*trusted* fallback font set is behind `config.EnableFallbackFont`, **default off**. Untrusted
source on by default, trusted source off by default. We are inverting that.

**What we lose, stated honestly.** Heavily typeset `.ass` — fansubbed anime especially — assumes
its bundled fonts, and ASS positioning is computed against specific font metrics, so substituting
does not merely change the look: signs can overlap, karaoke can drift, and line breaks move. That
audience is precisely the one [D11](#d11--subtitles-are-a-subsystem-not-a-feature) says we must
win. So this is a real cost, and the mitigation is **not** to weaken the default but to make the
opt-in good: the client shows that a track has embedded fonts, the user turns them on, and the
choice sticks per series like every other track preference
([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)). One
deliberate action, remembered — not a dialog per episode.

Two further reasons the client is the right decider, both matching
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest). **The risk is wildly
uneven across clients** — JASSUB's FreeType runs inside a WASM sandbox in the browser, while
libmpv's runs in-process and unconfined on Android and Apple — and only the client knows which
it is. And only the client knows its own FreeType version.

**When it is opted in, the path is still hardened.** The
[D55](#d55--the-served-font-set-is-the-cta-708-style-axis-in-variable-fonts-subset-per-subtitle)
subsetting pass is where the dangerous parts come off, and `hb-subset` has the controls already:

- `HB_SUBSET_FLAGS_NO_HINTING` drops `fpgm`/`prep`/`cvt ` — the **TrueType hinting bytecode**,
  a stack VM that FreeType interprets and a classic exploitation target.
- `--drop-tables+=CBDT,CBLC,sbix,SVG,EBDT,EBLC,COLR,CPAL` removes the embedded bitmap and colour
  tables — **where CVE-2020-15999 lived** — which a subtitle font has no use for.

Measured, and this is the unusual part:

```
CJK subset, as before                            582 K
CJK subset, --no-hinting                         498 K
CJK subset, --no-hinting --drop-tables+=…        498 K
```

**Hardening makes the output 14% smaller.** No trade-off to weigh, so apply it to every font we
serve, embedded or not.

Also: **never trust the container's declared MIME type.** Sniff the actual format, cap the
attachment size, cap the glyph count, and reject anything that is not a font we can parse — in
the node, before it is ever offered.

**What this does not claim.** Re-serialisation is **not** sanitisation. `hb-subset` parses the
hostile input with HarfBuzz, which has its own bug history, and a font crafted to attack the
*subsetter* is a live possibility. What it buys is that the client parses a file our tooling
emitted, with the hinting VM and bitmap tables gone, rather than the original bytes — and that
the parsing of hostile input happens in the node, inside the same Landlock+seccomp jail as
FFmpeg, rather than on someone's television.

**Status:** Provisional. The scope question it left open — per-user, per-library or per-server —
is **resolved by [D57](#d57--the-embedded-font-opt-in-is-per-user-sticky-per-series-and-the-client-may-still-refuse)**.
The default is not provisional.

---

## D57 — The embedded-font opt-in is per-user, sticky per series, and the client may still refuse

**Decided** (2026-09-19). Resolves the scope question left open by
[D56](#d56--embedded-fonts-are-off-by-default-the-client-must-ask-for-them). The opt-in is
**per user**, remembered at **series** scope, and it is a *preference* — the client still decides
whether to honour it on the device in front of it.

**It needs no new machinery.** This is precisely
[D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)'s
track-preference model with one more axis, and every rule there transfers unchanged:

- **Sticky scope is the nearest grouping ancestor** — the series for an episode, the item itself
  for a film. Exactly as D24 defines it.
- **Only an explicit user action updates the preference.** Nothing automatic may turn embedded
  fonts on, or the setting drifts toward whatever the files happen to contain — which here is
  not merely untidy, it is the security property quietly eroding.
- **Tri-valued: `on` / unset / `off`.** Unset means "ask"; `off` means the user said no and must
  not be asked again for that series. A user who declines once should not be nagged every
  episode.
- **Store a descriptor, re-resolve per file.** Never "use attachment index 3" — attachment
  indices shift between episodes the same way track indices do, which is the bug class D24
  exists to prevent.

The resolution chain is D24's, shortened, because there is deliberately **no per-library
default**:

```
explicit choice in this session
  → remembered choice for this series     ← the user's ask
    → off                                 ← the root default (D56)
```

**Why no per-library scope, when D24 has one.** A library default is how you would say "anime:
always load embedded fonts", and that is precisely the blanket grant D56 exists to prevent — it
converts a deliberate per-series decision into a standing permission over every file that lands
in a folder, including ones acquired later. Language preferences earn a library default because
getting them wrong is an annoyance; this one does not, because getting it wrong is a code
execution path. The asymmetry is the point.

**Why per-user rather than per-server.** The person who accepts the risk should be the person who
bears it. A server-wide switch means one household member's taste in fansubs decides what runs on
everyone else's television. It also keeps the blast radius legible: when something does go wrong,
"who turned this on" has an answer.

This has a consequence worth stating explicitly: **the served font resource must be authorised
per user**, not merely unguessable. If A opts in and B does not, B's client must not be able to
fetch the attachment — so the URL carries A's authorisation, not just a hash. That folds into the
existing signed-and-scoped-URL work rather than adding to it.

**And the client may still refuse, which is the part that matters most.** The preference says
*"this user wants correct typesetting for this series."* It does not say *"this device should
parse it."* Those are different claims and
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest) already separates them:
preference is user state, capability is device state, and the device decides.

The gap is large and real. In a browser, JASSUB's FreeType runs inside a WASM sandbox. On Android
and Apple, libmpv's runs in-process and unconfined, and may be FreeType ≤ 2.13.0 — the range
CVE-2025-27363 covers. So one user preference produces the right behaviour on both: the browser
honours it, and an old TV box declines and says why, without the user having to keep two settings
or understand the difference. The client that declines should say so in the same
"why is this different?" surface that explains transcoding decisions
([principles §3](../principles/README.md)) — *"this subtitle's own fonts weren't loaded on this
device"* — rather than silently rendering substituted metrics and letting the user wonder why the
signs overlap.

**Status:** Provisional in one detail. A per-device override — "never on this box", or conversely
"I know, do it anyway" — is probably wanted eventually, and is a strictly additive change to the
same chain. Not building it now. **The authorisation framing in this entry is corrected by
[D58](#d58--dont-hand-out-a-url-the-client-may-not-use-an-authorisation-check-that-fires-is-a-signal)**
— omission from the manifest is the mechanism; the server-side check is a backstop.

---

## D58 — Don't hand out a URL the client may not use; an authorisation check that fires is a signal

**Decided** (2026-09-19). Correcting the framing in
[D57](#d57--the-embedded-font-opt-in-is-per-user-sticky-per-series-and-the-client-may-still-refuse).

D57 said the served font "must be authorised per user, not merely unguessable" and that a
non-opted-in user's client "must not be able to fetch" it. That describes a server that **says
no a lot**, which is the wrong design. **A client asking for something it is not entitled to is
a bug**, and a design where that happens routinely cannot tell a bug from an attack.

**The mechanism is omission, not rejection.** A user who has not opted in gets a manifest that
**does not contain the embedded-font resource at all**. There is no URL to fetch, so there is
nothing for a correct client to decline to request — the illegal action is unreachable rather
than forbidden. This is just
[D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)'s model applied honestly: the
manifest is the menu, and you do not put a dish on the menu that the customer may not order.

**The server-side check still exists**, because defence in depth, and because a client is not a
trust boundary. But its role changes completely:

| | Routine control flow | Backstop |
| --- | --- | --- |
| Fires when | normal use | never, in a correct system |
| A rejection means | "working as intended" | **a bug, a stale client, or an attack** |
| Can you alert on it | no — it is noise | **yes** |

That table is the whole value of the correction. An enforcement check that fires during normal
operation is not enforcement, it is control flow wearing a security costume; it generates
exactly the traffic that would hide a real attempt. Move the decision into what the manifest
contains and the check goes quiet — and a quiet check is one whose every firing is worth
looking at.

**The general rule, because this is not about fonts.** It applies to signed and scoped media
URLs, the transcoder plugin's capability grants ([D19](#d19--the-transcoder-is-a-plugin-we-happen-to-maintain-plugin-tiers-become-three)),
the Jellyfin adapter, and every API surface where our own client is the caller:

> **Give a client only what it may use, and treat the refusal path as an alarm rather than a
> branch.**

**One distinction that must not be lost**, or this rule does real damage: it governs
**authorisation of our own cooperating client**, not **validation of untrusted input**. Node
reports are expected to arrive malformed sometimes and the core is expected to reject them
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global),
[D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides)) — that check *should*
fire, routinely, and alerting on it would be nonsense. Media is attacker-supplied and FFmpeg's
confinement is expected to be load-bearing
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)). Conflating
"validation of hostile data" with "authorisation of a first-party client" is how a project ends
up with no signal in either: the security events drown in the normal ones.

**Practical consequence for D57.** The per-user opt-in state decides **manifest content**, not
just request handling. That is a small implementation difference and a large operational one,
and it is cheaper to build this way than the other way.

**Status:** Provisional only in how the alarm surfaces — a log line, a counter, or something the
user sees. Promoted to [principles §10](../principles/README.md), because it decides arguments
well beyond this one.

---

## D59 — A node publishes a snapshot plus a journal; it observes, it does not interpret

**Decided** (2026-09-19). Correcting [scanner.md](../design/plans/scanner.md), whose Shape
section still described Navidrome-style pipeline phases — *enumerate → detect missing/moved →
**identify and enrich** → **resolve playlists/collections*** — written on 2026-09-17, before
[D33](#d33--hub-and-spoke-a-core-that-never-touches-media-and-n-media-nodes-that-do) moved the
scanner into the node and before
[D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides) made nodes sensors. The
last two of those phases are interpretation, and the document was never re-read after the
relocation.

### What a node actually has, and therefore what it can say

A node sees the directories and files under its roots, and nothing else. It has no library, no
user, no history beyond what it has itself observed, and no idea that the file it is looking at
is episode 4 of anything. So the only thing it can honestly offer is **a temporal view of a
subtree**: what is there now, and what has changed since.

That is a **snapshot plus a journal**:

- **Snapshot** — a complete enumeration of the node's roots at a point in time. This is
  [D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)'s
  report, renamed for what it actually is, keeping every property it already had: JSONL,
  header/entries/trailer, completeness in the filename via `.partial` → atomic rename, and
  completeness **scoped per root and per errored subtree**.
- **Journal** — append-only observations since that snapshot, one line each, same record
  grammar, same error and skip records with their scopes ([D40](#d40--error-and-skip-records-and-why-the-error-record-carries-the-completeness-scope)).

Both carry a **monotonic sequence number**, and the snapshot records the sequence it corresponds
to. The core tracks the last sequence it consumed. If the oldest journal line the node still has
is newer than that, there is a gap, and the core falls back to the snapshot.

**The snapshot is always a valid recovery point, so the node never has to know what the core has
read.** That is the property that keeps this inside D42: no back-channel, no acknowledgement
protocol, no node-side model of core state. Compaction is a node-local storage decision, and
the worst consequence of getting the bound wrong is that the core re-reads a snapshot.
**How that bound is chosen is [D60](#d60--snapshotting-and-discarding-are-different-operations-with-different-triggers)**,
which corrects this entry's conflation of the two.



### The snapshot is not only compaction; it is the only thing that corrects a missed event

Worth stating because it sets the interval. The journal is as good as the watcher that feeds it,
and [D27](#d27--change-detection-is-a-strategy-interface-that-reports-which-strategy-it-is-using)
established that watchers **fail silently** — `inotify` does not work over SMB or NFS and has
watch limits a large library exhausts. A journal cannot show you the event nobody delivered.

A snapshot enumerates, so it catches everything the journal missed. Compaction and correctness
turn out to want the same mechanism, which is convenient, but the correctness requirement is the
one that sets the floor: **periodic full snapshots are mandatory even with a perfect watcher**,
because you cannot know the watcher is perfect.

### The node does not parse filenames

A filename is **reported verbatim** — NFC form plus raw bytes ([D49](#d49--unicode-discipline-at-the-boundaries-enforced-by-tests)) — and never parsed into series, season or episode by the node.

The §9 test says why: the *right* parse depends on the library it lands in, on whether this
series already exists with a known convention, on whether absolute numbering is in play. The
node has none of that.

But the decisive reason is practical and has nothing to do with purity. **The parser will get
better.** If the node parses, then improving the parser means re-scanning every node across
every filesystem, including the NAS over SMB and the one that is currently offline. If the core
parses stored raw paths, improving the parser is a re-interpretation pass over the database that
touches no filesystem at all. That is the same argument as
[D13](#d13--no-telemetry-no-account-no-upsell--as-a-stated-promise)'s rebuildable index, and it
is worth a great deal the first time a regex is wrong.

So the filename parser, its Unicode-safe corpus, and the derivation of track *kind* from title
strings ("English (SDH)", "Signs & Songs") all move to the **core's identification stage**.

### But the node absolutely does look inside the file

This is the other half and it is not a contradiction. The extension is a **hint about whether
probing is worthwhile**, never a conclusion about content, because people misname things
constantly. The node probes and reports what is actually there: container, streams, codecs,
durations, disposition flags, track titles, attachment list.

That is observation and it stays in the node — the core never opens a media file
([D26](#d26--no-ffmpeg-in-the-core-process-confine-it-with-landlock--seccomp)). **When the probe
disagrees with the extension, report both.** The disagreement is itself a fact, and what it
means — a mislabelled file, a corrupt one, a deliberate container swap — is the core's call.

Canonicalisation that is lossless and context-free stays in the node, reported **alongside the
original**: a track's BCP 47 tag beside the raw `ger`/`deu` the container carried
([D24](#d24--language-is-a-preference-axis-and-track-selections-stick-to-the-series)), the NFC
path beside the raw bytes. Anything that is a *heuristic over text* is interpretation and goes
to the core, by the improve-later argument above.

### Report the event you were given; do not synthesise linkage

A node must not emit "this file moved from A to B" when what it saw was one path disappear and
another appear. Deciding those are the same file requires hash or inode history the core holds.
Report `A gone` and `B appeared`; the core decides it was a move.

The converse matters equally: when `inotify` hands over a `MOVED_FROM`/`MOVED_TO` pair sharing a
cookie, **the kernel linked them and reporting the link is reporting what you saw.** The rule is
not "never emit a move", it is *never invent a relationship the source did not give you*.

### Node-local state may change cost, never content

The node needs a probe cache keyed by path, size and mtime, or every snapshot re-probes fifty
thousand files. §9 warns specifically against giving a sensor a memory, so the boundary needs
stating:

> **Delete the node's local state and it must report exactly the same thing, only slower.**

A cache that can change *what* is reported has stopped being a cache and become judgement. This
is the test to apply whenever node-local storage is proposed.

### What happens to "phases"

The legibility goal behind them was real — *"logging around library scanning being pretty
difficult to interpret"* is a direct Jellyfin complaint — but it is already met by
[D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)'s status record
and [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)'s
`state`/`task` pair. Named activities survive as **progress labels**, not as a pipeline whose
later stages draw conclusions.

**Status:** Provisional. The compaction bound — maximum journal length before a fresh snapshot —
is unknown and needs measurement against a real library and a real watcher; it joins the other
threshold guesses in [performance-targets.md](../design/plans/performance-targets.md). This
**extends rather than supersedes D39**: every property of the report survives, applied to the
snapshot.

---

## D60 — Snapshotting and discarding are different operations with different triggers

**Decided** (2026-09-19). [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret)
said "a maximum journal length or size, after which a fresh snapshot is written and older pairs
may be dropped." That sentence conflates two operations that want opposite trigger conditions.
Separating them:

| Operation | Trigger | Why |
| --- | --- | --- |
| **Take a snapshot** | **temporal** — every `snapshot_interval`, plus on request | It is the correction for a silently-failed watcher, so its cadence is a correctness floor, not a housekeeping one |
| **Discard journal entries** | superseded by a snapshot **and** older than `journal_retention` | Retention is what buys the core time to catch up |
| **Emergency discard** | `journal_max_bytes` exceeded | A safety valve against disk exhaustion, and **loud** |

A snapshot no longer implies discarding anything. That alone removes most of the hazard.

### Why size is the wrong primary trigger

The obvious reason is the stated one: a large filesystem change can blow a size bound in
seconds and erase journal the core has not read. A core that is simply **switched off for the
weekend** — an ordinary thing for a home server — plus a torrent client finishing a season pack
is enough.

But the sharper reason is about *when* it fires. **The burst that blows a size bound is, almost
by definition, the anomalous event** — a mass deletion, a re-organisation, a share coming back
with different contents, a permissions change. Size-triggered compaction therefore forces a
fresh full enumeration at precisely the worst moment, and replaces an ordered record of what
happened with a single point-in-time reading taken mid-disruption. It is the backup that runs
during the incident.

Temporal triggering **decorrelates** the measurement from the events being measured, which is
the property you want from any instrument.

And the economics run the right way round. **A quiet library produces almost no journal**,
because a journal line is a changed file. So temporal retention is nearly free in the common
case and only costs during churn — which is exactly when the history is worth having. A size
bound is backwards: it is most restrictive precisely when there is most to say.

### Being honest about what this buys

It is **not** correctness of the library. The snapshot is a complete, scoped enumeration
([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)), so a
core that loses journal still converges — it just re-reads everything. The protections against a
misleading snapshot live in the core's own history and in
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse)'s
observed-empty-versus-not-enumerated distinction, not in the journal.

What it buys is three real things: **diagnosis** — fifty thousand ordered, timestamped events
distinguish "the user re-organised their library" from "the mount flapped" in a way two
snapshots six hours apart never will, and that is the question
[D41](#d41--diagnose-the-failure-boundary-by-walking-up-and-guard-the-silent-case-which-is-worse)
exists to answer; **cost** — re-reading a 48,000-line snapshot over SMB is not free; and **not
taking an expensive measurement during an anomaly**.

Overstating it as a data-loss fix would be wrong, and the design is easier to reason about with
the actual reason written down.

### Pure temporal has its own failure mode, hence the valve

"Retain for seven days" meets a rename loop, a misbehaving sync client, or a filesystem doing
something pathological, and the node fills its own disk. That is **worse** than a discarded
journal: a node that cannot write cannot write its status record
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)) either, so it
stops looking wedged and starts looking dead.

So `journal_max_bytes` exists as a backstop — and per
[principles §10](../principles/README.md), a backstop that fires is a signal, not a branch.
Hitting it means the node emits an `error`-class record saying it discarded history, how much,
and over what span. The core surfaces that; it does not silently absorb it.

When the valve trips the node **takes a snapshot and keeps as much journal as the budget
allows**, rather than discarding first. Snapshot-then-trim, never trim-then-snapshot.

### Defaults, and who sets them

`snapshot_interval` is a correctness parameter and wants to be short enough that a silently
dead watcher is caught within a day. `journal_retention` wants to be long enough to survive a
plausible core outage — days, not hours — and is nearly free when the library is quiet.

The node must **not** derive these from the core's polling behaviour. That would be the node
modelling core state, which [D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval) already
rejected for the cadence question, and a node under stress is not a reliable source for the
standard it is judged against. They are config fields
([D35](#d35--node-exchange-directory-progress-pair-config-generation-and-permissions-as-the-trust-boundary)),
with defaults the user can raise.

**`snapshot` joins the command vocabulary.** [D38](#d38--three-tier-state-reporting-and-an-ordered-command-list-in-the-config)
already defines an ordered command list — `cancel`, `scan`, `rescan <root>`, `pause`, `resume` —
so a core that wants a fresh enumeration now can ask, and one that knows it has fallen behind
can simply not ask. This is a *request for an action*, not the core dictating retention policy,
so it stays within D42.

**Status:** Provisional in its numbers, which are guesses until measured against a real library
and a real watcher — they belong with the other thresholds in
[performance-targets.md](../design/plans/performance-targets.md). The **separation** of the two
triggers is not provisional; it is a correction.

---

## D61 — Containment now, detection later and plugin-shaped; four affordances to keep open

**Decided** (2026-09-19). Proactive threat scanning of media, subtitles, fonts and posters is
**not for the first release**, and the research says its eventual shape is different from the
obvious one. What we do now is keep four seams open, all at near-zero cost, and avoid the two
mistakes that would close them.

### The surface is real and it is not only video

39 entries in CISA's Known Exploited Vulnerabilities catalogue touch media, image or font
parsing — Apple's Image I/O (CVE-2025-43300, August 2025), Samsung's image codec twice in 2025,
FreeType twice. So including subtitles, fonts and posters rather than only video is correct. And
posters are the one binary we fetch **from a third party by default** — TMDB — rather than from
the user's own disk.

### But every platform that has faced this chose containment over detection

**Apple built BlastDoor** after the 0-click iMessage attacks: it *"isolates, parses, transcodes,
and validates untrusted data"* with *"sandbox restrictions and memory safe validation of
output"*, almost no filesystem access and no network. Apple's own description mentions **no
signature scanning or content detection at all**. **Netflix's** published media tooling (Photon)
is *conformance* validation against SMPTE ST 2067-2, not threat detection. **Google's** answer is
fuzzing plus sandboxing — and **FFmpeg, FreeType, HarfBuzz and libass are all in OSS-Fuzz**, so
our entire parsing stack is already continuously fuzzed by a better-resourced programme than we
could run.

That last point reframes the investment. **Shipping current builds promptly beats scanning**,
and it is cheaper. A concrete deliverable falls out of it: the server should know which FFmpeg,
libass and FreeType it is running and tell the user when they are stale — which is the same
information [D53](#d53--font-coverage-is-not-assumable-the-server-serves-fonts-on-demand) wants
about client FreeType versions anyway.

### The scanner is a file parser, which is the thing we are defending against

ClamAV has **158 CVEs**, 29 since 2023, **seven on a single day** (2026-08-07) — XAR, Mach-O,
PDF, GPT, PESpin and two zip parsers, each "an unauthenticated remote attacker could…" from
parsing a file. Adding a scanner to defend FFmpeg means adding a component with a *worse*
parser-CVE record and pointing it at the same bytes.

This does not forbid it. It settles the shape: **any scanner is another hostile-input parser —
tier-3 plugin, inside the same jail, no additional privilege, never a trusted security
component.**

### "Look for shellcode" is the part that does not survive

Saying so plainly because it is the most tempting item. Modern exploits often contain **no
shellcode at all** — ROP and JOP chains are assembled from gadgets already in the process, and
data-only attacks corrupt state without injecting code; a current FFmpeg exploit is typically a
malformed header causing a type confusion, with nothing byte-like to match. And **compressed
video is high-entropy and statistically indistinguishable from packed or encrypted payloads**,
so the classic entropy tell is a 100% false-positive rate on our actual corpus.

The intent — *notice that this file is trying something* — is right. The mechanism below is
better.

### The four affordances to keep open now

Each costs approximately nothing today and is expensive to retrofit.

1. **Unknown record kinds must be skipped, not fatal.** This is the load-bearing one. Reports are
   JSONL and degrade per line ([D39](#d39--reports-are-jsonl-completeness-is-in-the-filename-and-is-scoped-not-global)),
   and the protocol is versioned ([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)).
   Make it an explicit rule **now** that the core ignores record kinds it does not recognise,
   counts them, and does not treat the report as malformed. Then an `anomaly` record can be added
   later without a protocol break or a node/core upgrade ordering problem.
2. **The jail must be able to report, not only kill.** seccomp already has the primitives:
   `SECCOMP_RET_LOG` (allow after logging), `SECCOMP_RET_TRAP` (catchable `SIGSYS`, so a worker
   can report before dying), `SECCOMP_RET_USER_NOTIF` (notify a supervisor that can decide), and
   `SECCOMP_FILTER_FLAG_LOG`. Choosing a filter action that can be observed costs nothing now,
   and it means **our sandbox is already a detector and we are simply not reading it**. A file
   that makes FFmpeg attempt an unexpected syscall is stronger evidence than any signature will
   ever be.
3. **Record per-file resource peaks while probing.** The realistic threat to a home server is
   **denial of service, not RCE** — ASS render bombs (Jellyfin ships `libassMemoryLimit` and
   `libassGlyphLimit` for exactly this), decompression bombs in WOFF2/brotli fonts and PNG
   posters, files that pin a CPU forever. We already have cgroups
   ([D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor),
   [D44](#d44--prefer-the-platforms-supervisor-ship-the-units-and-never-exit-when-you-could-report)), so peak
   CPU, memory and output size per file is free, and an outlier is visible with no new machinery.
4. **Quarantine is a library state, never a file operation.** Never move, never delete, never
   modify — [D7](#d7--the-files-are-the-source-of-truth-the-database-is-a-rebuildable-index) and
   [principles §2](../principles/README.md). A suspected file is *marked and not played by
   default*; the bytes are untouched.

### And the two mistakes to avoid

**The node must never conclude "malicious."** It reports observations — seccomp violation,
parser crash, declared type ≠ sniffed type, 1.2 GB decompressed from 40 KB — and the core
decides what they mean, per [D42](#d42--nodes-are-sensors-the-core-is-the-only-thing-that-decides)
and [D59](#d59--a-node-publishes-a-snapshot-plus-a-journal-it-observes-it-does-not-interpret).
A node that quarantines is a sensor making policy.

**Do not cry wolf.** These are the user's own files, acquired over fifteen years, many encoded by
broken tools. A server that warns about a 2004 home-video rip has not made anyone safer; it has
taught them to dismiss warnings, which is a security failure and not a cosmetic one.
[Principles §3](../principles/README.md) requires explaining the decision, and a warning the user
cannot act on is not an explanation. Default silent; surface only concrete observations; always
say what to do. *"This file used 40× the memory of anything else in your library while probing"*
is actionable. *"Possible shellcode detected"* is not.

**Status:** Provisional and deliberately unscheduled — see
[content-safety.md](../design/plans/content-safety.md). The four affordances are not
provisional; they are cheap now and awkward later, and item 1 in particular is a wire-format
rule that has to exist before the first node ships.

---

## D62 — Watch Together is a first-class feature, and the per-client surface is six methods

**Decided** (2026-09-19). Build synchronised remote playback with shared controls: anyone in the
group may play, pause and seek; volume stays local. Research:
[watch-together.md](../design/research/watch-together.md).

### Why this is a differentiator rather than a nice-to-have

**Almost every commercial implementation has been shut down.** Disney+ GroupWatch removed
2023-09-18; Amazon Prime Video Watch Party removed 2024-04-02 with no announcement; Sling early
2024; Netflix never shipped one at all; only Hulu survives. And **Plex removed Watch Together on
2025-02-25** for every platform except web.

Plex's own wording names the cause, and it is neither licensing nor lack of demand:

> While this feature won't be available for **most platforms as they get the new experience**,
> you can continue using the feature in our web app…

**It had to be implemented in every client, and it did not survive a client rewrite.**

Demand is not in doubt: **Teleparty**, a browser extension that exists solely because the
platforms will not do this, has **10,000,000 users**. The incumbents abandoned a wanted feature
for a reason we can engineer around, which is the best kind of opening.

### The requirement that follows directly from how theirs died

> **The client-specific surface must be small enough to reimplement in an afternoon.**

Everything else — clock-offset estimation, drift policy, group state, suspensions, command
sequencing, reconnection — lives in the shared TypeScript SDK that
[web-client.md](../design/plans/web-client.md) already calls the real deliverable. The client
implements six methods:

```
currentPosition() → time      seek(time)      play()
isBuffering() → bool          setRate(r)      pause()
```

A client that can do those six gets watch-together, and a client rewrite cannot casually drop
the feature because there is nearly nothing in the client to drop. This is the whole lesson from
five dead implementations, and it is a constraint on our own roadmap as much as a design note.

### The sync mechanism, taken from Jellyfin because it works

Jellyfin's SyncPlay is 7,635 server-side lines and the mechanism is sound:

- **Clock offset by NTP-style four-timestamp exchange**, keeping a rolling window and using the
  **minimum-delay sample**, because the least-delayed sample carries the least queuing noise.
  Greedy polling at 1 s initially, 60 s thereafter.
- **Two correction strategies with a threshold between them**: nudge `playbackRate` for about a
  second to absorb roughly 60–3000 ms of drift; seek for anything larger.
- **A four-state group machine**: `Idle`, `Waiting`, `Paused`, `Playing`, where `Waiting` means
  paused-until-everyone-is-ready.

Their `TODO` comments are the caveat and we should not discover it again the hard way:
*"SpeedToSync is failing on Safari"*, *"a hard time keeping up on Android Chrome"*. Rate nudging
is inconsistent per platform, so `setRate` is **optional** in the six-method contract and
seek-only is the always-available fallback. Whether rate nudging works on our clients is a
device-lab measurement, not an assumption.

### Suspensions with reasons, which is Apple's idea and the best one here

A participant temporarily detaches from group coordination, **stating why**; while suspended it
neither obeys nor issues group commands; several can be active at once; when the last clears it
rejoins. Apple enumerates `stallRecovery`, `audioSessionInterrupted`, `playingInterstitial`,
`userActionRequired`, `coordinatedPlaybackNotPossible` and — the important one —
**`userIsChangingCurrentTime`**.

That last is what makes shared seek control usable at all: **while you are dragging the scrub
bar you are suspended**, so your intermediate positions do not drag everyone else across the
timeline. Without it, "anyone can seek" is unusable, and it is not a thing one would think of
before shipping it badly once.

A reason is also better than a flag for the UI, per [principles §3](../principles/README.md):
"Karl is buffering" is something a person can wait for.

### Concurrent commands are normal, so the server serialises

"Anyone can control it" means two people pausing simultaneously, or one seeking while the other
resumes, is ordinary traffic rather than an error. The server is already in the path: each
command carries the group-state version it was issued against, the server assigns a monotonic
sequence, and clients reconcile **to the server**, never to each other. Commands are therefore
ordered and idempotent, which is what stops the both-pause case oscillating.

**Sync does not go over a WebRTC data channel.** `react-native-webrtc` does not support data
channels on tvOS at all, and it would make synchronisation depend on peer connectivity. It goes
over our own WebSocket, which is also the serialisation point.

**Status:** Provisional. Two questions are unanswered and both are group-size dependent: whether
one participant stalling pauses everyone (Jellyfin makes it a flag; obviously yes for two,
obviously no for eight), and whether the subtitle track is group state or local — one person may
need subtitles, and "seek to the funny bit" and "turn subtitles on" then behave inconsistently.

---

## D63 — Voice chat is WebRTC, and headphones are the supported configuration

**Decided** (2026-09-19). Voice chat over WebRTC, peer-to-peer where possible, with per-source
gain so one loud participant does not drown the film. **Headphones are the supported
configuration and the UI says so.** Speakers are best-effort.

### The hard part is echo cancellation, not the plumbing

The film is playing out loud while the microphone is open. Acoustic echo cancellation works by
subtracting a **reference signal** — what was sent to the speaker — from what the microphone
hears, and the AEC only gets a reference for audio it knows about. **Only the official playback
paths serve as references; custom playback through the Web Audio API does not reliably function
as one.** On native clients the film is decoded by libmpv, ExoPlayer or AVPlayer and reaches the
output by a path the WebRTC stack has no reference for at all.

So without headphones, **your microphone sends the film back to the other person**, delayed and
doubled against their own copy. That is not a tuning problem, and Bluetooth output makes it
worse because AEC needs the reference and the echo time-aligned while wireless adds variable
latency.

Saying "use headphones" is not a cop-out; it is what every voice product assumes in practice,
and with headphones at both ends there is no acoustic path and the entire class of problem
disappears.

### Ducking conflicts with AEC, and the resolution is a rule

Ducking the film when someone speaks means routing the film's audio through a mixer we control —
in a browser, the Web Audio API — which is **exactly what stops it being a usable AEC
reference**. So:

- **Headphones, no acoustic path:** route through Web Audio, duck freely, AEC irrelevant.
- **Speakers:** leave the film on the native path so AEC keeps its reference; accept manual
  levels instead of automatic ducking.

Per-source gain is independent of both and is simply a gain node per remote stream.

### On a television, the phone is the voice endpoint

Measured from the `react-native-webrtc` capability matrix: tvOS has audio/video but **no data
channels**, needs a locally built `WebRTC.framework`, and carries an open tvOS 16 compatibility
issue. Separately, an Apple TV has no app-accessible microphone and neither do most Android TV
boxes — and a microphone across the room from the speakers is the worst possible AEC geometry
even if one existed.

Phone-and-earbuds for voice, television for video. It is the only workable configuration and it
is already a familiar pattern.

**Status:** Provisional. **Extended by [D65](#d65--playback-offset-is-per-participant-and-it-is-an-echo-control)**,
which adds two tiers below headphones and makes speakers genuinely workable rather than
best-effort. Push-to-talk is the guaranteed answer for the speakers case and is unromantic for
the two-people-watching-a-film case this exists for; worth offering, wrong as a default. Text chat is deferred — and the honest note is that on a TV it is nearly useless, no
keyboard and one screen, which is presumably why Disney+ GroupWatch shipped emoji reactions
instead.

---

## D64 — Watch Together makes the remote-access question concrete, and supplies the premium tier's first honest member

**Decided** (2026-09-19). Watch-together is the use case that forces
[D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s open product
question, and it resolves *what the paid tier is for* even though it does not resolve whether we
operate one.

Three things are needed for the actual scenario — one participant at home, one travelling — and
they have very different costs:

1. **She must reach the media at all.** Already the project's stated open question
   ([remote-access.md](../design/research/remote-access.md)). Watch-together adds no new
   requirement here; it just makes the absence conspicuous.
2. **A rendezvous for synchronisation.** **Free.** If she is connected to the server, the server
   *is* the rendezvous, and [D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
   sync channel is the connection that already exists. No new infrastructure.
3. **A path for voice.** Peer-to-peer where NAT allows — remote-access.md's research puts direct
   connection above 90% with Tailscale-style hole punching — and a **TURN relay** for the
   remainder.

That third item is the point. A TURN relay is **unambiguously another machine that somebody must
run**, with bandwidth costs that scale with use, and it cannot be self-hosted by the person who
needs it, because the whole reason it exists is that their network cannot be reached.

Which makes it the cleanest possible test of the README's promise: *"we only charge for things
that genuinely require another machine."* Most candidate premium features fail that test on
inspection. This one passes it without argument, and the ~10% figure means the free path is the
common path rather than a crippled teaser.

**What this does not settle:** whether we operate a service at all. That still implies a
company, uptime obligations and an entity that can be pressured, and declining it remains
coherent — the cost is leaving the headline use case to Tailscale, which is a fine answer for
this project's likely early audience and a poor one for the household that motivated it.

**Status:** Provisional, and the least technical entry in this log. What it fixes is the *shape*
of the answer, not the answer. **Corrected by [D68](#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay)**
— voice needs no separate relay, because the media server is already mutually reachable. The
premium tier's justification is remote access itself, not voice.

---

## D65 — Playback offset is per-participant, and it is an echo control

**Decided** (2026-09-19). The group's shared state is a position **plus a per-participant
offset**. Target position for participant *i* is `P + offset_i`, default zero. This started as a
field observation and turns out to have a derivation, an optimum, and an automatic
configuration.

### The observation

Watching with two players and voice over WhatsApp, the echo was fixed by **deliberately delaying
one side's playback** so the leaked film audio arriving over the voice channel lined up with the
local copy. No implementation in the survey does this — Jellyfin, SharePlay and Teleparty all
target identical positions, which is the configuration that **maximises** the echo.

### Why it works, and where the optimum is

Let participant A be ahead of B by `d`, and let one-way voice latency be `L`.

B hears their own film at content position `p_B(t)`, and hears A's leaked room audio captured
`L` ago, which was at `p_A(t−L) = p_B(t) + d − L`. So:

```
misalignment at B  =  L − d      → zero when d = +L   (A ahead by L)
misalignment at A  =  L + d      → zero when d = −L   (A behind by L)
```

Three consequences, and the third is the useful one:

- **Both cannot be zero.** At `d = 0` — what everyone builds — *both* participants suffer a full
  `L` of misalignment. Zero offset is not neutral; it is the worst symmetric choice.
- **Delaying yourself fixes the echo you hear**, at the other person's expense. Which is exactly
  the observed behaviour and confirms the sign.
- **When only one side leaks, it is fully solvable.** If B wears headphones and A does not, only
  B hears an echo, and `d = +L` removes it completely. The rule:

  > **The participant whose room audio leaks runs *ahead* by the one-way voice latency.**

This matters perceptually rather than marginally. Below roughly 30 ms the precedent effect
fuses the two copies into mild coloration; by 150–300 ms — ordinary voice-path latency — it is a
distinct and very disruptive slapback. Moving from `L` to near zero is the difference between
unusable and unnoticeable.

### We can set it automatically, and nobody else can

We already measure round-trip latency on the voice connection, so `L ≈ RTT/2` is in hand. So
"align our audio" can be a button rather than a fiddle, with a manual nudge for the case where
the estimate is off — and the manual nudge is worth keeping regardless, because the user found
this by hand before anyone built it.

### The three-tier answer to echo, which supersedes "just use headphones"

[D63](#d63--voice-chat-is-webrtc-and-headphones-are-the-supported-configuration) was right that
headphones remove the problem and wrong to stop there, since headphones are not always possible.

| Tier | Mechanism | Cost |
| --- | --- | --- |
| 1 | **Headphones** — no acoustic path at all | none |
| 2 | **Offset alignment** — this decision | a field in the group state |
| 3 | **Content-referenced AEC** — use the decoded film audio as the echo reference | real work |

Tier 3 deserves stating because it is **uniquely available to us**. D63 observed that the
platform's AEC cannot cancel the film because it has no reference for audio played by libmpv,
ExoPlayer or AVPlayer. But *we are the player*. We know exactly what the film's audio is, sample
for sample, because we decoded it. Feeding that to a standalone echo canceller — `speexdsp`
(BSD-licensed, actively maintained) or the extracted `webrtc-audio-processing` (BSD-3-Clause) —
gives a **better reference than the operating system can supply**, because the OS only sees the
mixed output while we see the exact source. The difficulty is delay alignment against unknown
output latency, which adaptive cancellers estimate anyway.

Tiers 1 and 2 are nearly free and should ship together. Tier 3 is a later, separable
improvement.

### Why the data model change has to happen now

`P + offset_i` instead of `P` is a trivial generalisation on day one and an awkward retrofit
later, because it touches the group state record, the drift-correction target, and every
"are we in sync?" comparison. It also pays for itself elsewhere: a participant on a
consistently slower link can be given a standing offset instead of being seeked at constantly.

**Status:** Provisional in its automation only. The mechanism is derived and field-confirmed;
whether `RTT/2` is a good enough estimate of one-way voice latency in practice is measurable and
unmeasured, and asymmetric routes will make it wrong sometimes — hence the manual nudge.

---

## D66 — No clock synchronisation: monotonic local time and bounded round-trips

**Decided** (2026-09-19). Clients never synchronise clocks with the server or with each other.
Each keeps a **monotonic** local time, reports its position against it, and the server bounds
the uncertainty with a round-trip measurement. This replaces
[D62](#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
description of the offset exchange as "NTP-style".

### The question was whether device timers are good enough on their own

They are, for the interval that matters, and the arithmetic is worth writing down. **50 ppm
between two consumer devices' clocks is the realistic figure** — a crystal rated 48,000 Hz runs
at 48,000.5 on one device and 47,999.5 on another. So:

```
50 ppm over 10 minutes  →   30 ms      below Jellyfin's 60 ms "do nothing" threshold
50 ppm over 20 minutes  →   60 ms      at the threshold
50 ppm over 3 hours     →  540 ms      well past it
```

So the intuition is quantitatively right: **local timers need reconciling every ten to twenty
minutes, not every second.** Jellyfin polls at 60 s, which for drift alone is roughly 40× more
often than necessary.

### But clock drift is not the dominant term, which changes the design

A stall, a burst of dropped frames, or a seek landing on the nearest keyframe instead of the
requested position moves a client by 100 ms to 2 s **instantly**. No amount of timer accuracy
helps, and no polling interval catches it promptly without being wasteful the rest of the time.

So reconciliation is **two mechanisms, not one**:

- **Periodic**, on a timer measured in minutes, for slow clock and playback-rate drift.
- **Event-driven**, triggered by the client itself — the `isBuffering()` transition, a completed
  seek, a resume — which is when position actually jumps.

The event-driven half is the one that matters, and it is nearly free because those events are
already in the six-method client contract.

### What we never needed was a shared clock

This is the same trick the project already uses for node liveness
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle),
[D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)):
*the core knows how much time passed between its own observations, and does not care what time
any component thinks it is.*

Applied here: the client reports "my position was `P` when my monotonic clock read `M`". The
server timestamps its own send and receive, computes the round trip, and knows the client's
position **now** to within that round trip. Keeping the **minimum-RTT sample** over a short
window removes most of the queuing noise — that one trick is worth borrowing from Jellyfin and
is the only NTP-ish idea left.

No clock offset estimation, no absolute time, no dependence on device wall clocks.

**And using wall clocks would be actively worse.** A system clock can be *stepped* — an NTP
daemon correcting a phone mid-film, a timezone change, a user fixing the clock — producing a
spurious jump of arbitrary size in the middle of a sync calculation. Monotonic sources
(`performance.now()` in browsers and React Native, `CLOCK_MONOTONIC` and `mach_absolute_time`
natively, and Go's `time.Now()` which carries a monotonic reading that `Sub` uses) are immune to
all of it. So the simpler design is also the more correct one, which is not the usual trade.

**Status:** Provisional in its intervals. The 10–20 minute figure is derived from a 50 ppm
assumption, not measured on our hardware; the device lab should measure real playback-position
drift per platform, because playback-rate error will likely dominate crystal error and may be
much worse on cheap devices. Joins the threshold list in
[performance-targets.md](../design/plans/performance-targets.md).

---

## D67 — tvOS never gates a feature

**Decided** (2026-09-19). Where tvOS cannot do something, tvOS does without it. No feature is
cut, delayed or redesigned to keep the Apple TV client at parity.

**Why this needs saying.** [D30](#d30--tvos-is-cheap-from-inside-the-apple-stack-and-never-has-a-web-fallback) established
that tvOS is a small share of an Apple client's code, and
[tvos.md](../design/research/tvos.md) catalogued how it breaks nearly every cross-platform
option — no WebKit, a Focus Engine that needs UIKit views, TVMLKit deprecated. Watch Together
made the pattern concrete again: `react-native-webrtc` has no data channels on tvOS, needs a
locally built framework, and carries an open tvOS 16 compatibility issue; and an Apple TV has no
app-accessible microphone at all.

The temptation at each of those is to design around the weakest platform. Refused. The cost of
parity is paid by every other client and by every future feature, and it compounds.

**The escape hatch already exists**, which is what makes this safe rather than reckless.
[D18](#d18--the-jellyfin-compatible-surface-is-an-adapter-and-it-quarantines-device-profiles) ships a
Jellyfin-compatible adapter, and Swiftfin and Infuse are mature Apple TV clients that speak it.
So an Apple TV user is not stranded; they get a good third-party client with a reduced feature
set, which is exactly the outcome this decision accepts.

**The bail-out, stated so it is not a slow drift.** If the list of tvOS exceptions keeps growing,
**dropping first-party tvOS support entirely** is on the table and is not a failure — it is the
adapter doing its job. The trigger is qualitative and should be reviewed rather than computed:
when a tvOS workaround starts changing a design that would otherwise be simpler, that is the
signal.

**Status:** Provisional in scope: this is about *tvOS*, not about Apple. iOS and macOS are
ordinary first-class targets.

---

## D68 — TURN is not an independent requirement; the media server is already the relay

**Decided** (2026-09-19). Correcting
[D64](#d64--watch-together-makes-the-remote-access-question-concrete-and-supplies-the-premium-tiers-first-honest-member),
which claimed a TURN relay was the premium tier's first unarguable member. It is not, because we
are not solving the general peer-to-peer problem.

### The correction

For Watch Together the two endpoints are two people — but there is a third party both are
**already connected to by construction**. If she cannot reach the media server there is no film
to watch together. So by the time voice matters, a mutually reachable rendezvous **already
exists**, and it can carry the voice.

Three reasons this is not a grudging fallback but the better answer:

- **Bandwidth is nothing.** Opus speech is ~24–64 kbps per stream, against a film already
  crossing the same uplink.
- **Latency is nearly nothing**, which is the counter-intuitive part. A conventional TURN sits
  in a datacentre *between* the peers and adds a genuine detour. Ours sits **at one endpoint** —
  the server is on the home participant's LAN — so `her → server → him` is essentially
  `her → him`.
- **No new service, no new machine, no new failure mode.** It is the connection that is already
  carrying the film.

So the premium tier's justification is **remote access itself** — the rendezvous, the
certificates, the relay that makes the server reachable at all — and voice simply rides along.
That is a cleaner claim, and it *strengthens* the README's promise by removing a second
justification that does not survive inspection.

### What the research actually established about relays

Worth recording, because "about 10% need a relay" is too coarse to design around
([remote-access.md](../design/research/remote-access.md) §3).

**Only two situations make a relay unavoidable:**

1. **Both** endpoints behind hard (symmetric, endpoint-dependent-mapping) NAT.
2. UDP blocked outright at either end — corporate networks, hotels, captive portals — where the
   only remedy is a relay on **TCP/443 with TLS**.

**One** hard NAT is a solved problem, and the numbers are striking. Port prediction exploits the
birthday paradox: with 256 open source ports, 174 probes gives 50%, 1,024 gives 98%, 2,048 gives
99.9% — at 100 packets/second, *"half the time we'll get through in under 2 seconds."* **Two**
hard NATs require a collision in both directions simultaneously: roughly **170,000 probes per
side, about 28 minutes**. That is the cliff.

**Published relay rates vary by 8× and the gap is mostly effort, not luck:** Tailscale under
10%, general WebRTC 20–25%, corporate WebRTC 60–85%. The most transferable reason is that
**WebRTC has a deadline and Tailscale does not.** ICE must connect in a second or two, so it
gives up and relays; Tailscale **starts relayed and upgrades to direct in the background**,
which buys unlimited time to find a path nobody is waiting on.

**So: relay first, upgrade later.** Voice starts on the server relay — instant, always works —
and migrates to a direct path if hole punching succeeds. The user never learns which one they
have. This also means the relay path must be correct and pleasant rather than a degraded mode,
since it is what everyone gets for the first second and what a minority keep.

And the ladder underneath is worth having in the stated order, because each rung is cheap:
**IPv6 first** (no NAT at all, and it crossed **50% of Google's traffic on 2026-03-28**), then
UPnP-IGD / NAT-PMP / PCP for an explicit mapping, then STUN and simultaneous send, then port
prediction for a single hard NAT, then the server relay. Local addresses are tried first
regardless, because **many NATs do not hairpin** and two peers behind the same router otherwise
fail in the most confusing possible way.

IPv6's trajectory is itself an argument for restraint: this problem shrinks every year without
us, so the traversal cleverness should be bounded and the relay path should be good.

**Status:** Provisional in one respect. If watch-together ever grows past a handful of
participants, relaying N streams through a household uplink stops being free and a selective
forwarding unit becomes a real design question. At two to four people it is not.

---

## D69 — Group size is a priority order, not a capability: two, then four or five, then a party

**Decided** (2026-09-19). Build Watch Together for **two participants** first, **four or five**
second, and **party viewing** third and later. Leave the openings; do not build the machinery.

### Two is not just the first case, it is the case where the design is best

Worth noticing rather than treating two as a degenerate N:

- **[D65](#d65--playback-offset-is-per-participant-and-it-is-an-echo-control)'s echo alignment is
  *fully* solvable at two and only approximable above it.** With one person on headphones and
  one on speakers, the offset `d = L` removes the echo completely. With three or more, the
  pairwise-optimal offsets are over-determined and there is no assignment that satisfies every
  pair — it degrades to a reference participant and best-effort, which is what everyone else
  already has.
- **"Does one stall pause everyone?" has an obvious answer at two** and no obvious answer at
  five.
- **[D68](#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay)'s
  server relay is free at two** and merely cheap at five.

So the ordering is not a resource compromise; two is where the feature is *best*, and it is the
case that motivated it.

### Where the real ceiling is, and it is not the voice

Working the numbers, because the intuition points at the wrong thing. Opus speech with RTP,
SRTP, UDP and IP overhead at 50 packets/second is roughly **64 kbps per stream**. Per-source
gain is a stated requirement, so the server **forwards rather than mixes** — mixing would
destroy exactly the control that was asked for — which means `N(N−1)` outbound streams:

| N | Voice out of the home uplink |
| --- | --- |
| 2 | 0.13 Mbps |
| 5 | 1.3 Mbps |
| 10 | 5.8 Mbps |
| 20 | 24 Mbps |

Voice only becomes a problem around twenty. **The film is the constraint long before that.** A
1080p web-dl direct-plays at 5–10 Mbps and a remux at 20–30, against a typical household upload
of 10–40 Mbps on fibre and far less elsewhere — the existing note in
[remote-access.md](../design/research/remote-access.md) already puts a two-hour film at 20 Mbps.
**One** remote viewer on a remux can saturate a home uplink. Four or five remote viewers cannot
be served at all without reducing the bitrate.

### Which splits "party mode" into two unrelated features

This is the useful consequence, and it changes what the third priority even is:

- **LAN party** — ten people in one house. No NAT, no relay, no uplink involved, bandwidth
  effectively free. This is almost entirely a **UI** problem: a roster, a way to join, and
  sensible defaults for who may control what. Genuinely cheap, and reachable soon after 4–5.
- **Remote party** — ten people in ten houses. Bounded by the household's upload, so it is gated
  on **adaptive bitrate per viewer**, which is the bandwidth half of
  [transcoding.md](../design/plans/transcoding.md) that [D17](#d17--direct-play-first-transcoding-additive-and-optional)
  deliberately deferred. It is not a watch-together feature at all; it is a transcoding feature
  wearing watch-together's clothes.

So remote party viewing is not third on this list — **it is downstream of a different plan
entirely**, and saying so prevents it being scheduled as though watch-together work would
deliver it.

### The openings to leave, all cheap now and awkward later

Each of these is a shape rather than a feature, and none of them is work today:

1. **Membership is a list with per-member state**, not a pair. The moment it is a pair, a roster
   is a rewrite.
2. **Control is a per-participant capability, defaulting to everyone.** "Anyone can pause" is the
   requirement at two and at five; at twenty it becomes chaos and wants a host. If the capability
   exists from the start and is simply always granted, adding a host is a policy change rather
   than a redesign.
3. **The client handles a *set* of remote audio streams with per-source gain**, even when the set
   has one member. This costs nothing — per-source gain was already required — and it is the
   difference between adding a third participant and rewriting the audio path.
4. **Transport is a server-chosen policy, not a client assumption.** Mesh, server relay or
   eventually a selective forwarding unit should be a decision the client is told about, not one
   it is built around.
5. **The stall policy is a group setting** with a default that varies by size, rather than a
   hardcoded rule.

### What not to build

No selective forwarding unit. No active-speaker detection or top-K forwarding. No moderation
tooling. No roles beyond the capability flag in (2). No text chat. Each of these is a real
feature with a real cost and none of them earns its place at two participants — which is the
case that prompted the whole thing.

**Status:** Provisional in its boundaries. The 4–5 figure is a judgement about household uplinks
rather than a measurement; the honest number depends on what bitrate the library actually
direct-plays at, which is the same unknown [D17](#d17--direct-play-first-transcoding-additive-and-optional)
and the Phase 0 playback survey are already waiting on.

---

## D70 — Hosted fan-out is the right shape for remote party viewing, and the cheap part is keeping it possible

**Recorded, not scheduled** (2026-09-19). If remote party viewing ever happens, the shape is
**fan-out**: the household uploads **one** stream to a distribution node and N participants pull
from it, making the home uplink cost independent of N. Speculative and much later. What matters
now is that the openings cost almost nothing and one of them is easy to close by accident.

### Why the shape is obviously right

[D69](#d69--group-size-is-a-priority-order-not-a-capability-two-then-four-or-five-then-a-party)
established that remote party viewing is uplink-bound: `N × 10 Mbps` out of a house with 10–40
Mbps of upload. Fan-out makes that `1 × 10 Mbps` regardless of N. There is no cleverer answer,
and it is what every content-distribution system does.

### And it is the cheapest thing on the roadmap to build, because HLS was designed for it

The minimum viable fan-out node is **an HTTP caching reverse proxy**. Segments are immutable,
addressable and cacheable; clients already pull them over HTTP
([D2](#d2--playback-negotiation-the-client-decides-from-a-manifest)); one origin fetch serves
every participant. That is `nginx proxy_cache` in spirit, not a media system.

Better still: **synchronised viewing is the ideal caching workload.** Unsynchronised viewers of
one film sit at different positions and get poor locality; watch-together guarantees that every
participant wants the *same segment at the same moment*, so the hit rate approaches 100%. The
feature that creates the bandwidth problem also creates the conditions that make solving it
trivial.

### The one opening that is easy to close by accident

**Segment URLs must be cacheable**, which means the cache key must be derivable from the segment
path alone. Signing every segment URL per user — an obvious-looking application of the
signed-and-scoped-URL work in [threat-model.md](../design/plans/threat-model.md) — would give
each participant a unique URL for identical bytes and **defeat fan-out entirely**, silently, and
long before anyone tried to build it.

So: **authorise the session, not the segment.** The manifest is per-user and authorised; segment
paths are stable and content-addressed; the authorisation travels in a session token that the
cache key ignores. This is ordinary CDN practice and it costs nothing to get right now.

That constraint is the entire deliverable of this entry. Everything below is context.

### It must be able to be blind, and that is not DRM

A fan-out node duplicates the film to N recipients. If it can *read* the film, we have built
something we do not want to operate and users do not want to trust. It can be blind: the
household encrypts segments once with a key it distributes directly to the participants it
authorised, and the node forwards opaque bytes.

**This is not DRM and the distinction is load-bearing.** DRM is key escrow, licence servers,
device attestation and a compliance regime, and
[the overview](../design/overview.md) rules it out. This is transport confidentiality against
*the relay operator*, with the key held by the person who owns the file and handed to people
they chose. No licence server, no attestation, nothing to certify. The user can always decrypt
their own media; that is the point.

### The honest risk, which is not technical

A 1:1 relay for NAT traversal and a 1:N distribution service are the same bytes and a different
legal posture. Functionally, 1:N distribution of one person's film to ten others is the shape of
a pirate streaming service, however legitimate every individual party is. Mere-conduit and
hosting-provider protections are strongest for neutral 1:1 transmission and weaker for something
that looks like a distribution platform, and rightsholders notice fan-out.

Being blind helps the conduit argument and simultaneously means the only possible response to a
complaint is "we cannot see it", which is a mixed position rather than a clean one.

**This, not egress cost, is the reason to be slow here.** For reference the cost is a rounding
error: a two-hour film at 10 Mbps is ~9 GB, so a party of ten is ~90 GB, which at Hetzner's
roughly €1/TB beyond a large allowance is **about nine cents** — and at hyperscaler egress
prices is $7–8, which is the same provider-choice conclusion
[remote-access.md](../design/research/remote-access.md) §5 already reached.

### So build it as software, and host it only as a convenience

The fan-out node should be **something anyone can run on a €5 VPS**, which is
[D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s
self-hosted-assist tier doing its job. That is cheaper for us, keeps the exposure with the
person who owns the content, and keeps the hosted version honestly optional — *"we only charge
for things that genuinely require another machine"* reads very differently when the machine is
one you could have rented yourself.

**Status:** Speculative. Not scheduled, not costed beyond the above, and downstream of both
remote party viewing being wanted and the bandwidth half of
[transcoding.md](../design/plans/transcoding.md) existing. The **cacheable-segment-URL
constraint is not speculative** and should be respected from the first HLS endpoint.

---

## D71 — One relay artefact, three jobs, and the core dials out to it

**Decided** (2026-09-19). Generalising the fan-out node of
[D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)
from a party-mode speculation into **a third mode of the same binary**, useful at two
participants: `relay`, run on any publicly reachable host.

### It is one artefact doing three jobs

Three problems that looked separate turn out to be one deployment:

| Problem | Previously | Now |
| --- | --- | --- |
| The partner in a hotel cannot reach the server at all | [D25](#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s hosted-assist tier | the relay |
| Voice cannot hole-punch | [D68](#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay): the media server relays | the relay, when the media server is itself unreachable |
| A party exceeds the household uplink | [D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible): fan-out | the same relay, in cache mode |

D68's reasoning generalises rather than breaks: **the relay is whatever mutually reachable node
already exists.** Usually that is the media server. When the media server is the thing that
cannot be reached, it is a box in a datacentre instead.

### The property that matters most is not the relaying

**The core dials *out* to the relay.** Outbound connections always work, so:

- no port forwarding
- no dynamic DNS
- no UPnP
- **works behind CGNAT**, which is the case nothing else solves

Port forwarding is the single largest support burden in self-hosted anything. A dial-out relay
removes it, and that makes this **more than a fallback for hotels** — it is plausibly the
recommended setup for anyone who wants remote access and does not want to touch their router.

### Two modes, and the near-term one is trivial

- **Tunnel mode** — a byte pipe. TLS passes *through*; the relay terminates nothing, reads
  nothing, cannot tamper. Bandwidth is `N ×` the stream. This covers the stated case (two
  people, one in hotels) and is the one to build.
- **Cache mode** — the [D70](#d70--hosted-fan-out-is-the-right-shape-for-remote-party-viewing-and-the-cheap-part-is-keeping-it-possible)
  fan-out. The relay terminates HTTP so it can see segment *URLs*, while segment *payloads* stay
  encrypted end to end — so it caches ciphertext and still cannot read the film. Origin
  bandwidth becomes `1 ×`. Later, and only if party viewing happens.

The layering is the point: tunnel mode is a byte pipe because it can be, and cache mode gives up
exactly one thing — URL visibility — to buy fan-out.

### It is a role, so it inherits the machinery for free

`relay` sits beside `core` and `node` under
[D43](#d43--one-binary-one-process-per-role-and-a-thin-optional-supervisor)'s one-binary,
one-process-per-role rule, which means it arrives with a status record
([D34](#d34--a-node-status-record-that-distinguishes-dead-from-wedged-from-idle)), observation-based
liveness ([D37](#d37--the-core-judges-liveness-from-observation-not-from-a-node-declared-interval)),
platform supervision and hardened units
([D44](#d44--prefer-the-platforms-supervisor-ship-the-units-and-never-exit-when-you-could-report)),
and the report-don't-exit rule. No new operational surface.

It is also **untrusted by construction** — a rented box someone else administers — which is why
tunnel mode terminates nothing and cache mode sees only URLs. Clients authenticate the *core*,
never the relay.

**Placement guidance:** put the relay **near the home server**, because the core↔relay link is
persistent and carries everything, while the client↔relay leg is whatever the traveller's
network happens to be. A relay adjacent to the origin adds almost no latency; one on the far
side of the world adds all of it.

### The economics are the argument, and the comparison is stark

Plex is the direct prior art and it has gone the other way. Plex's own support page now states
that *"Accessing personal video content remotely does have certain requirements and **is not
available for free**"*, and its Relay is reported to be capped at **1 Mbps free / 2 Mbps paid**,
with enforcement recently tightened. 2 Mbps means heavy transcoding: a degraded experience by
construction.

A €5 VPS with a 20 TB allowance carries roughly **2,200 film-hours a month at 10 Mbps**,
uncapped. That is about **five times the bitrate of Plex's paid relay**, unmetered in practice,
for the price of a coffee.

The gap is not cleverness, it is **household economics versus platform economics**: Plex
subsidises a free tier across millions of users on commercial egress; a VPS you rent yourself
subsidises nobody and pays commodity rates. This is the clearest expression yet of the
README's promise — *"we only charge for things that genuinely require another machine"* — because
here the other machine is one **you** can rent, and we are not in the path at all.

### And the obvious shortcut is not available

Cloudflare Tunnel is the first thing everyone reaches for, and its terms forbid this in as many
words:

> "Cloudflare reserves the right to disable or limit your access to or use of the CDN … if you
> use or are suspected of using the CDN without such Paid Services to serve video or a
> disproportionate percentage of pictures, audio files, or other large files."

So the documentation must say this plainly rather than letting people discover it by being
rate-limited or banned. `frp`, `rathole` and a plain VPS are fine; the free CDN tunnel is not.

**Status:** Provisional. Tunnel mode is a small, well-understood component and should be
scheduled with [remote-access.md](../design/plans/remote-access.md); cache mode stays
speculative per D70. The open question this sharpens rather than answers: if self-hosting a
relay is *this* much better than the commercial offering, the hosted tier's remaining
justification is convenience for people who will not rent a VPS — which is a real market and a
much smaller claim than the one we started with.
